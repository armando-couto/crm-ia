package controllers

import (
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// ListTrackedGoals devolve as metas com o realizado do mês corrente, mais o
// catálogo de tipos para o modal de criação.
func ListTrackedGoals(ctx iris.Context) {
	goals, err := models.ListTrackedGoals(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": goals, "kinds": models.GoalKindLabels})
}

type trackedGoalRequest struct {
	Kind         string  `json:"kind"`
	Metric       string  `json:"metric"`
	UserID       *int64  `json:"user_id"`
	PipelineID   *int64  `json:"pipeline_id"`
	StageID      *int64  `json:"stage_id"`
	ActivityKind string  `json:"activity_kind"`
	Amount       float64 `json:"amount"`
	StartPeriod  string  `json:"start_period"`
	EndPeriod    string  `json:"end_period"`
}

func (r *trackedGoalRequest) apply(g *models.TrackedGoal) string {
	g.Kind = r.Kind
	g.Metric = r.Metric
	g.UserID = r.UserID
	g.PipelineID = r.PipelineID
	g.StageID = r.StageID
	g.ActivityKind = r.ActivityKind
	g.Amount = r.Amount
	g.StartPeriod = r.StartPeriod
	g.EndPeriod = r.EndPeriod
	if err := models.ValidateTrackedGoal(g); err != nil {
		return err.Error()
	}
	return ""
}

func CreateTrackedGoal(ctx iris.Context) {
	var req trackedGoalRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	goal := &models.TrackedGoal{}
	if msg := req.apply(goal); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if claims := middlewareClaims(ctx); claims != nil {
		goal.CreatedBy = &claims.UserID
	}

	if err := models.CreateTrackedGoal(utils.DB, goal); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditCreate, "meta", goal.ID,
		"criou a meta "+models.GoalKindLabels[goal.Kind])
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(goal)
}

func UpdateTrackedGoalByID(ctx iris.Context) {
	goal, err := models.TrackedGoalByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req trackedGoalRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.apply(goal); msg != "" {
		badRequest(ctx, msg)
		return
	}
	if err := models.UpdateTrackedGoal(utils.DB, goal); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "meta", goal.ID,
		"editou a meta "+models.GoalKindLabels[goal.Kind])
	ctx.JSON(goal)
}

func DeleteTrackedGoalByID(ctx iris.Context) {
	id := paramID(ctx)
	goal, err := models.TrackedGoalByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if err := models.DeleteTrackedGoal(utils.DB, id); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditDelete, "meta", id,
		"excluiu a meta "+models.GoalKindLabels[goal.Kind])
	ctx.JSON(iris.Map{"message": "meta removida"})
}

// TrackedGoalProgressHandler devolve a meta com a série mês a mês.
func TrackedGoalProgressHandler(ctx iris.Context) {
	goal, err := models.TrackedGoalByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	points, err := models.TrackedGoalProgress(utils.DB, goal)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"goal": goal, "points": points})
}
