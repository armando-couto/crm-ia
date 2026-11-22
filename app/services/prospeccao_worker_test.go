package services_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
)

// O worker é testado contra Postgres de verdade (a reivindicação usa FOR
// UPDATE SKIP LOCKED), com um processador de mentira no lugar dos reais.

func prepararJobs(t *testing.T) *sql.DB {
	t.Helper()
	db := testDB(t)
	if _, err := db.Exec(`TRUNCATE prospeccao_jobs, prospeccao_itens, prospeccao_listas RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	esperas := services.ProspeccaoEsperas
	// Sem espera entre as tentativas: o teste não dorme.
	services.ProspeccaoEsperas = []time.Duration{0, 0, 0}
	t.Cleanup(func() { services.ProspeccaoEsperas = esperas })
	return db
}

// processadorFalso conta itens em fatias de 2 e guarda o cursor no estado.
type processadorFalso struct {
	fatias     int
	encerrados []string
	falhar     func(fatia int) error
}

type estadoFalso struct {
	Proximo int `json:"proximo"`
}

func (p *processadorFalso) Fatia(ctx context.Context, e *services.ExecucaoJob) (services.Fatia, error) {
	p.fatias++
	if p.falhar != nil {
		if err := p.falhar(p.fatias); err != nil {
			return services.Fatia{}, err
		}
	}
	var par struct {
		Total int `json:"total"`
	}
	var est estadoFalso
	if err := e.Parametros(&par); err != nil {
		return services.Fatia{}, err
	}
	if err := e.Estado(&est); err != nil {
		return services.Fatia{}, err
	}
	if est.Proximo == 0 {
		if err := e.DefinirTotal(par.Total); err != nil {
			return services.Fatia{}, err
		}
	}
	feitos := min(2, par.Total-est.Proximo)
	est.Proximo += feitos
	if err := e.SalvarEstado(est); err != nil {
		return services.Fatia{}, err
	}
	if err := e.Progresso(feitos, 0, feitos*80); err != nil {
		return services.Fatia{}, err
	}
	if est.Proximo >= par.Total {
		return services.Fatia{Concluido: true, Mensagem: "pronto", Resultado: map[string]int{"entregues": est.Proximo}}, nil
	}
	return services.Fatia{Mensagem: "processando"}, nil
}

func (p *processadorFalso) Encerrar(ctx context.Context, e *services.ExecucaoJob, status string) (string, any, error) {
	p.encerrados = append(p.encerrados, status)
	return "parcial: " + status, map[string]int{"entregues": e.Job.Feitos}, nil
}

func registrarFalso(t *testing.T, tipo string, p services.ProcessadorJob) {
	t.Helper()
	services.RegistrarProcessadorJob(tipo, p)
	t.Cleanup(func() { services.RegistrarProcessadorJob(tipo, nil) })
}

func novoJob(t *testing.T, db *sql.DB, tipo, parametros string) *models.ProspJob {
	t.Helper()
	j := &models.ProspJob{Tipo: tipo, Parametros: []byte(parametros)}
	if err := models.CreateProspJob(db, j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestWorkerProcessaEmFatiasERetoma(t *testing.T) {
	db := prepararJobs(t)
	p := &processadorFalso{}
	registrarFalso(t, models.JobEnriquecer, p)
	job := novoJob(t, db, models.JobEnriquecer, `{"total": 5}`)
	ctx := context.Background()

	// Primeira fatia: 2 de 5, job segue rodando com o cursor gravado.
	if ok, err := services.ProcessarProximoJob(ctx, db); err != nil || !ok {
		t.Fatalf("primeira fatia: %v %v", ok, err)
	}
	j, _ := models.ProspJobByID(db, job.ID)
	if j.Status != models.JobRodando || j.Total != 5 || j.Feitos != 2 || j.CustoCentavos != 160 || j.Mensagem != "processando" || j.IniciadoEm == nil {
		t.Fatalf("depois da primeira fatia: %+v", j)
	}

	// "Reinício": um processador novo (sem memória) continua do estado gravado.
	p2 := &processadorFalso{}
	services.RegistrarProcessadorJob(models.JobEnriquecer, p2)
	if _, err := models.RecuperarProspJobsOrfaos(db); err != nil {
		t.Fatal(err)
	}
	n, err := services.ProcessarJobsAte(ctx, db, 10)
	if err != nil || n != 2 {
		t.Fatalf("faltavam 2 fatias (2 + 1 itens), rodou %d: %v", n, err)
	}
	j, _ = models.ProspJobByID(db, job.ID)
	if j.Status != models.JobConcluido || j.Feitos != 5 || j.CustoCentavos != 400 || j.Mensagem != "pronto" ||
		string(j.Resultado) != `{"entregues": 5}` || j.FinalizadoEm == nil {
		t.Fatalf("job concluído: %+v", j)
	}
	if len(p2.encerrados) != 0 {
		t.Fatal("Encerrar não é chamado em job concluído")
	}
	if ok, _ := services.ProcessarProximoJob(ctx, db); ok {
		t.Fatal("fila vazia: não havia mais nada a processar")
	}
}

func TestWorkerRepeteFalhaTransitoriaEDesisteNaQuarta(t *testing.T) {
	db := prepararJobs(t)
	ctx := context.Background()

	// Duas falhas transitórias e depois dá certo: o contador de tentativas zera.
	p := &processadorFalso{falhar: func(f int) error {
		if f <= 2 {
			return services.Transitorio(errors.New("provedor respondeu 429"))
		}
		return nil
	}}
	registrarFalso(t, models.JobEnriquecer, p)
	job := novoJob(t, db, models.JobEnriquecer, `{"total": 2}`)
	if ok, _ := services.ProcessarProximoJob(ctx, db); !ok {
		t.Fatal("deveria ter processado")
	}
	j, _ := models.ProspJobByID(db, job.ID)
	if j.Status != models.JobRodando || j.Tentativas != 1 || j.Mensagem != "aguardando para tentar de novo: provedor respondeu 429" {
		t.Fatalf("depois da primeira falha transitória: %+v", j)
	}
	if n, err := services.ProcessarJobsAte(ctx, db, 10); err != nil || n != 2 {
		t.Fatalf("segunda falha e a fatia que deu certo: %d, %v", n, err)
	}
	if j, _ = models.ProspJobByID(db, job.ID); j.Status != models.JobConcluido || j.Feitos != 2 {
		t.Fatalf("o job deveria ter concluído depois das novas tentativas: %+v", j)
	}

	// Falha transitória que não passa: 30 s, 2 min, 10 min e, na quarta, falhou.
	sempre := &processadorFalso{falhar: func(int) error {
		return &services.ErroPlataforma{Status: 503, StatusPainel: 503, Codigo: services.CodigoBaseEmCarga, Mensagem: "base indisponível"}
	}}
	registrarFalso(t, models.JobGerarLista, sempre)
	job2 := novoJob(t, db, models.JobGerarLista, `{"total": 2}`)
	n, err := services.ProcessarJobsAte(ctx, db, 10)
	if err != nil || n != 4 || sempre.fatias != 4 {
		t.Fatalf("esperava 4 tentativas, foram %d (%d fatias): %v", n, sempre.fatias, err)
	}
	j, _ = models.ProspJobByID(db, job2.ID)
	if j.Status != models.JobFalhou || j.FinalizadoEm == nil || len(sempre.encerrados) != 1 || sempre.encerrados[0] != models.JobFalhou ||
		j.Mensagem != "parcial: falhou" {
		t.Fatalf("job que falhou de vez: %+v (encerrados %v)", j, sempre.encerrados)
	}

	// Erro que não é transitório falha na hora, sem repetir.
	definitivo := &processadorFalso{falhar: func(int) error { return errors.New("credencial inválida") }}
	registrarFalso(t, models.JobEnviarCRM, definitivo)
	job3 := novoJob(t, db, models.JobEnviarCRM, `{}`)
	if n, _ := services.ProcessarJobsAte(ctx, db, 10); n != 1 {
		t.Fatalf("erro definitivo não repete: %d", n)
	}
	if j, _ = models.ProspJobByID(db, job3.ID); j.Status != models.JobFalhou || j.Mensagem != "parcial: falhou" {
		t.Fatalf("job com erro definitivo: %+v", j)
	}
}

func TestWorkerCancelamentoEPanico(t *testing.T) {
	db := prepararJobs(t)
	ctx := context.Background()

	// Cancelar no meio: a fatia em curso não conclui o job e o encerramento roda uma vez.
	var jobID int64
	p := &processadorFalso{}
	p.falhar = func(f int) error {
		if f == 2 {
			if _, err := models.CancelarProspJob(db, jobID); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}
	registrarFalso(t, models.JobEnriquecer, p)
	job := novoJob(t, db, models.JobEnriquecer, `{"total": 4}`)
	jobID = job.ID
	n, err := services.ProcessarJobsAte(ctx, db, 10)
	if err != nil || n != 3 { // fatia 1, fatia 2 (cancelada no meio) e o encerramento
		t.Fatalf("esperava 2 fatias e 1 encerramento, rodou %d: %v", n, err)
	}
	j, _ := models.ProspJobByID(db, job.ID)
	if j.Status != models.JobCancelado || j.LimpezaPendente || len(p.encerrados) != 1 || p.encerrados[0] != models.JobCancelado ||
		j.Mensagem != "parcial: cancelado" || string(j.Resultado) != `{"entregues": 4}` {
		t.Fatalf("job cancelado: %+v (encerrados %v)", j, p.encerrados)
	}

	// Cancelar antes de começar também passa pelo encerramento (a lista de um gerar_lista precisa sair de "gerando").
	pendente := novoJob(t, db, models.JobEnriquecer, `{"total": 4}`)
	if _, err := models.CancelarProspJob(db, pendente.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := services.ProcessarJobsAte(ctx, db, 10); n != 1 || len(p.encerrados) != 2 {
		t.Fatalf("o job cancelado ainda pendente deveria só ser encerrado: rodou %d, encerrados %v", n, p.encerrados)
	}

	// Pânico no processador não derruba o worker: vira nova tentativa e, insistindo, falha.
	registrarFalso(t, models.JobEnviarCRM, services.ProcessadorFunc(func(context.Context, *services.ExecucaoJob) (services.Fatia, error) {
		panic("índice fora do intervalo")
	}))
	comPanico := novoJob(t, db, models.JobEnviarCRM, `{}`)
	if n, err := services.ProcessarJobsAte(ctx, db, 10); err != nil || n != 4 {
		t.Fatalf("pânico: esperava 4 tentativas, foram %d: %v", n, err)
	}
	if j, _ = models.ProspJobByID(db, comPanico.ID); j.Status != models.JobFalhou || j.Mensagem == "" {
		t.Fatalf("job com pânico: %+v", j)
	}

	// Tipo sem processador registrado falha com mensagem clara, sem travar a fila.
	orfao := novoJob(t, db, models.JobGerarLista, `{}`)
	if n, _ := services.ProcessarJobsAte(ctx, db, 10); n != 1 {
		t.Fatalf("job sem processador: %d", n)
	}
	if j, _ = models.ProspJobByID(db, orfao.ID); j.Status != models.JobFalhou {
		t.Fatalf("job sem processador deveria falhar: %+v", j)
	}
	if services.ProspeccaoWorkerAtivo {
		t.Fatal("o worker só é iniciado em application.go, nunca nos testes")
	}
}
