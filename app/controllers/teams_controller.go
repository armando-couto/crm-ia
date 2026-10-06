package controllers

import (
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

func ListTeams(ctx iris.Context) {
	teams, err := models.ListTeams(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(teams)
}

type teamRequest struct {
	Name string `json:"name"`
}

func (r *teamRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "informe o nome da equipe"
	}
	return ""
}

func CreateTeam(ctx iris.Context) {
	var req teamRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	team := &models.Team{Name: req.Name}
	if err := models.CreateTeam(utils.DB, team); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(team)
}

func UpdateTeam(ctx iris.Context) {
	var req teamRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}
	team := &models.Team{ID: paramID(ctx), Name: req.Name}
	if err := models.UpdateTeam(utils.DB, team); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(team)
}

// DeleteTeam remove a equipe; os membros ficam sem equipe.
func DeleteTeam(ctx iris.Context) {
	if err := models.DeleteTeam(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "equipe removida"})
}
