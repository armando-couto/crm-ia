package controllers

import (
	"slices"
	"strings"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

func ListTasks(ctx iris.Context) {
	f := models.TaskFilter{
		OwnerID:   ctx.URLParamInt64Default("owner_id", 0),
		ContactID: ctx.URLParamInt64Default("contact_id", 0),
		CompanyID: ctx.URLParamInt64Default("company_id", 0),
		DealID:    ctx.URLParamInt64Default("deal_id", 0),
		ProjectID: ctx.URLParamInt64Default("project_id", 0),
		Status:    ctx.URLParam("status"),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	list, total, err := models.ListTasks(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Total = total
	ctx.JSON(iris.Map{"data": list, "pagination": f.Pagination})
}

type taskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Priority    string `json:"priority"`
	DueDate     string `json:"due_date"` // RFC3339 ou AAAA-MM-DD
	OwnerID     *int64 `json:"owner_id"`
	ContactID   *int64 `json:"contact_id"`
	CompanyID   *int64 `json:"company_id"`
	DealID      *int64 `json:"deal_id"`
	ProjectID   *int64 `json:"project_id"`
}

func (r *taskRequest) validate() string {
	r.Title = strings.TrimSpace(r.Title)
	if r.Title == "" {
		return "informe o título da tarefa"
	}
	if r.Type != "" && !slices.Contains(models.TaskTypes, r.Type) {
		return "tipo inválido (ligacao, email, reuniao ou tarefa)"
	}
	if r.Priority != "" && r.Priority != "baixa" && r.Priority != "media" && r.Priority != "alta" {
		return "prioridade inválida (baixa, media ou alta)"
	}
	return ""
}

func (r *taskRequest) dueDate() (*time.Time, string) {
	if r.DueDate == "" {
		return nil, ""
	}
	if t, err := time.Parse(time.RFC3339, r.DueDate); err == nil {
		return &t, ""
	}
	// Formato do input datetime-local do navegador.
	if t, err := time.ParseInLocation("2006-01-02T15:04", r.DueDate, time.Local); err == nil {
		return &t, ""
	}
	if t, err := time.ParseInLocation("2006-01-02", r.DueDate, time.Local); err == nil {
		return &t, ""
	}
	return nil, "data de vencimento inválida"
}

func (r *taskRequest) apply(t *models.Task, due *time.Time) {
	t.Title = r.Title
	t.Description = r.Description
	if r.Type != "" {
		t.Type = r.Type
	}
	if r.Priority != "" {
		t.Priority = r.Priority
	}
	t.DueDate = due
	t.OwnerID = r.OwnerID
	t.ContactID = r.ContactID
	t.CompanyID = r.CompanyID
	t.DealID = r.DealID
	t.ProjectID = r.ProjectID
}

func CreateTask(ctx iris.Context) {
	var req taskRequest
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

	task := &models.Task{}
	req.apply(task, due)
	if task.OwnerID == nil {
		if claims := middlewareClaims(ctx); claims != nil {
			task.OwnerID = &claims.UserID
		}
	}
	if err := models.CreateTask(utils.DB, task); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(task)
}

func UpdateTask(ctx iris.Context) {
	task, err := models.TaskByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	var req taskRequest
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
	req.apply(task, due)
	if err := models.UpdateTask(utils.DB, task); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(task)
}

// ToggleTask marca/desmarca a conclusão da tarefa.
func ToggleTask(ctx iris.Context) {
	var req struct {
		Done bool `json:"done"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	id := paramID(ctx)
	if err := models.ToggleTask(utils.DB, id, req.Done); err != nil {
		serverError(ctx, err)
		return
	}
	task, err := models.TaskByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(task)
}

func DeleteTask(ctx iris.Context) {
	if err := models.DeleteTask(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "tarefa removida"})
}
