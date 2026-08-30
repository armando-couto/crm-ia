package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	ActivityNota    = "nota"
	ActivityEmail   = "email"
	ActivityLigacao = "ligacao"
	ActivityReuniao = "reuniao"
	ActivitySistema = "sistema"
)

type Activity struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	Content   string          `json:"content"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	UserID    *int64          `json:"user_id"`
	UserName  string          `json:"user_name,omitempty"`
	ContactID *int64          `json:"contact_id"`
	CompanyID *int64          `json:"company_id"`
	DealID    *int64          `json:"deal_id"`
	CreatedAt time.Time       `json:"created_at"`
}

type ActivityFilter struct {
	ContactID int64
	CompanyID int64
	DealID    int64
	Limit     int
}

func ListActivities(db *sql.DB, f ActivityFilter) ([]Activity, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}

	where := []string{}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.ContactID > 0 {
		add("a.contact_id = $%d", f.ContactID)
	}
	if f.CompanyID > 0 {
		add("a.company_id = $%d", f.CompanyID)
	}
	if f.DealID > 0 {
		add("a.deal_id = $%d", f.DealID)
	}
	cond := "1=1"
	if len(where) > 0 {
		cond = strings.Join(where, " OR ")
	}

	args = append(args, f.Limit)
	rows, err := db.Query(fmt.Sprintf(`
		SELECT a.id, a.kind, a.content, a.metadata, a.user_id, COALESCE(u.name,''),
		       a.contact_id, a.company_id, a.deal_id, a.created_at
		FROM activities a
		LEFT JOIN users u ON u.id = a.user_id
		WHERE %s
		ORDER BY a.created_at DESC
		LIMIT $%d`, cond, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Activity{}
	for rows.Next() {
		var a Activity
		var metadata sql.NullString
		if err := rows.Scan(&a.ID, &a.Kind, &a.Content, &metadata, &a.UserID, &a.UserName,
			&a.ContactID, &a.CompanyID, &a.DealID, &a.CreatedAt); err != nil {
			return nil, err
		}
		if metadata.Valid {
			a.Metadata = json.RawMessage(metadata.String)
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func CreateActivity(db *sql.DB, a *Activity) error {
	var metadata any
	if len(a.Metadata) > 0 {
		metadata = string(a.Metadata)
	}
	return db.QueryRow(`
		INSERT INTO activities (kind, content, metadata, user_id, contact_id, company_id, deal_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		a.Kind, a.Content, metadata, a.UserID, a.ContactID, a.CompanyID, a.DealID,
	).Scan(&a.ID, &a.CreatedAt)
}

func DeleteActivity(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM activities WHERE id = $1`, id)
	return err
}
