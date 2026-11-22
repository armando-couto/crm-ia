package controllers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// Leads: caixa de leads (listar, criar, editar, ações em massa, converter,
// descartar, reabrir). Contrato: docs/PROSPECCAO.md seção 8.2, com as correções
// de docs/PROSPECCAO-ADENDO.md (visibilidade por dono: sem leads.assign só se
// enxerga o próprio lead, o sem dono e o de dono inativo; fora disso, 404).
//
// Sem leads.assign: fila=todos e owner_id alheio respondem 403; owner_id em
// POST só pode ser o próprio usuário (ou nulo); em PUT, só para assumir um lead
// sem dono; bulk "dono" exige leads.assign; DELETE exige leads.assign (rota).

const entidadeLead = "lead"

// leadsAgora é o relógio da caixa de leads (trocado nos testes).
var leadsAgora = time.Now

// leadsVisao monta a visão de quem chama.
func leadsVisao(ctx iris.Context) (models.LeadVisao, *services.Claims) {
	claims := middlewareClaims(ctx)
	return models.LeadVisao{UserID: claims.UserID, VeTodos: models.RoleCan(claims.Role, models.PermLeadsAssign)}, claims
}

func proibido(ctx iris.Context, msg string) {
	ctx.StopWithJSON(iris.StatusForbidden, iris.Map{"error": msg})
}

// responderLead relê o lead (com nomes e urgência) e responde.
func responderLead(ctx iris.Context, id int64, status int) {
	l, err := models.LeadByID(utils.DB, id, models.VisaoTotal(0))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if status != iris.StatusOK {
		ctx.StatusCode(status)
	}
	ctx.JSON(l)
}

// donoAtivo confere que o usuário existe e está ativo.
func donoAtivo(id int64) bool {
	u, err := models.UserByID(utils.DB, id)
	return err == nil && u.Active
}

// normalizarTelefonesLead limpa os telefones informados: só dígitos com DDD,
// sem repetir, tipo pelo tamanho quando não vier, origem "manual" e o
// primeiro como principal.
func normalizarTelefonesLead(in []models.ProspTelefone) ([]models.ProspTelefone, string) {
	out := []models.ProspTelefone{}
	vistos := map[string]bool{}
	for _, t := range in {
		n := models.NormalizarTelefone(t.Numero)
		if n == "" {
			return nil, fmt.Sprintf("telefone inválido: %q (informe com DDD)", t.Numero)
		}
		if vistos[n] {
			continue
		}
		vistos[n] = true
		t.Numero = n
		switch t.Tipo {
		case models.TelefoneCelular, models.TelefoneFixo, models.TelefoneIndefinido:
		case "":
			t.Tipo = models.TelefoneFixo
			if len(n) == 11 {
				t.Tipo = models.TelefoneCelular
			}
		default:
			return nil, "tipo de telefone inválido (celular, fixo ou indefinido)"
		}
		if t.Origem == "" {
			t.Origem = models.OrigemContatoManual
		}
		t.Pessoa = strings.TrimSpace(t.Pessoa)
		t.Principal = len(out) == 0
		out = append(out, t)
	}
	if len(out) > 20 {
		return nil, "informe até 20 telefones"
	}
	return out, ""
}

// -----------------------------------------------------------------------------
// GET /leads
// -----------------------------------------------------------------------------

// ListLeads: GET /leads → { data, pagination, contadores }.
func ListLeads(ctx iris.Context) {
	v, claims := leadsVisao(ctx)
	f := models.LeadFilter{
		Visao:       v,
		Fila:        strings.TrimSpace(ctx.URLParam("fila")),
		Estado:      strings.TrimSpace(ctx.URLParam("estado")),
		Urgencia:    strings.TrimSpace(ctx.URLParam("urgencia")),
		OwnerID:     ctx.URLParamInt64Default("owner_id", 0),
		ListaID:     ctx.URLParamInt64Default("lista_id", 0),
		Temperatura: strings.TrimSpace(ctx.URLParam("temperatura")),
		Etiqueta:    strings.TrimSpace(ctx.URLParam("etiqueta")),
		Q:           strings.TrimSpace(ctx.URLParam("q")),
		Sort:        strings.TrimSpace(ctx.URLParam("sort")),
		Dir:         strings.TrimSpace(ctx.URLParam("dir")),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	switch f.Fila {
	case "":
		f.Fila = "meus"
	case "meus", "sem_dono", "todos":
	default:
		unprocessable(ctx, "fila inválida (meus, sem_dono ou todos)")
		return
	}
	if !v.VeTodos && (f.Fila == "todos" || (f.OwnerID > 0 && f.OwnerID != claims.UserID)) {
		proibido(ctx, "você só pode ver os seus leads e os sem dono")
		return
	}
	if f.Estado != "" && !models.LeadEstadoValido(f.Estado) {
		unprocessable(ctx, "estado inválido")
		return
	}
	if f.Urgencia != "" {
		ok := false
		for _, u := range models.LeadUrgencias {
			ok = ok || u == f.Urgencia
		}
		if !ok {
			unprocessable(ctx, "urgência inválida (atrasado, hoje, futuro ou nenhuma)")
			return
		}
	}
	if f.Temperatura != "" && !models.LeadTemperaturaValida(f.Temperatura) {
		unprocessable(ctx, "temperatura inválida (fria, media ou quente)")
		return
	}
	switch f.Sort {
	case "", "urgencia", "nota", "criado_em", "titulo":
	default:
		unprocessable(ctx, "ordenação inválida (urgencia, nota, criado_em ou titulo)")
		return
	}
	if f.Dir != "" && !strings.EqualFold(f.Dir, "asc") && !strings.EqualFold(f.Dir, "desc") {
		unprocessable(ctx, "direção inválida (asc ou desc)")
		return
	}
	leads, total, cont, err := models.ListLeads(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Normalize()
	f.Total = total
	ctx.JSON(iris.Map{"data": leads, "pagination": f.Pagination, "contadores": cont})
}

// ListLeadListas: GET /leads/listas → { id, nome, abertos }[] (listas com leads visíveis ao usuário).
func ListLeadListas(ctx iris.Context) {
	v, _ := leadsVisao(ctx)
	listas, err := models.ListasDosLeads(utils.DB, v)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(listas)
}

// -----------------------------------------------------------------------------
// POST /leads
// -----------------------------------------------------------------------------

type criarLeadRequest struct {
	Titulo        string                 `json:"titulo"`
	CompanyID     *int64                 `json:"company_id"`
	ContactID     *int64                 `json:"contact_id"`
	OwnerID       json.RawMessage        `json:"owner_id"` // ausente = eu; null = sem dono
	Telefones     []models.ProspTelefone `json:"telefones"`
	Temperatura   string                 `json:"temperatura"`
	Etiquetas     []string               `json:"etiquetas"`
	ValorEstimado *float64               `json:"valor_estimado"`
}

// CreateLead: POST /leads → 201 Lead.
func CreateLead(ctx iris.Context) {
	v, claims := leadsVisao(ctx)
	var req criarLeadRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	l := &models.Lead{
		Titulo:      strings.TrimSpace(req.Titulo),
		Origem:      models.LeadOrigemManual,
		Estado:      models.LeadNovo,
		Temperatura: strings.TrimSpace(req.Temperatura),
		CriadoPor:   &claims.UserID,
	}
	// Dono: ausente = quem cria; null = sem dono; outro usuário só com leads.assign.
	me := claims.UserID
	l.OwnerID = &me
	if len(req.OwnerID) > 0 {
		var dono *int64
		if err := json.Unmarshal(req.OwnerID, &dono); err != nil {
			badRequest(ctx, "owner_id inválido")
			return
		}
		if dono != nil && *dono <= 0 {
			dono = nil
		}
		if dono != nil && *dono != me && !v.VeTodos {
			proibido(ctx, "você só pode criar leads para você mesmo")
			return
		}
		if dono != nil && !donoAtivo(*dono) {
			unprocessable(ctx, "o dono escolhido não existe ou está desativado")
			return
		}
		l.OwnerID = dono
	}
	if !models.LeadTemperaturaValida(l.Temperatura) {
		unprocessable(ctx, "temperatura inválida (fria, media ou quente)")
		return
	}
	if req.ValorEstimado != nil && *req.ValorEstimado < 0 {
		unprocessable(ctx, "o valor estimado não pode ser negativo")
		return
	}
	l.ValorEstimado = req.ValorEstimado
	tels, msg := normalizarTelefonesLead(req.Telefones)
	if msg != "" {
		unprocessable(ctx, msg)
		return
	}
	l.Telefones = tels
	l.Etiquetas = req.Etiquetas
	emails := []string{}

	var company *models.Company
	if req.CompanyID != nil && *req.CompanyID > 0 {
		c, err := models.CompanyByID(utils.DB, *req.CompanyID)
		if err == sql.ErrNoRows {
			unprocessable(ctx, "empresa não encontrada")
			return
		}
		if err != nil {
			serverError(ctx, err)
			return
		}
		company = c
		l.CompanyID = &c.ID
		l.CNPJ = models.NormalizarCNPJ(c.CNPJ)
		l.Cidade, l.UF = c.City, c.State
		if l.Titulo == "" {
			l.Titulo = c.Name
		}
		if c.Email != "" {
			emails = append(emails, strings.ToLower(c.Email))
		}
		if c.MEI != nil && *c.MEI {
			l.PessoaFisica = true
		}
	}
	if req.ContactID != nil && *req.ContactID > 0 {
		ct, err := models.ContactByID(utils.DB, *req.ContactID)
		if err == sql.ErrNoRows {
			unprocessable(ctx, "contato não encontrado")
			return
		}
		if err != nil {
			serverError(ctx, err)
			return
		}
		if company != nil && ct.CompanyID != nil && *ct.CompanyID != company.ID {
			unprocessable(ctx, "o contato não é dessa empresa")
			return
		}
		l.ContactID = &ct.ID
		if l.Titulo == "" {
			l.Titulo = strings.TrimSpace(ct.FirstName + " " + ct.LastName)
		}
		if e := strings.ToLower(strings.TrimSpace(ct.Email)); e != "" && (len(emails) == 0 || emails[0] != e) {
			emails = append([]string{e}, emails...)
		}
		if p := models.NormalizarTelefone(ct.Phone); p != "" {
			existe := false
			for _, t := range l.Telefones {
				existe = existe || t.Numero == p
			}
			if !existe {
				tipo := models.TelefoneFixo
				if len(p) == 11 {
					tipo = models.TelefoneCelular
				}
				nome := strings.TrimSpace(ct.FirstName + " " + ct.LastName)
				l.Telefones = append(l.Telefones, models.ProspTelefone{Numero: p, Tipo: tipo,
					Origem: models.OrigemContatoManual, Pessoa: nome, Principal: len(l.Telefones) == 0})
			}
		}
	}
	if l.Titulo == "" {
		unprocessable(ctx, "informe o título do lead")
		return
	}
	l.Emails = emails
	agora := leadsAgora()
	l.ProximaAcaoEm = &agora
	l.Resumo = services.ResumoDaEmpresa(company, agora)
	if _, err := models.CreateLead(utils.DB, l); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditCreate, entidadeLead, l.ID, "criou o lead "+l.Titulo)
	responderLead(ctx, l.ID, iris.StatusCreated)
}

// -----------------------------------------------------------------------------
// GET /leads/{id}
// -----------------------------------------------------------------------------

// GetLead: GET /leads/{id} → LeadCompleto (marca como visto; renova a trava de quem a detém).
func GetLead(ctx iris.Context) {
	v, claims := leadsVisao(ctx)
	id := paramID(ctx)
	l, err := models.LeadByID(utils.DB, id, v)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.MarcarLeadVisto(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	l.Visto = true
	if l.EmAtendimentoPor != nil && *l.EmAtendimentoPor == claims.UserID {
		if err := models.RenovarTravaLead(utils.DB, id, claims.UserID); err != nil {
			serverError(ctx, err)
			return
		}
	}
	completo, err := services.MontarLeadCompleto(utils.DB, l, claims.Name, leadsAgora())
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(completo)
}

// -----------------------------------------------------------------------------
// PUT /leads/{id}
// -----------------------------------------------------------------------------

// UpdateLead: PUT /leads/{id} (parcial) → Lead. Aceita titulo, owner_id,
// contact_id, temperatura, etiquetas, valor_estimado, telefones,
// proxima_acao_em e whatsapp_optin; chave ausente não muda.
func UpdateLead(ctx iris.Context) {
	v, claims := leadsVisao(ctx)
	id := paramID(ctx)
	l, err := models.LeadByID(utils.DB, id, v)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	var corpo map[string]json.RawMessage
	if err := ctx.ReadJSON(&corpo); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	var p models.LeadPatch
	resumo := []string{}
	ler := func(chave string, dest any) bool {
		raw, ok := corpo[chave]
		if !ok {
			return false
		}
		if err := json.Unmarshal(raw, dest); err != nil {
			badRequest(ctx, chave+" inválido")
			return false
		}
		return true
	}
	if _, ok := corpo["titulo"]; ok {
		var t string
		if !ler("titulo", &t) {
			return
		}
		t = strings.TrimSpace(t)
		if t == "" {
			unprocessable(ctx, "informe o título do lead")
			return
		}
		p.Titulo = &t
	}
	if _, ok := corpo["owner_id"]; ok {
		var dono *int64
		if !ler("owner_id", &dono) {
			return
		}
		if dono != nil && *dono <= 0 {
			dono = nil
		}
		if !v.VeTodos {
			// Sem leads.assign só dá para assumir um lead sem dono.
			mesmo := dono != nil && l.OwnerID != nil && *l.OwnerID == *dono && *dono == claims.UserID
			assumir := dono != nil && *dono == claims.UserID && l.SemDono()
			if !mesmo && !assumir {
				proibido(ctx, "você só pode assumir leads sem dono")
				return
			}
		}
		if dono != nil && !donoAtivo(*dono) {
			unprocessable(ctx, "o dono escolhido não existe ou está desativado")
			return
		}
		p.Owner = models.DefinirIDPtr(dono)
		resumo = append(resumo, "dono")
	}
	if _, ok := corpo["contact_id"]; ok {
		var contato *int64
		if !ler("contact_id", &contato) {
			return
		}
		if contato != nil && *contato <= 0 {
			contato = nil
		}
		if contato != nil {
			ct, err := models.ContactByID(utils.DB, *contato)
			if err == sql.ErrNoRows {
				unprocessable(ctx, "contato não encontrado")
				return
			}
			if err != nil {
				serverError(ctx, err)
				return
			}
			if l.CompanyID != nil && ct.CompanyID != nil && *ct.CompanyID != *l.CompanyID {
				unprocessable(ctx, "o contato não é da empresa do lead")
				return
			}
		}
		p.Contact = models.DefinirIDPtr(contato)
	}
	if _, ok := corpo["temperatura"]; ok {
		var t string
		if !ler("temperatura", &t) {
			return
		}
		if !models.LeadTemperaturaValida(t) {
			unprocessable(ctx, "temperatura inválida (fria, media ou quente)")
			return
		}
		p.Temperatura = &t
	}
	if _, ok := corpo["etiquetas"]; ok {
		var e []string
		if !ler("etiquetas", &e) {
			return
		}
		if e == nil {
			e = []string{}
		}
		p.Etiquetas = &e
	}
	if _, ok := corpo["valor_estimado"]; ok {
		var val *float64
		if !ler("valor_estimado", &val) {
			return
		}
		if val != nil && *val < 0 {
			unprocessable(ctx, "o valor estimado não pode ser negativo")
			return
		}
		p.ValorEstimado = models.OpcionalFloat{Definir: true, Valor: val}
	}
	if _, ok := corpo["telefones"]; ok {
		var tels []models.ProspTelefone
		if !ler("telefones", &tels) {
			return
		}
		limpos, msg := normalizarTelefonesLead(tels)
		if msg != "" {
			unprocessable(ctx, msg)
			return
		}
		p.Telefones = &limpos
	}
	if _, ok := corpo["proxima_acao_em"]; ok {
		var quando *time.Time
		if !ler("proxima_acao_em", &quando) {
			return
		}
		if quando == nil {
			p.ProximaAcaoEm = models.LimparTempo()
		} else {
			p.ProximaAcaoEm = models.DefinirTempo(*quando)
		}
	}
	if _, ok := corpo["whatsapp_optin"]; ok {
		var optin bool
		if !ler("whatsapp_optin", &optin) {
			return
		}
		p.WhatsAppOptin = &optin
		resumo = append(resumo, "whatsapp")
	}
	if err := models.UpdateLead(utils.DB, id, p); err != nil {
		if err == sql.ErrNoRows {
			notFound(ctx)
			return
		}
		serverError(ctx, err)
		return
	}
	for _, r := range resumo {
		switch r {
		case "dono":
			audit(ctx, models.AuditUpdate, entidadeLead, id, "mudou o dono do lead "+l.Titulo)
		case "whatsapp":
			txt := "registrou que o cliente autorizou WhatsApp no lead "
			if !*p.WhatsAppOptin {
				txt = "retirou a autorização de WhatsApp do lead "
			}
			audit(ctx, models.AuditUpdate, entidadeLead, id, txt+l.Titulo)
		}
	}
	responderLead(ctx, id, iris.StatusOK)
}

// DeleteLead: DELETE /leads/{id} → 204 (exige leads.assign, na rota). Os
// bloqueios e as ligações ficam.
func DeleteLead(ctx iris.Context) {
	v, _ := leadsVisao(ctx)
	id := paramID(ctx)
	l, err := models.LeadByID(utils.DB, id, v)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeleteLead(utils.DB, id); err != nil {
		handleDBError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, entidadeLead, id, "excluiu o lead "+l.Titulo)
	ctx.StatusCode(iris.StatusNoContent)
}

// -----------------------------------------------------------------------------
// POST /leads/bulk
// -----------------------------------------------------------------------------

type bulkLeadsRequest struct {
	IDs         []int64 `json:"ids"`
	Acao        string  `json:"acao"`
	OwnerID     *int64  `json:"owner_id"`
	Temperatura string  `json:"temperatura"`
	Etiqueta    string  `json:"etiqueta"`
	Motivo      string  `json:"motivo"`
}

// BulkLeads: POST /leads/bulk { ids, acao, ... } → { afetados } (acao "dono"
// exige leads.assign; ids fora da visão são ignorados).
func BulkLeads(ctx iris.Context) {
	v, _ := leadsVisao(ctx)
	var req bulkLeadsRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	ids := []int64{}
	vistos := map[int64]bool{}
	for _, id := range req.IDs {
		if id > 0 && !vistos[id] {
			vistos[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		unprocessable(ctx, "selecione ao menos um lead")
		return
	}
	if len(ids) > models.MaxLeadsBulk {
		unprocessable(ctx, fmt.Sprintf("a ação em massa aceita até %d leads por vez", models.MaxLeadsBulk))
		return
	}
	b := models.LeadBulk{IDs: ids, Acao: strings.TrimSpace(req.Acao), Temperatura: strings.TrimSpace(req.Temperatura),
		Etiqueta: strings.TrimSpace(req.Etiqueta), Motivo: strings.TrimSpace(req.Motivo)}
	var resumo string
	switch b.Acao {
	case models.LeadBulkDono:
		if !v.VeTodos {
			proibido(ctx, "distribuir leads exige a permissão de distribuir e redistribuir leads")
			return
		}
		if req.OwnerID != nil && *req.OwnerID > 0 {
			if !donoAtivo(*req.OwnerID) {
				unprocessable(ctx, "o dono escolhido não existe ou está desativado")
				return
			}
			b.OwnerID = req.OwnerID
		}
		resumo = "atribuiu dono a"
	case models.LeadBulkTemperatura:
		if !models.LeadTemperaturaValida(b.Temperatura) {
			unprocessable(ctx, "temperatura inválida (fria, media ou quente)")
			return
		}
		resumo = "mudou a temperatura de"
	case models.LeadBulkEtiqueta:
		if b.Etiqueta == "" {
			unprocessable(ctx, "informe a etiqueta")
			return
		}
		resumo = "etiquetou"
	case models.LeadBulkDescartar:
		if b.Motivo == "" {
			unprocessable(ctx, "informe o motivo do descarte")
			return
		}
		resumo = "descartou"
	case models.LeadBulkReabrir:
		b.ProximaAcao = models.LoadConfigProspeccao(utils.DB).AjustarJanela(leadsAgora())
		resumo = "reabriu"
	default:
		unprocessable(ctx, "ação inválida (dono, temperatura, etiqueta, descartar ou reabrir)")
		return
	}
	afetados, err := models.BulkLeads(utils.DB, v, b)
	if err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditBulk, entidadeLead, 0, fmt.Sprintf("%s %d lead(s) em massa", resumo, afetados))
	ctx.JSON(iris.Map{"afetados": afetados})
}

// ConverterLead: POST /leads/{id}/converter → { lead, deal }. A regra é da
// fila (services/prospeccao_conversao.go, controllers/fila_controller.go).
func ConverterLead(ctx iris.Context) { ConverterLeadDaFila(ctx) }

// -----------------------------------------------------------------------------
// POST /leads/{id}/descartar e /reabrir
// -----------------------------------------------------------------------------

type descartarLeadRequest struct {
	Motivo     string `json:"motivo"`
	Observacao string `json:"observacao"`
	Bloquear   bool   `json:"bloquear"`
}

// DescartarLead: POST /leads/{id}/descartar { motivo, observacao?, bloquear? } → Lead.
// bloquear põe na lista de "não contatar" o CNPJ do lead (a empresa inteira)
// ou, sem CNPJ, os telefones e e-mails dele; exige só leads.edit.
func DescartarLead(ctx iris.Context) {
	v, claims := leadsVisao(ctx)
	id := paramID(ctx)
	var req descartarLeadRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "corpo inválido")
		return
	}
	req.Motivo = strings.TrimSpace(req.Motivo)
	req.Observacao = strings.TrimSpace(req.Observacao)
	if req.Motivo == "" {
		unprocessable(ctx, "informe o motivo do descarte")
		return
	}
	if len([]rune(req.Motivo)) > 80 {
		unprocessable(ctx, "o motivo aceita até 80 caracteres")
		return
	}
	l, err := models.LeadByID(utils.DB, id, v)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if !l.Aberto() {
		ctx.StopWithJSON(iris.StatusConflict, iris.Map{"error": models.ErrLeadEncerrado.Error(), "codigo": "estado"})
		return
	}
	tx, err := utils.DB.Begin()
	if err != nil {
		serverError(ctx, err)
		return
	}
	defer tx.Rollback()
	if err := models.DescartarLead(tx, id, req.Motivo); err != nil {
		if errors.Is(err, models.ErrLeadEncerrado) {
			ctx.StopWithJSON(iris.StatusConflict, iris.Map{"error": err.Error(), "codigo": "estado"})
			return
		}
		handleDBError(ctx, err)
		return
	}
	bloqueados := 0
	if req.Bloquear {
		criar := func(tipo, valor string) error {
			b := &models.Bloqueio{Tipo: tipo, Valor: valor, Motivo: models.BloqueioManual, Canal: "leads",
				Observacao: req.Observacao, CriadoPor: &claims.UserID}
			if _, err := models.NormalizarValorBloqueio(tipo, valor); err != nil {
				return nil // valor que não dá para bloquear é ignorado
			}
			criado, err := models.CreateBloqueio(tx, b)
			if criado {
				bloqueados++
			}
			return err
		}
		if models.CNPJValido(l.CNPJ) {
			err = criar(models.BloqueioCNPJ, l.CNPJ)
		} else {
			for _, t := range l.Telefones {
				if err = criar(models.BloqueioTelefone, t.Numero); err != nil {
					break
				}
			}
			for _, e := range l.Emails {
				if err != nil {
					break
				}
				err = criar(models.BloqueioEmail, e)
			}
		}
		if err != nil {
			serverError(ctx, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		serverError(ctx, err)
		return
	}
	nota := "Lead descartado: " + req.Motivo
	if req.Observacao != "" {
		nota += " (" + req.Observacao + ")"
	}
	if l.CompanyID != nil {
		logActivity(ctx, models.Activity{Kind: models.ActivitySistema, Content: nota, CompanyID: l.CompanyID, ContactID: l.ContactID})
	}
	txt := "descartou o lead " + l.Titulo + " (" + req.Motivo + ")"
	if req.Bloquear {
		txt += fmt.Sprintf(" e bloqueou %d contato(s)", bloqueados)
	}
	audit(ctx, models.AuditUpdate, entidadeLead, id, txt)
	responderLead(ctx, id, iris.StatusOK)
}

// ReabrirLead: POST /leads/{id}/reabrir → Lead. Só de descartado (409 nos
// demais): volta a "tentando" se já houve ligação, senão "novo", com as
// tentativas zeradas e a próxima ação agora (ajustada à janela). Os
// bloqueios ficam.
func ReabrirLead(ctx iris.Context) {
	v, _ := leadsVisao(ctx)
	id := paramID(ctx)
	l, err := models.LeadByID(utils.DB, id, v)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if l.Estado != models.LeadDescartado {
		ctx.StopWithJSON(iris.StatusConflict, iris.Map{"error": "só um lead descartado pode ser reaberto", "codigo": "estado"})
		return
	}
	quando := models.LoadConfigProspeccao(utils.DB).AjustarJanela(leadsAgora())
	if err := models.ReabrirLead(utils.DB, id, quando); err != nil {
		if errors.Is(err, models.ErrLeadEstado) || errors.Is(err, models.ErrLeadEncerrado) {
			ctx.StopWithJSON(iris.StatusConflict, iris.Map{"error": "só um lead descartado pode ser reaberto", "codigo": "estado"})
			return
		}
		handleDBError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, entidadeLead, id, "reabriu o lead "+l.Titulo)
	responderLead(ctx, id, iris.StatusOK)
}
