package models

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/lib/pq"
)

// Tipos e status dos jobs de prospecção.
const (
	JobGerarLista = "gerar_lista"
	JobEnriquecer = "enriquecer"
	JobEnviarCRM  = "enviar_crm"

	JobPendente  = "pendente"
	JobRodando   = "rodando"
	JobConcluido = "concluido"
	JobFalhou    = "falhou"
	JobCancelado = "cancelado"
)

// JobTipos é a lista fechada de tipos.
var JobTipos = []string{JobGerarLista, JobEnriquecer, JobEnviarCRM}

// ErrJobAtivo: a lista já tem um job pendente ou rodando (índice único
// idx_prospeccao_jobs_ativo). O controller responde 409 com codigo "job_ativo"
// e o job ativo (ProspJobAtivoDaLista).
var ErrJobAtivo = errors.New("já existe um processamento em andamento nesta lista")

// ProspJob é um trabalho longo em segundo plano (interface Job). parametros
// (entrada) e estado (cursor de retomada) são internos e não vão para a tela.
type ProspJob struct {
	ID            int64           `json:"id"`
	Tipo          string          `json:"tipo"`
	ListaID       *int64          `json:"lista_id"`
	Status        string          `json:"status"`
	Total         int             `json:"total"`
	Feitos        int             `json:"feitos"`
	Erros         int             `json:"erros"`
	CustoCentavos int             `json:"custo_centavos"`
	Mensagem      string          `json:"mensagem"`
	Resultado     json.RawMessage `json:"resultado"`
	CriadoPor     *int64          `json:"criado_por"`
	CriadoEm      time.Time       `json:"criado_em"`
	IniciadoEm    *time.Time      `json:"iniciado_em"`
	FinalizadoEm  *time.Time      `json:"finalizado_em"`

	Parametros        json.RawMessage `json:"-"`
	Estado            json.RawMessage `json:"-"`
	Tentativas        int             `json:"-"`
	ProximaExecucaoEm time.Time       `json:"-"`
	LimpezaPendente   bool            `json:"-"`
}

// Ativo diz se o job ainda está pendente ou rodando.
func (j *ProspJob) Ativo() bool { return j.Status == JobPendente || j.Status == JobRodando }

const prospJobCols = `id, tipo, lista_id, status, total, feitos, erros, custo_centavos, COALESCE(mensagem,''),
	resultado, criado_por, criado_em, iniciado_em, finalizado_em, parametros, estado, tentativas,
	proxima_execucao_em, limpeza_pendente`

func scanProspJob(row interface{ Scan(...any) error }) (*ProspJob, error) {
	var j ProspJob
	var resultado, parametros, estado []byte
	if err := row.Scan(&j.ID, &j.Tipo, &j.ListaID, &j.Status, &j.Total, &j.Feitos, &j.Erros, &j.CustoCentavos,
		&j.Mensagem, &resultado, &j.CriadoPor, &j.CriadoEm, &j.IniciadoEm, &j.FinalizadoEm, &parametros, &estado,
		&j.Tentativas, &j.ProximaExecucaoEm, &j.LimpezaPendente); err != nil {
		return nil, err
	}
	if len(resultado) > 0 {
		j.Resultado = resultado
	}
	j.Parametros, j.Estado = parametros, estado
	return &j, nil
}

func scanProspJobs(rows *sql.Rows) ([]ProspJob, error) {
	defer rows.Close()
	jobs := []ProspJob{}
	for rows.Next() {
		j, err := scanProspJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

// rawJSON aceita json.RawMessage, []byte, string ou qualquer valor serializável.
func rawJSON(v any, padrao string) string {
	switch t := v.(type) {
	case nil:
		return padrao
	case json.RawMessage:
		if len(t) == 0 {
			return padrao
		}
		return string(t)
	case []byte:
		if len(t) == 0 {
			return padrao
		}
		return string(t)
	}
	return jsonOu(v, padrao)
}

// CreateProspJob grava o job como pendente. Os handlers só inserem: quem
// executa é o worker (services.StartProspeccaoWorker). Devolve ErrJobAtivo se
// a lista já tem job pendente ou rodando. Dentro de uma transação, a violação
// aborta a transação de quem chamou.
func CreateProspJob(db Executor, j *ProspJob) error {
	if !slices.Contains(JobTipos, j.Tipo) {
		return fmt.Errorf("tipo de job inválido: %q", j.Tipo)
	}
	j.Status = JobPendente
	err := db.QueryRow(`
		INSERT INTO prospeccao_jobs (tipo, lista_id, parametros, estado, total, mensagem, criado_por)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, $5, NULLIF($6, ''), $7)
		RETURNING id, criado_em, proxima_execucao_em`,
		j.Tipo, j.ListaID, rawJSON(j.Parametros, "{}"), rawJSON(j.Estado, "{}"), j.Total, j.Mensagem, j.CriadoPor,
	).Scan(&j.ID, &j.CriadoEm, &j.ProximaExecucaoEm)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" && pqErr.Constraint == "idx_prospeccao_jobs_ativo" {
		return ErrJobAtivo
	}
	return err
}

// ProspJobByID devolve sql.ErrNoRows quando não existe.
func ProspJobByID(db Executor, id int64) (*ProspJob, error) {
	return scanProspJob(db.QueryRow(`SELECT `+prospJobCols+` FROM prospeccao_jobs WHERE id = $1`, id))
}

// ProspJobAtivoDaLista devolve o job pendente ou rodando da lista, ou nil.
func ProspJobAtivoDaLista(db Executor, listaID int64) (*ProspJob, error) {
	j, err := scanProspJob(db.QueryRow(`SELECT `+prospJobCols+` FROM prospeccao_jobs
		WHERE lista_id = $1 AND status IN ('pendente','rodando') LIMIT 1`, listaID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// ProspJobsDaLista devolve os jobs ativos e o último de cada tipo (o que
// GET /prospeccao/listas/{id} mostra), do mais recente para o mais antigo.
func ProspJobsDaLista(db *sql.DB, listaID int64) ([]ProspJob, error) {
	rows, err := db.Query(`
		SELECT `+prospJobCols+` FROM prospeccao_jobs
		WHERE lista_id = $1 AND (status IN ('pendente','rodando') OR id IN (
		    SELECT DISTINCT ON (tipo) id FROM prospeccao_jobs WHERE lista_id = $1 ORDER BY tipo, criado_em DESC, id DESC))
		ORDER BY criado_em DESC, id DESC`, listaID)
	if err != nil {
		return nil, err
	}
	return scanProspJobs(rows)
}

// ReivindicarProspJob pega o próximo job a executar com FOR UPDATE SKIP LOCKED
// e o marca como rodando. Entram: jobs pendentes ou rodando cujo prazo
// (proxima_execucao_em) venceu, e jobs cancelados com limpeza pendente. O
// prazo é empurrado em "arrendamento": enquanto a fatia roda ninguém mais pega
// o job; se o processo morrer, o job volta à fila quando o prazo vencer (ou na
// subida, por RecuperarProspJobsOrfaos). tipos vazio = qualquer tipo. Devolve
// nil, nil quando não há nada a fazer.
func ReivindicarProspJob(db *sql.DB, tipos []string, arrendamento time.Duration) (*ProspJob, error) {
	if arrendamento <= 0 {
		arrendamento = 5 * time.Minute
	}
	if tipos == nil {
		tipos = []string{}
	}
	j, err := scanProspJob(db.QueryRow(`
		UPDATE prospeccao_jobs SET
		    status = CASE WHEN status = 'pendente' THEN 'rodando' ELSE status END,
		    iniciado_em = COALESCE(iniciado_em, NOW()),
		    proxima_execucao_em = NOW() + make_interval(secs => $2),
		    atualizado_em = NOW()
		WHERE id = (
		    SELECT id FROM prospeccao_jobs
		    WHERE proxima_execucao_em <= NOW()
		      AND (status IN ('pendente','rodando') OR (status = 'cancelado' AND limpeza_pendente))
		      AND (cardinality($1::text[]) = 0 OR tipo = ANY($1))
		    ORDER BY proxima_execucao_em, id
		    FOR UPDATE SKIP LOCKED
		    LIMIT 1)
		RETURNING `+prospJobCols, pq.Array(tipos), arrendamento.Seconds()))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// RecuperarProspJobsOrfaos devolve à fila, na subida da aplicação, os jobs que
// ficaram "rodando" (ou com limpeza pendente) quando o processo anterior parou.
func RecuperarProspJobsOrfaos(db *sql.DB) (int, error) {
	res, err := db.Exec(`
		UPDATE prospeccao_jobs SET proxima_execucao_em = NOW(), atualizado_em = NOW()
		WHERE (status = 'rodando' OR (status = 'cancelado' AND limpeza_pendente)) AND proxima_execucao_em > NOW()`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SetProspJobEstado grava o cursor de retomada. Deve ser chamado ANTES de
// qualquer efeito que não possa ser repetido (ex.: o consultation_id de um
// lote pago, a lista de lotes de uma extração).
func SetProspJobEstado(db Executor, id int64, estado any) error {
	_, err := db.Exec(`UPDATE prospeccao_jobs SET estado = $2::jsonb, atualizado_em = NOW() WHERE id = $1`,
		id, rawJSON(estado, "{}"))
	return err
}

// SetProspJobTotal define o total a processar.
func SetProspJobTotal(db Executor, id int64, total int) error {
	_, err := db.Exec(`UPDATE prospeccao_jobs SET total = $2, atualizado_em = NOW() WHERE id = $1`, id, total)
	return err
}

// SomarProspJobProgresso soma ao progresso (valores são deltas da fatia).
// custoCentavos é o que o uso mensal de enriquecimento enxerga.
func SomarProspJobProgresso(db Executor, id int64, feitos, erros, custoCentavos int) error {
	_, err := db.Exec(`
		UPDATE prospeccao_jobs
		SET feitos = feitos + $2, erros = erros + $3, custo_centavos = custo_centavos + $4, atualizado_em = NOW()
		WHERE id = $1`, id, feitos, erros, custoCentavos)
	return err
}

// SetProspJobMensagem troca a mensagem de andamento mostrada na tela.
func SetProspJobMensagem(db Executor, id int64, mensagem string) error {
	_, err := db.Exec(`UPDATE prospeccao_jobs SET mensagem = NULLIF($2, ''), atualizado_em = NOW() WHERE id = $1`, id, mensagem)
	return err
}

// ReagendarProspJob devolve o job rodando à fila para a próxima fatia (ou
// nova tentativa) daqui a "espera" (zero = já), contada no relógio do banco,
// que é o mesmo que decide a reivindicação. tentativas é o número de falhas
// transitórias seguidas (0 depois de uma fatia que deu certo). Não mexe em job
// que já saiu de "rodando" (cancelado no meio da fatia).
func ReagendarProspJob(db Executor, id int64, espera time.Duration, tentativas int, mensagem string) error {
	_, err := db.Exec(`
		UPDATE prospeccao_jobs
		SET proxima_execucao_em = NOW() + make_interval(secs => $2), tentativas = $3,
		    mensagem = COALESCE(NULLIF($4, ''), mensagem), atualizado_em = NOW()
		WHERE id = $1 AND status = 'rodando'`, id, max(espera, 0).Seconds(), tentativas, mensagem)
	return err
}

// FinalizarProspJob encerra um job rodando como concluido ou falhou, com o
// resumo final. Devolve false se o job já não estava rodando (foi cancelado).
func FinalizarProspJob(db Executor, id int64, status, mensagem string, resultado any) (bool, error) {
	if status != JobConcluido && status != JobFalhou {
		return false, fmt.Errorf("status final inválido: %q", status)
	}
	var res any
	if resultado != nil {
		res = rawJSON(resultado, "null")
	}
	r, err := db.Exec(`
		UPDATE prospeccao_jobs
		SET status = $2::varchar, mensagem = NULLIF($3, ''), resultado = COALESCE($4::jsonb, resultado),
		    finalizado_em = NOW(), limpeza_pendente = FALSE, atualizado_em = NOW()
		WHERE id = $1 AND status IN ('pendente','rodando')`, id, status, mensagem, res)
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	return n > 0, nil
}

// CancelarProspJob marca o job como cancelado (POST /prospeccao/jobs/{id}/cancelar).
// O worker para na próxima fatia e ainda roda o encerramento do tipo
// (limpeza_pendente): é nele que um lote já pago é lido e que a lista de um
// gerar_lista interrompido vira "pronta" (parcial) ou "falhou". Job que já
// terminou volta como está. sql.ErrNoRows quando não existe.
func CancelarProspJob(db *sql.DB, id int64) (*ProspJob, error) {
	_, err := db.Exec(`
		UPDATE prospeccao_jobs
		SET status = 'cancelado', mensagem = 'cancelado pelo usuário', finalizado_em = NOW(),
		    limpeza_pendente = TRUE, proxima_execucao_em = NOW(), atualizado_em = NOW()
		WHERE id = $1 AND status IN ('pendente','rodando')`, id)
	if err != nil {
		return nil, err
	}
	return ProspJobByID(db, id)
}

// ConcluirLimpezaProspJob marca que o encerramento do job cancelado terminou;
// resultado e mensagem, se informados, substituem os gravados.
func ConcluirLimpezaProspJob(db Executor, id int64, mensagem string, resultado any) error {
	var res any
	if resultado != nil {
		res = rawJSON(resultado, "null")
	}
	_, err := db.Exec(`
		UPDATE prospeccao_jobs
		SET limpeza_pendente = FALSE, mensagem = COALESCE(NULLIF($2, ''), mensagem),
		    resultado = COALESCE($3::jsonb, resultado), atualizado_em = NOW()
		WHERE id = $1`, id, mensagem, res)
	return err
}

// ProspJobCancelado consulta se o job saiu de "rodando" (para o handler parar
// no meio de uma fatia longa).
func ProspJobCancelado(db Executor, id int64) (bool, error) {
	var status string
	if err := db.QueryRow(`SELECT status FROM prospeccao_jobs WHERE id = $1`, id).Scan(&status); err != nil {
		return false, err
	}
	return status != JobRodando && status != JobPendente, nil
}

// CustoEnriquecimentoJobs soma quantidade e custo dos jobs de enriquecimento
// criados em [de, ate): é a parcela das listas no uso mensal (a outra vem de
// company_enrichments; models.EnrichmentUsage junta as duas numa consulta).
func CustoEnriquecimentoJobs(db Executor, de, ate time.Time) (consultas, custoCentavos int, err error) {
	err = db.QueryRow(`
		SELECT COALESCE(SUM(feitos), 0), COALESCE(SUM(custo_centavos), 0)
		FROM prospeccao_jobs WHERE tipo = 'enriquecer' AND criado_em >= $1 AND criado_em < $2`,
		de, ate).Scan(&consultas, &custoCentavos)
	return
}
