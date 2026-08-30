package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Company struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Domain    string    `json:"domain"`
	Phone     string    `json:"phone"`
	Industry  string    `json:"industry"`
	City      string    `json:"city"`
	State     string    `json:"state"`
	OwnerID   *int64    `json:"owner_id"`
	OwnerName string    `json:"owner_name,omitempty"`
	Contacts  int       `json:"contacts_count,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CompanyFilter struct {
	Search  string
	OwnerID int64
	Pagination
}

func ListCompanies(db *sql.DB, f CompanyFilter) ([]Company, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}
	if f.Search != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		where = append(where, fmt.Sprintf("(LOWER(c.name) LIKE $%d OR LOWER(COALESCE(c.domain,'')) LIKE $%d)", len(args), len(args)))
	}
	if f.OwnerID > 0 {
		args = append(args, f.OwnerID)
		where = append(where, fmt.Sprintf("c.owner_id = $%d", len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM companies c WHERE `+cond, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(`
		SELECT c.id, c.name, COALESCE(c.domain,''), COALESCE(c.phone,''), COALESCE(c.industry,''),
		       COALESCE(c.city,''), COALESCE(c.state,''), c.owner_id, COALESCE(u.name,''),
		       (SELECT COUNT(*) FROM contacts ct WHERE ct.company_id = c.id),
		       c.created_at, c.updated_at
		FROM companies c
		LEFT JOIN users u ON u.id = c.owner_id
		WHERE %s
		ORDER BY c.name
		LIMIT $%d OFFSET $%d`, cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Company{}
	for rows.Next() {
		var c Company
		if err := rows.Scan(&c.ID, &c.Name, &c.Domain, &c.Phone, &c.Industry, &c.City, &c.State,
			&c.OwnerID, &c.OwnerName, &c.Contacts, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, c)
	}
	return list, total, rows.Err()
}

func CompanyByID(db *sql.DB, id int64) (*Company, error) {
	var c Company
	err := db.QueryRow(`
		SELECT c.id, c.name, COALESCE(c.domain,''), COALESCE(c.phone,''), COALESCE(c.industry,''),
		       COALESCE(c.city,''), COALESCE(c.state,''), c.owner_id, COALESCE(u.name,''), c.created_at, c.updated_at
		FROM companies c
		LEFT JOIN users u ON u.id = c.owner_id
		WHERE c.id = $1`, id,
	).Scan(&c.ID, &c.Name, &c.Domain, &c.Phone, &c.Industry, &c.City, &c.State,
		&c.OwnerID, &c.OwnerName, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func CreateCompany(db *sql.DB, c *Company) error {
	return db.QueryRow(`
		INSERT INTO companies (name, domain, phone, industry, city, state, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		c.Name, c.Domain, c.Phone, c.Industry, c.City, c.State, c.OwnerID,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func UpdateCompany(db *sql.DB, c *Company) error {
	_, err := db.Exec(`
		UPDATE companies SET name = $1, domain = $2, phone = $3, industry = $4,
		       city = $5, state = $6, owner_id = $7, updated_at = NOW()
		WHERE id = $8`,
		c.Name, c.Domain, c.Phone, c.Industry, c.City, c.State, c.OwnerID, c.ID)
	return err
}

func DeleteCompany(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM companies WHERE id = $1`, id)
	return err
}
