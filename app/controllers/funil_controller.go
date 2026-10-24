package controllers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// Prazos padrão do funil: etapa sem prazo_dias usa 14 dias; sem atividade há
// mais de 7 dias entra em risco (ver docs/FUNIL-E-INTELIGENCIA.md).
const (
	funilPrazoPadraoDias  = 14
	funilSemAtividadeDias = 7
)

// Funil devolve a análise do pipeline para a tela /funil.
// GET /funil?pipeline_id=&periodo=30&modo=fluxo&owner_id=
// Sem pipeline_id usa o primeiro pipeline. Quem não tem forecast.view só vê os
// próprios negócios.
func Funil(ctx iris.Context) {
	f := models.FunilFilter{
		PipelineID:       ctx.URLParamInt64Default("pipeline_id", 0),
		Periodo:          ctx.URLParamDefault("periodo", "30"),
		Modo:             ctx.URLParamDefault("modo", models.FunilModoFluxo),
		PrazoPadraoDias:  funilPrazoPadraoDias,
		SemAtividadeDias: funilSemAtividadeDias,
		Now:              time.Now(),
	}
	if f.Modo != models.FunilModoCoorte && f.Modo != models.FunilModoFluxo {
		badRequest(ctx, models.ErrFunilModoInvalido.Error())
		return
	}
	if owner := ctx.URLParamInt64Default("owner_id", 0); owner > 0 {
		f.OwnerID = &owner
	}
	if claims := middlewareClaims(ctx); claims != nil && !models.RoleCan(claims.Role, models.PermForecastView) {
		proprio := claims.UserID
		f.OwnerID = &proprio
	}

	if f.PipelineID == 0 {
		pipelines, err := models.ListPipelines(utils.DB)
		if err != nil {
			serverError(ctx, err)
			return
		}
		if len(pipelines) == 0 {
			ctx.StopWithJSON(iris.StatusNotFound, iris.Map{"error": "nenhum pipeline cadastrado"})
			return
		}
		f.PipelineID = pipelines[0].ID
	}

	funil, err := models.LoadFunil(utils.DB, f)
	switch {
	case errors.Is(err, models.ErrFunilPeriodoInvalido), errors.Is(err, models.ErrFunilModoInvalido):
		badRequest(ctx, err.Error())
		return
	case errors.Is(err, sql.ErrNoRows):
		ctx.StopWithJSON(iris.StatusNotFound, iris.Map{"error": "pipeline não encontrado"})
		return
	case err != nil:
		serverError(ctx, err)
		return
	}
	ctx.JSON(funil)
}
