package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TimeWindow é uma janela de atendimento no formato "09:00"–"12:00".
type TimeWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// BookingPage é a página pública de agendamento de uma pessoa da equipe.
type BookingPage struct {
	ID          int64                   `json:"id"`
	UserID      int64                   `json:"user_id"`
	UserName    string                  `json:"user_name,omitempty"`
	UserEmail   string                  `json:"user_email,omitempty"`
	Slug        string                  `json:"slug"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	Location    string                  `json:"location"`
	DurationMin int                     `json:"duration_min"`
	BufferMin   int                     `json:"buffer_min"`
	DaysAhead   int                     `json:"days_ahead"`
	NoticeHours int                     `json:"notice_hours"`
	WeeklyHours map[string][]TimeWindow `json:"weekly_hours"`
	Active      bool                    `json:"active"`
	Bookings    int                     `json:"bookings"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

// DaySlots são os horários livres de um dia.
type DaySlots struct {
	Date  string   `json:"date"`
	Slots []string `json:"slots"`
}

// DefaultWeeklyHours é a agenda comercial sugerida: seg–sex, 9–12 e 13–18.
func DefaultWeeklyHours() map[string][]TimeWindow {
	week := map[string][]TimeWindow{}
	for day := 1; day <= 5; day++ {
		week[strconv.Itoa(day)] = []TimeWindow{
			{Start: "09:00", End: "12:00"},
			{Start: "13:00", End: "18:00"},
		}
	}
	return week
}

// ValidateWeeklyHours confere formato e ordem das janelas.
func ValidateWeeklyHours(week map[string][]TimeWindow) error {
	if len(week) == 0 {
		return fmt.Errorf("defina pelo menos um dia de atendimento")
	}
	for day, windows := range week {
		n, err := strconv.Atoi(day)
		if err != nil || n < 0 || n > 6 {
			return fmt.Errorf("dia da semana inválido: %s", day)
		}
		for _, w := range windows {
			start, err := parseClock(w.Start)
			if err != nil {
				return fmt.Errorf("horário inválido: %s", w.Start)
			}
			end, err := parseClock(w.End)
			if err != nil {
				return fmt.Errorf("horário inválido: %s", w.End)
			}
			if end <= start {
				return fmt.Errorf("a janela %s–%s termina antes de começar", w.Start, w.End)
			}
		}
	}
	return nil
}

// parseClock lê "HH:MM" como duração desde a meia-noite.
func parseClock(value string) (time.Duration, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("formato esperado HH:MM")
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("horário fora do intervalo")
	}
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute, nil
}

const bookingSelect = `
	SELECT b.id, b.user_id, COALESCE(u.name,''), COALESCE(u.email,''), b.slug, b.title,
	       b.description, b.location, b.duration_min, b.buffer_min, b.days_ahead,
	       b.notice_hours, b.weekly_hours, b.active, b.bookings, b.created_at, b.updated_at
	FROM booking_pages b
	LEFT JOIN users u ON u.id = b.user_id`

func scanBookingPage(row interface{ Scan(...any) error }) (*BookingPage, error) {
	var p BookingPage
	var week []byte
	err := row.Scan(&p.ID, &p.UserID, &p.UserName, &p.UserEmail, &p.Slug, &p.Title,
		&p.Description, &p.Location, &p.DurationMin, &p.BufferMin, &p.DaysAhead,
		&p.NoticeHours, &week, &p.Active, &p.Bookings, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.WeeklyHours = map[string][]TimeWindow{}
	if len(week) > 0 {
		if err := json.Unmarshal(week, &p.WeeklyHours); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

func ListBookingPages(db *sql.DB) ([]BookingPage, error) {
	rows, err := db.Query(bookingSelect + ` ORDER BY u.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []BookingPage{}
	for rows.Next() {
		p, err := scanBookingPage(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	return list, rows.Err()
}

func BookingPageByUser(db *sql.DB, userID int64) (*BookingPage, error) {
	return scanBookingPage(db.QueryRow(bookingSelect+` WHERE b.user_id = $1`, userID))
}

func BookingPageBySlug(db *sql.DB, slug string) (*BookingPage, error) {
	return scanBookingPage(db.QueryRow(bookingSelect+` WHERE b.slug = $1`, slug))
}

func SaveBookingPage(db *sql.DB, p *BookingPage) error {
	week, err := json.Marshal(p.WeeklyHours)
	if err != nil {
		return err
	}
	if p.ID == 0 {
		return db.QueryRow(`
			INSERT INTO booking_pages (user_id, slug, title, description, location,
			    duration_min, buffer_min, days_ahead, notice_hours, weekly_hours, active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			RETURNING id, created_at, updated_at`,
			p.UserID, p.Slug, p.Title, p.Description, p.Location, p.DurationMin,
			p.BufferMin, p.DaysAhead, p.NoticeHours, week, p.Active,
		).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	}
	_, err = db.Exec(`
		UPDATE booking_pages SET slug = $1, title = $2, description = $3, location = $4,
		    duration_min = $5, buffer_min = $6, days_ahead = $7, notice_hours = $8,
		    weekly_hours = $9, active = $10, updated_at = NOW()
		WHERE id = $11`,
		p.Slug, p.Title, p.Description, p.Location, p.DurationMin, p.BufferMin,
		p.DaysAhead, p.NoticeHours, week, p.Active, p.ID)
	return err
}

func DeleteBookingPage(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM booking_pages WHERE id = $1`, id)
	return err
}

// busyRanges devolve as reuniões já marcadas do dono da página no período.
func busyRanges(db *sql.DB, userID int64, from, to time.Time) ([][2]time.Time, error) {
	rows, err := db.Query(`
		SELECT starts_at, COALESCE(ends_at, starts_at + INTERVAL '30 minutes')
		FROM meetings
		WHERE user_id = $1 AND status <> 'cancelada' AND starts_at < $3 AND
		      COALESCE(ends_at, starts_at + INTERVAL '30 minutes') > $2`,
		userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	busy := [][2]time.Time{}
	for rows.Next() {
		var start, end time.Time
		if err := rows.Scan(&start, &end); err != nil {
			return nil, err
		}
		busy = append(busy, [2]time.Time{start, end})
	}
	return busy, rows.Err()
}

// AvailableSlots monta os horários livres da página, dia a dia, no fuso do
// servidor. Considera as janelas semanais, a duração + intervalo entre reuniões,
// o aviso mínimo e o que já está na agenda de quem atende.
func AvailableSlots(db *sql.DB, page *BookingPage, now time.Time) ([]DaySlots, error) {
	if page.DurationMin <= 0 {
		page.DurationMin = 30
	}
	daysAhead := page.DaysAhead
	if daysAhead <= 0 || daysAhead > 90 {
		daysAhead = 14
	}

	from := now
	to := now.AddDate(0, 0, daysAhead+1)
	busy, err := busyRanges(db, page.UserID, from, to)
	if err != nil {
		return nil, err
	}

	earliest := now.Add(time.Duration(page.NoticeHours) * time.Hour)
	step := time.Duration(page.DurationMin+page.BufferMin) * time.Minute
	duration := time.Duration(page.DurationMin) * time.Minute

	days := []DaySlots{}
	for offset := 0; offset <= daysAhead; offset++ {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
			AddDate(0, 0, offset)

		windows := page.WeeklyHours[strconv.Itoa(int(day.Weekday()))]
		if len(windows) == 0 {
			continue
		}

		slots := []string{}
		for _, w := range windows {
			start, err := parseClock(w.Start)
			if err != nil {
				continue
			}
			end, err := parseClock(w.End)
			if err != nil {
				continue
			}
			for at := day.Add(start); !at.Add(duration).After(day.Add(end)); at = at.Add(step) {
				if at.Before(earliest) {
					continue
				}
				if overlapsBusy(at, at.Add(duration), busy) {
					continue
				}
				slots = append(slots, at.Format("15:04"))
			}
		}
		if len(slots) == 0 {
			continue
		}
		sort.Strings(slots)
		days = append(days, DaySlots{Date: day.Format("2006-01-02"), Slots: slots})
	}
	return days, nil
}

func overlapsBusy(start, end time.Time, busy [][2]time.Time) bool {
	for _, b := range busy {
		if start.Before(b[1]) && end.After(b[0]) {
			return true
		}
	}
	return false
}

// SlotAvailable confere de novo, na hora de gravar, se o horário continua livre
// (duas pessoas podem abrir a página ao mesmo tempo).
func SlotAvailable(db *sql.DB, page *BookingPage, start time.Time) (bool, error) {
	duration := time.Duration(page.DurationMin) * time.Minute
	end := start.Add(duration)

	windows := page.WeeklyHours[strconv.Itoa(int(start.Weekday()))]
	if len(windows) == 0 {
		return false, nil
	}
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	inWindow := false
	for _, w := range windows {
		ws, err1 := parseClock(w.Start)
		we, err2 := parseClock(w.End)
		if err1 != nil || err2 != nil {
			continue
		}
		if !start.Before(day.Add(ws)) && !end.After(day.Add(we)) {
			inWindow = true
			break
		}
	}
	if !inWindow {
		return false, nil
	}

	busy, err := busyRanges(db, page.UserID, start, end)
	if err != nil {
		return false, err
	}
	return !overlapsBusy(start, end, busy), nil
}

// CreateBooking grava a reunião do agendamento público.
func CreateBooking(db *sql.DB, page *BookingPage, contactID int64, start time.Time, notes string) (*Meeting, error) {
	end := start.Add(time.Duration(page.DurationMin) * time.Minute)
	meeting := &Meeting{
		Title:     page.Title,
		Status:    "agendada",
		StartsAt:  start,
		EndsAt:    &end,
		Location:  page.Location,
		Notes:     notes,
		ContactID: &contactID,
		UserID:    &page.UserID,
	}
	if err := CreateMeeting(db, meeting); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`UPDATE meetings SET booking_page_id = $1 WHERE id = $2`,
		page.ID, meeting.ID); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`UPDATE booking_pages SET bookings = bookings + 1 WHERE id = $1`,
		page.ID); err != nil {
		return nil, err
	}
	return meeting, nil
}
