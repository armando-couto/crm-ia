package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Ações registradas na trilha de auditoria.
const (
	AuditLogin       = "login"
	AuditLoginFail   = "login_falha"
	AuditCreate      = "criar"
	AuditUpdate      = "editar"
	AuditDelete      = "excluir"
	AuditExport      = "exportar"
	AuditImport      = "importar"
	AuditBulk        = "acao_em_massa"
	AuditPermissions = "permissoes"
)

// AuditActions alimenta o filtro da tela de auditoria.
var AuditActions = []string{
	AuditLogin, AuditLoginFail, AuditCreate, AuditUpdate, AuditDelete,
	AuditExport, AuditImport, AuditBulk, AuditPermissions,
}

type AuditEntry struct {
	ID        int64           `json:"id"`
	UserID    *int64          `json:"user_id"`
	UserName  string          `json:"user_name"`
	Action    string          `json:"action"`
	Entity    string          `json:"entity"`
	EntityID  *int64          `json:"entity_id"`
	Summary   string          `json:"summary"`
	Details   json.RawMessage `json:"details,omitempty"`
	IP        string          `json:"ip"`
	CreatedAt time.Time       `json:"created_at"`
}

type AuditFilter struct {
	UserID int64
	Action string
	Entity string
	Search string
	Days   int
	Pagination
}

// RecordAudit grava uma entrada na trilha. Falhas são devolvidas ao chamador,
// que decide se apenas registra no log (a auditoria nunca bloqueia a operação).
func RecordAudit(db *sql.DB, e *AuditEntry) error {
	var details any
	if len(e.Details) > 0 {
		details = string(e.Details)
	}
	_, err := db.Exec(`
		INSERT INTO audit_log (user_id, user_name, action, entity, entity_id, summary, details, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.UserID, e.UserName, e.Action, e.Entity, e.EntityID, e.Summary, details, e.IP)
	return err
}

func ListAudit(db *sql.DB, f AuditFilter) ([]AuditEntry, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.UserID > 0 {
		add("user_id = $%d", f.UserID)
	}
	if f.Action != "" {
		add("action = $%d", f.Action)
	}
	if f.Entity != "" {
		add("entity = $%d", f.Entity)
	}
	if f.Search != "" {
		add("LOWER(summary) LIKE $%d", "%"+strings.ToLower(f.Search)+"%")
	}
	if f.Days > 0 {
		add("created_at >= NOW() - make_interval(days => $%d)", f.Days)
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(`
		SELECT id, user_id, user_name, action, entity, entity_id, summary, details, ip, created_at
		FROM audit_log WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var details sql.NullString
		if err := rows.Scan(&e.ID, &e.UserID, &e.UserName, &e.Action, &e.Entity, &e.EntityID,
			&e.Summary, &details, &e.IP, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		if details.Valid {
			e.Details = json.RawMessage(details.String)
		}
		list = append(list, e)
	}
	return list, total, rows.Err()
}
