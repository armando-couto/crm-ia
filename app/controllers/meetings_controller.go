package controllers

import (
	"fmt"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/kataras/iris/v12"
)

func ListMeetings(ctx iris.Context) {
	f := models.MeetingFilter{
		ContactID: ctx.URLParamInt64Default("contact_id", 0),
		CompanyID: ctx.URLParamInt64Default("company_id", 0),
		DealID:    ctx.URLParamInt64Default("deal_id", 0),
		UserID:    ctx.URLParamInt64Default("user_id", 0),
		Status:    ctx.URLParam("status"),
		Period:    ctx.URLParam("period"),
		Pagination: models.Pagination{
			Page:    ctx.URLParamIntDefault("page", 1),
			PerPage: ctx.URLParamIntDefault("per_page", 25),
		},
	}
	list, total, err := models.ListMeetings(utils.DB, f)
	if err != nil {
		serverError(ctx, err)
		return
	}
	f.Total = total
	ctx.JSON(iris.Map{"data": list, "pagination": f.Pagination})
}

type meetingRequest struct {
	Title     string `json:"title"`
	Status    string `json:"status"`
	StartsAt  string `json:"starts_at"`
	EndsAt    string `json:"ends_at"`
	Location  string `json:"location"`
	Notes     string `json:"notes"`
	ContactID *int64 `json:"contact_id"`
	CompanyID *int64 `json:"company_id"`
	DealID    *int64 `json:"deal_id"`
	UserID    *int64 `json:"user_id"`
}

func parseMeetingTime(value string) (*time.Time, bool) {
	if value == "" {
		return nil, true
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return &t, true
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", value, time.Local); err == nil {
		return &t, true
	}
	return nil, false
}

func (r *meetingRequest) toModel() (*models.Meeting, string) {
	r.Title = strings.TrimSpace(r.Title)
	if r.Title == "" {
		return nil, "informe o título da reunião"
	}
	if r.Status != "" && !models.ValidMeetingStatus(r.Status) {
		return nil, "status inválido (agendada, realizada, cancelada ou nao_compareceu)"
	}
	starts, ok := parseMeetingTime(r.StartsAt)
	if !ok || starts == nil {
		return nil, "informe a data/hora de início"
	}
	ends, ok := parseMeetingTime(r.EndsAt)
	if !ok {
		return nil, "data/hora de término inválida"
	}
	if ends != nil && ends.Before(*starts) {
		return nil, "o término não pode ser antes do início"
	}
	return &models.Meeting{
		Title:     r.Title,
		Status:    r.Status,
		StartsAt:  *starts,
		EndsAt:    ends,
		Location:  strings.TrimSpace(r.Location),
		Notes:     r.Notes,
		ContactID: r.ContactID,
		CompanyID: r.CompanyID,
		DealID:    r.DealID,
		UserID:    r.UserID,
	}, ""
}

func CreateMeeting(ctx iris.Context) {
	var req meetingRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	meeting, msg := req.toModel()
	if msg != "" {
		badRequest(ctx, msg)
		return
	}
	if meeting.UserID == nil {
		if claims := middlewareClaims(ctx); claims != nil {
			meeting.UserID = &claims.UserID
		}
	}
	if err := models.CreateMeeting(utils.DB, meeting); err != nil {
		serverError(ctx, err)
		return
	}
	if meeting.ContactID != nil {
		logActivity(ctx, models.Activity{
			Kind:      models.ActivityReuniao,
			Content:   fmt.Sprintf("Reunião agendada: %s (%s)", meeting.Title, meeting.StartsAt.Format("02/01/2006 15:04")),
			ContactID: meeting.ContactID,
			CompanyID: meeting.CompanyID,
			DealID:    meeting.DealID,
		})
		go services.SequenceExitOnMeeting(utils.DB, *meeting.ContactID)
	}
	ctx.StatusCode(iris.StatusCreated)
	ctx.JSON(meeting)
}

func UpdateMeeting(ctx iris.Context) {
	existing, err := models.MeetingByID(utils.DB, paramID(ctx))
	if err != nil {
		handleDBError(ctx, err)
		return
	}

	var req meetingRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	meeting, msg := req.toModel()
	if msg != "" {
		badRequest(ctx, msg)
		return
	}
	meeting.ID = existing.ID
	if meeting.Status == "" {
		meeting.Status = existing.Status
	}
	if meeting.UserID == nil {
		meeting.UserID = existing.UserID
	}
	if err := models.UpdateMeeting(utils.DB, meeting); err != nil {
		serverError(ctx, err)
		return
	}

	updated, err := models.MeetingByID(utils.DB, meeting.ID)
	if err != nil {
		handleDBError(ctx, err)
		return
	}
	ctx.JSON(updated)
}

func DeleteMeeting(ctx iris.Context) {
	if err := models.DeleteMeeting(utils.DB, paramID(ctx)); err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"message": "reunião removida"})
}
