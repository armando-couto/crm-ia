package controllers

import (
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// ListConversations lista as conversas da caixa de entrada, filtradas pela
// fila (nao_atribuido | minhas | todas) e pelo status.
func ListConversations(ctx iris.Context) {
	claims := middlewareClaims(ctx)
	filter := models.ConversationFilter{
		Status: ctx.URLParam("status"),
		Queue:  ctx.URLParam("queue"),
		UserID: claims.UserID,
	}
	list, err := models.ListConversations(utils.DB, filter)
	if err != nil {
		serverError(ctx, err)
		return
	}
	unread, _ := models.CountUnreadConversations(utils.DB)
	counters, err := models.LoadInboxCounters(utils.DB, claims.UserID)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": list, "unread": unread, "counters": counters})
}

// AssignConversation define o dono da conversa (nulo devolve para a fila).
func AssignConversation(ctx iris.Context) {
	var req struct {
		OwnerID *int64 `json:"owner_id"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	id := paramID(ctx)
	if err := models.AssignConversation(utils.DB, id, req.OwnerID); err != nil {
		serverError(ctx, err)
		return
	}
	conv, err := models.ConversationByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(conv)
}

// AddConversationComment registra um comentário interno na conversa: aparece
// na thread para a equipe, mas nenhum e-mail é enviado ao contato.
func AddConversationComment(ctx iris.Context) {
	var req struct {
		Body string `json:"body"`
	}
	if err := ctx.ReadJSON(&req); err != nil || strings.TrimSpace(req.Body) == "" {
		badRequest(ctx, "escreva o comentário")
		return
	}

	conv, err := models.ConversationByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	msg := &models.ConversationMessage{
		ConversationID: conv.ID,
		Direction:      "comentario",
		Subject:        conv.Subject,
		Body:           strings.TrimSpace(req.Body),
	}
	if claims := middlewareClaims(ctx); claims != nil {
		msg.UserID = &claims.UserID
		msg.UserName = claims.Name
	}
	if err := models.AddConversationMessage(utils.DB, msg); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(msg)
}

// GetConversation abre a conversa (marcando como lida) com suas mensagens.
func GetConversation(ctx iris.Context) {
	id := paramID(ctx)
	conv, err := models.ConversationByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	messages, err := models.ConversationMessages(utils.DB, id)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if conv.Unread {
		if err := models.MarkConversationRead(utils.DB, id); err == nil {
			conv.Unread = false
		}
	}
	ctx.JSON(iris.Map{"conversation": conv, "messages": messages})
}

// ReplyConversation responde a conversa por e-mail via Mandrill.
func ReplyConversation(ctx iris.Context) {
	var req struct {
		Body string `json:"body"`
	}
	if err := ctx.ReadJSON(&req); err != nil || strings.TrimSpace(req.Body) == "" {
		badRequest(ctx, "informe a mensagem")
		return
	}
	req.Body = strings.TrimSpace(req.Body)

	conv, err := models.ConversationByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if services.Mail == nil {
		badRequest(ctx, "serviço de e-mail não configurado")
		return
	}

	subject := "Re: " + conv.Subject
	html := strings.ReplaceAll(req.Body, "\n", "<br>")
	if err := services.Mail.Send(conv.PeerEmail, conv.ContactName, subject, html); err != nil {
		serverError(ctx, err)
		return
	}

	msg := &models.ConversationMessage{
		ConversationID: conv.ID,
		Direction:      "enviada",
		FromEmail:      models.NormalizeEmail(remetenteAtual()),
		ToEmail:        conv.PeerEmail,
		Subject:        subject,
		Body:           req.Body,
	}
	if claims := middlewareClaims(ctx); claims != nil {
		msg.UserID = &claims.UserID
		msg.UserName = claims.Name
	}
	if err := models.AddConversationMessage(utils.DB, msg); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(msg)
}

// SetConversationStatus abre/fecha a conversa.
func SetConversationStatus(ctx iris.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if err := ctx.ReadJSON(&req); err != nil || (req.Status != "aberta" && req.Status != "fechada") {
		badRequest(ctx, "status inválido (aberta ou fechada)")
		return
	}
	id := paramID(ctx)
	if err := models.SetConversationStatus(utils.DB, id, req.Status); err != nil {
		serverError(ctx, err)
		return
	}
	conv, err := models.ConversationByID(utils.DB, id)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(conv)
}

// MandrillInboundWebhook recebe os e-mails de entrada do Mandrill
// (rota pública: o Mandrill posta form-encoded com o campo mandrill_events).
// Configure a rota de inbound no painel do Mandrill apontando para
// POST {app_url}/api/webhooks/mandrill/inbound e copie a webhook key para
// mandrill_webhook_key no .env — sem ela o endpoint recusa tudo.
func MandrillInboundWebhook(ctx iris.Context) {
	if err := verifyMandrillRequest(ctx); err != nil {
		ctx.Application().Logger().Warnf("webhook do Mandrill recusado (%v) vindo de %s", err, clientIP(ctx))
		ctx.StopWithJSON(iris.StatusUnauthorized, iris.Map{"error": "assinatura inválida"})
		return
	}

	payload := ctx.FormValue("mandrill_events")
	if payload == "" {
		// O Mandrill envia um POST vazio ao validar a URL do webhook.
		ctx.JSON(iris.Map{"message": "ok"})
		return
	}
	events, err := services.ParseInboundEvents(payload)
	if err != nil {
		badRequest(ctx, "payload inválido")
		return
	}
	created, err := services.ProcessInboundEvents(utils.DB, events)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"created": created})
}

// verifyMandrillRequest confere o X-Mandrill-Signature sobre todos os campos do
// POST. A URL usada no cálculo é a cadastrada no painel (mandrill_webhook_url);
// sem ela, reconstruímos a partir do app_url.
func verifyMandrillRequest(ctx iris.Context) error {
	if err := ctx.Request().ParseForm(); err != nil {
		return err
	}
	params := map[string]string{}
	for key, values := range ctx.Request().PostForm {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	return services.VerifyMandrillSignature(
		ctx.GetHeader("X-Mandrill-Signature"), utils.MandrillWebhookURL, params)
}

// remetenteAtual é o "de" das respostas da caixa de entrada: o configurado
// pelo cliente ou, na falta, o padrão do ambiente.
func remetenteAtual() string {
	if e := models.LoadEmailSettings(utils.DB); e != nil && e.FromEmail != "" {
		return e.FromEmail
	}
	return utils.Cfg.FromEmail
}
