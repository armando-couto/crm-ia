package controllers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"
	"github.com/kataras/iris/v12"
)

// Fila de ligações: próximo lead com trava, resultado da ligação em um clique,
// resumo do dia, disponibilidade da agenda e conversão do lead em negócio.
// Contrato: docs/PROSPECCAO.md seção 8.2, com as correções de
// docs/PROSPECCAO-ADENDO.md. As regras moram em services/prospeccao_fila.go,
// prospeccao_janela.go e prospeccao_conversao.go.

// filaAgora é o relógio da fila (trocado nos testes).
var filaAgora = time.Now

// filaVisao é quem está olhando: sem leads.assign, só os próprios leads, os
// sem dono e os de dono inativo (o resto responde 404).
func filaVisao(ctx iris.Context) (models.LeadVisao, *services.Claims) {
	claims := middlewareClaims(ctx)
	if claims == nil {
		return models.LeadVisao{}, nil
	}
	return models.LeadVisao{UserID: claims.UserID, VeTodos: models.RoleCan(claims.Role, models.PermLeadsAssign)}, claims
}

// filaErro traduz os erros da fila para o status do contrato (o que não é
// erro de regra vira 500).
func filaErro(ctx iris.Context, err error) {
	var emAtendimento *models.ErroLeadEmAtendimento
	var resultado *services.ErroResultado
	var conversao *services.ErroConversao
	switch {
	case errors.Is(err, sql.ErrNoRows):
		notFound(ctx)
	case errors.Is(err, models.ErrLeadEncerrado):
		ctx.StopWithJSON(iris.StatusConflict, iris.Map{"error": err.Error(), "codigo": "estado"})
	case errors.Is(err, services.ErrAgendaOcupada):
		ctx.StopWithJSON(iris.StatusConflict, iris.Map{"error": err.Error(), "codigo": "agenda_ocupada"})
	case errors.As(err, &emAtendimento):
		ctx.StopWithJSON(iris.StatusLocked, iris.Map{
			"error": err.Error(), "em_atendimento_por": emAtendimento.UserID, "nome": emAtendimento.Nome,
		})
	case errors.As(err, &resultado):
		ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"error": resultado.Msg})
	case errors.As(err, &conversao):
		ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"error": conversao.Msg})
	default:
		serverError(ctx, err)
	}
}

// ResultadoLead: POST /leads/{id}/resultado (ResultadoRequest) → ResultadoResposta.
// Não exige trava prévia (o painel de /leads usa a mesma rota); a trava de
// outra pessoa ainda válida responde 423; a mesma chave repetida devolve a
// resposta original.
func ResultadoLead(ctx iris.Context) {
	v, claims := filaVisao(ctx)
	if claims == nil {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "não autenticado"})
		return
	}
	var req services.ResultadoPedido
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	res, err := services.RegistrarResultado(utils.DB, paramID(ctx), v, req, filaAgora())
	if err != nil {
		filaErro(ctx, err)
		return
	}
	if req.WhatsAppOptin != nil && *req.WhatsAppOptin {
		audit(ctx, models.AuditUpdate, "lead", res.Lead.ID, "registrou a autorização de WhatsApp do lead "+res.Lead.Titulo)
	}
	ctx.JSON(res)
}

// LiberarLead: POST /leads/{id}/liberar → 204 (solta a trava da fila; sem efeito para quem não a detém).
func LiberarLead(ctx iris.Context) {
	v, claims := filaVisao(ctx)
	if claims == nil {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "não autenticado"})
		return
	}
	id := paramID(ctx)
	if _, err := models.LeadByID(utils.DB, id, v); err != nil {
		handleDBError(ctx, err)
		return
	}
	if _, err := models.LiberarLead(utils.DB, id, v.UserID); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusNoContent)
}

// FilaResumo: GET /leads/fila/resumo → ResumoFila.
func FilaResumo(ctx iris.Context) {
	_, claims := filaVisao(ctx)
	if claims == nil {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "não autenticado"})
		return
	}
	r, err := services.ResumoDaFila(utils.DB, claims.UserID, ctx.URLParamInt64Default("lista_id", 0), filaAgora())
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(r)
}

// FilaProximo: POST /leads/fila/proximo { lista_id?, pular_ids? } → { lead, motivo_vazio, janela }.
// Solta a trava anterior do usuário e devolve primeiro o lead que ele já tem
// travado (recarregar a página não troca de lead). Pular não muda o lead.
func FilaProximo(ctx iris.Context) {
	_, claims := filaVisao(ctx)
	if claims == nil {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "não autenticado"})
		return
	}
	var req struct {
		ListaID  int64   `json:"lista_id"`
		PularIDs []int64 `json:"pular_ids"`
		PularID  int64   `json:"pular_id"` // forma antiga, um id só
	}
	if ctx.GetContentLength() > 0 {
		if err := ctx.ReadJSON(&req); err != nil {
			badRequest(ctx, "dados inválidos")
			return
		}
	}
	if req.PularID > 0 {
		req.PularIDs = append(req.PularIDs, req.PularID)
	}
	if len(req.PularIDs) > models.MaxPularIDs {
		ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"error": "pular_ids aceita até 200 leads"})
		return
	}
	r, err := services.ProximoLead(utils.DB, claims.UserID, claims.Name, req.ListaID, req.PularIDs, filaAgora())
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(r)
}

// AgendaDisponibilidade: GET /agenda/disponibilidade?user_id&dias → horários livres e ocupados por dia.
// user_id de outra pessoa exige meetings.view e precisa ser um usuário ativo.
func AgendaDisponibilidade(ctx iris.Context) {
	_, claims := filaVisao(ctx)
	if claims == nil {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "não autenticado"})
		return
	}
	userID := ctx.URLParamInt64Default("user_id", claims.UserID)
	if userID <= 0 {
		userID = claims.UserID
	}
	if userID != claims.UserID {
		if !models.RoleCan(claims.Role, models.PermMeetingsView) {
			ctx.StopWithJSON(iris.StatusForbidden, iris.Map{"error": "seu perfil não pode ver a agenda de outra pessoa"})
			return
		}
		var ativo bool
		err := utils.DB.QueryRow(`SELECT active FROM users WHERE id = $1`, userID).Scan(&ativo)
		if err == sql.ErrNoRows || (err == nil && !ativo) {
			notFound(ctx)
			return
		}
		if err != nil {
			serverError(ctx, err)
			return
		}
	}
	dias := ctx.URLParamIntDefault("dias", services.DiasAgendaPadrao)
	if dias < 1 || dias > services.MaxDiasAgenda {
		ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"error": "dias deve ficar entre 1 e 14"})
		return
	}
	cfg := models.LoadConfigProspeccao(utils.DB)
	d, err := services.DisponibilidadeAgenda(utils.DB, userID, dias, cfg, filaAgora())
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(d)
}

// ConverterLeadDaFila é o corpo de POST /leads/{id}/converter
// ({ pipeline_id?, stage_id?, nome?, valor?, owner_id? } → { lead, deal }):
// só em lead aberto (409 "estado" nos demais), destino validado (422).
// O handler registrado (ConverterLead, em leads_controller.go) delega para cá.
func ConverterLeadDaFila(ctx iris.Context) {
	v, claims := filaVisao(ctx)
	if claims == nil {
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "não autenticado"})
		return
	}
	var req services.ConverterPedido
	if ctx.GetContentLength() > 0 {
		if err := ctx.ReadJSON(&req); err != nil {
			badRequest(ctx, "dados inválidos")
			return
		}
	}
	if req.Valor != nil && *req.Valor < 0 {
		ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"error": "o valor do negócio não pode ser negativo"})
		return
	}
	res, err := services.ConverterLeadEmNegocio(utils.DB, paramID(ctx), v, req)
	if err != nil {
		filaErro(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "lead", res.Lead.ID, "converteu o lead "+res.Lead.Titulo+" em negócio")
	ctx.JSON(res)
}
