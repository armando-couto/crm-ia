package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Date é uma data sem hora (coluna DATE) que vai ao JSON como "AAAA-MM-DD".
type Date struct{ time.Time }

// ParseDate aceita "AAAA-MM-DD" (ou RFC 3339); vazio devolve nil.
func ParseDate(s string) (*Date, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if len(s) > 10 {
		s = s[:10]
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	return &Date{t}, nil
}

func (d Date) String() string { return d.Format("2006-01-02") }

func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	p, err := ParseDate(s)
	if err != nil {
		return err
	}
	if p == nil {
		*d = Date{}
		return nil
	}
	*d = *p
	return nil
}

func (d *Date) Scan(v any) error {
	switch t := v.(type) {
	case nil:
		*d = Date{}
	case time.Time:
		*d = Date{t}
	case string:
		p, err := ParseDate(t)
		if err != nil {
			return err
		}
		if p != nil {
			*d = *p
		}
	case []byte:
		return d.Scan(string(t))
	default:
		return fmt.Errorf("não é possível ler data de %T", v)
	}
	return nil
}

func (d Date) Value() (driver.Value, error) { return d.Time, nil }

// JSONB é um documento JSON lido/gravado como está (coluna JSONB). Vazio vira {}.
type JSONB []byte

func (j JSONB) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("{}"), nil
	}
	return []byte(j), nil
}

func (j *JSONB) UnmarshalJSON(b []byte) error {
	*j = append(JSONB(nil), b...)
	return nil
}

func (j *JSONB) Scan(v any) error {
	switch t := v.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append(JSONB(nil), t...)
	case string:
		*j = JSONB(t)
	default:
		return fmt.Errorf("não é possível ler JSON de %T", v)
	}
	return nil
}

func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "{}", nil
	}
	return string(j), nil
}

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

	// Dados cadastrais (Receita Federal e provedores de enriquecimento).
	LegalName            string     `json:"legal_name"`
	TradeName            string     `json:"trade_name"`
	CNAECode             string     `json:"cnae_code"`
	CNAEDescription      string     `json:"cnae_description"`
	LegalNature          string     `json:"legal_nature"`
	FoundedAt            *Date      `json:"founded_at"`
	Capital              *float64   `json:"capital"`
	CompanySize          string     `json:"company_size"`
	RegistrationStatus   string     `json:"registration_status"`
	RegistrationStatusAt *Date      `json:"registration_status_at"`
	Simples              *bool      `json:"simples"`
	MEI                  *bool      `json:"mei"`
	Street               string     `json:"street"`
	StreetNumber         string     `json:"street_number"`
	Complement           string     `json:"complement"`
	Neighborhood         string     `json:"neighborhood"`
	Zip                  string     `json:"zip"`
	Email                string     `json:"email"`
	Website              string     `json:"website"`
	EmployeesRange       string     `json:"employees_range"`
	RevenueRange         string     `json:"revenue_range"`
	Intel                JSONB      `json:"intel"`
	EnrichedAt           *time.Time `json:"enriched_at"`
}

// companyBusinessColumns são as colunas extras lidas em todas as consultas.
const companyBusinessColumns = `
	COALESCE(c.ec_number,''), COALESCE(c.economic_group,''), COALESCE(c.cnpj,''),
	c.accredited_at, COALESCE(c.representative,''), COALESCE(c.instagram,''),
	c.products, c.machines_count, c.is_client, COALESCE(c.anticipation_mode,''),
	c.validator, c.do_not_disturb,
	c.is_target, c.target_tier, COALESCE(c.target_notes,''), c.target_since,
	COALESCE(c.legal_name,''), COALESCE(c.trade_name,''), COALESCE(c.cnae_code,''),
	COALESCE(c.cnae_description,''), COALESCE(c.legal_nature,''), c.founded_at, c.capital,
	COALESCE(c.company_size,''), COALESCE(c.registration_status,''), c.registration_status_at,
	c.simples, c.mei, COALESCE(c.street,''), COALESCE(c.street_number,''), COALESCE(c.complement,''),
	COALESCE(c.neighborhood,''), COALESCE(c.zip,''), COALESCE(c.email,''), COALESCE(c.website,''),
	COALESCE(c.employees_range,''), COALESCE(c.revenue_range,''), c.intel, c.enriched_at`

// businessScanTargets devolve os destinos de scan das colunas de negócio.
func (c *Company) businessScanTargets(products *pq.StringArray) []any {
	return []any{
		&c.ECNumber, &c.EconomicGroup, &c.CNPJ, &c.AccreditedAt, &c.Representative,
		&c.Instagram, products, &c.MachinesCount, &c.IsClient, &c.AnticipationMode,
		&c.Validator, &c.DoNotDisturb,
		&c.IsTarget, &c.TargetTier, &c.TargetNotes, &c.TargetSince,
		&c.LegalName, &c.TradeName, &c.CNAECode, &c.CNAEDescription, &c.LegalNature,
		&c.FoundedAt, &c.Capital, &c.CompanySize, &c.RegistrationStatus, &c.RegistrationStatusAt,
		&c.Simples, &c.MEI, &c.Street, &c.StreetNumber, &c.Complement, &c.Neighborhood,
		&c.Zip, &c.Email, &c.Website, &c.EmployeesRange, &c.RevenueRange, &c.Intel, &c.EnrichedAt,
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

// companySelect é a consulta de uma empresa completa; where usa o alias c.
const companySelect = `
		SELECT c.id, c.name, COALESCE(c.domain,''), COALESCE(c.phone,''), COALESCE(c.industry,''),
		       COALESCE(c.city,''), COALESCE(c.state,''), c.owner_id, COALESCE(u.name,''), c.created_at, c.updated_at, ` +
	companyBusinessColumns + `
		FROM companies c
		LEFT JOIN users u ON u.id = c.owner_id
		WHERE `

func scanCompany(row *sql.Row) (*Company, error) {
	var c Company
	var products pq.StringArray
	targets := append([]any{&c.ID, &c.Name, &c.Domain, &c.Phone, &c.Industry, &c.City, &c.State,
		&c.OwnerID, &c.OwnerName, &c.CreatedAt, &c.UpdatedAt}, c.businessScanTargets(&products)...)
	if err := row.Scan(targets...); err != nil {
		return nil, err
	}
	c.Products = products
	if c.Products == nil {
		c.Products = []string{}
	}
	return &c, nil
}

func CompanyByID(db *sql.DB, id int64) (*Company, error) {
	return scanCompany(db.QueryRow(companySelect+`c.id = $1`, id))
}

// CompanyByCNPJ localiza a empresa pelos 14 dígitos do CNPJ, ignorando a
// pontuação gravada. sql.ErrNoRows quando não existe.
func CompanyByCNPJ(db *sql.DB, digits string) (*Company, error) {
	digits = SomenteDigitos(digits)
	if digits == "" {
		return nil, sql.ErrNoRows
	}
	return scanCompany(db.QueryRow(companySelect+`regexp_replace(COALESCE(c.cnpj,''), '\D', '', 'g') = $1`, digits))
}

// SomenteDigitos remove tudo que não é número.
func SomenteDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// enrichedScanArgs são os valores das colunas cadastrais, na ordem de enrichedColumns.
var enrichedColumns = []string{
	"legal_name", "trade_name", "cnae_code", "cnae_description", "legal_nature", "founded_at", "capital",
	"company_size", "registration_status", "registration_status_at", "simples", "mei", "street",
	"street_number", "complement", "neighborhood", "zip", "email", "website", "employees_range", "revenue_range",
}

func (c *Company) enrichedArgs() []any {
	var founded, statusAt any
	if c.FoundedAt != nil {
		founded = c.FoundedAt.Time
	}
	if c.RegistrationStatusAt != nil {
		statusAt = c.RegistrationStatusAt.Time
	}
	return []any{
		c.LegalName, c.TradeName, c.CNAECode, c.CNAEDescription, c.LegalNature, founded, c.Capital,
		c.CompanySize, c.RegistrationStatus, statusAt, c.Simples, c.MEI, c.Street,
		c.StreetNumber, c.Complement, c.Neighborhood, c.Zip, c.Email, c.Website, c.EmployeesRange, c.RevenueRange,
	}
}

// placeholders devolve "$de, $de+1, ..." com n posições.
func placeholders(de, n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("$%d", de+i)
	}
	return strings.Join(parts, ", ")
}

func CreateCompany(db *sql.DB, c *Company) error {
	if c.Products == nil {
		c.Products = []string{}
	}
	args := []any{
		c.Name, c.Domain, c.Phone, c.Industry, c.City, c.State, c.OwnerID,
		c.ECNumber, c.EconomicGroup, c.CNPJ, c.AccreditedAt, c.Representative, c.Instagram,
		pq.Array(c.Products), c.MachinesCount, c.IsClient, c.AnticipationMode, c.Validator, c.DoNotDisturb,
	}
	args = append(args, c.enrichedArgs()...)
	return db.QueryRow(`
		INSERT INTO companies (name, domain, phone, industry, city, state, owner_id,
		    ec_number, economic_group, cnpj, accredited_at, representative, instagram,
		    products, machines_count, is_client, anticipation_mode, validator, do_not_disturb,
		    `+strings.Join(enrichedColumns, ", ")+`)
		VALUES (`+placeholders(1, len(args))+`)
		RETURNING id, created_at, updated_at`, args...,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func UpdateCompany(db *sql.DB, c *Company) error {
	if c.Products == nil {
		c.Products = []string{}
	}
	args := []any{
		c.Name, c.Domain, c.Phone, c.Industry, c.City, c.State, c.OwnerID,
		c.ECNumber, c.EconomicGroup, c.CNPJ, c.AccreditedAt, c.Representative, c.Instagram,
		pq.Array(c.Products), c.MachinesCount, c.IsClient, c.AnticipationMode, c.Validator, c.DoNotDisturb,
	}
	args = append(args, c.enrichedArgs()...)
	sets := make([]string, 0, len(enrichedColumns))
	for i, col := range enrichedColumns {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, 20+i))
	}
	args = append(args, c.ID)
	_, err := db.Exec(`
		UPDATE companies SET name = $1, domain = $2, phone = $3, industry = $4,
		       city = $5, state = $6, owner_id = $7,
		       ec_number = $8, economic_group = $9, cnpj = $10, accredited_at = $11,
		       representative = $12, instagram = $13, products = $14, machines_count = $15,
		       is_client = $16, anticipation_mode = $17, validator = $18, do_not_disturb = $19,
		       `+strings.Join(sets, ", ")+`,
		       updated_at = NOW()
		WHERE id = $`+fmt.Sprint(len(args)), args...)
	return err
}

// EnrichableColumns são as colunas que o enriquecimento pode preencher.
var EnrichableColumns = map[string]bool{
	"name": true, "industry": true, "domain": true, "phone": true, "city": true, "state": true,
	"legal_name": true, "trade_name": true, "cnae_code": true, "cnae_description": true,
	"legal_nature": true, "founded_at": true, "capital": true, "company_size": true,
	"registration_status": true, "registration_status_at": true, "simples": true, "mei": true,
	"street": true, "street_number": true, "complement": true, "neighborhood": true, "zip": true,
	"email": true, "website": true, "employees_range": true, "revenue_range": true,
}

// ApplyEnrichedFields grava só as colunas da lista branca; chaves desconhecidas
// são ignoradas. Datas chegam como "AAAA-MM-DD" e viram DATE.
func ApplyEnrichedFields(db *sql.DB, companyID int64, campos map[string]any) error {
	sets := []string{}
	args := []any{}
	for _, col := range EnrichableColumnsOrdenadas() {
		v, ok := campos[col]
		if !ok {
			continue
		}
		switch col {
		case "founded_at", "registration_status_at":
			s, _ := v.(string)
			d, err := ParseDate(s)
			if err != nil {
				return fmt.Errorf("%s: data inválida", col)
			}
			if d == nil {
				v = nil
			} else {
				v = d.Time
			}
		case "capital":
			if f, ok := v.(float64); ok {
				v = f
			} else {
				v = nil
			}
		case "simples", "mei":
			if b, ok := v.(bool); ok {
				v = b
			} else {
				v = nil
			}
		default:
			if s, ok := v.(string); ok {
				v = strings.TrimSpace(s)
			} else {
				v = fmt.Sprint(v)
			}
		}
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, companyID)
	_, err := db.Exec(`UPDATE companies SET `+strings.Join(sets, ", ")+
		`, updated_at = NOW() WHERE id = $`+fmt.Sprint(len(args)), args...)
	return err
}

// EnrichableColumnsOrdenadas devolve a lista branca em ordem estável.
func EnrichableColumnsOrdenadas() []string {
	cols := []string{"name", "industry", "domain", "phone", "city", "state"}
	return append(cols, enrichedColumns...)
}

// SetCompanyIntel grava o perfil consolidado e a data do enriquecimento.
func SetCompanyIntel(db *sql.DB, companyID int64, intel []byte) error {
	if len(intel) == 0 {
		intel = []byte("{}")
	}
	_, err := db.Exec(`UPDATE companies SET intel = $1, enriched_at = NOW(), updated_at = NOW() WHERE id = $2`,
		string(intel), companyID)
	return err
}

func DeleteCompany(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM companies WHERE id = $1`, id)
	return err
}
