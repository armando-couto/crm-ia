package controllers

import (
	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

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
