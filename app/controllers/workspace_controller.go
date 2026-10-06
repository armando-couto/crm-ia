package controllers

import (
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// WorkspaceHandler devolve o painel de trabalho do usuário logado: o que está
// atrasado, o que é de hoje e o que está pedindo atenção na carteira dele.
// ?user_id permite a um gestor olhar o dia de alguém da equipe.
func WorkspaceHandler(ctx iris.Context) {
	claims := middlewareClaims(ctx)
	userID := claims.UserID

	if other := ctx.URLParamInt64Default("user_id", 0); other > 0 && other != userID {
		// Ver o dia de outra pessoa exige permissão de gestão de equipe.
		if !models.RoleCan(claims.Role, models.PermForecastView) {
			ctx.StopWithJSON(iris.StatusForbidden,
				iris.Map{"error": "você só pode ver o próprio espaço de trabalho"})
			return
		}
		userID = other
	}

	workspace, err := models.LoadWorkspace(utils.DB, userID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(workspace)
}
