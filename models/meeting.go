package models

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

var MeetingStatuses = []string{"agendada", "realizada", "cancelada", "nao_compareceu"}

func ValidMeetingStatus(s string) bool {
	return slices.Contains(MeetingStatuses, s)
}

type Meeting struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Status      string     `json:"status"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at"`
	Location    string     `json:"location"`
	Notes       string     `json:"notes"`
	ContactID   *int64     `json:"contact_id"`
	ContactName string     `json:"contact_name,omitempty"`
	CompanyID   *int64     `json:"company_id"`
	DealID      *int64     `json:"deal_id"`
	UserID      *int64     `json:"user_id"`
	UserName    string     `json:"user_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type MeetingFilter struct {
	ContactID int64
	UserID    int64
	Status    string
	Period    string // proximas | passadas
	Pagination
}

const meetingSelect = `
	SELECT m.id, m.title, m.status, m.starts_at, m.ends_at, COALESCE(m.location,''), COALESCE(m.notes,''),
	       m.contact_id, COALESCE(ct.first_name || ' ' || COALESCE(ct.last_name,''), ''),
	       m.company_id, m.deal_id, m.user_id, COALESCE(u.name,''), m.created_at, m.updated_at
	FROM meetings m
	LEFT JOIN contacts ct ON ct.id = m.contact_id
	LEFT JOIN users u ON u.id = m.user_id`

func scanMeeting(row interface{ Scan(...any) error }) (*Meeting, error) {
	var m Meeting
	err := row.Scan(&m.ID, &m.Title, &m.Status, &m.StartsAt, &m.EndsAt, &m.Location, &m.Notes,
		&m.ContactID, &m.ContactName, &m.CompanyID, &m.DealID, &m.UserID, &m.UserName,
		&m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	m.ContactName = strings.TrimSpace(m.ContactName)
	return &m, nil
}

func ListMeetings(db *sql.DB, f MeetingFilter) ([]Meeting, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.ContactID > 0 {
		add("m.contact_id = $%d", f.ContactID)
	}
	if f.UserID > 0 {
		add("m.user_id = $%d", f.UserID)
	}
	if f.Status != "" {
		add("m.status = $%d", f.Status)
	}
	order := "m.starts_at DESC"
	switch f.Period {
	case "proximas":
		where = append(where, "m.starts_at >= NOW()")
		order = "m.starts_at ASC"
	case "passadas":
		where = append(where, "m.starts_at < NOW()")
	}
	cond := strings.Join(where, " AND ")

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM meetings m WHERE `+cond, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(`%s WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		meetingSelect, cond, order, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Meeting{}
	for rows.Next() {
		m, err := scanMeeting(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *m)
	}
	return list, total, rows.Err()
}

func MeetingByID(db *sql.DB, id int64) (*Meeting, error) {
	row := db.QueryRow(meetingSelect+` WHERE m.id = $1`, id)
	return scanMeeting(row)
}

func CreateMeeting(db *sql.DB, m *Meeting) error {
	if m.Status == "" {
		m.Status = "agendada"
	}
	return db.QueryRow(`
		INSERT INTO meetings (title, status, starts_at, ends_at, location, notes, contact_id, company_id, deal_id, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`,
		m.Title, m.Status, m.StartsAt, m.EndsAt, m.Location, m.Notes,
		m.ContactID, m.CompanyID, m.DealID, m.UserID,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

func UpdateMeeting(db *sql.DB, m *Meeting) error {
	_, err := db.Exec(`
		UPDATE meetings SET title = $1, status = $2, starts_at = $3, ends_at = $4, location = $5,
		       notes = $6, contact_id = $7, company_id = $8, deal_id = $9, user_id = $10, updated_at = NOW()
		WHERE id = $11`,
		m.Title, m.Status, m.StartsAt, m.EndsAt, m.Location, m.Notes,
		m.ContactID, m.CompanyID, m.DealID, m.UserID, m.ID)
	return err
}

func DeleteMeeting(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM meetings WHERE id = $1`, id)
	return err
}
