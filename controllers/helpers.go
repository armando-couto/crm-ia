package controllers

import (
	"database/sql"

	"fixpay/fix-crm/middleware"
	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// middlewareClaims encurta o acesso às claims do usuário autenticado.
func middlewareClaims(ctx iris.Context) *services.Claims {
	return middleware.CurrentClaims(ctx)
}

// logActivity registra um evento na timeline sem interromper a requisição em caso de erro.
func logActivity(ctx iris.Context, a models.Activity) {
	if claims := middlewareClaims(ctx); claims != nil {
		a.UserID = &claims.UserID
	}
	if err := models.CreateActivity(utils.DB, &a); err != nil {
		ctx.Application().Logger().Errorf("falha ao registrar atividade: %v", err)
	}
}

func badRequest(ctx iris.Context, msg string) {
	ctx.StopWithJSON(iris.StatusBadRequest, iris.Map{"error": msg})
}

func notFound(ctx iris.Context) {
	ctx.StopWithJSON(iris.StatusNotFound, iris.Map{"error": "registro não encontrado"})
}

func serverError(ctx iris.Context, err error) {
	ctx.Application().Logger().Error(err)
	ctx.StopWithJSON(iris.StatusInternalServerError, iris.Map{"error": "erro interno, tente novamente"})
}

// handleDBError responde 404 para registros inexistentes e 500 para o restante.
func handleDBError(ctx iris.Context, err error) {
	if err == sql.ErrNoRows {
		notFound(ctx)
		return
	}
	serverError(ctx, err)
}

func paramID(ctx iris.Context) int64 {
	return ctx.Params().GetInt64Default("id", 0)
}
