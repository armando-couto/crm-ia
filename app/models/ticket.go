package models

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

var TicketStatuses = []string{"aberto", "pendente", "resolvido", "fechado"}

func ValidTicketStatus(s string) bool {
	return slices.Contains(TicketStatuses, s)
}

type Ticket struct {
	ID          int64      `json:"id"`
	Subject     string     `json:"subject"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	ContactID   *int64     `json:"contact_id"`
	ContactName string     `json:"contact_name,omitempty"`
	CompanyID   *int64     `json:"company_id"`
	CompanyName string     `json:"company_name,omitempty"`
	OwnerID     *int64     `json:"owner_id"`
	OwnerName   string     `json:"owner_name,omitempty"`
	ClosedAt    *time.Time `json:"closed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type TicketFilter struct {
	Search    string
	Status    string
	OwnerID   int64
	ContactID int64
	CompanyID int64
	Pagination
}

const ticketSelect = `
	SELECT t.id, t.subject, COALESCE(t.description,''), t.status, t.priority,
	       t.contact_id, COALESCE(ct.first_name || ' ' || COALESCE(ct.last_name,''), ''),
	       t.company_id, COALESCE(co.name,''),
	       t.owner_id, COALESCE(u.name,''),
	       t.closed_at, t.created_at, t.updated_at
	FROM tickets t
	LEFT JOIN contacts ct ON ct.id = t.contact_id
	LEFT JOIN companies co ON co.id = t.company_id
	LEFT JOIN users u ON u.id = t.owner_id`

func scanTicket(row interface{ Scan(...any) error }) (*Ticket, error) {
	var t Ticket
	err := row.Scan(&t.ID, &t.Subject, &t.Description, &t.Status, &t.Priority,
		&t.ContactID, &t.ContactName, &t.CompanyID, &t.CompanyName,
		&t.OwnerID, &t.OwnerName, &t.ClosedAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.ContactName = strings.TrimSpace(t.ContactName)
	return &t, nil
}

func ListTickets(db *sql.DB, f TicketFilter) ([]Ticket, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Search != "" {
		add("LOWER(t.subject) LIKE $%d", "%"+strings.ToLower(f.Search)+"%")
	}
	if f.Status != "" {
		add("t.status = $%d", f.Status)
	}
	if f.OwnerID > 0 {
		add("t.owner_id = $%d", f.OwnerID)
	}
	if f.ContactID > 0 {
		add("t.contact_id = $%d", f.ContactID)
	}
	if f.CompanyID > 0 {
		add("t.company_id = $%d", f.CompanyID)
	}
	cond := strings.Join(where, " AND ")

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM tickets t WHERE `+cond, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(`%s WHERE %s ORDER BY t.updated_at DESC LIMIT $%d OFFSET $%d`,
		ticketSelect, cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *t)
	}
	return list, total, rows.Err()
}

func TicketByID(db *sql.DB, id int64) (*Ticket, error) {
	row := db.QueryRow(ticketSelect+` WHERE t.id = $1`, id)
	return scanTicket(row)
}

func CreateTicket(db *sql.DB, t *Ticket) error {
	if t.Status == "" {
		t.Status = "aberto"
	}
	if t.Priority == "" {
		t.Priority = "media"
	}
	return db.QueryRow(`
		INSERT INTO tickets (subject, description, status, priority, contact_id, company_id, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		t.Subject, t.Description, t.Status, t.Priority, t.ContactID, t.CompanyID, t.OwnerID,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

// UpdateTicket grava alterações; fecha/reabre closed_at conforme o status.
func UpdateTicket(db *sql.DB, t *Ticket) error {
	closed := t.Status == "resolvido" || t.Status == "fechado"
	_, err := db.Exec(`
		UPDATE tickets SET subject = $1, description = $2, status = $3, priority = $4,
		       contact_id = $5, company_id = $6, owner_id = $7,
		       closed_at = CASE WHEN $8 THEN COALESCE(closed_at, NOW()) ELSE NULL END,
		       updated_at = NOW()
		WHERE id = $9`,
		t.Subject, t.Description, t.Status, t.Priority, t.ContactID, t.CompanyID, t.OwnerID, closed, t.ID)
	return err
}

func DeleteTicket(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM tickets WHERE id = $1`, id)
	return err
}
