package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

// SavedView é uma visualização salva: um conjunto de filtros com nome,
// exibido como aba nas telas de contatos e empresas.
type SavedView struct {
	ID            int64           `json:"id"`
	Entity        string          `json:"entity"` // contacts | companies
	Name          string          `json:"name"`
	Filters       json.RawMessage `json:"filters"`
	Position      int             `json:"position"`
	CreatedBy     *int64          `json:"created_by"`
	CreatedByName string          `json:"created_by_name,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

func ValidViewEntity(entity string) bool {
	return entity == "contacts" || entity == "companies" || entity == "deals"
}

// ListSavedViews lista as visualizações de uma entidade; com entity vazia,
// lista todas (central "Todas as visualizações").
func ListSavedViews(db *sql.DB, entity string) ([]SavedView, error) {
	query := `
		SELECT v.id, v.entity, v.name, v.filters, v.position, v.created_by, COALESCE(u.name,''), v.created_at
		FROM saved_views v
		LEFT JOIN users u ON u.id = v.created_by`
	args := []any{}
	if entity != "" {
		query += ` WHERE v.entity = $1`
		args = append(args, entity)
	}
	query += ` ORDER BY v.entity, v.position, v.id`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	views := []SavedView{}
	for rows.Next() {
		var v SavedView
		var filters string
		if err := rows.Scan(&v.ID, &v.Entity, &v.Name, &filters, &v.Position, &v.CreatedBy, &v.CreatedByName, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Filters = json.RawMessage(filters)
		views = append(views, v)
	}
	return views, rows.Err()
}

func CreateSavedView(db *sql.DB, v *SavedView) error {
	return db.QueryRow(`
		INSERT INTO saved_views (entity, name, filters, position, created_by)
		VALUES ($1, $2, $3,
		        COALESCE((SELECT MAX(position) + 1 FROM saved_views WHERE entity = $4), 0),
		        $5)
		RETURNING id, position, created_at`,
		v.Entity, v.Name, string(v.Filters), v.Entity, v.CreatedBy,
	).Scan(&v.ID, &v.Position, &v.CreatedAt)
}

func RenameSavedView(db *sql.DB, id int64, name string) error {
	_, err := db.Exec(`UPDATE saved_views SET name = $1 WHERE id = $2`, name, id)
	return err
}

func DeleteSavedView(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM saved_views WHERE id = $1`, id)
	return err
}
