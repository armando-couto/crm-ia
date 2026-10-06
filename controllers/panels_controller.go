package controllers

import (
	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// ListPanels devolve os painéis visíveis para o usuário e qual está pendurado
// no espaço de trabalho dele.
func ListPanels(ctx iris.Context) {
	claims := middlewareClaims(ctx)

	panels, err := models.ListPanels(utils.DB, claims.UserID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	current, err := models.WorkspaceDashboardID(utils.DB, claims.UserID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": panels, "workspace_dashboard_id": current})
}

// GetPanel abre o painel já com os relatórios executados.
func GetPanel(ctx iris.Context) {
	panel, err := models.RunPanel(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(panel)
}

type panelRequest struct {
	Name   string `json:"name"`
	Shared *bool  `json:"shared"`
}

func CreatePanel(ctx iris.Context) {
	var req panelRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	panel := &models.Panel{Name: req.Name, Shared: true}
	if req.Shared != nil {
		panel.Shared = *req.Shared
	}
	if claims := middlewareClaims(ctx); claims != nil {
		panel.CreatedBy = &claims.UserID
	}

	if err := models.CreatePanel(utils.DB, panel); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	audit(ctx, models.AuditCreate, "painel", panel.ID, "criou o painel "+panel.Name)
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(panel)
}

func UpdatePanelByID(ctx iris.Context) {
	panel, err := models.PanelByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req panelRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	panel.Name = req.Name
	if req.Shared != nil {
		panel.Shared = *req.Shared
	}

	if err := models.UpdatePanel(utils.DB, panel); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	audit(ctx, models.AuditUpdate, "painel", panel.ID, "editou o painel "+panel.Name)
	ctx.JSON(panel)
}

func DeletePanelByID(ctx iris.Context) {
	id := paramID(ctx)
	panel, err := models.PanelByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeletePanel(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "painel", id, "excluiu o painel "+panel.Name)
	ctx.JSON(iris.Map{"message": "painel removido"})
}

// AddPanelItem pendura um relatório no painel.
func AddPanelItem(ctx iris.Context) {
	panelID := paramID(ctx)

	var req struct {
		ReportID int64  `json:"report_id"`
		Width    string `json:"width"`
	}
	if err := ctx.ReadJSON(&req); err != nil || req.ReportID == 0 {
		badRequest(ctx, "informe o relatório")
		return
	}
	if _, err := models.ReportByID(utils.DB, req.ReportID); err != nil {
		badRequest(ctx, "relatório não encontrado")
		return
	}

	if err := models.AddPanelItem(utils.DB, panelID, req.ReportID, req.Width); err != nil {
		serverError(ctx, err)
		return
	}
	panel, err := models.PanelByID(utils.DB, panelID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(panel)
}

func RemovePanelItem(ctx iris.Context) {
	panelID := paramID(ctx)
	itemID := ctx.Params().GetInt64Default("itemId", 0)

	if err := models.RemovePanelItem(utils.DB, panelID, itemID); err != nil {
		serverError(ctx, err)
		return
	}
	panel, err := models.PanelByID(utils.DB, panelID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(panel)
}

// SetWorkspaceDashboard escolhe o painel que aparece na aba Painel do espaço
// de trabalho de quem está logado.
func SetWorkspaceDashboard(ctx iris.Context) {
	claims := middlewareClaims(ctx)

	var req struct {
		DashboardID *int64 `json:"dashboard_id"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if req.DashboardID != nil {
		if _, err := models.PanelByID(utils.DB, *req.DashboardID); err != nil {
			badRequest(ctx, "painel não encontrado")
			return
		}
	}

	if err := models.SetWorkspaceDashboard(utils.DB, claims.UserID, req.DashboardID); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"workspace_dashboard_id": req.DashboardID})
}
