package controllers

import (
	"fmt"
	"strings"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// ListTargetAccounts devolve as contas-alvo com o resumo de cada uma.
// ?owner_id=me limita às contas de quem está olhando.
func ListTargetAccounts(ctx iris.Context) {
	ownerID := ctx.URLParamInt64Default("owner_id", 0)
	if ctx.URLParam("owner_id") == "me" {
		if claims := middlewareClaims(ctx); claims != nil {
			ownerID = claims.UserID
		}
	}

	accounts, err := models.ListTargetAccounts(utils.DB, ownerID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	summary, err := models.LoadTargetSummary(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{
		"data":         accounts,
		"summary":      summary,
		"buying_roles": models.BuyingRoleLabels,
	})
}

// SetTargetAccount marca ou desmarca a empresa como conta-alvo.
func SetTargetAccount(ctx iris.Context) {
	id := paramID(ctx)
	company, err := models.CompanyByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req struct {
		IsTarget bool   `json:"is_target"`
		Tier     int    `json:"target_tier"`
		Notes    string `json:"target_notes"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if req.IsTarget && req.Tier == 0 {
		req.Tier = 2
	}

	if err := models.SetTargetAccount(utils.DB, id, req.IsTarget, req.Tier,
		strings.TrimSpace(req.Notes)); err != nil {
		badRequest(ctx, err.Error())
		return
	}

	action := "removeu das contas-alvo"
	content := "Empresa saiu das contas-alvo"
	if req.IsTarget {
		action = fmt.Sprintf("marcou como conta-alvo (tier %d)", req.Tier)
		content = fmt.Sprintf("Empresa marcada como conta-alvo tier %d", req.Tier)
	}
	logActivity(ctx, models.Activity{
		Kind:      models.ActivitySistema,
		Content:   content,
		CompanyID: &id,
	})
	audit(ctx, models.AuditUpdate, "empresa", id, action+": "+company.Name)

	updated, err := models.CompanyByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(updated)
}
