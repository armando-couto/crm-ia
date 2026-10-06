package controllers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

// ListActivities retorna a timeline de um contato, empresa ou negócio.
func ListActivities(ctx iris.Context) {
	f := models.ActivityFilter{
		ContactID: ctx.URLParamInt64Default("contact_id", 0),
		CompanyID: ctx.URLParamInt64Default("company_id", 0),
		DealID:    ctx.URLParamInt64Default("deal_id", 0),
		TicketID:  ctx.URLParamInt64Default("ticket_id", 0),
		UserID:    ctx.URLParamInt64Default("user_id", 0),
		Kind:      ctx.URLParam("kind"),
		Search:    ctx.URLParam("q"),
		Days:      ctx.URLParamIntDefault("days", 0),
		Limit:     ctx.URLParamIntDefault("limit", 50),
		Feed:      ctx.URLParamBoolDefault("feed", false),
	}
	// Sem registro específico só passa quando é o feed geral de atividades.
	if !f.Feed && f.ContactID == 0 && f.CompanyID == 0 && f.DealID == 0 && f.TicketID == 0 {
		badRequest(ctx, "informe contact_id, company_id, deal_id ou ticket_id")
		return
	}
	list, err := models.ListActivities(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": list})
}

type activityRequest struct {
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	ContactID *int64 `json:"contact_id"`
	CompanyID *int64 `json:"company_id"`
	DealID    *int64 `json:"deal_id"`
	TicketID  *int64 `json:"ticket_id"`
}

// CreateActivityHandler registra manualmente nota, ligação ou reunião na timeline.
func CreateActivityHandler(ctx iris.Context) {
	var req activityRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		badRequest(ctx, "informe o conteúdo")
		return
	}
	if req.Kind == "" {
		req.Kind = models.ActivityNota
	}
	if req.Kind != models.ActivityNota && req.Kind != models.ActivityLigacao && req.Kind != models.ActivityReuniao {
		badRequest(ctx, "tipo inválido (nota, ligacao ou reuniao)")
		return
	}
	if req.ContactID == nil && req.CompanyID == nil && req.DealID == nil && req.TicketID == nil {
		badRequest(ctx, "vincule a atividade a um contato, empresa, negócio ou ticket")
		return
	}

	a := &models.Activity{
		Kind:      req.Kind,
		Content:   req.Content,
		ContactID: req.ContactID,
		CompanyID: req.CompanyID,
		DealID:    req.DealID,
		TicketID:  req.TicketID,
	}
	if claims := middlewareClaims(ctx); claims != nil {
		a.UserID = &claims.UserID
	}
	if err := models.CreateActivity(utils.DB, a); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(a)
}

type sendEmailRequest struct {
	ContactID int64  `json:"contact_id"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	DealID    *int64 `json:"deal_id"`
}

// SendEmail envia e-mail ao contato via Mandrill e registra na timeline.
func SendEmail(ctx iris.Context) {
	var req sendEmailRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Body = strings.TrimSpace(req.Body)
	if req.ContactID == 0 || req.Subject == "" || req.Body == "" {
		badRequest(ctx, "informe contato, assunto e mensagem")
		return
	}

	contact, err := models.ContactByID(utils.DB, req.ContactID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	if contact.Email == "" {
		badRequest(ctx, "o contato não possui e-mail cadastrado")
		return
	}
	if services.Mail == nil {
		badRequest(ctx, "serviço de e-mail não configurado")
		return
	}

	name := strings.TrimSpace(contact.FirstName + " " + contact.LastName)
	html := strings.ReplaceAll(req.Body, "\n", "<br>")

	// Registra o envio e instrumenta o HTML (pixel de abertura + links rastreados).
	msg := &models.EmailMessage{
		Subject:   req.Subject,
		ToEmail:   contact.Email,
		Body:      html,
		ContactID: &contact.ID,
		DealID:    req.DealID,
		Source:    models.EmailSourceManual,
	}
	if claims := middlewareClaims(ctx); claims != nil {
		msg.UserID = &claims.UserID
	}
	tracked, err := services.TrackOutgoingEmail(utils.DB, msg)
	if err != nil {
		// Falha no rastreio não impede o envio: segue com o HTML original.
		ctx.Application().Logger().Errorf("falha ao preparar o rastreio do e-mail: %v", err)
		tracked = html
	}

	if err := services.Mail.Send(contact.Email, name, req.Subject, tracked); err != nil {
		serverError(ctx, fmt.Errorf("falha no envio via Mandrill: %w", err))
		return
	}

	meta, _ := json.Marshal(iris.Map{
		"to": contact.Email, "subject": req.Subject, "email_message_id": msg.ID,
	})
	a := models.Activity{
		Kind:      models.ActivityEmail,
		Content:   fmt.Sprintf("E-mail enviado: %s", req.Subject),
		Metadata:  meta,
		ContactID: &contact.ID,
		DealID:    req.DealID,
	}
	logActivity(ctx, a)
	ctx.JSON(iris.Map{"message": "e-mail enviado com sucesso", "email_message_id": msg.ID})
}
