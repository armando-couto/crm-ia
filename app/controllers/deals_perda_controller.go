package controllers

import (
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
	"github.com/kataras/iris/v12"
)

// Motivos de perda de negócio. Contrato: docs/PROSPECCAO.md seção 9, com as
// correções de docs/PROSPECCAO-ADENDO.md. A gravação do motivo no fechamento e
// na mudança de etapa fica em deals_controller.go (CloseDealHandler e MoveDeal).

// GetMotivosPerda: GET /settings/motivos-perda → { motivos, exigir } (só
// autenticação: o vendedor precisa da lista para o diálogo de perda).
func GetMotivosPerda(ctx iris.Context) {
	ctx.JSON(models.LoadMotivosPerda(utils.DB))
}

// UpdateMotivosPerda: PUT /settings/motivos-perda (settings.pipelines).
func UpdateMotivosPerda(ctx iris.Context) {
	// Parte do que está gravado: mandar só "exigir" não apaga a lista.
	m := models.LoadMotivosPerda(utils.DB)
	if err := ctx.ReadJSON(&m); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if err := m.Validate(); err != nil {
		ctx.StopWithJSON(iris.StatusUnprocessableEntity, iris.Map{"error": err.Error()})
		return
	}
	if err := models.SaveMotivosPerda(utils.DB, &m, userIDPtr(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "configuracao", 0, "alterou os motivos de perda de negócio")
	ctx.JSON(m)
}
