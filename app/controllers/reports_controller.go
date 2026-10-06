package controllers

import (
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// ListReports devolve os relatórios visíveis para o usuário e o catálogo de
// entidades, métricas e agrupamentos que a tela de montagem oferece.
func ListReports(ctx iris.Context) {
	claims := middlewareClaims(ctx)

	reports, err := models.ListReports(utils.DB, claims.UserID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{
		"data":    reports,
		"catalog": models.ReportCatalog(),
		"kinds":   models.ReportKindLabels,
	})
}

type reportRequest struct {
	Kind        string               `json:"kind"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Entity      string               `json:"entity"`
	Metric      string               `json:"metric"`
	Dimension   string               `json:"dimension"`
	Filters     models.ReportFilters `json:"filters"`
	Chart       string               `json:"chart"`
	Shared      *bool                `json:"shared"`
}

func (r *reportRequest) apply(report *models.Report) string {
	report.Kind = r.Kind
	report.Name = strings.TrimSpace(r.Name)
	report.Description = strings.TrimSpace(r.Description)
	report.Entity = r.Entity
	report.Metric = r.Metric
	report.Dimension = r.Dimension
	report.Filters = r.Filters
	report.Chart = r.Chart
	report.Shared = true
	if r.Shared != nil {
		report.Shared = *r.Shared
	}
	if err := models.ValidateReport(report); err != nil {
		return err.Error()
	}
	return ""
}

func CreateReport(ctx iris.Context) {
	var req reportRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	report := &models.Report{}
	if msg := req.apply(report); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if claims := middlewareClaims(ctx); claims != nil {
		report.CreatedBy = &claims.UserID
	}

	if err := models.CreateReport(utils.DB, report); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditCreate, "relatorio", report.ID, "criou o relatório "+report.Name)
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(report)
}

func UpdateReportByID(ctx iris.Context) {
	report, err := models.ReportByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req reportRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.apply(report); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if err := models.UpdateReport(utils.DB, report); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "relatorio", report.ID, "editou o relatório "+report.Name)
	ctx.JSON(report)
}

func DeleteReportByID(ctx iris.Context) {
	id := paramID(ctx)
	report, err := models.ReportByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeleteReport(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "relatorio", id, "excluiu o relatório "+report.Name)
	ctx.JSON(iris.Map{"message": "relatório removido"})
}

// RunReportByID executa um relatório salvo.
func RunReportByID(ctx iris.Context) {
	report, err := models.ReportByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	// Período pode ser sobrescrito na hora de olhar, sem alterar o salvo.
	if days := ctx.URLParamIntDefault("days", 0); days > 0 {
		report.Filters.Days = days
	}

	result, err := models.RunReport(utils.DB, report)
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}
	ctx.JSON(iris.Map{"report": report, "result": result})
}

// PreviewReport executa uma configuração ainda não salva, para a pessoa ver o
// resultado enquanto monta.
func PreviewReport(ctx iris.Context) {
	var req reportRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	report := &models.Report{}
	// Na prévia o nome ainda pode estar vazio.
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "Prévia"
	}
	if msg := req.apply(report); msg != "" {
		badRequest(ctx, msg)
		return
	}

	result, err := models.RunReport(utils.DB, report)
	if err != nil {
		badRequest(ctx, err.Error())
		return
	}
	ctx.JSON(iris.Map{"result": result})
}
