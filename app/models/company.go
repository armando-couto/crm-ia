package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
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

	// Campos de negócio (cadastro comercial).
	ECNumber         string     `json:"ec_number"`
	EconomicGroup    string     `json:"economic_group"`
	CNPJ             string     `json:"cnpj"`
	AccreditedAt     *time.Time `json:"accredited_at"`
	Representative   string     `json:"representative"`
	Instagram        string     `json:"instagram"`
	Products         []string   `json:"products"`
	MachinesCount    int        `json:"machines_count"`
	IsClient         bool       `json:"is_client"`
	AnticipationMode string     `json:"anticipation_mode"`
	Validator        bool       `json:"validator"`
	DoNotDisturb     bool       `json:"do_not_disturb"`

	// Conta-alvo: empresa que a equipe escolheu perseguir (tier 1 = prioridade).
	IsTarget    bool       `json:"is_target"`
	TargetTier  int        `json:"target_tier"`
	TargetNotes string     `json:"target_notes"`
	TargetSince *time.Time `json:"target_since"`
}

// companyBusinessColumns são as colunas extras lidas em todas as consultas.
const companyBusinessColumns = `
	COALESCE(c.ec_number,''), COALESCE(c.economic_group,''), COALESCE(c.cnpj,''),
	c.accredited_at, COALESCE(c.representative,''), COALESCE(c.instagram,''),
	c.products, c.machines_count, c.is_client, COALESCE(c.anticipation_mode,''),
	c.validator, c.do_not_disturb,
	c.is_target, c.target_tier, COALESCE(c.target_notes,''), c.target_since`

// businessScanTargets devolve os destinos de scan das colunas de negócio.
func (c *Company) businessScanTargets(products *pq.StringArray) []any {
	return []any{
		&c.ECNumber, &c.EconomicGroup, &c.CNPJ, &c.AccreditedAt, &c.Representative,
		&c.Instagram, products, &c.MachinesCount, &c.IsClient, &c.AnticipationMode,
		&c.Validator, &c.DoNotDisturb,
		&c.IsTarget, &c.TargetTier, &c.TargetNotes, &c.TargetSince,
	}
}

type CompanyFilter struct {
	Search      string
	OwnerID     int64
	Industry    string
	Unassigned  bool // sem dono
	CreatedDays int  // criadas nos últimos N dias
	SortBy      string
	SortDir     string
	Advanced    *AdvancedFilters
	Pagination
}

// companyOrderBy valida a ordenação contra uma lista fechada de colunas.
func companyOrderBy(sortBy, sortDir string) string {
	column := map[string]string{
		"name":       "LOWER(c.name)",
		"created_at": "c.created_at",
		"updated_at": "c.updated_at",
		"contacts":   "contacts_count",
	}[sortBy]
	if column == "" {
		column = "LOWER(c.name)"
	}
	dir := "DESC"
	if sortBy == "" || strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	return column + " " + dir + " NULLS LAST"
}

func ListCompanies(db *sql.DB, f CompanyFilter) ([]Company, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}
	if f.Search != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		where = append(where, fmt.Sprintf(
			"(LOWER(c.name) LIKE $%d OR LOWER(COALESCE(c.domain,'')) LIKE $%d OR COALESCE(c.cnpj,'') LIKE $%d OR COALESCE(c.ec_number,'') LIKE $%d)",
			len(args), len(args), len(args), len(args)))
	}
	if f.OwnerID > 0 {
		args = append(args, f.OwnerID)
		where = append(where, fmt.Sprintf("c.owner_id = $%d", len(args)))
	}
	if f.Industry != "" {
		args = append(args, strings.ToLower(f.Industry))
		where = append(where, fmt.Sprintf("LOWER(COALESCE(c.industry,'')) = $%d", len(args)))
	}
	if f.Unassigned {
		where = append(where, "c.owner_id IS NULL")
	}
	if f.CreatedDays > 0 {
		args = append(args, f.CreatedDays)
		where = append(where, fmt.Sprintf("c.created_at >= NOW() - make_interval(days => $%d)", len(args)))
	}
	if adv := BuildAdvancedWhere(f.Advanced, CompanyFilterFieldsSpec, &args); adv != "" {
		where = append(where, adv)
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
		       (SELECT COUNT(*) FROM contacts ct WHERE ct.company_id = c.id) AS contacts_count,
		       c.created_at, c.updated_at, `+companyBusinessColumns+`
		FROM companies c
		LEFT JOIN users u ON u.id = c.owner_id
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`, cond, companyOrderBy(f.SortBy, f.SortDir), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Company{}
	for rows.Next() {
		var c Company
		var products pq.StringArray
		targets := append([]any{&c.ID, &c.Name, &c.Domain, &c.Phone, &c.Industry, &c.City, &c.State,
			&c.OwnerID, &c.OwnerName, &c.Contacts, &c.CreatedAt, &c.UpdatedAt}, c.businessScanTargets(&products)...)
		if err := rows.Scan(targets...); err != nil {
			return nil, 0, err
		}
		c.Products = products
		if c.Products == nil {
			c.Products = []string{}
		}
		list = append(list, c)
	}
	return list, total, rows.Err()
}

func CompanyByID(db *sql.DB, id int64) (*Company, error) {
	var c Company
	var products pq.StringArray
	targets := append([]any{&c.ID, &c.Name, &c.Domain, &c.Phone, &c.Industry, &c.City, &c.State,
		&c.OwnerID, &c.OwnerName, &c.CreatedAt, &c.UpdatedAt}, c.businessScanTargets(&products)...)
	err := db.QueryRow(`
		SELECT c.id, c.name, COALESCE(c.domain,''), COALESCE(c.phone,''), COALESCE(c.industry,''),
		       COALESCE(c.city,''), COALESCE(c.state,''), c.owner_id, COALESCE(u.name,''), c.created_at, c.updated_at, `+
		companyBusinessColumns+`
		FROM companies c
		LEFT JOIN users u ON u.id = c.owner_id
		WHERE c.id = $1`, id,
	).Scan(targets...)
	if err != nil {
		return nil, err
	}
	c.Products = products
	if c.Products == nil {
		c.Products = []string{}
	}
	return &c, nil
}

func CreateCompany(db *sql.DB, c *Company) error {
	if c.Products == nil {
		c.Products = []string{}
	}
	return db.QueryRow(`
		INSERT INTO companies (name, domain, phone, industry, city, state, owner_id,
		    ec_number, economic_group, cnpj, accredited_at, representative, instagram,
		    products, machines_count, is_client, anticipation_mode, validator, do_not_disturb)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id, created_at, updated_at`,
		c.Name, c.Domain, c.Phone, c.Industry, c.City, c.State, c.OwnerID,
		c.ECNumber, c.EconomicGroup, c.CNPJ, c.AccreditedAt, c.Representative, c.Instagram,
		pq.Array(c.Products), c.MachinesCount, c.IsClient, c.AnticipationMode, c.Validator, c.DoNotDisturb,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func UpdateCompany(db *sql.DB, c *Company) error {
	if c.Products == nil {
		c.Products = []string{}
	}
	_, err := db.Exec(`
		UPDATE companies SET name = $1, domain = $2, phone = $3, industry = $4,
		       city = $5, state = $6, owner_id = $7,
		       ec_number = $8, economic_group = $9, cnpj = $10, accredited_at = $11,
		       representative = $12, instagram = $13, products = $14, machines_count = $15,
		       is_client = $16, anticipation_mode = $17, validator = $18, do_not_disturb = $19,
		       updated_at = NOW()
		WHERE id = $20`,
		c.Name, c.Domain, c.Phone, c.Industry, c.City, c.State, c.OwnerID,
		c.ECNumber, c.EconomicGroup, c.CNPJ, c.AccreditedAt, c.Representative, c.Instagram,
		pq.Array(c.Products), c.MachinesCount, c.IsClient, c.AnticipationMode, c.Validator, c.DoNotDisturb, c.ID)
	return err
}

func DeleteCompany(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM companies WHERE id = $1`, id)
	return err
}
