package models

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"
)

// -----------------------------------------------------------------------------
// Leads: empresas a trabalhar, fora do funil até virarem negócio. Nascem do
// envio de uma lista de prospecção ao CRM (ou à mão) e são trabalhados na
// caixa de leads e na fila de ligações.
// -----------------------------------------------------------------------------

// Estados do lead.
const (
	LeadNovo       = "novo"
	LeadTentando   = "tentando"
	LeadRetornar   = "retornar"
	LeadConvertido = "convertido"
	LeadDescartado = "descartado"
)

// LeadEstados é a lista fechada; LeadEstadosAbertos, os que ainda estão na fila.
var (
	LeadEstados        = []string{LeadNovo, LeadTentando, LeadRetornar, LeadConvertido, LeadDescartado}
	LeadEstadosAbertos = []string{LeadNovo, LeadTentando, LeadRetornar}
)

func LeadEstadoValido(s string) bool { return slices.Contains(LeadEstados, s) }
func LeadEstadoAberto(s string) bool { return slices.Contains(LeadEstadosAbertos, s) }

// Origens do lead (também gravadas em deals.origem na conversão).
const (
	LeadOrigemProspeccao = "prospeccao"
	LeadOrigemManual     = "manual"
	LeadOrigemFormulario = "formulario"
	LeadOrigemImportacao = "importacao"
)

var LeadOrigens = []string{LeadOrigemProspeccao, LeadOrigemManual, LeadOrigemFormulario, LeadOrigemImportacao}

// LeadTemperaturas aceita vazio (sem temperatura).
var LeadTemperaturas = []string{"", "fria", "media", "quente"}

func LeadTemperaturaValida(s string) bool { return slices.Contains(LeadTemperaturas, s) }

// Urgência (semáforo) calculada a partir de proxima_acao_em.
const (
	UrgenciaAtrasado = "atrasado"
	UrgenciaHoje     = "hoje"
	UrgenciaFuturo   = "futuro"
	UrgenciaNenhuma  = "nenhuma"
)

var LeadUrgencias = []string{UrgenciaAtrasado, UrgenciaHoje, UrgenciaFuturo, UrgenciaNenhuma}

// Resultados de uma ligação na fila (calls.desfecho, leads.ultimo_resultado).
const (
	ResultadoAtendeu      = "atendeu"
	ResultadoReuniao      = "reuniao"
	ResultadoRetornar     = "retornar"
	ResultadoSemInteresse = "sem_interesse"
	ResultadoNaoEDecisor  = "nao_e_decisor"
	ResultadoNaoAtendeu   = "nao_atendeu"
	ResultadoCaixaPostal  = "caixa_postal"
	ResultadoOcupado      = "ocupado"
	ResultadoNumeroErrado = "numero_errado"
	ResultadoNaoLigarMais = "nao_ligar_mais"
)

// resultadoOutcome é o valor de calls.outcome de cada resultado.
var resultadoOutcome = map[string]string{
	ResultadoAtendeu:      "conectada",
	ResultadoReuniao:      "conectada",
	ResultadoRetornar:     "conectada",
	ResultadoSemInteresse: "conectada",
	ResultadoNaoEDecisor:  "conectada",
	ResultadoNaoAtendeu:   "sem_resposta",
	ResultadoCaixaPostal:  "caixa_postal",
	ResultadoOcupado:      "ocupado",
	ResultadoNumeroErrado: "numero_errado",
	ResultadoNaoLigarMais: "conectada",
}

// ResultadosLead é a lista fechada, na ordem dos atalhos 1 a 0 da fila.
var ResultadosLead = []string{ResultadoAtendeu, ResultadoReuniao, ResultadoRetornar, ResultadoSemInteresse,
	ResultadoNaoEDecisor, ResultadoNaoAtendeu, ResultadoCaixaPostal, ResultadoOcupado, ResultadoNumeroErrado,
	ResultadoNaoLigarMais}

// ResultadosSemContato são os que contam tentativa na cadência.
var ResultadosSemContato = []string{ResultadoNaoAtendeu, ResultadoCaixaPostal, ResultadoOcupado}

func ResultadoValido(r string) bool { _, ok := resultadoOutcome[r]; return ok }

// OutcomeDoResultado devolve o calls.outcome do resultado ("" se inválido).
func OutcomeDoResultado(r string) string { return resultadoOutcome[r] }

// ResultadoConectado diz se alguém atendeu (zera o contador de tentativas).
func ResultadoConectado(r string) bool { return resultadoOutcome[r] == "conectada" }

// ResultadoContaTentativa diz se o resultado incrementa leads.tentativas.
func ResultadoContaTentativa(r string) bool { return slices.Contains(ResultadosSemContato, r) }

// TravaFilaDuracao é quanto dura a trava de atendimento da fila.
const TravaFilaDuracao = 20 * time.Minute

// Erros de estado do lead.
var (
	// ErrLeadEncerrado: lead já convertido ou descartado (409, codigo "estado").
	ErrLeadEncerrado = errors.New("este lead já foi encerrado")
	// ErrLeadEstado: a ação não vale para o estado atual (409).
	ErrLeadEstado = errors.New("ação indisponível para o estado atual do lead")
)

// ErroLeadEmAtendimento: o lead está travado por outro usuário (423).
type ErroLeadEmAtendimento struct {
	UserID int64
	Nome   string
}

func (e *ErroLeadEmAtendimento) Error() string { return "lead em atendimento por " + e.Nome }

// Sinal é um motivo para ligar ou um ponto de atenção do resumo do lead.
type Sinal struct {
	Tipo  string `json:"tipo"`
	Nivel string `json:"nivel"` // ok | atencao | risco
	Texto string `json:"texto"`
}

// ResumoDecisor é um decisor com seus telefones, já em ordem.
type ResumoDecisor struct {
	Nome      string          `json:"nome"`
	Cargo     string          `json:"cargo"`
	Telefones []ProspTelefone `json:"telefones"`
}

// ResumoCadastro é o cadastro da empresa mostrado na ficha do lead.
type ResumoCadastro struct {
	RazaoSocial  string       `json:"razao_social"`
	NomeFantasia string       `json:"nome_fantasia"`
	CNAE         string       `json:"cnae"`
	Porte        string       `json:"porte"`
	Abertura     *string      `json:"abertura"`
	Capital      *float64     `json:"capital"`
	Natureza     string       `json:"natureza"`
	Simples      bool         `json:"simples"`
	MEI          bool         `json:"mei"`
	Endereco     string       `json:"endereco"`
	Municipio    string       `json:"municipio"`
	UF           string       `json:"uf"`
	Site         string       `json:"site"`
	Email        string       `json:"email"`
	Socios       []ProspSocio `json:"socios"`
	Divida       *ProspDivida `json:"divida"`
}

// ResumoLead é a ficha pronta do lead (leads.resumo), montada no envio ao CRM.
type ResumoLead struct {
	Frases    []string        `json:"frases"`
	Decisores []ResumoDecisor `json:"decisores"`
	Sinais    []Sinal         `json:"sinais"`
	Cadastro  ResumoCadastro  `json:"cadastro"`
}

// Lead é a linha da caixa de leads (interface Lead).
type Lead struct {
	ID               int64           `json:"id"`
	Titulo           string          `json:"titulo"`
	CompanyID        *int64          `json:"company_id"`
	CompanyName      string          `json:"company_name"`
	ContactID        *int64          `json:"contact_id"`
	ContactName      string          `json:"contact_name"`
	CNPJ             string          `json:"cnpj"`
	Cidade           string          `json:"cidade"`
	UF               string          `json:"uf"`
	Origem           string          `json:"origem"`
	ListaID          *int64          `json:"lista_id"`
	ListaNome        string          `json:"lista_nome"`
	ItemID           *int64          `json:"item_id"`
	OwnerID          *int64          `json:"owner_id"`
	OwnerName        string          `json:"owner_name"`
	Estado           string          `json:"estado"`
	Urgencia         string          `json:"urgencia"`
	Temperatura      string          `json:"temperatura"`
	Etiquetas        []string        `json:"etiquetas"`
	Nota             int             `json:"nota"`
	ValorEstimado    *float64        `json:"valor_estimado"`
	Telefones        []ProspTelefone `json:"telefones"`
	Emails           []string        `json:"emails"`
	Tentativas       int             `json:"tentativas"`
	UltimoContatoEm  *time.Time      `json:"ultimo_contato_em"`
	UltimoResultado  string          `json:"ultimo_resultado"`
	ProximaAcaoEm    *time.Time      `json:"proxima_acao_em"`
	MotivoDescarte   string          `json:"motivo_descarte"`
	DealID           *int64          `json:"deal_id"`
	MeetingID        *int64          `json:"meeting_id"`
	ConvertidoEm     *time.Time      `json:"convertido_em"`
	WhatsAppOptin    bool            `json:"whatsapp_optin"`
	PessoaFisica     bool            `json:"pessoa_fisica"`
	EmAtendimentoPor *int64          `json:"em_atendimento_por"` // nulo quando a trava venceu
	Visto            bool            `json:"visto"`
	CriadoEm         time.Time       `json:"criado_em"`
	AtualizadoEm     time.Time       `json:"atualizado_em"`

	// Fora do JSON da listagem: a ficha (LeadCompleto.resumo), o dono ativo
	// ou não, até quando vale a trava e quem criou.
	Resumo           ResumoLead `json:"-"`
	OwnerAtivo       bool       `json:"-"`
	EmAtendimentoAte *time.Time `json:"-"`
	CriadoPor        *int64     `json:"-"`
	BaseLegal        string     `json:"-"`
}

// Aberto diz se o lead ainda está em novo, tentando ou retornar.
func (l *Lead) Aberto() bool { return LeadEstadoAberto(l.Estado) }

// SemDono diz se o lead conta como sem dono (dono nulo ou desativado).
func (l *Lead) SemDono() bool { return l.OwnerID == nil || !l.OwnerAtivo }

// LeadVisao é quem está olhando. Sem leads.assign (VeTodos = false) toda
// consulta recebe o predicado "meu, sem dono ou de dono inativo", e o que
// fica fora dele se comporta como inexistente (sql.ErrNoRows → 404).
type LeadVisao struct {
	UserID  int64
	VeTodos bool
}

// VisaoTotal é a visão de quem tem leads.assign e a dos jobs.
func VisaoTotal(userID int64) LeadVisao { return LeadVisao{UserID: userID, VeTodos: true} }

const (
	leadSemDonoSQL = `(l.owner_id IS NULL OR ow.active = FALSE)`
	leadAbertoSQL  = `l.estado IN ('novo','tentando','retornar')`
	leadFromSQL    = `
	FROM leads l
	LEFT JOIN companies c ON c.id = l.company_id
	LEFT JOIN contacts ct ON ct.id = l.contact_id
	LEFT JOIN prospeccao_listas pl ON pl.id = l.lista_id
	LEFT JOIN users ow ON ow.id = l.owner_id
	WHERE `
)

// leadConsulta acumula argumentos e condições das consultas de leads.
type leadConsulta struct {
	args []any
}

func (q *leadConsulta) arg(v any) string {
	q.args = append(q.args, v)
	return fmt.Sprintf("$%d", len(q.args))
}

// urgencia devolve a expressão SQL do semáforo. "Hoje" é o dia no fuso do
// ambiente (time.Local), por isso os limites entram como argumentos.
func (q *leadConsulta) urgencia(agora time.Time) string {
	agora = agora.In(time.Local)
	hoje := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.Local)
	h, a := q.arg(hoje), q.arg(hoje.AddDate(0, 0, 1))
	return `CASE WHEN NOT ` + leadAbertoSQL + ` OR l.proxima_acao_em IS NULL THEN 'nenhuma'
	             WHEN l.proxima_acao_em < ` + h + `::timestamptz THEN 'atrasado'
	             WHEN l.proxima_acao_em < ` + a + `::timestamptz THEN 'hoje'
	             ELSE 'futuro' END`
}

// visivel devolve o predicado de visibilidade ("TRUE" para quem vê todos).
func (q *leadConsulta) visivel(v LeadVisao) string {
	if v.VeTodos {
		return "TRUE"
	}
	return `(l.owner_id = ` + q.arg(v.UserID) + ` OR ` + leadSemDonoSQL + `)`
}

func (q *leadConsulta) selectLead(agora time.Time) string {
	return `SELECT l.id, l.titulo, l.company_id, COALESCE(c.name,''), l.contact_id,
	       COALESCE(TRIM(ct.first_name || ' ' || COALESCE(ct.last_name,'')),''),
	       l.cnpj, l.cidade, l.uf, l.origem, l.lista_id, COALESCE(pl.nome,''), l.item_id,
	       l.owner_id, COALESCE(ow.name,''), COALESCE(ow.active, FALSE), l.estado, ` + q.urgencia(agora) + `,
	       l.temperatura, l.etiquetas, l.nota, l.valor_estimado, l.telefones, l.emails, l.resumo,
	       l.tentativas, l.ultimo_contato_em, l.ultimo_resultado, l.proxima_acao_em, l.motivo_descarte,
	       l.deal_id, l.meeting_id, l.convertido_em, l.whatsapp_optin, l.pessoa_fisica,
	       CASE WHEN l.em_atendimento_ate > NOW() THEN l.em_atendimento_por END,
	       CASE WHEN l.em_atendimento_ate > NOW() THEN l.em_atendimento_ate END,
	       l.visto_em IS NOT NULL, l.criado_por, l.base_legal, l.criado_em, l.atualizado_em` + leadFromSQL
}

func scanLead(row interface{ Scan(...any) error }) (*Lead, error) {
	var l Lead
	var telefones, emails, resumo []byte
	if err := row.Scan(&l.ID, &l.Titulo, &l.CompanyID, &l.CompanyName, &l.ContactID, &l.ContactName,
		&l.CNPJ, &l.Cidade, &l.UF, &l.Origem, &l.ListaID, &l.ListaNome, &l.ItemID,
		&l.OwnerID, &l.OwnerName, &l.OwnerAtivo, &l.Estado, &l.Urgencia,
		&l.Temperatura, pq.Array(&l.Etiquetas), &l.Nota, &l.ValorEstimado, &telefones, &emails, &resumo,
		&l.Tentativas, &l.UltimoContatoEm, &l.UltimoResultado, &l.ProximaAcaoEm, &l.MotivoDescarte,
		&l.DealID, &l.MeetingID, &l.ConvertidoEm, &l.WhatsAppOptin, &l.PessoaFisica,
		&l.EmAtendimentoPor, &l.EmAtendimentoAte, &l.Visto, &l.CriadoPor, &l.BaseLegal,
		&l.CriadoEm, &l.AtualizadoEm); err != nil {
		return nil, err
	}
	l.Telefones, l.Emails = []ProspTelefone{}, []string{}
	_ = json.Unmarshal(telefones, &l.Telefones)
	_ = json.Unmarshal(emails, &l.Emails)
	_ = json.Unmarshal(resumo, &l.Resumo)
	if l.Etiquetas == nil {
		l.Etiquetas = []string{}
	}
	return &l, nil
}

// LeadByID devolve o lead se ele estiver dentro da visão; sql.ErrNoRows quando
// não existe ou é de outra pessoa (o controller responde 404 nos dois casos).
func LeadByID(db Executor, id int64, v LeadVisao) (*Lead, error) {
	q := &leadConsulta{}
	sel := q.selectLead(time.Now())
	return scanLead(db.QueryRow(sel+`l.id = `+q.arg(id)+` AND `+q.visivel(v), q.args...))
}

// LeadFilter é a consulta de GET /leads.
type LeadFilter struct {
	Visao       LeadVisao
	Fila        string // meus (padrão) | sem_dono | todos
	Estado      string
	Urgencia    string
	OwnerID     int64
	ListaID     int64
	Temperatura string
	Etiqueta    string
	Q           string
	Sort        string // urgencia (padrão) | nota | criado_em | titulo
	Dir         string
	Pagination
}

// ContadoresLeads acompanha a listagem. meus, sem_dono e todos ignoram os
// filtros (só respeitam a visão); os de estado e de urgência valem para a
// fila e os filtros atuais, ignorando estado e urgencia.
type ContadoresLeads struct {
	Meus       int `json:"meus"`
	SemDono    int `json:"sem_dono"`
	Todos      int `json:"todos"`
	Novo       int `json:"novo"`
	Tentando   int `json:"tentando"`
	Retornar   int `json:"retornar"`
	Convertido int `json:"convertido"`
	Descartado int `json:"descartado"`
	Atrasados  int `json:"atrasados"`
	Hoje       int `json:"hoje"`
}

// filtroBase monta as condições de fila e dos filtros que não são estado nem
// urgência. Quem não vê todos e pede fila "todos" ou o dono de outra pessoa
// continua limitado pelo predicado de visibilidade (o controller decide se
// responde 403 antes de chegar aqui).
func (f LeadFilter) filtroBase(q *leadConsulta) []string {
	where := []string{q.visivel(f.Visao)}
	switch f.Fila {
	case "sem_dono":
		where = append(where, leadSemDonoSQL)
	case "todos":
	default:
		if f.OwnerID == 0 {
			where = append(where, `l.owner_id = `+q.arg(f.Visao.UserID))
		}
	}
	if f.OwnerID > 0 {
		where = append(where, `l.owner_id = `+q.arg(f.OwnerID))
	}
	if f.ListaID > 0 {
		where = append(where, `l.lista_id = `+q.arg(f.ListaID))
	}
	if f.Temperatura != "" {
		where = append(where, `l.temperatura = `+q.arg(f.Temperatura))
	}
	if e := strings.TrimSpace(f.Etiqueta); e != "" {
		where = append(where, q.arg(e)+` = ANY(l.etiquetas)`)
	}
	if s := strings.TrimSpace(f.Q); s != "" {
		p := q.arg(likeContem(s))
		cond := `(l.titulo ILIKE ` + p + ` OR COALESCE(c.name,'') ILIKE ` + p + ` OR l.cidade ILIKE ` + p
		if c := NormalizarCNPJ(s); len(c) >= 8 && strings.Trim(c[:8], "0123456789") == "" {
			cond += ` OR l.cnpj LIKE ` + q.arg(c) + ` || '%'`
		}
		if d := SomenteDigitos(s); len(d) >= 8 {
			cond += ` OR l.telefones::text LIKE ` + q.arg("%"+d+"%")
		}
		where = append(where, cond+`)`)
	}
	return where
}

// ListLeads devolve a página, o total da consulta e os contadores.
func ListLeads(db *sql.DB, f LeadFilter) ([]Lead, int, ContadoresLeads, error) {
	f.Normalize()
	agora := time.Now()
	var cont ContadoresLeads

	// Contadores: uma consulta só, com FILTER.
	qc := &leadConsulta{}
	urg := qc.urgencia(agora)
	vis := qc.visivel(f.Visao)
	base := strings.Join(f.filtroBase(qc), " AND ")
	err := db.QueryRow(`
		SELECT COUNT(*) FILTER (WHERE l.owner_id = `+qc.arg(f.Visao.UserID)+`),
		       COUNT(*) FILTER (WHERE `+leadSemDonoSQL+`),
		       COUNT(*),
		       COUNT(*) FILTER (WHERE f.ok AND l.estado = 'novo'),
		       COUNT(*) FILTER (WHERE f.ok AND l.estado = 'tentando'),
		       COUNT(*) FILTER (WHERE f.ok AND l.estado = 'retornar'),
		       COUNT(*) FILTER (WHERE f.ok AND l.estado = 'convertido'),
		       COUNT(*) FILTER (WHERE f.ok AND l.estado = 'descartado'),
		       COUNT(*) FILTER (WHERE f.ok AND `+urg+` = 'atrasado'),
		       COUNT(*) FILTER (WHERE f.ok AND `+urg+` = 'hoje')
		FROM leads l
		LEFT JOIN companies c ON c.id = l.company_id
		LEFT JOIN users ow ON ow.id = l.owner_id
		CROSS JOIN LATERAL (SELECT (`+base+`) AS ok) f
		WHERE `+vis, qc.args...).Scan(&cont.Meus, &cont.SemDono, &cont.Todos, &cont.Novo, &cont.Tentando,
		&cont.Retornar, &cont.Convertido, &cont.Descartado, &cont.Atrasados, &cont.Hoje)
	if err != nil {
		return nil, 0, cont, err
	}

	q := &leadConsulta{}
	sel := q.selectLead(agora)
	where := f.filtroBase(q)
	if f.Estado != "" {
		where = append(where, `l.estado = `+q.arg(f.Estado))
	}
	if f.Urgencia != "" {
		where = append(where, `(`+q.urgencia(agora)+`) = `+q.arg(f.Urgencia))
	}
	cond := strings.Join(where, " AND ")

	// A contagem reaproveita os mesmos argumentos; os dois primeiros (limites
	// do dia, usados só no SELECT) entram numa condição neutra para o
	// PostgreSQL conseguir deduzir o tipo deles.
	var total int
	if err := db.QueryRow(`SELECT COUNT(*)`+leadFromSQL+`($1::timestamptz IS NOT NULL AND $2::timestamptz IS NOT NULL) AND `+cond,
		q.args...).Scan(&total); err != nil {
		return nil, 0, cont, err
	}

	desc := strings.EqualFold(f.Dir, "desc")
	dir := func(padraoDesc bool) string {
		if f.Dir == "" {
			desc = padraoDesc
		}
		if desc {
			return " DESC"
		}
		return " ASC"
	}
	var ordem string
	switch f.Sort {
	case "nota":
		ordem = "l.nota" + dir(true)
	case "criado_em":
		ordem = "l.criado_em" + dir(true)
	case "titulo":
		ordem = "LOWER(l.titulo)" + dir(false)
	default: // urgência: abertos primeiro, do mais vencido ao mais distante; sem data por último
		ordem = `(` + leadAbertoSQL + `) DESC, l.proxima_acao_em` + dir(false) + ` NULLS LAST, l.nota DESC`
	}
	rows, err := db.Query(sel+cond+` ORDER BY `+ordem+`, l.id LIMIT `+q.arg(f.PerPage)+` OFFSET `+q.arg(f.Offset()), q.args...)
	if err != nil {
		return nil, 0, cont, err
	}
	defer rows.Close()
	leads := []Lead{}
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, 0, cont, err
		}
		leads = append(leads, *l)
	}
	return leads, total, cont, rows.Err()
}

// CreateLead grava o lead e preenche id e datas. Com ItemID (lead vindo de
// lista) a inserção é idempotente: se o item já tem lead, nada é gravado,
// l.ID recebe o id existente e criado volta false.
func CreateLead(db Executor, l *Lead) (criado bool, err error) {
	l.Titulo = cortarTexto(l.Titulo, 200)
	if l.Titulo == "" {
		return false, fmt.Errorf("informe o título do lead")
	}
	if l.Origem == "" {
		l.Origem = LeadOrigemManual
	}
	if l.Estado == "" {
		l.Estado = LeadNovo
	}
	if l.BaseLegal == "" {
		l.BaseLegal = "legitimo_interesse"
	}
	if l.Telefones == nil {
		l.Telefones = []ProspTelefone{}
	}
	if l.Emails == nil {
		l.Emails = []string{}
	}
	if l.Etiquetas == nil {
		l.Etiquetas = []string{}
	}
	err = db.QueryRow(`
		INSERT INTO leads (titulo, company_id, contact_id, cnpj, cidade, uf, origem, lista_id, item_id, owner_id,
		    estado, temperatura, etiquetas, nota, valor_estimado, telefones, emails, resumo, proxima_acao_em,
		    whatsapp_optin, pessoa_fisica, base_legal, criado_por)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		ON CONFLICT (item_id) WHERE item_id IS NOT NULL DO NOTHING
		RETURNING id, criado_em, atualizado_em`,
		l.Titulo, l.CompanyID, l.ContactID, NormalizarCNPJ(l.CNPJ), cortarTexto(l.Cidade, 120),
		cortarTexto(strings.ToUpper(l.UF), 2), l.Origem, l.ListaID, l.ItemID, l.OwnerID,
		l.Estado, l.Temperatura, pq.Array(l.Etiquetas), limitarNota(l.Nota), l.ValorEstimado,
		jsonOu(l.Telefones, "[]"), jsonOu(l.Emails, "[]"), jsonOu(l.Resumo, "{}"), l.ProximaAcaoEm,
		l.WhatsAppOptin, l.PessoaFisica, l.BaseLegal, l.CriadoPor,
	).Scan(&l.ID, &l.CriadoEm, &l.AtualizadoEm)
	if err == sql.ErrNoRows && l.ItemID != nil {
		err = db.QueryRow(`SELECT id, criado_em, atualizado_em FROM leads WHERE item_id = $1`, *l.ItemID).
			Scan(&l.ID, &l.CriadoEm, &l.AtualizadoEm)
		return false, err
	}
	return err == nil, err
}

// OpcionalFloat e OpcionalTempo: como OpcionalID, para valor e data anuláveis.
type OpcionalFloat struct {
	Definir bool
	Valor   *float64
}

type OpcionalTempo struct {
	Definir bool
	Em      *time.Time
}

// DefinirTempo grava o instante informado.
func DefinirTempo(t time.Time) OpcionalTempo { return OpcionalTempo{Definir: true, Em: &t} }

// LimparTempo grava nulo.
func LimparTempo() OpcionalTempo { return OpcionalTempo{Definir: true} }

// LeadPatch é a atualização parcial do lead: só o que vier preenchido é
// gravado. Serve ao PUT /leads/{id} e às transições da fila (resultado,
// converter, descartar, reabrir), que rodam em transação.
type LeadPatch struct {
	Titulo          *string
	Owner           OpcionalID
	Contact         OpcionalID
	Company         OpcionalID
	Temperatura     *string
	Etiquetas       *[]string
	ValorEstimado   OpcionalFloat
	Telefones       *[]ProspTelefone
	Emails          *[]string
	Resumo          *ResumoLead
	ProximaAcaoEm   OpcionalTempo
	WhatsAppOptin   *bool
	Nota            *int
	Estado          *string // descartado carimba descartado_em; convertido, convertido_em
	Tentativas      *int
	UltimoContatoEm *time.Time
	UltimoResultado *string
	MotivoDescarte  *string
	Deal            OpcionalID
	Meeting         OpcionalID
	LiberarTrava    bool // limpa em_atendimento_por / em_atendimento_ate
}

// UpdateLead aplica o patch. sql.ErrNoRows quando o lead não existe.
func UpdateLead(db Executor, id int64, p LeadPatch) error {
	sets := []string{"atualizado_em = NOW()"}
	args := []any{id}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if p.Titulo != nil {
		t := cortarTexto(*p.Titulo, 200)
		if t == "" {
			return fmt.Errorf("informe o título do lead")
		}
		add("titulo", t)
	}
	if p.Owner.Definir {
		add("owner_id", p.Owner.ID)
	}
	if p.Contact.Definir {
		add("contact_id", p.Contact.ID)
	}
	if p.Company.Definir {
		add("company_id", p.Company.ID)
	}
	if p.Temperatura != nil {
		if !LeadTemperaturaValida(*p.Temperatura) {
			return fmt.Errorf("temperatura inválida")
		}
		add("temperatura", *p.Temperatura)
	}
	if p.Etiquetas != nil {
		add("etiquetas", pq.Array(limparEtiquetas(*p.Etiquetas)))
	}
	if p.ValorEstimado.Definir {
		add("valor_estimado", p.ValorEstimado.Valor)
	}
	if p.Telefones != nil {
		add("telefones", jsonOu(*p.Telefones, "[]"))
	}
	if p.Emails != nil {
		add("emails", jsonOu(*p.Emails, "[]"))
	}
	if p.Resumo != nil {
		add("resumo", jsonOu(*p.Resumo, "{}"))
	}
	if p.ProximaAcaoEm.Definir {
		add("proxima_acao_em", p.ProximaAcaoEm.Em)
	}
	if p.WhatsAppOptin != nil {
		add("whatsapp_optin", *p.WhatsAppOptin)
		if *p.WhatsAppOptin {
			sets = append(sets, "whatsapp_optin_em = COALESCE(whatsapp_optin_em, NOW())")
		} else {
			sets = append(sets, "whatsapp_optin_em = NULL")
		}
	}
	if p.Nota != nil {
		add("nota", limitarNota(*p.Nota))
	}
	if p.Estado != nil {
		if !LeadEstadoValido(*p.Estado) {
			return fmt.Errorf("estado de lead inválido: %q", *p.Estado)
		}
		add("estado", *p.Estado)
		switch *p.Estado {
		case LeadDescartado:
			sets = append(sets, "descartado_em = NOW()")
		case LeadConvertido:
			sets = append(sets, "convertido_em = COALESCE(convertido_em, NOW())")
		default:
			sets = append(sets, "descartado_em = NULL")
		}
	}
	if p.Tentativas != nil {
		add("tentativas", max(0, *p.Tentativas))
	}
	if p.UltimoContatoEm != nil {
		add("ultimo_contato_em", *p.UltimoContatoEm)
	}
	if p.UltimoResultado != nil {
		add("ultimo_resultado", *p.UltimoResultado)
	}
	if p.MotivoDescarte != nil {
		add("motivo_descarte", cortarTexto(*p.MotivoDescarte, 80))
	}
	if p.Deal.Definir {
		add("deal_id", p.Deal.ID)
	}
	if p.Meeting.Definir {
		add("meeting_id", p.Meeting.ID)
	}
	if p.LiberarTrava {
		sets = append(sets, "em_atendimento_por = NULL", "em_atendimento_ate = NULL")
	}
	res, err := db.Exec(`UPDATE leads SET `+strings.Join(sets, ", ")+` WHERE id = $1`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// limparEtiquetas apara, tira vazias e repetidas e limita a 20 etiquetas de 40 caracteres.
func limparEtiquetas(in []string) []string {
	out := []string{}
	for _, e := range in {
		e = cortarTexto(e, 40)
		if e != "" && !slices.Contains(out, e) && len(out) < 20 {
			out = append(out, e)
		}
	}
	return out
}

// DeleteLead apaga o lead (bloqueios e chamadas ficam). sql.ErrNoRows quando não existe.
func DeleteLead(db Executor, id int64) error {
	res, err := db.Exec(`DELETE FROM leads WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MarcarLeadVisto carimba a primeira abertura da ficha.
func MarcarLeadVisto(db Executor, id int64) error {
	_, err := db.Exec(`UPDATE leads SET visto_em = NOW() WHERE id = $1 AND visto_em IS NULL`, id)
	return err
}

// Ações de POST /leads/bulk.
const (
	LeadBulkDono        = "dono"
	LeadBulkTemperatura = "temperatura"
	LeadBulkEtiqueta    = "etiqueta"
	LeadBulkDescartar   = "descartar"
	LeadBulkReabrir     = "reabrir"

	// MaxLeadsBulk é o teto de ids por ação em massa.
	MaxLeadsBulk = 500
)

// LeadBulk é uma ação em massa. O controller confere a permissão (dono exige
// leads.assign; as demais, leads.edit) e a validade de motivo e dono.
type LeadBulk struct {
	IDs         []int64
	Acao        string
	OwnerID     *int64    // dono (nil = tirar o dono)
	Temperatura string    // temperatura
	Etiqueta    string    // etiqueta
	Motivo      string    // descartar
	ProximaAcao time.Time // reabrir: "agora" já ajustado à janela
}

// BulkLeads aplica a ação aos ids que estão dentro da visão e devolve quantos
// mudaram. Ids fora da visão, ou em estado que a ação não alcança, são
// ignorados (não contam em afetados).
func BulkLeads(db Executor, v LeadVisao, b LeadBulk) (int, error) {
	if len(b.IDs) == 0 {
		return 0, nil
	}
	if len(b.IDs) > MaxLeadsBulk {
		return 0, fmt.Errorf("a ação em massa aceita até %d leads por vez", MaxLeadsBulk)
	}
	q := &leadConsulta{}
	alvo := `l.id = ANY(` + q.arg(pq.Array(b.IDs)) + `) AND ` + q.visivel(v)
	var set, extra string
	switch b.Acao {
	case LeadBulkDono:
		set = `owner_id = ` + q.arg(b.OwnerID) + `::bigint`
	case LeadBulkTemperatura:
		if !LeadTemperaturaValida(b.Temperatura) {
			return 0, fmt.Errorf("temperatura inválida")
		}
		set = `temperatura = ` + q.arg(b.Temperatura)
	case LeadBulkEtiqueta:
		e := cortarTexto(b.Etiqueta, 40)
		if e == "" {
			return 0, fmt.Errorf("informe a etiqueta")
		}
		p := q.arg(e)
		set = `etiquetas = array_append(x.etiquetas, ` + p + `::text)`
		extra = ` AND NOT (` + p + `::text = ANY(l.etiquetas)) AND cardinality(l.etiquetas) < 20`
	case LeadBulkDescartar:
		motivo := cortarTexto(b.Motivo, 80)
		if motivo == "" {
			return 0, fmt.Errorf("informe o motivo do descarte")
		}
		set = `estado = 'descartado', motivo_descarte = ` + q.arg(motivo) + `, descartado_em = NOW(),
		       proxima_acao_em = NULL, em_atendimento_por = NULL, em_atendimento_ate = NULL`
		extra = ` AND ` + leadAbertoSQL
	case LeadBulkReabrir:
		quando := b.ProximaAcao
		if quando.IsZero() {
			quando = time.Now()
		}
		set = `estado = CASE WHEN EXISTS (SELECT 1 FROM calls k WHERE k.lead_id = x.id) THEN 'tentando' ELSE 'novo' END,
		       tentativas = 0, motivo_descarte = '', descartado_em = NULL, proxima_acao_em = ` + q.arg(quando)
		extra = ` AND l.estado = 'descartado'`
	default:
		return 0, fmt.Errorf("ação em massa inválida")
	}
	res, err := db.Exec(`
		UPDATE leads x SET `+set+`, atualizado_em = NOW()
		WHERE x.id IN (SELECT l.id FROM leads l LEFT JOIN users ow ON ow.id = l.owner_id WHERE `+alvo+extra+`)`, q.args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DescartarLead encerra um lead aberto com o motivo (rótulo configurado ou
// código reservado). ErrLeadEncerrado se já estava convertido ou descartado.
func DescartarLead(db Executor, id int64, motivo string) error {
	res, err := db.Exec(`
		UPDATE leads SET estado = 'descartado', motivo_descarte = $2, descartado_em = NOW(), proxima_acao_em = NULL,
		       em_atendimento_por = NULL, em_atendimento_ate = NULL, atualizado_em = NOW()
		WHERE id = $1 AND estado IN ('novo','tentando','retornar')`, id, cortarTexto(motivo, 80))
	return leadMudou(db, id, res, err, ErrLeadEncerrado)
}

// ReabrirLead devolve um descartado à fila: "tentando" se já houve ligação,
// senão "novo"; zera tentativas, limpa o motivo e agenda a próxima ação em
// "quando" (agora ajustado à janela). Não remove bloqueios. ErrLeadEstado se
// o lead não estava descartado.
func ReabrirLead(db Executor, id int64, quando time.Time) error {
	res, err := db.Exec(`
		UPDATE leads SET
		       estado = CASE WHEN EXISTS (SELECT 1 FROM calls k WHERE k.lead_id = leads.id) THEN 'tentando' ELSE 'novo' END,
		       tentativas = 0, motivo_descarte = '', descartado_em = NULL, proxima_acao_em = $2, atualizado_em = NOW()
		WHERE id = $1 AND estado = 'descartado'`, id, quando)
	return leadMudou(db, id, res, err, ErrLeadEstado)
}

// ConverterLead marca um lead aberto como convertido, com o negócio (e a
// reunião, se houver). ErrLeadEncerrado se já estava convertido ou descartado.
func ConverterLead(db Executor, id, dealID int64, meetingID *int64) error {
	res, err := db.Exec(`
		UPDATE leads SET estado = 'convertido', convertido_em = NOW(), deal_id = $2,
		       meeting_id = COALESCE($3::bigint, meeting_id), proxima_acao_em = NULL,
		       em_atendimento_por = NULL, em_atendimento_ate = NULL, atualizado_em = NOW()
		WHERE id = $1 AND estado IN ('novo','tentando','retornar')`, id, dealID, meetingID)
	return leadMudou(db, id, res, err, ErrLeadEncerrado)
}

// leadMudou traduz "zero linhas" em sql.ErrNoRows (não existe) ou no erro de estado.
func leadMudou(db Executor, id int64, res sql.Result, err, erroEstado error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var existe bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM leads WHERE id = $1)`, id).Scan(&existe); err != nil {
		return err
	}
	if !existe {
		return sql.ErrNoRows
	}
	return erroEstado
}

// LeadAbertoDaEmpresa devolve o id do lead aberto (novo, tentando ou
// retornar) mais antigo da empresa, ou nil. Serve a duplicados = "vincular":
// não se cria um segundo lead aberto para a mesma empresa.
func LeadAbertoDaEmpresa(db Executor, companyID int64) (*int64, error) {
	var id int64
	err := db.QueryRow(`
		SELECT id FROM leads WHERE company_id = $1 AND estado IN ('novo','tentando','retornar')
		ORDER BY id LIMIT 1`, companyID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// LeadListaResumo é a linha de GET /leads/listas.
type LeadListaResumo struct {
	ID      int64  `json:"id"`
	Nome    string `json:"nome"`
	Abertos int    `json:"abertos"`
}

// ListasDosLeads devolve as listas de prospecção que têm leads visíveis ao
// usuário, com quantos estão abertos (filtro de /leads e seletor da fila).
func ListasDosLeads(db *sql.DB, v LeadVisao) ([]LeadListaResumo, error) {
	q := &leadConsulta{}
	rows, err := db.Query(`
		SELECT pl.id, pl.nome, COUNT(*) FILTER (WHERE `+leadAbertoSQL+`)
		FROM leads l
		JOIN prospeccao_listas pl ON pl.id = l.lista_id
		LEFT JOIN users ow ON ow.id = l.owner_id
		WHERE `+q.visivel(v)+`
		GROUP BY pl.id, pl.nome
		ORDER BY pl.id DESC`, q.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeadListaResumo{}
	for rows.Next() {
		var r LeadListaResumo
		if err := rows.Scan(&r.ID, &r.Nome, &r.Abertos); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// -----------------------------------------------------------------------------
// Fila de ligações: trava de atendimento, próximo lead e histórico.
// -----------------------------------------------------------------------------

// FilaOpcoes é o pedido de POST /leads/fila/proximo.
type FilaOpcoes struct {
	UserID         int64
	ListaID        int64   // 0 = qualquer lista
	PularIDs       []int64 // leads pulados nesta sessão (até 200)
	IncluirSemDono bool    // configuração incluir_sem_dono_na_fila
}

// MaxPularIDs é o teto de pular_ids.
const MaxPularIDs = 200

// elegivel devolve a condição "pode sair na fila do usuário" (sem a trava).
func (o FilaOpcoes) elegivel(q *leadConsulta) string {
	u := q.arg(o.UserID)
	cond := leadAbertoSQL + ` AND (l.owner_id = ` + u
	if o.IncluirSemDono {
		cond += ` OR ` + leadSemDonoSQL
	}
	cond += `) AND (l.proxima_acao_em <= NOW() OR (l.proxima_acao_em IS NULL AND l.estado = 'novo'))
		AND NOT EXISTS (SELECT 1 FROM contato_bloqueios b WHERE b.tipo = 'cnpj' AND b.valor = l.cnpj AND l.cnpj <> '')`
	if o.ListaID > 0 {
		cond += ` AND l.lista_id = ` + q.arg(o.ListaID)
	}
	return cond
}

// ProximoLeadDaFila escolhe e trava o próximo lead do usuário. Primeiro solta
// as outras travas dele; se ele já tem um lead travado que não está em
// PularIDs, devolve esse (recarregar a página não troca de lead). Senão pega,
// com FOR UPDATE SKIP LOCKED, o primeiro elegível: retornos vencidos (mais
// antigo primeiro), depois tentativas vencidas, depois novos por nota; sem
// trava válida de outra pessoa e sem bloqueio de CNPJ. A trava dura
// TravaFilaDuracao. Devolve nil, nil quando a fila está vazia. Quem chama
// confere antes a janela de contato (fora dela nem consulta).
func ProximoLeadDaFila(db *sql.DB, o FilaOpcoes) (*Lead, error) {
	if len(o.PularIDs) > MaxPularIDs {
		return nil, fmt.Errorf("pular_ids aceita até %d leads", MaxPularIDs)
	}
	if o.PularIDs == nil {
		o.PularIDs = []int64{}
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Lead que o usuário já estava atendendo.
	var id int64
	err = tx.QueryRow(`
		SELECT id FROM leads l
		WHERE l.em_atendimento_por = $1 AND l.em_atendimento_ate > NOW() AND `+leadAbertoSQL+`
		  AND NOT (l.id = ANY($2))
		ORDER BY l.em_atendimento_ate DESC LIMIT 1 FOR UPDATE OF l SKIP LOCKED`,
		o.UserID, pq.Array(o.PularIDs)).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if _, err := tx.Exec(`
		UPDATE leads SET em_atendimento_por = NULL, em_atendimento_ate = NULL
		WHERE em_atendimento_por = $1 AND id <> $2`, o.UserID, id); err != nil {
		return nil, err
	}

	if id == 0 {
		q := &leadConsulta{}
		cond := o.elegivel(q)
		err = tx.QueryRow(`
			SELECT l.id FROM leads l
			LEFT JOIN users ow ON ow.id = l.owner_id
			WHERE `+cond+`
			  AND (l.em_atendimento_por IS NULL OR l.em_atendimento_ate <= NOW())
			  AND NOT (l.id = ANY(`+q.arg(pq.Array(o.PularIDs))+`))
			ORDER BY CASE l.estado WHEN 'retornar' THEN 0 WHEN 'tentando' THEN 1 ELSE 2 END,
			         CASE WHEN l.estado = 'novo' THEN NULL ELSE l.proxima_acao_em END NULLS LAST,
			         l.nota DESC, l.id
			LIMIT 1 FOR UPDATE OF l SKIP LOCKED`, q.args...).Scan(&id)
		if err == sql.ErrNoRows {
			return nil, tx.Commit() // fila vazia (as travas antigas foram soltas)
		}
		if err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(`
		UPDATE leads SET em_atendimento_por = $2, em_atendimento_ate = NOW() + make_interval(secs => $3)
		WHERE id = $1`, id, o.UserID, TravaFilaDuracao.Seconds()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return LeadByID(db, id, VisaoTotal(o.UserID))
}

// LiberarLead solta a trava se ela for do usuário (de quem não a detém, não
// faz nada). Devolve se soltou.
func LiberarLead(db Executor, id, userID int64) (bool, error) {
	res, err := db.Exec(`
		UPDATE leads SET em_atendimento_por = NULL, em_atendimento_ate = NULL
		WHERE id = $1 AND em_atendimento_por = $2`, id, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RenovarTravaLead estende por TravaFilaDuracao a trava que o usuário detém
// (GET /leads/{id} feito pelo dono da trava). Sem trava dele, não faz nada.
func RenovarTravaLead(db Executor, id, userID int64) error {
	_, err := db.Exec(`
		UPDATE leads SET em_atendimento_ate = NOW() + make_interval(secs => $3)
		WHERE id = $1 AND em_atendimento_por = $2 AND em_atendimento_ate > NOW()`,
		id, userID, TravaFilaDuracao.Seconds())
	return err
}

// PrepararResultadoLead é a primeira instrução de POST /leads/{id}/resultado,
// dentro da transação de quem chama: trava a linha (SELECT ... FOR UPDATE),
// confere a visão, o estado e a trava de atendimento, e solta a trava.
// Erros: sql.ErrNoRows (não existe ou fora da visão → 404), ErrLeadEncerrado
// (409, codigo "estado") e *ErroLeadEmAtendimento (423). Devolve o lead como
// estava, já sem a trava.
func PrepararResultadoLead(tx *sql.Tx, id int64, v LeadVisao) (*Lead, error) {
	var estado string
	var por sql.NullInt64
	var travado bool
	var nome string
	err := tx.QueryRow(`
		SELECT l.estado, l.em_atendimento_por, COALESCE(l.em_atendimento_ate > NOW(), FALSE),
		       COALESCE((SELECT u.name FROM users u WHERE u.id = l.em_atendimento_por), '')
		FROM leads l WHERE l.id = $1 FOR UPDATE OF l`, id).Scan(&estado, &por, &travado, &nome)
	if err != nil {
		return nil, err
	}
	lead, err := LeadByID(tx, id, v)
	if err != nil {
		return nil, err
	}
	if !LeadEstadoAberto(estado) {
		return nil, ErrLeadEncerrado
	}
	if por.Valid && travado && por.Int64 != v.UserID {
		return nil, &ErroLeadEmAtendimento{UserID: por.Int64, Nome: nome}
	}
	if _, err := tx.Exec(`
		UPDATE leads SET em_atendimento_por = NULL, em_atendimento_ate = NULL, atualizado_em = NOW()
		WHERE id = $1`, id); err != nil {
		return nil, err
	}
	lead.EmAtendimentoPor, lead.EmAtendimentoAte = nil, nil
	return lead, nil
}

// Tentativa é uma ligação do histórico do lead (interface Tentativa).
type Tentativa struct {
	ID              int64     `json:"id"`
	Resultado       string    `json:"resultado"`
	Numero          string    `json:"numero"`
	DuracaoSegundos int       `json:"duracao_segundos"`
	Notas           string    `json:"notas"`
	UserName        string    `json:"user_name"`
	CriadoEm        time.Time `json:"criado_em"`
}

// TentativasDoLead devolve todas as ligações do lead, da mais recente para a
// mais antiga (lidas de calls: lead_id, desfecho, numero).
func TentativasDoLead(db Executor, leadID int64) ([]Tentativa, error) {
	rows, err := db.Query(`
		SELECT k.id, k.desfecho, k.numero, k.duration_seconds, COALESCE(k.notes,''), COALESCE(u.name,''), k.called_at
		FROM calls k LEFT JOIN users u ON u.id = k.user_id
		WHERE k.lead_id = $1 ORDER BY k.called_at DESC, k.id DESC`, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tentativa{}
	for rows.Next() {
		var t Tentativa
		if err := rows.Scan(&t.ID, &t.Resultado, &t.Numero, &t.DuracaoSegundos, &t.Notas, &t.UserName, &t.CriadoEm); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TentativasSemContatoDesde conta as ligações sem contato (não atendeu, caixa
// postal, ocupado) do lead desde o instante informado: com o início do dia,
// responde se o limite tentativas_dia já foi atingido.
func TentativasSemContatoDesde(db Executor, leadID int64, desde time.Time) (int, error) {
	var n int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM calls WHERE lead_id = $1 AND called_at >= $2 AND desfecho = ANY($3)`,
		leadID, desde, pq.Array(ResultadosSemContato)).Scan(&n)
	return n, err
}

// LeadTemLigacao diz se o lead já teve alguma ligação registrada.
func LeadTemLigacao(db Executor, leadID int64) (bool, error) {
	var tem bool
	err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM calls WHERE lead_id = $1)`, leadID).Scan(&tem)
	return tem, err
}

// NumerosDaFila são os números de GET /leads/fila/resumo que vêm do banco (a
// meta do dia e a janela vêm das configurações).
type NumerosDaFila struct {
	NaFila           int `json:"na_fila"`
	Novos            int `json:"novos"`
	RetornosVencidos int `json:"retornos_vencidos"`
	RetornosHoje     int `json:"retornos_hoje"`
	FeitasHoje       int `json:"feitas_hoje"`
	ConectadasHoje   int `json:"conectadas_hoje"`
	ReunioesHoje     int `json:"reunioes_hoje"`
}

// ResumoDaFila conta a fila do usuário (mesma regra de elegibilidade de
// ProximoLeadDaFila, sem olhar travas) e as ligações que ele fez hoje.
func ResumoDaFila(db *sql.DB, o FilaOpcoes, agora time.Time) (NumerosDaFila, error) {
	var n NumerosDaFila
	agora = agora.In(time.Local)
	hoje := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.Local)
	amanha := hoje.AddDate(0, 0, 1)

	q := &leadConsulta{}
	cond := o.elegivel(q)
	if err := db.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE l.estado = 'novo'),
		       COUNT(*) FILTER (WHERE l.estado = 'retornar')
		FROM leads l LEFT JOIN users ow ON ow.id = l.owner_id
		WHERE `+cond, q.args...).Scan(&n.NaFila, &n.Novos, &n.RetornosVencidos); err != nil {
		return n, err
	}
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM leads l
		WHERE l.owner_id = $1 AND l.estado = 'retornar' AND l.proxima_acao_em > NOW() AND l.proxima_acao_em < $2`,
		o.UserID, amanha).Scan(&n.RetornosHoje); err != nil {
		return n, err
	}
	err := db.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE outcome = 'conectada'),
		       COUNT(*) FILTER (WHERE desfecho = 'reuniao')
		FROM calls WHERE user_id = $1 AND lead_id IS NOT NULL AND called_at >= $2 AND called_at < $3`,
		o.UserID, hoje, amanha).Scan(&n.FeitasHoje, &n.ConectadasHoje, &n.ReunioesHoje)
	return n, err
}
