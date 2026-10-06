package controllers

import (
	"fmt"
	"strings"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

func ListCalls(ctx iris.Context) {
	f := models.CallFilter{
		ContactID: ctx.URLParamInt64Default("contact_id", 0),
		UserID:    ctx.URLParamInt64Default("user_id", 0),
		Outcome:   ctx.URLParam("outcome"),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	list, total, err := models.ListCalls(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Total = total
	ctx.JSON(iris.Map{"data": list, "pagination": f.Pagination})
}

type callRequest struct {
	Direction string `json:"direction"`
	Outcome   string `json:"outcome"`
	Duration  int    `json:"duration_seconds"`
	Notes     string `json:"notes"`
	CalledAt  string `json:"called_at"` // RFC3339 ou datetime-local
	ContactID *int64 `json:"contact_id"`
	CompanyID *int64 `json:"company_id"`
	DealID    *int64 `json:"deal_id"`
}

// CreateCall registra a chamada e replica na timeline do contato.
func CreateCall(ctx iris.Context) {
	var req callRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if req.ContactID == nil || *req.ContactID == 0 {
		badRequest(ctx, "vincule a chamada a um contato")
		return
	}
	if req.Direction != "" && req.Direction != "entrada" && req.Direction != "saida" {
		badRequest(ctx, "direção inválida (entrada ou saida)")
		return
	}
	if req.Outcome != "" && !models.ValidCallOutcome(req.Outcome) {
		badRequest(ctx, "resultado inválido")
		return
	}
	if req.Duration < 0 {
		badRequest(ctx, "duração não pode ser negativa")
		return
	}

	call := &models.Call{
		Direction: req.Direction,
		Outcome:   req.Outcome,
		Duration:  req.Duration,
		Notes:     strings.TrimSpace(req.Notes),
		ContactID: req.ContactID,
		CompanyID: req.CompanyID,
		DealID:    req.DealID,
	}
	if req.CalledAt != "" {
		if t, err := time.Parse(time.RFC3339, req.CalledAt); err == nil {
			call.CalledAt = t
		} else if t, err := time.ParseInLocation("2006-01-02T15:04", req.CalledAt, time.Local); err == nil {
			call.CalledAt = t
		} else {
			badRequest(ctx, "data/hora da chamada inválida")
			return
		}
	}
	if claims := middlewareClaims(ctx); claims != nil {
		call.UserID = &claims.UserID
	}
	if err := models.CreateCall(utils.DB, call); err != nil {
		serverError(ctx, err)
		return
	}

	content := fmt.Sprintf("Chamada registrada (%s)", call.Outcome)
	if call.Notes != "" {
		content = fmt.Sprintf("%s: %s", content, call.Notes)
	}
	logActivity(ctx, models.Activity{
		Kind:      models.ActivityLigacao,
		Content:   content,
		ContactID: call.ContactID,
		CompanyID: call.CompanyID,
		DealID:    call.DealID,
	})
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(call)
}

func DeleteCall(ctx iris.Context) {
	if err := models.DeleteCall(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "chamada removida"})
}
