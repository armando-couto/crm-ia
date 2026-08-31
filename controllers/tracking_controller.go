package controllers

import (
	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// TrackOpen responde o pixel 1x1 e contabiliza a abertura. É rota pública
// (chamada pelo cliente de e-mail do destinatário) e nunca devolve erro:
// token inválido também recebe a imagem, para não vazar quais tokens existem.
func TrackOpen(ctx iris.Context) {
	token := ctx.Params().Get("token")
	if id, err := models.EmailMessageIDByToken(utils.DB, token); err == nil {
		if err := models.RecordEmailOpen(utils.DB, id, clientIP(ctx), ctx.GetHeader("User-Agent")); err != nil {
			ctx.Application().Logger().Errorf("falha ao registrar abertura: %v", err)
		}
		markSequenceEngagement(ctx, id)
	}

	ctx.Header("Content-Type", "image/gif")
	// Sem cache: cada leitura do e-mail precisa bater aqui de novo.
	ctx.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	ctx.Header("Pragma", "no-cache")
	ctx.Write(services.TrackingPixel)
}

// TrackClick conta o clique e redireciona para o destino original.
func TrackClick(ctx iris.Context) {
	target, ok := services.DecodeTrackedURL(ctx.URLParam("u"))
	if !ok {
		// Link adulterado: manda para a home em vez de redirecionar às cegas.
		ctx.Redirect(utils.AppURL, iris.StatusFound)
		return
	}

	token := ctx.Params().Get("token")
	if id, err := models.EmailMessageIDByToken(utils.DB, token); err == nil {
		if err := models.RecordEmailClick(utils.DB, id, target, clientIP(ctx),
			ctx.GetHeader("User-Agent")); err != nil {
			ctx.Application().Logger().Errorf("falha ao registrar clique: %v", err)
		}
		markSequenceEngagement(ctx, id)
	}
	ctx.Redirect(target, iris.StatusFound)
}

// markSequenceEngagement avisa a sequência dinâmica que o contato interagiu:
// é isso que faz a cadência trocar as etapas automáticas pelas manuais.
func markSequenceEngagement(ctx iris.Context, messageID int64) {
	msg, err := models.EmailMessageByID(utils.DB, messageID)
	if err != nil || msg.ContactID == nil {
		return
	}
	services.SequenceMarkEngaged(utils.DB, *msg.ContactID)
}

// ListEmailMessages lista os e-mails enviados (com filtro por contato/negócio).
func ListEmailMessages(ctx iris.Context) {
	filter := models.EmailFilter{
		ContactID: ctx.URLParamInt64Default("contact_id", 0),
		DealID:    ctx.URLParamInt64Default("deal_id", 0),
		UserID:    ctx.URLParamInt64Default("user_id", 0),
		Days:      ctx.URLParamIntDefault("days", 0),
		Limit:     ctx.URLParamIntDefault("limit", 50),
	}
	list, err := models.ListEmailMessages(utils.DB, filter)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": list})
}

// GetEmailMessage devolve um envio com o histórico de aberturas e cliques.
func GetEmailMessage(ctx iris.Context) {
	id := paramID(ctx)
	msg, err := models.EmailMessageByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	events, err := models.ListEmailEvents(utils.DB, id)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": msg, "events": events})
}

// EmailStatsHandler resume envios, aberturas e cliques do período.
func EmailStatsHandler(ctx iris.Context) {
	stats, err := models.LoadEmailStats(utils.DB, ctx.URLParamIntDefault("days", 30))
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(stats)
}
