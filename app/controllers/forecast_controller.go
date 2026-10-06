package controllers

import (
	"fmt"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// ForecastHandler devolve a previsão do mês por vendedor, contra as metas.
func ForecastHandler(ctx iris.Context) {
	forecast, err := models.LoadForecast(utils.DB,
		ctx.URLParam("period"), ctx.URLParamInt64Default("pipeline_id", 0))
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}
	ctx.JSON(forecast)
}

// ListGoals devolve as metas do mês (equipe e por pessoa).
func ListGoals(ctx iris.Context) {
	goals, err := models.ListGoals(utils.DB, ctx.URLParam("period"))
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}
	ctx.JSON(iris.Map{"data": goals})
}

// SaveGoal grava a meta de um vendedor ou da equipe no mês. Valor zero apaga.
func SaveGoal(ctx iris.Context) {
	var req struct {
		UserID *int64  `json:"user_id"`
		Period string  `json:"period"`
		Amount float64 `json:"amount"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	if err := models.SaveGoal(utils.DB, req.UserID, req.Period, req.Amount); err != nil {
		badRequest(ctx, err.Error())
		return
	}

	alvo := "da equipe"
	if req.UserID != nil {
		alvo = "individual"
	}
	audit(ctx, models.AuditUpdate, "meta", 0, "definiu a meta "+alvo+" de "+req.Period)

	goals, err := models.ListGoals(utils.DB, req.Period)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": goals})
}

// SalesAnalyticsHandler devolve taxa de ganho, ticket médio, ciclo de vendas,
// funil por etapa e volume de interações do período.
func SalesAnalyticsHandler(ctx iris.Context) {
	analytics, err := models.LoadSalesAnalytics(utils.DB,
		ctx.URLParamIntDefault("days", 90), ctx.URLParamInt64Default("pipeline_id", 0))
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(analytics)
}

// CategoryForecastHandler devolve a visão por categoria de previsão do mês.
func CategoryForecastHandler(ctx iris.Context) {
	forecast, err := models.LoadCategoryForecast(utils.DB,
		ctx.URLParam("period"), ctx.URLParamInt64Default("pipeline_id", 0))
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}
	ctx.JSON(iris.Map{"forecast": forecast, "categories": models.ForecastCategoryLabels})
}

// MySubmissionHandler devolve o envio de previsão do próprio vendedor no mês.
func MySubmissionHandler(ctx iris.Context) {
	claims := middlewareClaims(ctx)
	sub, err := models.MySubmission(utils.DB, claims.UserID, ctx.URLParam("period"))
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}
	ctx.JSON(iris.Map{"submission": sub})
}

// SubmitForecast grava a previsão que o vendedor submete para o mês.
func SubmitForecast(ctx iris.Context) {
	claims := middlewareClaims(ctx)

	var req struct {
		Period string  `json:"period"`
		Amount float64 `json:"amount"`
		Note   string  `json:"note"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	if err := models.SaveSubmission(utils.DB, claims.UserID, req.Period,
		req.Amount, strings.TrimSpace(req.Note)); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	audit(ctx, models.AuditUpdate, "previsao", 0,
		fmt.Sprintf("enviou a previsão de %s", req.Period))

	sub, err := models.MySubmission(utils.DB, claims.UserID, req.Period)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"submission": sub})
}

// SetDealForecastCategory muda a categoria de previsão do negócio.
func SetDealForecastCategory(ctx iris.Context) {
	deal, err := models.DealByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req struct {
		Category string `json:"forecast_category"`
	}
	if err := ctx.ReadJSON(&req); err != nil || !models.ValidForecastCategory(req.Category) {
		badRequest(ctx, "categoria inválida (excluido, pipeline, melhor_caso, comprometido ou fechado)")
		return
	}
	if req.Category == "" {
		req.Category = models.ForecastPipeline
	}

	deal.ForecastCategory = req.Category
	if err := models.UpdateDeal(utils.DB, deal); err != nil {
		serverError(ctx, err)
		return
	}
	logActivity(ctx, models.Activity{
		Kind:      models.ActivitySistema,
		Content:   "Categoria de previsão: " + models.ForecastCategoryLabels[req.Category],
		DealID:    &deal.ID,
		ContactID: deal.ContactID,
		CompanyID: deal.CompanyID,
	})
	ctx.JSON(deal)
}
