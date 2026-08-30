package models

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

var LifecycleStages = []string{"lead", "mql", "sql", "oportunidade", "cliente", "perdido"}

func ValidLifecycleStage(s string) bool {
	return slices.Contains(LifecycleStages, s)
}

type Contact struct {
	ID             int64     `json:"id"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	JobTitle       string    `json:"job_title"`
	LifecycleStage string    `json:"lifecycle_stage"`
	Source         string    `json:"source"`
	CompanyID      *int64    `json:"company_id"`
	CompanyName    string    `json:"company_name,omitempty"`
	OwnerID        *int64    `json:"owner_id"`
	OwnerName      string    `json:"owner_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ContactFilter struct {
	Search         string
	OwnerID        int64
	CompanyID      int64
	LifecycleStage string
	Source         string
	Pagination
}

const contactSelect = `
	SELECT c.id, c.first_name, COALESCE(c.last_name,''), COALESCE(c.email,''), COALESCE(c.phone,''),
	       COALESCE(c.job_title,''), c.lifecycle_stage, COALESCE(c.source,''),
	       c.company_id, COALESCE(co.name,''), c.owner_id, COALESCE(u.name,''),
	       c.created_at, c.updated_at
	FROM contacts c
	LEFT JOIN companies co ON co.id = c.company_id
	LEFT JOIN users u ON u.id = c.owner_id`

func scanContact(row interface{ Scan(...any) error }) (*Contact, error) {
	var c Contact
	err := row.Scan(&c.ID, &c.FirstName, &c.LastName, &c.Email, &c.Phone, &c.JobTitle,
		&c.LifecycleStage, &c.Source, &c.CompanyID, &c.CompanyName, &c.OwnerID, &c.OwnerName,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func ListContacts(db *sql.DB, f ContactFilter) ([]Contact, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}
	if f.Search != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		where = append(where, fmt.Sprintf(
			"(LOWER(c.first_name || ' ' || COALESCE(c.last_name,'')) LIKE $%d OR LOWER(COALESCE(c.email,'')) LIKE $%d)",
			len(args), len(args)))
	}
	if f.OwnerID > 0 {
		args = append(args, f.OwnerID)
		where = append(where, fmt.Sprintf("c.owner_id = $%d", len(args)))
	}
	if f.CompanyID > 0 {
		args = append(args, f.CompanyID)
		where = append(where, fmt.Sprintf("c.company_id = $%d", len(args)))
	}
	if f.LifecycleStage != "" {
		args = append(args, f.LifecycleStage)
		where = append(where, fmt.Sprintf("c.lifecycle_stage = $%d", len(args)))
	}
	if f.Source != "" {
		args = append(args, f.Source)
		where = append(where, fmt.Sprintf("c.source = $%d", len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM contacts c WHERE `+cond, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(`%s WHERE %s ORDER BY c.updated_at DESC LIMIT $%d OFFSET $%d`,
		contactSelect, cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Contact{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *c)
	}
	return list, total, rows.Err()
}

func ContactByID(db *sql.DB, id int64) (*Contact, error) {
	row := db.QueryRow(contactSelect+` WHERE c.id = $1`, id)
	return scanContact(row)
}

func CreateContact(db *sql.DB, c *Contact) error {
	if c.LifecycleStage == "" {
		c.LifecycleStage = "lead"
	}
	return db.QueryRow(`
		INSERT INTO contacts (first_name, last_name, email, phone, job_title, lifecycle_stage, source, company_id, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`,
		c.FirstName, c.LastName, NormalizeEmail(c.Email), c.Phone, c.JobTitle,
		c.LifecycleStage, c.Source, c.CompanyID, c.OwnerID,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func UpdateContact(db *sql.DB, c *Contact) error {
	_, err := db.Exec(`
		UPDATE contacts SET first_name = $1, last_name = $2, email = $3, phone = $4, job_title = $5,
		       lifecycle_stage = $6, source = $7, company_id = $8, owner_id = $9, updated_at = NOW()
		WHERE id = $10`,
		c.FirstName, c.LastName, NormalizeEmail(c.Email), c.Phone, c.JobTitle,
		c.LifecycleStage, c.Source, c.CompanyID, c.OwnerID, c.ID)
	return err
}

func DeleteContact(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM contacts WHERE id = $1`, id)
	return err
}
