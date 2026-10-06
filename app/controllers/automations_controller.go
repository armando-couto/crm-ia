package controllers

import (
	"encoding/json"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// ListAutomations devolve as automações junto do catálogo de gatilhos e ações
// que a tela de montagem precisa.
func ListAutomations(ctx iris.Context) {
	list, err := models.ListAutomations(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}

	// Quantos contatos estão no meio de cada sequência.
	pending := map[int64]int{}
	for _, a := range list {
		if total, err := models.CountActiveEnrollments(utils.DB, a.ID); err == nil && total > 0 {
			pending[a.ID] = total
		}
	}

	ctx.JSON(iris.Map{
		"data":          list,
		"triggers":      models.TriggerLabels,
		"actions":       models.ActionLabels,
		"time_triggers": models.TimeTriggers,
		"pending":       pending,
	})
}

type automationRequest struct {
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	TriggerKind   string                    `json:"trigger_kind"`
	TriggerConfig json.RawMessage           `json:"trigger_config"`
	Actions       []models.AutomationAction `json:"actions"`
	Active        *bool                     `json:"active"`
}

func (r *automationRequest) apply(a *models.Automation) string {
	a.Name = strings.TrimSpace(r.Name)
	a.Description = strings.TrimSpace(r.Description)
	a.TriggerKind = r.TriggerKind
	a.TriggerConfig = r.TriggerConfig
	a.Actions = r.Actions
	a.Active = true
	if r.Active != nil {
		a.Active = *r.Active
	}
	if err := models.ValidateAutomation(a); err != nil {
		return err.Error()
	}
	return ""
}

func CreateAutomationHandler(ctx iris.Context) {
	var req automationRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	auto := &models.Automation{}
	if msg := req.apply(auto); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if claims := middlewareClaims(ctx); claims != nil {
		auto.CreatedBy = &claims.UserID
	}

	if err := models.CreateAutomation(utils.DB, auto); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditCreate, "automacao", auto.ID, "criou a automação "+auto.Name)
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(auto)
}

func UpdateAutomationHandler(ctx iris.Context) {
	auto, err := models.AutomationByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req automationRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.apply(auto); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if err := models.UpdateAutomation(utils.DB, auto); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "automacao", auto.ID, "editou a automação "+auto.Name)
	ctx.JSON(auto)
}

func DeleteAutomationHandler(ctx iris.Context) {
	id := paramID(ctx)
	auto, err := models.AutomationByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeleteAutomation(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "automacao", id, "excluiu a automação "+auto.Name)
	ctx.JSON(iris.Map{"message": "automação removida"})
}

// ListAutomationRunsHandler mostra o histórico de execuções da automação.
func ListAutomationRunsHandler(ctx iris.Context) {
	runs, err := models.ListAutomationRuns(utils.DB, paramID(ctx),
		ctx.URLParamIntDefault("limit", 50))
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": runs})
}

// RunAutomationNow executa a automação contra um registro escolhido a mão,
// para a equipe testar antes de deixar rodando sozinha.
func RunAutomationNow(ctx iris.Context) {
	auto, err := models.AutomationByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req struct {
		ContactID int64 `json:"contact_id"`
		DealID    int64 `json:"deal_id"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	var subject services.Subject
	switch {
	case req.DealID > 0:
		deal, err := models.DealByID(utils.DB, req.DealID)
		if err != nil {
			badRequest(ctx, "negócio não encontrado")
			return
		}
		subject = services.SubjectFromDeal(deal)
	case req.ContactID > 0:
		contact, err := models.ContactByID(utils.DB, req.ContactID)
		if err != nil {
			badRequest(ctx, "contato não encontrado")
			return
		}
		subject = services.SubjectFromContact(contact)
	default:
		badRequest(ctx, "informe um contato ou um negócio para o teste")
		return
	}

	services.RunAutomation(utils.DB, auto, subject, 0)
	audit(ctx, models.AuditUpdate, "automacao", auto.ID,
		"executou manualmente a automação "+auto.Name)

	runs, err := models.ListAutomationRuns(utils.DB, auto.ID, 5)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "automação executada", "runs": runs})
}
