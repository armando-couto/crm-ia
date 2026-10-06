package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ImportRecord é uma linha do histórico de importações de arquivos.
type ImportRecord struct {
	ID              int64     `json:"id"`
	FileName        string    `json:"file_name"`
	Entity          string    `json:"entity"` // contatos | empresas
	Status          string    `json:"status"` // concluida | falhou
	TotalRows       int       `json:"total_rows"`
	NewRecords      int       `json:"new_records"`
	UpdatedRecords  int       `json:"updated_records"`
	NewAssociations int       `json:"new_associations"`
	ErrorCount      int       `json:"error_count"`
	Errors          []string  `json:"errors"`
	CreatedBy       *int64    `json:"created_by"`
	CreatedByName   string    `json:"created_by_name,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateImport grava o resultado de uma carga no histórico.
func CreateImport(db *sql.DB, r *ImportRecord) error {
	if r.Errors == nil {
		r.Errors = []string{}
	}
	errsJSON, err := json.Marshal(r.Errors)
	if err != nil {
		return err
	}
	return db.QueryRow(`
		INSERT INTO imports (file_name, entity, status, total_rows, new_records,
		                     updated_records, new_associations, error_count, errors, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`,
		r.FileName, r.Entity, r.Status, r.TotalRows, r.NewRecords,
		r.UpdatedRecords, r.NewAssociations, r.ErrorCount, errsJSON, r.CreatedBy,
	).Scan(&r.ID, &r.CreatedAt)
}

// ListImports devolve o histórico, das cargas mais recentes para as antigas.
func ListImports(db *sql.DB) ([]ImportRecord, error) {
	rows, err := db.Query(`
		SELECT i.id, i.file_name, i.entity, i.status, i.total_rows, i.new_records,
		       i.updated_records, i.new_associations, i.error_count, i.errors,
		       i.created_by, COALESCE(u.name,''), i.created_at
		FROM imports i
		LEFT JOIN users u ON u.id = i.created_by
		ORDER BY i.created_at DESC
		LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []ImportRecord{}
	for rows.Next() {
		var r ImportRecord
		var errsJSON []byte
		if err := rows.Scan(&r.ID, &r.FileName, &r.Entity, &r.Status, &r.TotalRows,
			&r.NewRecords, &r.UpdatedRecords, &r.NewAssociations, &r.ErrorCount,
			&errsJSON, &r.CreatedBy, &r.CreatedByName, &r.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(errsJSON, &r.Errors); err != nil {
			r.Errors = []string{}
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
