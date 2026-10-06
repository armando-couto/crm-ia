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
	TicketID  *int64          `json:"ticket_id"`
	CreatedAt time.Time       `json:"created_at"`

	// Preenchidos no feed geral, para mostrar a que registro a atividade pertence.
	ContactName string `json:"contact_name,omitempty"`
	CompanyName string `json:"company_name,omitempty"`
	DealName    string `json:"deal_name,omitempty"`
}

type ActivityFilter struct {
	ContactID int64
	CompanyID int64
	DealID    int64
	TicketID  int64
	UserID    int64  // quem registrou
	Kind      string // filtra por tipo (nota, email, ligacao, reuniao, sistema)
	Search    string // busca no conteúdo
	Days      int    // últimos N dias
	Limit     int
	// Feed geral: sem registro específico, lista a equipe inteira.
	Feed bool
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
	if f.TicketID > 0 {
		add("a.ticket_id = $%d", f.TicketID)
	}
	cond := "1=1"
	if len(where) > 0 {
		cond = "(" + strings.Join(where, " OR ") + ")"
	}
	if f.UserID > 0 {
		args = append(args, f.UserID)
		cond += fmt.Sprintf(" AND a.user_id = $%d", len(args))
	}
	if f.Days > 0 {
		args = append(args, f.Days)
		cond += fmt.Sprintf(" AND a.created_at >= NOW() - make_interval(days => $%d)", len(args))
	}
	if f.Kind != "" {
		args = append(args, f.Kind)
		cond += fmt.Sprintf(" AND a.kind = $%d", len(args))
	}
	if f.Search != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		cond += fmt.Sprintf(" AND LOWER(a.content) LIKE $%d", len(args))
	}

	args = append(args, f.Limit)
	rows, err := db.Query(fmt.Sprintf(`
		SELECT a.id, a.kind, a.content, a.metadata, a.user_id, COALESCE(u.name,''),
		       a.contact_id, a.company_id, a.deal_id, a.ticket_id, a.created_at,
		       COALESCE(TRIM(ct.first_name || ' ' || ct.last_name), ''),
		       COALESCE(co.name, ''), COALESCE(d.name, '')
		FROM activities a
		LEFT JOIN users u ON u.id = a.user_id
		LEFT JOIN contacts ct ON ct.id = a.contact_id
		LEFT JOIN companies co ON co.id = a.company_id
		LEFT JOIN deals d ON d.id = a.deal_id
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
			&a.ContactID, &a.CompanyID, &a.DealID, &a.TicketID, &a.CreatedAt,
			&a.ContactName, &a.CompanyName, &a.DealName); err != nil {
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
		INSERT INTO activities (kind, content, metadata, user_id, contact_id, company_id, deal_id, ticket_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`,
		a.Kind, a.Content, metadata, a.UserID, a.ContactID, a.CompanyID, a.DealID, a.TicketID,
	).Scan(&a.ID, &a.CreatedAt)
}

func DeleteActivity(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM activities WHERE id = $1`, id)
	return err
}
