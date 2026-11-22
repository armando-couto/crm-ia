package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// -----------------------------------------------------------------------------
// Worker dos jobs de prospecção (gerar lista, enriquecer, enviar ao CRM).
//
// Os handlers HTTP só inserem o job (models.CreateProspJob). Quem executa é
// este worker: uma goroutine, iniciada em application.go, que pega um job por
// vez com FOR UPDATE SKIP LOCKED e chama o processador do tipo para UMA FATIA
// curta. O processador grava o progresso e o cursor de retomada e devolve; o
// worker reagenda a próxima fatia. Assim nada depende da aba aberta, o
// progresso é consultável e, depois de um reinício, o job continua de onde
// parou (RecuperarProspJobsOrfaos na subida).
//
// Nos testes de controller o worker fica desligado (StartProspeccaoWorker não
// é chamado); testes que querem executar um job chamam ProcessarProximoJob.
// -----------------------------------------------------------------------------

// Fatia é o que o processador devolve depois de trabalhar um pedaço do job.
type Fatia struct {
	// Concluido encerra o job como "concluido" com Mensagem e Resultado.
	Concluido bool
	// Espera adia a próxima fatia (ex.: aguardar o lote do provedor); zero =
	// roda de novo em seguida.
	Espera time.Duration
	// Mensagem, se não vazia, substitui a mensagem de andamento do job.
	Mensagem string
	// Resultado é o resumo final (só lido quando Concluido).
	Resultado any
}

// ProcessadorJob trata um tipo de job. Fatia deve ser curta (segundos, não
// minutos), repetível a partir do estado gravado e nunca usar o contexto de
// uma requisição. Erro comum encerra o job como "falhou"; erro transitório
// (ErroTransitorio, ou ErroPlataforma com Transitorio()) reagenda com espera
// de 30 s, 2 min e 10 min e só falha na quarta vez seguida.
type ProcessadorJob interface {
	Fatia(ctx context.Context, e *ExecucaoJob) (Fatia, error)
}

// EncerradorJob é opcional: chamado uma vez quando o job termina em "falhou"
// ou "cancelado" (nunca em "concluido"), para o tipo arrumar o que deixou pela
// metade. Ex.: gerar_lista deixa a lista "pronta" (parcial) ou "falhou";
// enriquecer ainda lê e grava o lote já pago. O que devolver em mensagem e
// resultado (se não vazios) substitui o que está no job.
type EncerradorJob interface {
	Encerrar(ctx context.Context, e *ExecucaoJob, status string) (mensagem string, resultado any, err error)
}

// ProcessadorFunc adapta uma função a ProcessadorJob.
type ProcessadorFunc func(ctx context.Context, e *ExecucaoJob) (Fatia, error)

func (f ProcessadorFunc) Fatia(ctx context.Context, e *ExecucaoJob) (Fatia, error) { return f(ctx, e) }

var (
	processadoresMu sync.RWMutex
	processadores   = map[string]ProcessadorJob{}
)

// RegistrarProcessadorJob liga um tipo de job ao seu processador. Cada tipo é
// registrado uma vez, no init() do arquivo que o implementa
// (services/prospeccao_*.go); registrar de novo substitui (útil em testes).
func RegistrarProcessadorJob(tipo string, p ProcessadorJob) {
	processadoresMu.Lock()
	defer processadoresMu.Unlock()
	if p == nil {
		delete(processadores, tipo)
		return
	}
	processadores[tipo] = p
}

func processadorDe(tipo string) ProcessadorJob {
	processadoresMu.RLock()
	defer processadoresMu.RUnlock()
	return processadores[tipo]
}

// ErroTransitorio marca um erro que vale tentar de novo (rede, 409/429 do
// provedor, 503 da plataforma).
type ErroTransitorio struct{ Causa error }

func (e *ErroTransitorio) Error() string { return e.Causa.Error() }
func (e *ErroTransitorio) Unwrap() error { return e.Causa }

// Transitorio embrulha err como transitório (nil continua nil).
func Transitorio(err error) error {
	if err == nil {
		return nil
	}
	return &ErroTransitorio{Causa: err}
}

// erroTransitorio decide se o worker tenta de novo.
func erroTransitorio(err error) bool {
	var t *ErroTransitorio
	if errors.As(err, &t) {
		return true
	}
	if p, ok := ComoErroPlataforma(err); ok {
		return p.Transitorio()
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrPlataformaInacessivel)
}

// Parâmetros do worker. Variáveis para os testes encurtarem.
var (
	// ProspeccaoWorkerIntervalo: de quanto em quanto tempo o worker confere a fila ociosa.
	ProspeccaoWorkerIntervalo = 2 * time.Second
	// ProspeccaoFatiaPrazo: prazo do contexto de uma fatia.
	ProspeccaoFatiaPrazo = 2 * time.Minute
	// ProspeccaoArrendamento: por quanto tempo um job reivindicado fica fora
	// da fila; se o processo morrer no meio, o job volta depois disso.
	ProspeccaoArrendamento = 5 * time.Minute
	// ProspeccaoEsperas: espera depois da 1ª, 2ª e 3ª falha transitória
	// seguida; a falha seguinte encerra o job como "falhou".
	ProspeccaoEsperas = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}
)

// ProspeccaoWorkerAtivo indica se o worker foi iniciado neste processo.
var ProspeccaoWorkerAtivo bool

// ExecucaoJob é o que o processador recebe: o job como estava ao ser
// reivindicado e os meios de gravar progresso e estado.
type ExecucaoJob struct {
	DB  *sql.DB
	Job *models.ProspJob
}

// Parametros decodifica a entrada do job em v.
func (e *ExecucaoJob) Parametros(v any) error {
	if len(e.Job.Parametros) == 0 {
		return nil
	}
	return json.Unmarshal(e.Job.Parametros, v)
}

// Estado decodifica o cursor de retomada em v (vazio na primeira fatia).
func (e *ExecucaoJob) Estado(v any) error {
	if len(e.Job.Estado) == 0 {
		return nil
	}
	return json.Unmarshal(e.Job.Estado, v)
}

// SalvarEstado grava o cursor de retomada na hora. Chame ANTES de qualquer
// efeito que não possa ser repetido (o consultation_id de um lote pago, os
// lotes de uma extração).
func (e *ExecucaoJob) SalvarEstado(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := models.SetProspJobEstado(e.DB, e.Job.ID, json.RawMessage(b)); err != nil {
		return err
	}
	e.Job.Estado = b
	return nil
}

// DefinirTotal grava o total a processar.
func (e *ExecucaoJob) DefinirTotal(total int) error {
	e.Job.Total = total
	return models.SetProspJobTotal(e.DB, e.Job.ID, total)
}

// Progresso soma ao progresso do job (deltas). custoCentavos é o que entra no
// uso mensal de enriquecimento.
func (e *ExecucaoJob) Progresso(feitos, erros, custoCentavos int) error {
	e.Job.Feitos += feitos
	e.Job.Erros += erros
	e.Job.CustoCentavos += custoCentavos
	return models.SomarProspJobProgresso(e.DB, e.Job.ID, feitos, erros, custoCentavos)
}

// Mensagem troca a mensagem de andamento.
func (e *ExecucaoJob) Mensagem(m string) error {
	e.Job.Mensagem = m
	return models.SetProspJobMensagem(e.DB, e.Job.ID, m)
}

// Cancelado consulta se o job foi cancelado enquanto a fatia rodava (para
// parar cedo numa fatia mais longa). Em erro de banco responde false.
func (e *ExecucaoJob) Cancelado() bool {
	c, err := models.ProspJobCancelado(e.DB, e.Job.ID)
	return err == nil && c
}

// UserID devolve quem criou o job (para atividades e auditoria gravadas pelo worker).
func (e *ExecucaoJob) UserID() *int64 { return e.Job.CriadoPor }

// StartProspeccaoWorker devolve à fila os jobs órfãos e sobe a goroutine que
// os executa. Chamado uma vez, em application.go.
func StartProspeccaoWorker(db *sql.DB) {
	ProspeccaoWorkerAtivo = true
	if n, err := models.RecuperarProspJobsOrfaos(db); err != nil {
		log.Printf("prospecção: falha ao recuperar jobs órfãos: %v", err)
	} else if n > 0 {
		log.Printf("prospecção: %d job(s) retomado(s) depois do reinício", n)
	}
	go func() {
		for {
			if !rodarFilaDeJobs(db) {
				time.Sleep(ProspeccaoWorkerIntervalo)
			}
		}
	}()
	log.Printf("Prospecção: worker de jobs ativo (confere a fila a cada %s)", ProspeccaoWorkerIntervalo)
}

// rodarFilaDeJobs executa um job e diz se havia trabalho. Nunca deixa o
// pânico subir: a goroutine do worker não pode morrer.
func rodarFilaDeJobs(db *sql.DB) (trabalhou bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("prospecção: pânico no worker: %v", r)
			trabalhou = false
		}
	}()
	trabalhou, err := ProcessarProximoJob(context.Background(), db)
	if err != nil {
		log.Printf("prospecção: worker: %v", err)
		return false
	}
	return trabalhou
}

// ProcessarProximoJob reivindica o próximo job da fila e roda uma fatia dele
// (ou o encerramento, se foi cancelado). Devolve false quando não havia job a
// executar. O erro devolvido é de infraestrutura (banco); erro do processador
// vira estado do job.
func ProcessarProximoJob(ctx context.Context, db *sql.DB) (bool, error) {
	job, err := models.ReivindicarProspJob(db, nil, ProspeccaoArrendamento)
	if err != nil || job == nil {
		return false, err
	}
	e := &ExecucaoJob{DB: db, Job: job}
	p := processadorDe(job.Tipo)

	if job.Status == models.JobCancelado {
		msg, res := encerrarJob(ctx, p, e, models.JobCancelado)
		return true, models.ConcluirLimpezaProspJob(db, job.ID, msg, res)
	}
	if p == nil {
		_, err := models.FinalizarProspJob(db, job.ID, models.JobFalhou,
			fmt.Sprintf("tipo de processamento %q indisponível nesta versão", job.Tipo), nil)
		return true, err
	}

	fatia, errFatia := rodarFatia(ctx, p, e)
	switch {
	case errFatia == nil && fatia.Concluido:
		_, err = models.FinalizarProspJob(db, job.ID, models.JobConcluido, fatia.Mensagem, fatia.Resultado)
	case errFatia == nil:
		err = models.ReagendarProspJob(db, job.ID, fatia.Espera, 0, fatia.Mensagem)
	case erroTransitorio(errFatia) && job.Tentativas < len(ProspeccaoEsperas):
		espera := ProspeccaoEsperas[job.Tentativas]
		log.Printf("prospecção: job %d (%s): falha transitória %d, nova tentativa em %s: %v",
			job.ID, job.Tipo, job.Tentativas+1, espera, errFatia)
		err = models.ReagendarProspJob(db, job.ID, espera, job.Tentativas+1,
			"aguardando para tentar de novo: "+errFatia.Error())
	default:
		log.Printf("prospecção: job %d (%s) falhou: %v", job.ID, job.Tipo, errFatia)
		var falhou bool
		falhou, err = models.FinalizarProspJob(db, job.ID, models.JobFalhou, errFatia.Error(), nil)
		if err == nil && falhou {
			if msg, res := encerrarJob(ctx, p, e, models.JobFalhou); msg != "" || res != nil {
				err = models.ConcluirLimpezaProspJob(db, job.ID, msg, res)
			}
		}
	}
	return true, err
}

// rodarFatia chama o processador com prazo e transforma pânico em erro
// transitório (o job tenta de novo e, se insistir, falha).
func rodarFatia(ctx context.Context, p ProcessadorJob, e *ExecucaoJob) (f Fatia, err error) {
	ctx, cancelar := context.WithTimeout(ctx, ProspeccaoFatiaPrazo)
	defer cancelar()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("prospecção: pânico no job %d (%s): %v", e.Job.ID, e.Job.Tipo, r)
			err = Transitorio(fmt.Errorf("erro inesperado no processamento: %v", r))
		}
	}()
	return p.Fatia(ctx, e)
}

// encerrarJob roda o Encerrar do tipo, se houver, sem deixar erro nem pânico
// impedirem o fim do job (uma limpeza que falha fica só no log).
func encerrarJob(ctx context.Context, p ProcessadorJob, e *ExecucaoJob, status string) (mensagem string, resultado any) {
	enc, ok := p.(EncerradorJob)
	if !ok {
		return "", nil
	}
	ctx, cancelar := context.WithTimeout(ctx, ProspeccaoFatiaPrazo)
	defer cancelar()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("prospecção: pânico ao encerrar o job %d (%s): %v", e.Job.ID, e.Job.Tipo, r)
			mensagem, resultado = "", nil
		}
	}()
	mensagem, resultado, err := enc.Encerrar(ctx, e, status)
	if err != nil {
		log.Printf("prospecção: encerramento do job %d (%s) falhou: %v", e.Job.ID, e.Job.Tipo, err)
	}
	return mensagem, resultado
}

// ProcessarJobsAte roda fatias até a fila esvaziar ou o limite ser atingido
// (para testes e para drenar a fila sem a goroutine). Devolve quantas rodou.
func ProcessarJobsAte(ctx context.Context, db *sql.DB, limite int) (int, error) {
	n := 0
	for n < limite {
		trabalhou, err := ProcessarProximoJob(ctx, db)
		if err != nil {
			return n, err
		}
		if !trabalhou {
			break
		}
		n++
	}
	return n, nil
}
