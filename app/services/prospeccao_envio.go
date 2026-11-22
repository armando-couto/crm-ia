package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/armando-couto/crm-ia/app/models"
)

// -----------------------------------------------------------------------------
// Envio ao CRM (docs/PROSPECCAO.md 8.1, com o adendo): pedido, validação,
// prévia e criação do job "enviar_crm". O processamento item a item fica em
// prospeccao_envio_job.go; a empresa, a inteligência e os contatos, em
// prospeccao_envio_empresa.go.
// -----------------------------------------------------------------------------

// Valores de EnvioRequest.Duplicados e de DistribuicaoEnvio.Modo.
const (
	DuplicadosPular    = "pular"
	DuplicadosVincular = "vincular"

	DistribuicaoUnico   = "unico"
	DistribuicaoRodizio = "rodizio"

	// MaxPorDiaEnvio é o teto de leads por vendedor por dia na distribuição.
	MaxPorDiaEnvio = 1000
	// MaxUsuariosEnvio é o teto de vendedores num rodízio.
	MaxUsuariosEnvio = 200
)

// DistribuicaoEnvio diz quem recebe os leads (interface EnviarRequest.distribuicao).
type DistribuicaoEnvio struct {
	Modo     string  `json:"modo"`
	Usuarios []int64 `json:"usuarios"`
	PorDia   int     `json:"por_dia,omitempty"`
}

// EnvioRequest é o corpo de POST /prospeccao/listas/{id}/enviar e da prévia.
type EnvioRequest struct {
	Selecao       models.ProspSelecao `json:"selecao"`
	Duplicados    string              `json:"duplicados"`
	CriarContatos *bool               `json:"criar_contatos"`
	Distribuicao  DistribuicaoEnvio   `json:"distribuicao"`
	Temperatura   string              `json:"temperatura"`
	Etiquetas     []string            `json:"etiquetas"`
	ContaAlvoTier int                 `json:"conta_alvo_tier"`
}

// ErroEnvio é um pedido inválido (o controller responde 422 com a mensagem).
type ErroEnvio struct{ Mensagem string }

func (e *ErroEnvio) Error() string { return e.Mensagem }

func erroEnvio(format string, a ...any) error { return &ErroEnvio{Mensagem: fmt.Sprintf(format, a...)} }

// ComoErroEnvio diz se err é um pedido inválido.
func ComoErroEnvio(err error) (*ErroEnvio, bool) {
	var e *ErroEnvio
	ok := errors.As(err, &e)
	return e, ok
}

// Normalizar aplica os padrões e confere o pedido: seleção, modo dos
// duplicados, distribuição (vendedores existentes e ativos), temperatura,
// etiquetas e prioridade de conta-alvo. Erros são *ErroEnvio.
func (r *EnvioRequest) Normalizar(db *sql.DB) error {
	if err := r.Selecao.Validar(); err != nil {
		return &ErroEnvio{Mensagem: err.Error()}
	}
	r.Duplicados = strings.TrimSpace(r.Duplicados)
	if r.Duplicados == "" {
		r.Duplicados = DuplicadosPular
	}
	if r.Duplicados != DuplicadosPular && r.Duplicados != DuplicadosVincular {
		return erroEnvio("duplicados deve ser pular ou vincular")
	}
	if r.CriarContatos == nil {
		sim := true
		r.CriarContatos = &sim
	}
	d := &r.Distribuicao
	d.Modo = strings.TrimSpace(d.Modo)
	if d.Modo == "" {
		d.Modo = DistribuicaoUnico
	}
	usuarios := []int64{}
	for _, u := range d.Usuarios {
		if u > 0 && !slices.Contains(usuarios, u) {
			usuarios = append(usuarios, u)
		}
	}
	d.Usuarios = usuarios
	switch d.Modo {
	case DistribuicaoUnico:
		if len(d.Usuarios) > 1 {
			return erroEnvio("na distribuição para um vendedor só, escolha um único usuário")
		}
	case DistribuicaoRodizio:
		if len(d.Usuarios) == 0 {
			return erroEnvio("escolha ao menos um vendedor para o rodízio")
		}
		if len(d.Usuarios) > MaxUsuariosEnvio {
			return erroEnvio("o rodízio aceita até %d vendedores", MaxUsuariosEnvio)
		}
	default:
		return erroEnvio("modo de distribuição inválido (unico ou rodizio)")
	}
	if d.PorDia < 0 || d.PorDia > MaxPorDiaEnvio {
		return erroEnvio("leads por dia deve ficar entre 1 e %d", MaxPorDiaEnvio)
	}
	if len(d.Usuarios) > 0 {
		ativos, err := usuariosAtivos(db, d.Usuarios)
		if err != nil {
			return err
		}
		for _, u := range d.Usuarios {
			if !ativos[u] {
				return erroEnvio("o usuário %d não existe ou está desativado", u)
			}
		}
	}
	if !models.LeadTemperaturaValida(r.Temperatura) {
		return erroEnvio("temperatura inválida (fria, media ou quente)")
	}
	etiquetas := []string{}
	for _, e := range r.Etiquetas {
		e = strings.Join(strings.Fields(e), " ")
		if len([]rune(e)) > 40 {
			e = string([]rune(e)[:40])
		}
		if e != "" && !slices.Contains(etiquetas, e) && len(etiquetas) < 20 {
			etiquetas = append(etiquetas, e)
		}
	}
	r.Etiquetas = etiquetas
	if r.ContaAlvoTier < 0 || r.ContaAlvoTier > 3 {
		return erroEnvio("a prioridade de conta-alvo deve ser 0, 1, 2 ou 3")
	}
	return nil
}

// usuariosAtivos devolve quais dos ids são usuários ativos.
func usuariosAtivos(db models.Executor, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.Query(`SELECT id FROM users WHERE id = ANY($1) AND active = TRUE`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// nomesDosUsuarios devolve id → nome.
func nomesDosUsuarios(db models.Executor, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.Query(`SELECT id, name FROM users WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var nome string
		if err := rows.Scan(&id, &nome); err != nil {
			return nil, err
		}
		out[id] = nome
	}
	return out, rows.Err()
}

// -----------------------------------------------------------------------------
// Prévia
// -----------------------------------------------------------------------------

// EnvioPorUsuario é uma linha de EnviarPrevia.por_usuario e de resultado.por_usuario.
type EnvioPorUsuario struct {
	UserID     int64  `json:"user_id"`
	Nome       string `json:"nome"`
	Quantidade int    `json:"quantidade"`
}

// EnvioPrevia é a resposta de POST /prospeccao/listas/{id}/enviar/previa.
// aptos é quantos leads devem nascer: os itens em novo, enriquecido ou
// sem_resultado, menos os de CNPJ bloqueado e, com duplicados = "pular", menos
// os que já são empresa do CRM.
type EnvioPrevia struct {
	Selecionados    int               `json:"selecionados"`
	Aptos           int               `json:"aptos"`
	DuplicadosNoCRM int               `json:"duplicados_no_crm"`
	Bloqueados      int               `json:"bloqueados"`
	JaEnviados      int               `json:"ja_enviados"`
	Descartados     int               `json:"descartados"`
	PessoaFisica    int               `json:"pessoa_fisica"`
	SemTelefone     int               `json:"sem_telefone"`
	PorUsuario      []EnvioPorUsuario `json:"por_usuario"`
}

// PreviaEnvio conta o que o envio faria com a seleção, sem gravar nada. O
// pedido já deve estar normalizado.
func PreviaEnvio(db *sql.DB, listaID int64, r EnvioRequest) (*EnvioPrevia, error) {
	res, err := models.ResumoProspSelecao(db, listaID, r.Selecao)
	if err != nil {
		return nil, err
	}
	p := &EnvioPrevia{
		Selecionados: res.Total,
		JaEnviados:   res.PorEstado[models.ItemEnviado],
		Descartados:  res.PorEstado[models.ItemDescartado],
		Bloqueados:   res.PorEstado[models.ItemBloqueado],
		PessoaFisica: res.PessoaFisica,
		SemTelefone:  res.SemTelefone,
		PorUsuario:   []EnvioPorUsuario{},
	}
	aptos := 0
	for _, e := range models.ItemEstadosAptosEnvio {
		aptos += res.PorEstado[e]
	}
	// CNPJ bloqueado e empresa já no CRM, entre os aptos (em páginas de ids).
	var bloqueados, noCRM int
	var apos int64
	for {
		ids, err := models.ResolveProspSelecao(db, listaID, r.Selecao, models.ItemEstadosAptosEnvio, apos, models.MaxSelecaoIDs)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			break
		}
		apos = ids[len(ids)-1]
		var b, c int
		if err := db.QueryRow(`
			SELECT COUNT(*) FILTER (WHERE x.bloqueado),
			       COUNT(*) FILTER (WHERE NOT x.bloqueado AND x.no_crm)
			FROM (
			    SELECT EXISTS (SELECT 1 FROM contato_bloqueios b WHERE b.tipo = 'cnpj' AND b.valor = i.cnpj) AS bloqueado,
			           (i.company_id IS NULL AND EXISTS (SELECT 1 FROM companies c WHERE `+models.CNPJNormSQL+` = i.cnpj)) AS no_crm
			    FROM prospeccao_itens i WHERE i.id = ANY($1)
			) x`, pq.Array(ids)).Scan(&b, &c); err != nil {
			return nil, err
		}
		bloqueados += b
		noCRM += c
		if len(ids) < models.MaxSelecaoIDs {
			break
		}
	}
	p.Bloqueados += bloqueados
	p.DuplicadosNoCRM = noCRM
	p.Aptos = aptos - bloqueados
	if r.Duplicados == DuplicadosPular {
		p.Aptos -= noCRM
	}
	p.Aptos = max(0, p.Aptos)

	// Distribuição prevista: rodízio em ordem fixa (todos ativos, conferido
	// na normalização).
	us := r.Distribuicao.Usuarios
	if len(us) > 0 && p.Aptos > 0 {
		nomes, err := nomesDosUsuarios(db, us)
		if err != nil {
			return nil, err
		}
		if r.Distribuicao.Modo == DistribuicaoUnico {
			us = us[:1]
		}
		n := len(us)
		for i, u := range us {
			q := p.Aptos / n
			if i < p.Aptos%n {
				q++
			}
			p.PorUsuario = append(p.PorUsuario, EnvioPorUsuario{UserID: u, Nome: nomes[u], Quantidade: q})
		}
	}
	return p, nil
}

// -----------------------------------------------------------------------------
// Job
// -----------------------------------------------------------------------------

// parametrosEnvio é o que o handler grava em prospeccao_jobs.parametros.
type parametrosEnvio struct {
	Selecao       models.ProspSelecao `json:"selecao"`
	Duplicados    string              `json:"duplicados"`
	CriarContatos bool                `json:"criar_contatos"`
	Distribuicao  DistribuicaoEnvio   `json:"distribuicao"`
	Temperatura   string              `json:"temperatura"`
	Etiquetas     []string            `json:"etiquetas"`
	ContaAlvoTier int                 `json:"conta_alvo_tier"`
}

// EnfileirarEnvio cria o job enviar_crm da lista (o pedido já normalizado).
// models.ErrJobAtivo quando a lista já tem job pendente ou rodando.
func EnfileirarEnvio(db *sql.DB, listaID int64, r EnvioRequest, userID *int64) (*models.ProspJob, error) {
	par := parametrosEnvio{
		Selecao:       r.Selecao,
		Duplicados:    r.Duplicados,
		CriarContatos: r.CriarContatos == nil || *r.CriarContatos,
		Distribuicao:  r.Distribuicao,
		Temperatura:   r.Temperatura,
		Etiquetas:     r.Etiquetas,
		ContaAlvoTier: r.ContaAlvoTier,
	}
	b, err := json.Marshal(par)
	if err != nil {
		return nil, err
	}
	total, err := models.CountProspSelecao(db, listaID, r.Selecao, models.ItemEstadosAptosEnvio)
	if err != nil {
		return nil, err
	}
	j := &models.ProspJob{
		Tipo: models.JobEnviarCRM, ListaID: &listaID, Parametros: b, Total: total,
		Mensagem: "aguardando a vez", CriadoPor: userID,
	}
	if err := models.CreateProspJob(db, j); err != nil {
		return nil, err
	}
	return models.ProspJobByID(db, j.ID)
}

// -----------------------------------------------------------------------------
// Distribuição no tempo: N leads por vendedor por dia útil.
// -----------------------------------------------------------------------------

// DiaUtil diz se o dia é de segunda a sexta e não é feriado nacional.
func DiaUtil(t time.Time) bool {
	t = t.In(time.Local)
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	return !models.FeriadoNacional(t)
}

// ProximaAcaoDistribuida devolve a próxima ação do lead de posição indice
// (0, 1, 2...) na carteira de um vendedor que recebe porDia leads por dia
// útil, contando a partir do dia de inicio. Sem porDia, ou no primeiro dia
// útil quando ele é o de hoje, é agora; nos demais dias, o início da janela de
// contato (horaInicio) daquele dia. Tudo em time.Local.
func ProximaAcaoDistribuida(inicio, agora time.Time, indice, porDia, horaInicio int) time.Time {
	if porDia <= 0 || indice < 0 {
		return agora
	}
	inicio = inicio.In(time.Local)
	d := time.Date(inicio.Year(), inicio.Month(), inicio.Day(), 0, 0, 0, 0, time.Local)
	for !DiaUtil(d) {
		d = d.AddDate(0, 0, 1)
	}
	for k := indice / porDia; k > 0; {
		d = d.AddDate(0, 0, 1)
		if DiaUtil(d) {
			k--
		}
	}
	a := agora.In(time.Local)
	hoje := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.Local)
	if !d.After(hoje) {
		return agora
	}
	return time.Date(d.Year(), d.Month(), d.Day(), horaInicio, 0, 0, 0, time.Local)
}
