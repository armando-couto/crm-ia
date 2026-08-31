package models_test

import (
	"database/sql"
	"strconv"
	"testing"
	"time"

	"fixpay/fix-crm/models"
)

func TestValidateWeeklyHours(t *testing.T) {
	if err := models.ValidateWeeklyHours(models.DefaultWeeklyHours()); err != nil {
		t.Fatalf("a agenda comercial padrão deveria ser válida: %v", err)
	}
}

func TestValidateWeeklyHoursRejectsBadInput(t *testing.T) {
	cases := map[string]map[string][]models.TimeWindow{
		"sem dias":         {},
		"dia inválido":     {"9": {{Start: "09:00", End: "12:00"}}},
		"hora inválida":    {"1": {{Start: "25:00", End: "26:00"}}},
		"formato errado":   {"1": {{Start: "9h", End: "12h"}}},
		"fim antes início": {"1": {{Start: "18:00", End: "09:00"}}},
		"início igual fim": {"1": {{Start: "09:00", End: "09:00"}}},
	}
	for name, week := range cases {
		if err := models.ValidateWeeklyHours(week); err == nil {
			t.Errorf("%s: deveria ser recusado", name)
		}
	}
}

// A grade respeita duração, intervalo e o fim da janela.
func TestAvailableSlotsGrid(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	// Quarta-feira, 2 de setembro de 2026, meia-noite.
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local)
	page := &models.BookingPage{
		UserID:      0,
		DurationMin: 30,
		BufferMin:   0,
		DaysAhead:   0,
		NoticeHours: 0,
		WeeklyHours: map[string][]models.TimeWindow{
			strconv.Itoa(int(now.Weekday())): {{Start: "09:00", End: "11:00"}},
		},
	}

	// DaysAhead 0 cai no padrão de 14 dias, então voltam várias quartas;
	// o que importa aqui é a grade do primeiro dia.
	days, err := models.AvailableSlots(db, page, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) == 0 {
		t.Fatal("nenhum dia com horários")
	}
	if days[0].Date != "2026-09-02" {
		t.Fatalf("primeiro dia deveria ser a própria quarta, veio %s", days[0].Date)
	}
	want := []string{"09:00", "09:30", "10:00", "10:30"}
	if len(days[0].Slots) != len(want) {
		t.Fatalf("horários inesperados: %v", days[0].Slots)
	}
	for i, slot := range want {
		if days[0].Slots[i] != slot {
			t.Fatalf("horário %d = %s, esperado %s (%v)", i, days[0].Slots[i], slot, days[0].Slots)
		}
	}
}

// O intervalo entre reuniões espaça a grade.
func TestAvailableSlotsRespectsBuffer(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local)
	page := &models.BookingPage{
		DurationMin: 30,
		BufferMin:   15,
		NoticeHours: 0,
		WeeklyHours: map[string][]models.TimeWindow{
			strconv.Itoa(int(now.Weekday())): {{Start: "09:00", End: "11:00"}},
		},
	}

	days, err := models.AvailableSlots(db, page, now)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"09:00", "09:45", "10:30"}
	got := days[0].Slots
	if len(got) != len(want) {
		t.Fatalf("com intervalo de 15min esperava %v, veio %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("esperava %v, veio %v", want, got)
		}
	}
}

// O aviso mínimo corta os horários próximos demais.
func TestAvailableSlotsRespectsNotice(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	// São 9h; com 2h de aviso, o primeiro horário livre é 11h.
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.Local)
	page := &models.BookingPage{
		DurationMin: 60,
		NoticeHours: 2,
		WeeklyHours: map[string][]models.TimeWindow{
			strconv.Itoa(int(now.Weekday())): {{Start: "09:00", End: "13:00"}},
		},
	}

	days, err := models.AvailableSlots(db, page, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) == 0 || days[0].Slots[0] != "11:00" {
		t.Fatalf("com 2h de aviso o primeiro horário deveria ser 11:00, veio %v", days)
	}
}

// Reunião já marcada some da grade.
func TestAvailableSlotsSkipsBusy(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	user := createTestUser(t, db, "agenda@fixpay.com.br")
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local)

	// Ocupa das 10h às 10h30.
	start := time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)
	end := start.Add(30 * time.Minute)
	if err := models.CreateMeeting(db, &models.Meeting{
		Title: "Reunião existente", Status: "agendada",
		StartsAt: start, EndsAt: &end, UserID: &user,
	}); err != nil {
		t.Fatal(err)
	}

	page := &models.BookingPage{
		UserID:      user,
		DurationMin: 30,
		NoticeHours: 0,
		WeeklyHours: map[string][]models.TimeWindow{
			strconv.Itoa(int(now.Weekday())): {{Start: "09:00", End: "11:00"}},
		},
	}

	days, err := models.AvailableSlots(db, page, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range days[0].Slots {
		if slot == "10:00" {
			t.Fatalf("o horário ocupado não deveria aparecer: %v", days[0].Slots)
		}
	}
	if len(days[0].Slots) != 3 {
		t.Fatalf("esperava 3 horários livres, veio %v", days[0].Slots)
	}
}

// Reunião cancelada volta a liberar o horário.
func TestAvailableSlotsIgnoresCancelled(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	user := createTestUser(t, db, "cancelada@fixpay.com.br")
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local)
	start := time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)
	end := start.Add(30 * time.Minute)
	if err := models.CreateMeeting(db, &models.Meeting{
		Title: "Cancelada", Status: "cancelada",
		StartsAt: start, EndsAt: &end, UserID: &user,
	}); err != nil {
		t.Fatal(err)
	}

	page := &models.BookingPage{
		UserID:      user,
		DurationMin: 30,
		NoticeHours: 0,
		WeeklyHours: map[string][]models.TimeWindow{
			strconv.Itoa(int(now.Weekday())): {{Start: "09:00", End: "11:00"}},
		},
	}
	days, err := models.AvailableSlots(db, page, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(days[0].Slots) != 4 {
		t.Fatalf("reunião cancelada não deveria bloquear: %v", days[0].Slots)
	}
}

// Dia sem janela configurada não entra na lista.
func TestAvailableSlotsSkipsClosedDays(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local)
	page := &models.BookingPage{
		DurationMin: 30,
		DaysAhead:   0,
		NoticeHours: 0,
		// Só domingo aberto; hoje é quarta.
		WeeklyHours: map[string][]models.TimeWindow{"0": {{Start: "09:00", End: "11:00"}}},
	}

	days, err := models.AvailableSlots(db, page, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range days {
		if d.Date == "2026-09-02" {
			t.Fatalf("quarta está fechada e não deveria aparecer: %v", days)
		}
	}
}

func TestSlotAvailable(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	user := createTestUser(t, db, "slot@fixpay.com.br")
	page := &models.BookingPage{
		UserID:      user,
		DurationMin: 30,
		WeeklyHours: map[string][]models.TimeWindow{"3": {{Start: "09:00", End: "11:00"}}},
	}

	livre := time.Date(2026, 9, 2, 9, 30, 0, 0, time.Local) // quarta
	ok, err := models.SlotAvailable(db, page, livre)
	if err != nil || !ok {
		t.Fatalf("horário dentro da janela deveria estar livre (ok=%v, err=%v)", ok, err)
	}

	// Fora da janela.
	fora := time.Date(2026, 9, 2, 20, 0, 0, 0, time.Local)
	if ok, _ := models.SlotAvailable(db, page, fora); ok {
		t.Fatal("horário fora da janela não pode ser aceito")
	}

	// Dia fechado (quinta não está configurada).
	fechado := time.Date(2026, 9, 3, 9, 30, 0, 0, time.Local)
	if ok, _ := models.SlotAvailable(db, page, fechado); ok {
		t.Fatal("dia fechado não pode aceitar agendamento")
	}

	// Depois de ocupar, o mesmo horário deixa de estar livre.
	end := livre.Add(30 * time.Minute)
	if err := models.CreateMeeting(db, &models.Meeting{
		Title: "Ocupada", Status: "agendada", StartsAt: livre, EndsAt: &end, UserID: &user,
	}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := models.SlotAvailable(db, page, livre); ok {
		t.Fatal("horário já ocupado não pode ser aceito")
	}
}

// createTestUser cria um usuário mínimo para pendurar as reuniões do teste.
func createTestUser(t *testing.T, db *sql.DB, email string) int64 {
	t.Helper()
	// cleanTables não mexe em users: limpa o resíduo da rodada anterior.
	if _, err := db.Exec(`DELETE FROM users WHERE email = $1`, email); err != nil {
		t.Fatal(err)
	}
	user := &models.User{Name: "Agenda", Email: email, Role: models.RoleSeller,
		Active: true, PasswordHash: "x"}
	if err := models.CreateUser(db, user); err != nil {
		t.Fatal(err)
	}
	return user.ID
}
