package controllers

import (
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

func ListPipelines(ctx iris.Context) {
	pipelines, err := models.ListPipelines(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(pipelines)
}

type pipelineRequest struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
}

func CreatePipeline(ctx iris.Context) {
	var req pipelineRequest
	if err := ctx.ReadJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		badRequest(ctx, "informe o nome do pipeline")
		return
	}
	p := &models.Pipeline{Name: strings.TrimSpace(req.Name), Position: req.Position, Stages: []models.PipelineStage{}}
	if err := models.CreatePipeline(utils.DB, p); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(p)
}

func UpdatePipeline(ctx iris.Context) {
	var req pipelineRequest
	if err := ctx.ReadJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		badRequest(ctx, "informe o nome do pipeline")
		return
	}
	p := &models.Pipeline{ID: paramID(ctx), Name: strings.TrimSpace(req.Name), Position: req.Position}
	if err := models.UpdatePipeline(utils.DB, p); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(p)
}

func DeletePipeline(ctx iris.Context) {
	if err := models.DeletePipeline(utils.DB, paramID(ctx)); err != nil {
		badRequest(ctx, "não foi possível remover: verifique se há negócios neste pipeline")
		return
	}
	ctx.JSON(iris.Map{"message": "pipeline removido"})
}

type stageRequest struct {
	Name        string `json:"name"`
	Position    int    `json:"position"`
	Probability int    `json:"probability"`
	IsWon       bool   `json:"is_won"`
	IsLost      bool   `json:"is_lost"`
}

func (r *stageRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "informe o nome da etapa"
	}
	if r.Probability < 0 || r.Probability > 100 {
		return "probabilidade deve estar entre 0 e 100"
	}
	if r.IsWon && r.IsLost {
		return "a etapa não pode ser de ganho e perda ao mesmo tempo"
	}
	return ""
}

func CreateStage(ctx iris.Context) {
	var req stageRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	s := &models.PipelineStage{
		PipelineID:  paramID(ctx),
		Name:        req.Name,
		Position:    req.Position,
		Probability: req.Probability,
		IsWon:       req.IsWon,
		IsLost:      req.IsLost,
	}
	if err := models.CreateStage(utils.DB, s); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(s)
}

func UpdateStage(ctx iris.Context) {
	stage, err := models.StageByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	var req stageRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	stage.Name = req.Name
	stage.Position = req.Position
	stage.Probability = req.Probability
	stage.IsWon = req.IsWon
	stage.IsLost = req.IsLost
	if err := models.UpdateStage(utils.DB, stage); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(stage)
}

// ReorderStages aplica a nova ordem das fases do pipeline (drag/setas no editor).
func ReorderStages(ctx iris.Context) {
	var req struct {
		StageIDs []int64 `json:"stage_ids"`
	}
	if err := ctx.ReadJSON(&req); err != nil || len(req.StageIDs) == 0 {
		badRequest(ctx, "informe a ordem das fases")
		return
	}
	if err := models.ReorderStages(utils.DB, paramID(ctx), req.StageIDs); err != nil {
		badRequest(ctx, "não foi possível reordenar: verifique se as fases pertencem ao pipeline")
		return
	}
	ctx.JSON(iris.Map{"message": "ordem atualizada"})
}

func DeleteStage(ctx iris.Context) {
	if err := models.DeleteStage(utils.DB, paramID(ctx)); err != nil {
		badRequest(ctx, "não foi possível remover: mova os negócios desta etapa antes")
		return
	}
	ctx.JSON(iris.Map{"message": "etapa removida"})
}
