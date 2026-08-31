package controllers

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/kataras/iris/v12"
)

// ===== Configuração (área logada) =====

// GetMyBookingPage devolve a página de agendamento do usuário logado, ou um
// rascunho com a agenda comercial padrão quando ele ainda não tem uma.
func GetMyBookingPage(ctx iris.Context) {
	claims := middlewareClaims(ctx)

	page, err := models.BookingPageByUser(utils.DB, claims.UserID)
	if err == sql.ErrNoRows {
		page = &models.BookingPage{
			UserID:      claims.UserID,
			Slug:        models.Slugify(claims.Name),
			Title:       "Agende uma conversa",
			DurationMin: 30,
			BufferMin:   0,
			DaysAhead:   14,
			NoticeHours: 4,
			WeeklyHours: models.DefaultWeeklyHours(),
			Active:      true,
		}
		ctx.JSON(iris.Map{"page": page, "exists": false, "base_url": strings.TrimSuffix(utils.AppURL, "/")})
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"page": page, "exists": true, "base_url": strings.TrimSuffix(utils.AppURL, "/")})
}

type bookingPageRequest struct {
	Slug        string                         `json:"slug"`
	Title       string                         `json:"title"`
	Description string                         `json:"description"`
	Location    string                         `json:"location"`
	DurationMin int                            `json:"duration_min"`
	BufferMin   int                            `json:"buffer_min"`
	DaysAhead   int                            `json:"days_ahead"`
	NoticeHours int                            `json:"notice_hours"`
	WeeklyHours map[string][]models.TimeWindow `json:"weekly_hours"`
	Active      *bool                          `json:"active"`
}

// SaveMyBookingPage cria ou atualiza a página do usuário logado.
func SaveMyBookingPage(ctx iris.Context) {
	claims := middlewareClaims(ctx)

	var req bookingPageRequest
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}

	page, err := models.BookingPageByUser(utils.DB, claims.UserID)
	if err == sql.ErrNoRows {
		page = &models.BookingPage{UserID: claims.UserID}
	} else if err != nil {
		serverError(ctx, err)
		return
	}

	slug := models.Slugify(req.Slug)
	if slug == "" {
		slug = models.Slugify(claims.Name)
	}
	if slug == "" {
		badRequest(ctx, "informe o endereço da sua página")
		return
	}
	if err := models.ValidateWeeklyHours(req.WeeklyHours); err != nil {
		badRequest(ctx, err.Error())
		return
	}
	if req.DurationMin < 5 || req.DurationMin > 480 {
		badRequest(ctx, "a duração deve ficar entre 5 e 480 minutos")
		return
	}
	if req.BufferMin < 0 || req.BufferMin > 240 {
		badRequest(ctx, "o intervalo entre reuniões deve ficar entre 0 e 240 minutos")
		return
	}
	if req.DaysAhead < 1 || req.DaysAhead > 90 {
		badRequest(ctx, "a agenda pode abrir de 1 a 90 dias à frente")
		return
	}
	if req.NoticeHours < 0 || req.NoticeHours > 168 {
		badRequest(ctx, "o aviso mínimo deve ficar entre 0 e 168 horas")
		return
	}

	page.Slug = slug
	page.Title = strings.TrimSpace(req.Title)
	if page.Title == "" {
		page.Title = "Agende uma conversa"
	}
	page.Description = strings.TrimSpace(req.Description)
	page.Location = strings.TrimSpace(req.Location)
	page.DurationMin = req.DurationMin
	page.BufferMin = req.BufferMin
	page.DaysAhead = req.DaysAhead
	page.NoticeHours = req.NoticeHours
	page.WeeklyHours = req.WeeklyHours
	page.Active = true
	if req.Active != nil {
		page.Active = *req.Active
	}

	if err := models.SaveBookingPage(utils.DB, page); err != nil {
		if strings.Contains(err.Error(), "booking_pages_slug_key") {
			badRequest(ctx, "já existe uma página com este endereço")
			return
		}
		serverError(ctx, err)
		return
	}
	audit(ctx, models.AuditUpdate, "agendamento", page.ID, "salvou a própria página de agendamento")
	ctx.JSON(page)
}

// ListBookingPages mostra as páginas de toda a equipe (para quem administra).
func ListBookingPages(ctx iris.Context) {
	pages, err := models.ListBookingPages(utils.DB)
	if err != nil {
		serverError(ctx, err)
		return
	}
	ctx.JSON(iris.Map{"data": pages, "base_url": strings.TrimSuffix(utils.AppURL, "/")})
}

// ===== Rotas públicas =====

// GetPublicBooking devolve o cabeçalho da página e os horários livres.
func GetPublicBooking(ctx iris.Context) {
	page, err := models.BookingPageBySlug(utils.DB, ctx.Params().Get("slug"))
	if err == sql.ErrNoRows || (err == nil && !page.Active) {
		ctx.StopWithJSON(iris.StatusNotFound, iris.Map{"error": "página não encontrada"})
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}

	days, err := models.AvailableSlots(utils.DB, page, time.Now())
	if err != nil {
		serverError(ctx, err)
		return
	}

	ctx.JSON(iris.Map{
		"slug":         page.Slug,
		"title":        page.Title,
		"description":  page.Description,
		"location":     page.Location,
		"duration_min": page.DurationMin,
		"host_name":    page.UserName,
		"days":         days,
	})
}

// BookPublicSlot grava o agendamento vindo da página pública.
func BookPublicSlot(ctx iris.Context) {
	if services.PublicRateLimited(clientIP(ctx)) {
		ctx.StopWithJSON(iris.StatusTooManyRequests,
			iris.Map{"error": "muitos agendamentos deste endereço: tente novamente em alguns minutos"})
		return
	}

	page, err := models.BookingPageBySlug(utils.DB, ctx.Params().Get("slug"))
	if err == sql.ErrNoRows || (err == nil && !page.Active) {
		ctx.StopWithJSON(iris.StatusNotFound, iris.Map{"error": "página não encontrada"})
		return
	}
	if err != nil {
		serverError(ctx, err)
		return
	}

	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Phone string `json:"phone"`
		Notes string `json:"notes"`
		// Data e hora escolhidas, no fuso do servidor: "2026-09-10" e "14:30".
		Date   string `json:"date"`
		Time   string `json:"time"`
		Gotcha string `json:"_gotcha"`
	}
	if err := ctx.ReadJSON(&req); err != nil {
		badRequest(ctx, "dados inválidos")
		return
	}
	if strings.TrimSpace(req.Gotcha) != "" {
		// Robô: responde ok sem agendar nada.
		ctx.JSON(iris.Map{"message": "Agendamento confirmado."})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = models.NormalizeEmail(req.Email)
	if req.Name == "" || !strings.Contains(req.Email, "@") {
		badRequest(ctx, "informe seu nome e um e-mail válido")
		return
	}

	start, err := time.ParseInLocation("2006-01-02 15:04",
		strings.TrimSpace(req.Date)+" "+strings.TrimSpace(req.Time), time.Local)
	if err != nil {
		badRequest(ctx, "horário inválido")
		return
	}
	if start.Before(time.Now()) {
		badRequest(ctx, "esse horário já passou")
		return
	}

	// Confere de novo na hora de gravar: alguém pode ter pegado o horário.
	free, err := models.SlotAvailable(utils.DB, page, start)
	if err != nil {
		serverError(ctx, err)
		return
	}
	if !free {
		ctx.StopWithJSON(iris.StatusConflict,
			iris.Map{"error": "esse horário acabou de ser ocupado, escolha outro"})
		return
	}

	// Reaproveita o contato quando o e-mail já é conhecido.
	contact, err := models.ContactByEmail(utils.DB, req.Email)
	if err == sql.ErrNoRows {
		first, last := splitName(req.Name)
		contact = &models.Contact{
			FirstName:      first,
			LastName:       last,
			Email:          req.Email,
			Phone:          strings.TrimSpace(req.Phone),
			LifecycleStage: "lead",
			Source:         "agendamento",
			OwnerID:        &page.UserID,
		}
		if err := models.CreateContact(utils.DB, contact); err != nil {
			serverError(ctx, err)
			return
		}
	} else if err != nil {
		serverError(ctx, err)
		return
	}

	meeting, err := models.CreateBooking(utils.DB, page, contact.ID, start, strings.TrimSpace(req.Notes))
	if err != nil {
		serverError(ctx, err)
		return
	}

	content := fmt.Sprintf("Reunião agendada pelo link público para %s",
		start.Format("02/01/2006 às 15:04"))
	if req.Notes != "" {
		content += "\n" + strings.TrimSpace(req.Notes)
	}
	if err := models.CreateActivity(utils.DB, &models.Activity{
		Kind:      models.ActivityReuniao,
		Content:   content,
		ContactID: &contact.ID,
		UserID:    &page.UserID,
	}); err != nil {
		ctx.Application().Logger().Errorf("falha ao registrar a atividade do agendamento: %v", err)
	}

	// Confirmação para quem agendou e aviso para quem vai atender.
	go services.NotifyBooking(utils.DB, page, contact, start)

	ctx.JSON(iris.Map{
		"message":    "Agendamento confirmado! Você vai receber a confirmação por e-mail.",
		"meeting_id": meeting.ID,
		"starts_at":  start,
	})
}

// splitName separa "Ana Maria Silva" em nome e sobrenome.
func splitName(full string) (string, string) {
	parts := strings.Fields(full)
	if len(parts) == 0 {
		return full, ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}
