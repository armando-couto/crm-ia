package models

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

var CallOutcomes = []string{"conectada", "sem_resposta", "caixa_postal", "ocupado", "numero_errado"}

func ValidCallOutcome(s string) bool {
	return slices.Contains(CallOutcomes, s)
}

type Call struct {
	ID          int64     `json:"id"`
	Direction   string    `json:"direction"` // entrada | saida
	Outcome     string    `json:"outcome"`
	Duration    int       `json:"duration_seconds"`
	Notes       string    `json:"notes"`
	CalledAt    time.Time `json:"called_at"`
	ContactID   *int64    `json:"contact_id"`
	ContactName string    `json:"contact_name,omitempty"`
	CompanyID   *int64    `json:"company_id"`
	DealID      *int64    `json:"deal_id"`
	UserID      *int64    `json:"user_id"`
	UserName    string    `json:"user_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type CallFilter struct {
	ContactID int64
	UserID    int64
	Outcome   string
	Pagination
}

func ListCalls(db *sql.DB, f CallFilter) ([]Call, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.ContactID > 0 {
		add("c.contact_id = $%d", f.ContactID)
	}
	if f.UserID > 0 {
		add("c.user_id = $%d", f.UserID)
	}
	if f.Outcome != "" {
		add("c.outcome = $%d", f.Outcome)
	}
	cond := strings.Join(where, " AND ")

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM calls c WHERE `+cond, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(`
		SELECT c.id, c.direction, c.outcome, c.duration_seconds, COALESCE(c.notes,''), c.called_at,
		       c.contact_id, COALESCE(ct.first_name || ' ' || COALESCE(ct.last_name,''), ''),
		       c.company_id, c.deal_id, c.user_id, COALESCE(u.name,''), c.created_at
		FROM calls c
		LEFT JOIN contacts ct ON ct.id = c.contact_id
		LEFT JOIN users u ON u.id = c.user_id
		WHERE %s
		ORDER BY c.called_at DESC
		LIMIT $%d OFFSET $%d`, cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Call{}
	for rows.Next() {
		var c Call
		if err := rows.Scan(&c.ID, &c.Direction, &c.Outcome, &c.Duration, &c.Notes, &c.CalledAt,
			&c.ContactID, &c.ContactName, &c.CompanyID, &c.DealID, &c.UserID, &c.UserName, &c.CreatedAt); err != nil {
			return nil, 0, err
		}
		c.ContactName = strings.TrimSpace(c.ContactName)
		list = append(list, c)
	}
	return list, total, rows.Err()
}

func CreateCall(db *sql.DB, c *Call) error {
	if c.Direction == "" {
		c.Direction = "saida"
	}
	if c.Outcome == "" {
		c.Outcome = "conectada"
	}
	if c.CalledAt.IsZero() {
		c.CalledAt = time.Now()
	}
	return db.QueryRow(`
		INSERT INTO calls (direction, outcome, duration_seconds, notes, called_at, contact_id, company_id, deal_id, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`,
		c.Direction, c.Outcome, c.Duration, c.Notes, c.CalledAt, c.ContactID, c.CompanyID, c.DealID, c.UserID,
	).Scan(&c.ID, &c.CreatedAt)
}

func DeleteCall(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM calls WHERE id = $1`, id)
	return err
}
