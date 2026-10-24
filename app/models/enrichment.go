package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// -----------------------------------------------------------------------------
// Histórico de consultas de enriquecimento e a fonte de cada campo aplicado.
// -----------------------------------------------------------------------------

// Status de uma consulta.
const (
	EnrichmentOK           = "ok"
	EnrichmentSemResultado = "sem_resultado"
	EnrichmentErro         = "erro"
)

// Enrichment é uma linha de company_enrichments.
type Enrichment struct {
	ID              int64           `json:"id"`
	CompanyID       int64           `json:"company_id"`
	Provedor        string          `json:"provedor"`
	Status          string          `json:"status"`
	Mensagem        string          `json:"mensagem"`
	Datasets        string          `json:"datasets"`
	Demo            bool            `json:"demo"`
	CustoCentavos   int             `json:"custo_centavos"`
	Perfil          json.RawMessage `json:"perfil"`
	Bruto           json.RawMessage `json:"bruto,omitempty"`
	AplicadoEm      *time.Time      `json:"aplicado_em"`
	CamposAplicados []string        `json:"campos_aplicados"`
	UserID          *int64          `json:"user_id"`
	UserName        string          `json:"user_name,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

const enrichmentColumns = `e.id, e.company_id, e.provedor, e.status, COALESCE(e.mensagem,''), COALESCE(e.datasets,''),
	e.demo, e.custo_centavos, e.perfil, e.bruto, e.aplicado_em, e.campos_aplicados, e.user_id, COALESCE(u.name,''), e.created_at`

func scanEnrichment(rows interface{ Scan(dest ...any) error }) (*Enrichment, error) {
	var e Enrichment
	var perfil, bruto []byte
	var campos pq.StringArray
	if err := rows.Scan(&e.ID, &e.CompanyID, &e.Provedor, &e.Status, &e.Mensagem, &e.Datasets,
		&e.Demo, &e.CustoCentavos, &perfil, &bruto, &e.AplicadoEm, &campos, &e.UserID, &e.UserName, &e.CreatedAt); err != nil {
		return nil, err
	}
	if len(perfil) == 0 {
		perfil = []byte("{}")
	}
	e.Perfil = json.RawMessage(perfil)
	if len(bruto) > 0 {
		e.Bruto = json.RawMessage(bruto)
	}
	e.CamposAplicados = campos
	if e.CamposAplicados == nil {
		e.CamposAplicados = []string{}
	}
	return &e, nil
}

// InsertEnrichment grava uma consulta (aplicado_em NULL até o apply).
func InsertEnrichment(db *sql.DB, e *Enrichment) error {
	if len(e.Perfil) == 0 {
		e.Perfil = json.RawMessage("{}")
	}
	var bruto any
	if len(e.Bruto) > 0 {
		bruto = string(e.Bruto)
	}
	var mensagem, datasets any
	if e.Mensagem != "" {
		mensagem = e.Mensagem
	}
	if e.Datasets != "" {
		datasets = e.Datasets
	}
	return db.QueryRow(`
		INSERT INTO company_enrichments (company_id, provedor, status, mensagem, datasets, demo,
		    custo_centavos, perfil, bruto, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`,
		e.CompanyID, e.Provedor, e.Status, mensagem, datasets, e.Demo,
		e.CustoCentavos, string(e.Perfil), bruto, e.UserID,
	).Scan(&e.ID, &e.CreatedAt)
}

// EnrichmentsByIDs devolve as consultas pedidas que pertencem à empresa.
func EnrichmentsByIDs(db *sql.DB, companyID int64, ids []int64) ([]Enrichment, error) {
	if len(ids) == 0 {
		return []Enrichment{}, nil
	}
	rows, err := db.Query(`
		SELECT `+enrichmentColumns+`
		FROM company_enrichments e
		LEFT JOIN users u ON u.id = e.user_id
		WHERE e.company_id = $1 AND e.id = ANY($2)
		ORDER BY e.created_at`, companyID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Enrichment{}
	for rows.Next() {
		e, err := scanEnrichment(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *e)
	}
	return list, rows.Err()
}

// LastOkEnrichment devolve a última consulta ok do provedor para a empresa
// com menos de maxAge (0 = qualquer idade); nil quando não há.
func LastOkEnrichment(db *sql.DB, companyID int64, provedor string, maxAge time.Duration) (*Enrichment, error) {
	limite := time.Time{}
	if maxAge > 0 {
		limite = time.Now().Add(-maxAge)
	}
	e, err := scanEnrichment(db.QueryRow(`
		SELECT `+enrichmentColumns+`
		FROM company_enrichments e
		LEFT JOIN users u ON u.id = e.user_id
		WHERE e.company_id = $1 AND e.provedor = $2 AND e.status = 'ok' AND NOT e.demo
		  AND e.created_at >= $3
		ORDER BY e.created_at DESC
		LIMIT 1`, companyID, provedor, limite))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

// LastOkEnrichmentsPerProvider devolve a última consulta ok de cada provedor.
func LastOkEnrichmentsPerProvider(db *sql.DB, companyID int64) ([]Enrichment, error) {
	rows, err := db.Query(`
		SELECT DISTINCT ON (e.provedor) `+enrichmentColumns+`
		FROM company_enrichments e
		LEFT JOIN users u ON u.id = e.user_id
		WHERE e.company_id = $1 AND e.status = 'ok'
		ORDER BY e.provedor, e.created_at DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Enrichment{}
	for rows.Next() {
		e, err := scanEnrichment(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *e)
	}
	return list, rows.Err()
}

// MarkEnrichmentsApplied marca as consultas como aplicadas com os campos usados.
func MarkEnrichmentsApplied(db *sql.DB, ids []int64, campos []string) error {
	if len(ids) == 0 {
		return nil
	}
	if campos == nil {
		campos = []string{}
	}
	_, err := db.Exec(`
		UPDATE company_enrichments SET aplicado_em = NOW(), campos_aplicados = $2
		WHERE id = ANY($1)`, pq.Array(ids), pq.Array(campos))
	return err
}

// EnrichmentHistoryItem é a linha do histórico mostrada na ficha.
type EnrichmentHistoryItem struct {
	ID            int64      `json:"id"`
	Provedor      string     `json:"provedor"`
	Status        string     `json:"status"`
	Mensagem      string     `json:"mensagem,omitempty"`
	Demo          bool       `json:"demo"`
	CustoCentavos int        `json:"custo_centavos"`
	Datasets      string     `json:"datasets"`
	UserName      string     `json:"user_name"`
	CreatedAt     time.Time  `json:"created_at"`
	AplicadoEm    *time.Time `json:"aplicado_em"`
}

// ListEnrichmentHistory devolve as últimas consultas da empresa.
func ListEnrichmentHistory(db *sql.DB, companyID int64, limit int) ([]EnrichmentHistoryItem, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT e.id, e.provedor, e.status, COALESCE(e.mensagem,''), e.demo, e.custo_centavos,
		       COALESCE(e.datasets,''), COALESCE(u.name,''), e.created_at, e.aplicado_em
		FROM company_enrichments e
		LEFT JOIN users u ON u.id = e.user_id
		WHERE e.company_id = $1
		ORDER BY e.created_at DESC
		LIMIT $2`, companyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []EnrichmentHistoryItem{}
	for rows.Next() {
		var h EnrichmentHistoryItem
		if err := rows.Scan(&h.ID, &h.Provedor, &h.Status, &h.Mensagem, &h.Demo, &h.CustoCentavos,
			&h.Datasets, &h.UserName, &h.CreatedAt, &h.AplicadoEm); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, rows.Err()
}

// FieldSource é a origem de um campo aplicado.
type FieldSource struct {
	Campo        string    `json:"campo,omitempty"`
	Provedor     string    `json:"provedor"`
	EnrichmentID *int64    `json:"enrichment_id,omitempty"`
	FetchedAt    time.Time `json:"fetched_at"`
}

// UpsertFieldSources grava a origem de vários campos de uma vez.
func UpsertFieldSources(db *sql.DB, companyID int64, fontes []FieldSource) error {
	if len(fontes) == 0 {
		return nil
	}
	values := []string{}
	args := []any{companyID}
	for _, f := range fontes {
		args = append(args, f.Campo, f.Provedor, f.EnrichmentID)
		n := len(args)
		values = append(values, fmt.Sprintf("($1, $%d, $%d, $%d, NOW())", n-2, n-1, n))
	}
	_, err := db.Exec(`
		INSERT INTO company_field_sources (company_id, campo, provedor, enrichment_id, fetched_at)
		VALUES `+strings.Join(values, ", ")+`
		ON CONFLICT (company_id, campo) DO UPDATE SET provedor = EXCLUDED.provedor,
		    enrichment_id = EXCLUDED.enrichment_id, fetched_at = NOW()`, args...)
	return err
}

// FieldSourcesMap devolve campo → origem.
func FieldSourcesMap(db *sql.DB, companyID int64) (map[string]FieldSource, error) {
	rows, err := db.Query(`
		SELECT campo, provedor, enrichment_id, fetched_at
		FROM company_field_sources WHERE company_id = $1`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]FieldSource{}
	for rows.Next() {
		var f FieldSource
		var campo string
		if err := rows.Scan(&campo, &f.Provedor, &f.EnrichmentID, &f.FetchedAt); err != nil {
			return nil, err
		}
		m[campo] = f
	}
	return m, rows.Err()
}

// DeleteCompanyIntel apaga histórico, fontes e o perfil consolidado. Os
// campos já aplicados na ficha permanecem.
func DeleteCompanyIntel(db *sql.DB, companyID int64) error {
	if _, err := db.Exec(`DELETE FROM company_field_sources WHERE company_id = $1`, companyID); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM company_enrichments WHERE company_id = $1`, companyID); err != nil {
		return err
	}
	_, err := db.Exec(`UPDATE companies SET intel = '{}'::jsonb, enriched_at = NULL WHERE id = $1`, companyID)
	return err
}

// UsoProvedor é o consumo de uma fonte no mês.
type UsoProvedor struct {
	Consultas     int `json:"consultas"`
	CustoCentavos int `json:"custo_centavos"`
}

// UsoMes é o consumo do mês, total e por fonte.
type UsoMes struct {
	Mes            string                 `json:"mes"` // AAAA-MM
	Consultas      int                    `json:"consultas"`
	CustoCentavos  int                    `json:"custo_centavos"`
	LimiteCentavos int                    `json:"limite_centavos,omitempty"`
	PorProvedor    map[string]UsoProvedor `json:"por_provedor"`
}

// EnrichmentUsage soma as consultas (não demo) do mês indicado.
func EnrichmentUsage(db *sql.DB, mes time.Time) (UsoMes, error) {
	inicio := time.Date(mes.Year(), mes.Month(), 1, 0, 0, 0, 0, time.UTC)
	fim := inicio.AddDate(0, 1, 0)
	uso := UsoMes{Mes: inicio.Format("2006-01"), PorProvedor: map[string]UsoProvedor{}}
	rows, err := db.Query(`
		SELECT provedor, COUNT(*), COALESCE(SUM(custo_centavos),0)
		FROM company_enrichments
		WHERE created_at >= $1 AND created_at < $2 AND NOT demo
		GROUP BY provedor`, inicio, fim)
	if err != nil {
		return uso, err
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		var u UsoProvedor
		if err := rows.Scan(&p, &u.Consultas, &u.CustoCentavos); err != nil {
			return uso, err
		}
		uso.PorProvedor[p] = u
		uso.Consultas += u.Consultas
		uso.CustoCentavos += u.CustoCentavos
	}
	return uso, rows.Err()
}

// EnrichmentUsageMeses devolve o consumo dos últimos n meses (mais recente primeiro).
func EnrichmentUsageMeses(db *sql.DB, n int) ([]UsoMes, error) {
	if n <= 0 {
		n = 6
	}
	agora := time.Now()
	inicio := time.Date(agora.Year(), agora.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(n - 1), 0)
	rows, err := db.Query(`
		SELECT to_char(date_trunc('month', created_at), 'YYYY-MM') AS mes, provedor, COUNT(*), COALESCE(SUM(custo_centavos),0)
		FROM company_enrichments
		WHERE created_at >= $1 AND NOT demo
		GROUP BY 1, provedor
		ORDER BY 1 DESC`, inicio)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	porMes := map[string]*UsoMes{}
	ordem := []string{}
	for rows.Next() {
		var mes, p string
		var u UsoProvedor
		if err := rows.Scan(&mes, &p, &u.Consultas, &u.CustoCentavos); err != nil {
			return nil, err
		}
		m, ok := porMes[mes]
		if !ok {
			m = &UsoMes{Mes: mes, PorProvedor: map[string]UsoProvedor{}}
			porMes[mes] = m
			ordem = append(ordem, mes)
		}
		m.PorProvedor[p] = u
		m.Consultas += u.Consultas
		m.CustoCentavos += u.CustoCentavos
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	list := make([]UsoMes, 0, len(ordem))
	for _, mes := range ordem {
		list = append(list, *porMes[mes])
	}
	if len(list) == 0 {
		list = append(list, UsoMes{Mes: agora.Format("2006-01"), PorProvedor: map[string]UsoProvedor{}})
	}
	return list, nil
}
