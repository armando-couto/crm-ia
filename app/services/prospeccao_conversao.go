package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// Conversão de lead em negócio (POST /leads/{id}/converter e o resultado
// "reuniao" da fila). Contrato: docs/PROSPECCAO.md seção 8.2 com as correções
// de docs/PROSPECCAO-ADENDO.md (destino do negócio, origem, ordem dos passos).

// ErroConversao é um pedido de conversão que não pode ser atendido como veio
// (422 com a mensagem).
type ErroConversao struct{ Msg string }

func (e *ErroConversao) Error() string { return e.Msg }

// ErrAgendaOcupada: o dono da reunião já tem compromisso no horário (409,
// codigo "agenda_ocupada"); o pedido pode ser reenviado com forcar.
var ErrAgendaOcupada = errors.New("o horário escolhido já está ocupado na agenda")

// DestinoNegocio é o funil e a etapa onde o negócio nasce.
type DestinoNegocio struct {
	PipelineID int64
	StageID    int64
}

type etapaFunil struct {
	id, pipelineID int64
	nome           string
	aberta         bool
}

// etapasDosFunis lê as etapas na ordem dos funis e das posições.
func etapasDosFunis(db models.Executor) ([]etapaFunil, error) {
	rows, err := db.Query(`
		SELECT s.id, s.pipeline_id, s.name, NOT (s.is_won OR s.is_lost)
		FROM pipeline_stages s JOIN pipelines p ON p.id = s.pipeline_id
		ORDER BY p.position, p.id, s.position, s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var etapas []etapaFunil
	for rows.Next() {
		var e etapaFunil
		if err := rows.Scan(&e.id, &e.pipelineID, &e.nome, &e.aberta); err != nil {
			return nil, err
		}
		etapas = append(etapas, e)
	}
	return etapas, rows.Err()
}

// ResolverDestino escolhe o funil e a etapa do negócio: o primeiro par válido
// entre o pedido, a lista do lead e as configurações (válido = a etapa existe,
// pertence ao funil e não é de ganho nem de perda; funil sem etapa usa a
// primeira etapa aberta dele); sem par válido, a primeira etapa aberta do
// primeiro funil cujo nome contém "reuni" (sem caixa), senão a primeira etapa
// aberta do primeiro funil. Etapa do pedido que não pertence ao funil do
// pedido é *ErroConversao (422).
//
// "Primeira etapa cujo nome contém reuni" é procurada no primeiro funil (a
// pergunta "em qual funil" do adendo); funil sem etapa aberta é pulado.
func ResolverDestino(db models.Executor, pedidoPipeline, pedidoStage *int64, lead *models.Lead, cfg models.ConfigProspeccao) (DestinoNegocio, error) {
	etapas, err := etapasDosFunis(db)
	if err != nil {
		return DestinoNegocio{}, err
	}
	porID := map[int64]etapaFunil{}
	for _, e := range etapas {
		porID[e.id] = e
	}
	primeiraAberta := func(pipelineID int64) (int64, bool) {
		for _, e := range etapas {
			if e.pipelineID == pipelineID && e.aberta {
				return e.id, true
			}
		}
		return 0, false
	}
	valido := func(pipelineID, stageID *int64) (DestinoNegocio, bool) {
		switch {
		case pipelineID == nil && stageID == nil:
			return DestinoNegocio{}, false
		case stageID != nil:
			e, ok := porID[*stageID]
			if !ok || !e.aberta || (pipelineID != nil && e.pipelineID != *pipelineID) {
				return DestinoNegocio{}, false
			}
			return DestinoNegocio{PipelineID: e.pipelineID, StageID: e.id}, true
		default:
			if s, ok := primeiraAberta(*pipelineID); ok {
				return DestinoNegocio{PipelineID: *pipelineID, StageID: s}, true
			}
			return DestinoNegocio{}, false
		}
	}

	if pedidoStage != nil && pedidoPipeline != nil {
		if e, ok := porID[*pedidoStage]; ok && e.pipelineID != *pedidoPipeline {
			return DestinoNegocio{}, &ErroConversao{"a etapa não pertence ao funil informado"}
		}
	}
	candidatos := [][2]*int64{{pedidoPipeline, pedidoStage}}
	if lead != nil && lead.ListaID != nil {
		var p, s sql.NullInt64
		err := db.QueryRow(`SELECT pipeline_id, stage_id FROM prospeccao_listas WHERE id = $1`, *lead.ListaID).Scan(&p, &s)
		if err != nil && err != sql.ErrNoRows {
			return DestinoNegocio{}, err
		}
		candidatos = append(candidatos, [2]*int64{nulo(p), nulo(s)})
	}
	candidatos = append(candidatos, [2]*int64{cfg.PipelineID, cfg.StageID})
	for _, c := range candidatos {
		if d, ok := valido(c[0], c[1]); ok {
			return d, nil
		}
	}

	if len(etapas) > 0 {
		primeiro := etapas[0].pipelineID
		for _, e := range etapas {
			if e.pipelineID == primeiro && e.aberta && strings.Contains(strings.ToLower(e.nome), "reuni") {
				return DestinoNegocio{PipelineID: e.pipelineID, StageID: e.id}, nil
			}
		}
		if s, ok := primeiraAberta(primeiro); ok {
			return DestinoNegocio{PipelineID: primeiro, StageID: s}, nil
		}
	}
	for _, e := range etapas {
		if e.aberta {
			return DestinoNegocio{PipelineID: e.pipelineID, StageID: e.id}, nil
		}
	}
	return DestinoNegocio{}, &ErroConversao{"cadastre um funil com uma etapa aberta antes de converter leads"}
}

func nulo(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

// usuarioAtivo confere que o usuário existe e está ativo.
func usuarioAtivo(db models.Executor, id int64) (string, bool, error) {
	var nome string
	var ativo bool
	err := db.QueryRow(`SELECT name, active FROM users WHERE id = $1`, id).Scan(&nome, &ativo)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return nome, ativo, err
}

// registrarAtividade grava a atividade na transação de quem chama (mesmo
// INSERT de models.CreateActivity).
func registrarAtividade(db models.Executor, a *models.Activity) error {
	return db.QueryRow(`
		INSERT INTO activities (kind, content, user_id, contact_id, company_id, deal_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		a.Kind, a.Content, a.UserID, a.ContactID, a.CompanyID, a.DealID,
	).Scan(&a.ID, &a.CreatedAt)
}

// criarReuniao grava a reunião na transação de quem chama (mesmo INSERT de
// models.CreateMeeting).
func criarReuniao(db models.Executor, m *models.Meeting) error {
	if m.Status == "" {
		m.Status = "agendada"
	}
	return db.QueryRow(`
		INSERT INTO meetings (title, status, starts_at, ends_at, location, notes, contact_id, company_id, deal_id, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`,
		m.Title, m.Status, m.StartsAt, m.EndsAt, m.Location, m.Notes,
		m.ContactID, m.CompanyID, m.DealID, m.UserID,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

// NovoNegocio é o pedido de criação do negócio a partir do lead.
type NovoNegocio struct {
	Nome    string
	Valor   *float64
	OwnerID int64
	Destino DestinoNegocio
}

// criarNegocioDoLead cria o negócio (origem = origem do lead, lead_id) na
// transação de quem chama. Valor: o informado, senão o valor estimado do lead.
func criarNegocioDoLead(tx models.Executor, lead *models.Lead, n NovoNegocio) (*models.Deal, error) {
	nome := strings.TrimSpace(n.Nome)
	if nome == "" {
		nome = strings.TrimSpace(lead.CompanyName)
	}
	if nome == "" {
		nome = lead.Titulo
	}
	if len([]rune(nome)) > 200 {
		nome = string([]rune(nome)[:200])
	}
	valor := 0.0
	switch {
	case n.Valor != nil:
		valor = *n.Valor
	case lead.ValorEstimado != nil:
		valor = *lead.ValorEstimado
	}
	if valor < 0 {
		return nil, &ErroConversao{"o valor do negócio não pode ser negativo"}
	}
	owner := n.OwnerID
	leadID := lead.ID
	d := &models.Deal{
		Name:        nome,
		Amount:      valor,
		PipelineID:  n.Destino.PipelineID,
		StageID:     n.Destino.StageID,
		ContactID:   lead.ContactID,
		CompanyID:   lead.CompanyID,
		OwnerID:     &owner,
		Temperature: temperaturaDoNegocio(lead.Temperatura),
		LeadID:      &leadID,
		Origem:      lead.Origem,
	}
	if err := models.CreateDeal(tx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// temperaturaDoNegocio traduz a temperatura do lead (fria|media|quente) para
// a do negócio, que usa os mesmos valores.
func temperaturaDoNegocio(t string) string {
	if models.ValidDealTemperature(t) {
		return t
	}
	return ""
}

// ConverterPedido é o corpo de POST /leads/{id}/converter.
type ConverterPedido struct {
	PipelineID *int64   `json:"pipeline_id"`
	StageID    *int64   `json:"stage_id"`
	Nome       string   `json:"nome"`
	Valor      *float64 `json:"valor"`
	OwnerID    *int64   `json:"owner_id"`
}

// ConversaoLead é a resposta de POST /leads/{id}/converter.
type ConversaoLead struct {
	Lead *models.Lead `json:"lead"`
	Deal *models.Deal `json:"deal"`
}

// ConverterLeadEmNegocio converte o lead aberto em negócio numa transação:
// confere visão e estado (sql.ErrNoRows → 404; models.ErrLeadEncerrado →
// 409), resolve o destino, cria o negócio (dono: owner_id, senão o dono do
// lead, senão quem converte) e marca o lead convertido. Lead sem dono passa a
// ser de quem converteu.
func ConverterLeadEmNegocio(db *sql.DB, id int64, v models.LeadVisao, p ConverterPedido) (*ConversaoLead, error) {
	cfg := models.LoadConfigProspeccao(db)
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SELECT 1 FROM leads WHERE id = $1 FOR UPDATE`, id); err != nil {
		return nil, err
	}
	lead, err := models.LeadByID(tx, id, v)
	if err != nil {
		return nil, err
	}
	if !lead.Aberto() {
		return nil, models.ErrLeadEncerrado
	}
	dono := v.UserID
	switch {
	case p.OwnerID != nil && *p.OwnerID > 0:
		dono = *p.OwnerID
	case !lead.SemDono():
		dono = *lead.OwnerID
	}
	if _, ativo, err := usuarioAtivo(tx, dono); err != nil {
		return nil, err
	} else if !ativo {
		return nil, &ErroConversao{"o responsável pelo negócio não existe ou está inativo"}
	}
	destino, err := ResolverDestino(tx, p.PipelineID, p.StageID, lead, cfg)
	if err != nil {
		return nil, err
	}
	deal, err := criarNegocioDoLead(tx, lead, NovoNegocio{Nome: p.Nome, Valor: p.Valor, OwnerID: dono, Destino: destino})
	if err != nil {
		return nil, err
	}
	if lead.SemDono() {
		eu := v.UserID
		if err := models.UpdateLead(tx, id, models.LeadPatch{Owner: models.DefinirID(eu)}); err != nil {
			return nil, err
		}
	}
	if err := models.ConverterLead(tx, id, deal.ID, nil); err != nil {
		return nil, err
	}
	autor := v.UserID
	if err := registrarAtividade(tx, &models.Activity{
		Kind:      models.ActivitySistema,
		Content:   fmt.Sprintf("Negócio criado a partir do lead %q", lead.Titulo),
		UserID:    &autor,
		DealID:    &deal.ID,
		ContactID: deal.ContactID,
		CompanyID: deal.CompanyID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	depoisDeCriarNegocio(db, deal, v.UserID)

	out := &ConversaoLead{}
	if out.Lead, err = models.LeadByID(db, id, models.VisaoTotal(v.UserID)); err != nil {
		return nil, err
	}
	if out.Deal, err = models.DealByID(db, deal.ID); err != nil {
		return nil, err
	}
	return out, nil
}

// depoisDeCriarNegocio dispara, fora da transação, o que a criação de negócio
// pelo quadro também dispara: aviso ao dono e automações de "negócio criado".
func depoisDeCriarNegocio(db *sql.DB, deal *models.Deal, autor int64) {
	d := *deal
	EmSegundoPlano(func() {
		NotifyAssignment(db, d.OwnerID, autor, NotifyDeal, d.Name, fmt.Sprintf("/negocios/%d", d.ID))
	})
	EmSegundoPlano(func() { FireDealCreated(db, &d) })
}

// fimDaReuniao devolve o fim a partir do início e da duração em minutos.
func fimDaReuniao(inicio time.Time, duracaoMin int) time.Time {
	return inicio.Add(time.Duration(duracaoMin) * time.Minute)
}
