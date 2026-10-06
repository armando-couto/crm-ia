package controllers

import (
	"fmt"
	"strings"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

func ListTickets(ctx iris.Context) {
	f := models.TicketFilter{
		Search:    ctx.URLParam("q"),
		Status:    ctx.URLParam("status"),
		OwnerID:   ctx.URLParamInt64Default("owner_id", 0),
		ContactID: ctx.URLParamInt64Default("contact_id", 0),
		CompanyID: ctx.URLParamInt64Default("company_id", 0),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	list, total, err := models.ListTickets(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Total = total
	ctx.JSON(iris.Map{"data": list, "pagination": f.Pagination})
}

func GetTicket(ctx iris.Context) {
	ticket, err := models.TicketByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(ticket)
}

type ticketRequest struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	ContactID   *int64 `json:"contact_id"`
	CompanyID   *int64 `json:"company_id"`
	OwnerID     *int64 `json:"owner_id"`
}

func (r *ticketRequest) validate() string {
	r.Subject = strings.TrimSpace(r.Subject)
	if r.Subject == "" {
		return "informe o assunto do ticket"
	}
	if r.Status != "" && !models.ValidTicketStatus(r.Status) {
		return "status inválido (aberto, pendente, resolvido ou fechado)"
	}
	if r.Priority != "" && r.Priority != "baixa" && r.Priority != "media" && r.Priority != "alta" {
		return "prioridade inválida (baixa, media ou alta)"
	}
	return ""
}

func CreateTicket(ctx iris.Context) {
	var req ticketRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	ticket := &models.Ticket{
		Subject:     req.Subject,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		ContactID:   req.ContactID,
		CompanyID:   req.CompanyID,
		OwnerID:     req.OwnerID,
	}
	if ticket.OwnerID == nil {
		if claims := middlewareClaims(ctx); claims != nil {
			ticket.OwnerID = &claims.UserID
		}
	}
	if err := models.CreateTicket(utils.DB, ticket); err != nil {
		serverError(ctx, err)
		return
	}
	logActivity(ctx, models.Activity{
		Kind:      models.ActivitySistema,
		Content:   "Ticket criado",
		TicketID:  &ticket.ID,
		ContactID: ticket.ContactID,
		CompanyID: ticket.CompanyID,
	})
	if claims := middlewareClaims(ctx); claims != nil {
		go services.NotifyAssignment(utils.DB, ticket.OwnerID, claims.UserID,
			services.NotifyTicket, ticket.Subject, fmt.Sprintf("/tickets/%d", ticket.ID))
	}
	go services.FireTicketCreated(utils.DB, ticket)

	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(ticket)
}

func UpdateTicket(ctx iris.Context) {
	ticket, err := models.TicketByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req ticketRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if msg := req.validate(); msg != "" {
		badRequest(ctx, msg)
		return
	}

	previousStatus := ticket.Status
	ticket.Subject = req.Subject
	ticket.Description = req.Description
	if req.Status != "" {
		ticket.Status = req.Status
	}
	if req.Priority != "" {
		ticket.Priority = req.Priority
	}
	ticket.ContactID = req.ContactID
	ticket.CompanyID = req.CompanyID
	ticket.OwnerID = req.OwnerID
	if err := models.UpdateTicket(utils.DB, ticket); err != nil {
		serverError(ctx, err)
		return
	}
	if previousStatus != ticket.Status {
		logActivity(ctx, models.Activity{
			Kind:      models.ActivitySistema,
			Content:   fmt.Sprintf("Status alterado de %s para %s", previousStatus, ticket.Status),
			TicketID:  &ticket.ID,
			ContactID: ticket.ContactID,
		})
	}

	updated, err := models.TicketByID(utils.DB, ticket.ID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(updated)
}

func DeleteTicket(ctx iris.Context) {
	if err := models.DeleteTicket(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "ticket removido"})
}
