package controllers

import (
	"strings"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// Dashboard retorna as métricas consolidadas do CRM.
func Dashboard(ctx iris.Context) {
	pipelineID := ctx.URLParamInt64Default("pipeline_id", 0)
	d, err := models.LoadDashboard(utils.DB, pipelineID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(d)
}

// Search é a busca global (contatos, empresas e negócios).
func Search(ctx iris.Context) {
	term := strings.TrimSpace(ctx.URLParam("q"))
	if len(term) < 2 {
		ctx.JSON(iris.Map{"data": []models.SearchResult{}})
		return
	}
	results, err := models.GlobalSearch(utils.DB, term)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": results})
}
