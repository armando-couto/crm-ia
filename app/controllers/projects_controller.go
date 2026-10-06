package controllers

import (
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

func ListProjects(ctx iris.Context) {
	list, err := models.ListProjects(utils.DB, ctx.URLParam("status"))
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(list)
}

func GetProject(ctx iris.Context) {
	project, err := models.ProjectByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(project)
}

type projectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	DueDate     string `json:"due_date"` // AAAA-MM-DD
	OwnerID     *int64 `json:"owner_id"`
}

func (r *projectRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "informe o nome do projeto"
	}
	if r.Status != "" && !models.ValidProjectStatus(r.Status) {
		return "status inválido (ativo, concluido ou arquivado)"
	}
	return ""
}

func (r *projectRequest) dueDate() (*time.Time, string) {
	if r.DueDate == "" {
		return nil, ""
	}
	t, err := time.Parse("2006-01-02", r.DueDate)
	if err != nil {
		return nil, "prazo inválido (use AAAA-MM-DD)"
	}
	return &t, ""
}

func CreateProject(ctx iris.Context) {
	var req projectRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	due, msg := req.dueDate()
	if msg != "" {
		badRequest(ctx, msg)
		return
	}

	project := &models.Project{
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		DueDate:     due,
		OwnerID:     req.OwnerID,
	}
	if project.OwnerID == nil {
		if claims := middlewareClaims(ctx); claims != nil {
			project.OwnerID = &claims.UserID
		}
	}
	if err := models.CreateProject(utils.DB, project); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(project)
}

func UpdateProject(ctx iris.Context) {
	project, err := models.ProjectByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req projectRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	due, msg := req.dueDate()
	if msg != "" {
		badRequest(ctx, msg)
		return
	}

	project.Name = req.Name
	project.Description = req.Description
	if req.Status != "" {
		project.Status = req.Status
	}
	project.DueDate = due
	project.OwnerID = req.OwnerID
	if err := models.UpdateProject(utils.DB, project); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(project)
}

func DeleteProject(ctx iris.Context) {
	if err := models.DeleteProject(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "projeto removido"})
}
