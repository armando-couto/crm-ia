package controllers_test

import (
	"testing"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

// bookingRow monta a linha da página no formato do bookingSelect.
func bookingRow(id, userID int64, slug string, active bool, week string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "user_id", "user_name", "user_email", "slug", "title", "description",
		"location", "duration_min", "buffer_min", "days_ahead", "notice_hours",
		"weekly_hours", "active", "bookings", "created_at", "updated_at",
	}).AddRow(id, userID, "Ana", "ana@fixpay.com.br", slug, "Agende uma conversa", "",
		"Google Meet", 30, 0, 14, 0, []byte(week), active, 2, now, now)
}

// Sem página salva, o CRM sugere a agenda comercial padrão.
func TestGetMyBookingPageSuggestsDefault(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM booking_pages").
		WithArgs(int64(1)).
		WillReturnError(sqlNoRowsErr())

	resp := e.GET("/api/v1/booking/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("exists").IsEqual(false)
	page := resp.Value("page").Object()
	page.Value("duration_min").IsEqual(30)
	// Segunda a sexta vêm marcadas por padrão.
	page.Value("weekly_hours").Object().ContainsKey("1")
	page.Value("weekly_hours").Object().ContainsKey("5")
}

func TestSaveMyBookingPageValidatesDuration(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM booking_pages").WillReturnError(sqlNoRowsErr())

	e.PUT("/api/v1/booking/me").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"slug":         "ana",
			"duration_min": 2,
			"days_ahead":   14,
			"weekly_hours": map[string]any{"1": []map[string]string{{"start": "09:00", "end": "18:00"}}},
		}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("duração")
}

func TestSaveMyBookingPageValidatesWindows(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM booking_pages").WillReturnError(sqlNoRowsErr())

	e.PUT("/api/v1/booking/me").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"slug":         "ana",
			"duration_min": 30,
			"days_ahead":   14,
			"weekly_hours": map[string]any{"1": []map[string]string{{"start": "18:00", "end": "09:00"}}},
		}).
		Expect().Status(iris.StatusBadRequest)
}

// A página pública mostra os horários livres sem expor dados internos.
func TestGetPublicBooking(t *testing.T) {
	e, mock, _ := newTestApp(t)

	// Deixa a semana inteira aberta para sempre haver horário.
	all := `{"0":[{"start":"09:00","end":"18:00"}],"1":[{"start":"09:00","end":"18:00"}],
	         "2":[{"start":"09:00","end":"18:00"}],"3":[{"start":"09:00","end":"18:00"}],
	         "4":[{"start":"09:00","end":"18:00"}],"5":[{"start":"09:00","end":"18:00"}],
	         "6":[{"start":"09:00","end":"18:00"}]}`

	mock.ExpectQuery("FROM booking_pages").
		WithArgs("ana").
		WillReturnRows(bookingRow(1, 2, "ana", true, all))
	mock.ExpectQuery("FROM meetings").
		WillReturnRows(sqlmock.NewRows([]string{"starts_at", "ends_at"}))

	resp := e.GET("/api/public/booking/ana").
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("host_name").IsEqual("Ana")
	resp.Value("duration_min").IsEqual(30)
	resp.Value("days").Array().NotEmpty()
	// Nada de interno na resposta pública.
	resp.NotContainsKey("user_id")
	resp.NotContainsKey("bookings")
}

func TestGetPublicBookingPausedIsNotFound(t *testing.T) {
	e, mock, _ := newTestApp(t)

	mock.ExpectQuery("FROM booking_pages").
		WithArgs("pausada").
		WillReturnRows(bookingRow(1, 2, "pausada", false, `{}`))

	e.GET("/api/public/booking/pausada").Expect().Status(iris.StatusNotFound)
}

func TestBookPublicSlotRequiresNameAndEmail(t *testing.T) {
	e, mock, _ := newTestApp(t)
	services.ResetRateLimiter()

	mock.ExpectQuery("FROM booking_pages").
		WithArgs("ana").
		WillReturnRows(bookingRow(1, 2, "ana", true, `{"1":[{"start":"09:00","end":"18:00"}]}`))

	e.POST("/api/public/booking/ana").
		WithJSON(map[string]string{"name": "", "email": "sem-arroba", "date": "2026-09-07", "time": "10:00"}).
		Expect().Status(iris.StatusBadRequest)
}

func TestBookPublicSlotRejectsPast(t *testing.T) {
	e, mock, _ := newTestApp(t)
	services.ResetRateLimiter()

	mock.ExpectQuery("FROM booking_pages").
		WithArgs("ana").
		WillReturnRows(bookingRow(1, 2, "ana", true, `{"1":[{"start":"09:00","end":"18:00"}]}`))

	e.POST("/api/public/booking/ana").
		WithJSON(map[string]string{
			"name": "Ana", "email": "ana@cliente.com", "date": "2020-01-06", "time": "10:00",
		}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("passou")
}

// Horário fora da janela é recusado com 409.
func TestBookPublicSlotRejectsUnavailable(t *testing.T) {
	e, mock, _ := newTestApp(t)
	services.ResetRateLimiter()

	future := time.Now().AddDate(0, 0, 3)
	mock.ExpectQuery("FROM booking_pages").
		WithArgs("ana").
		WillReturnRows(bookingRow(1, 2, "ana", true, `{}`)) // nenhum dia aberto

	e.POST("/api/public/booking/ana").
		WithJSON(map[string]string{
			"name": "Ana", "email": "ana@cliente.com",
			"date": future.Format("2006-01-02"), "time": "10:00",
		}).
		Expect().Status(iris.StatusConflict)
}

// O campo isca responde ok sem agendar.
func TestBookPublicSlotHoneypot(t *testing.T) {
	e, mock, _ := newTestApp(t)
	services.ResetRateLimiter()

	mock.ExpectQuery("FROM booking_pages").
		WithArgs("ana").
		WillReturnRows(bookingRow(1, 2, "ana", true, `{"1":[{"start":"09:00","end":"18:00"}]}`))

	e.POST("/api/public/booking/ana").
		WithJSON(map[string]string{
			"name": "Robô", "email": "spam@spam.com", "date": "2026-09-07",
			"time": "10:00", "_gotcha": "x",
		}).
		Expect().Status(iris.StatusOK)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBookingRequiresPermission(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 60, Email: "x@fixpay.com.br", Role: "desconhecido"})

	e.GET("/api/v1/booking/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden)
}
