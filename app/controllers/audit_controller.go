package controllers

import (
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// ListAuditLog devolve a trilha de auditoria paginada, com filtros por usuário,
// ação, entidade, período e busca no resumo.
func ListAuditLog(ctx iris.Context) {
	filter := models.AuditFilter{
		UserID: ctx.URLParamInt64Default("user_id", 0),
		Action: ctx.URLParam("action"),
		Entity: ctx.URLParam("entity"),
		Search: ctx.URLParam("q"),
		Days:   ctx.URLParamIntDefault("days", 30),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 50),
		},
	}

	filter.Normalize()
	entries, total, err := models.ListAudit(utils.DB, filter)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{
		"data":     entries,
		"total":    total,
		"page":     filter.Page,
		"per_page": filter.PerPage,
		"actions":  models.AuditActions,
	})
}
