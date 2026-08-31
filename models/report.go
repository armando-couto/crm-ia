package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// A montagem do relatório é feita só com peças desta lista fechada: nada do que
// o usuário digita entra na consulta, apenas as chaves escolhidas.

// reportEntity descreve uma entidade disponível no construtor.
type reportEntity struct {
	Label      string
	From       string // FROM + JOINs
	DateColumn string // coluna usada no filtro de período
	OwnerCol   string // coluna de dono (vazio quando não tem)
	Metrics    map[string]reportMetric
	Dimensions map[string]reportDimension
}

type reportMetric struct {
	Label string
	Expr  string
	Money bool
}

type reportDimension struct {
	Label string
	Expr  string
	// Order define a ordenação: por rótulo (padrão) ou pelo próprio valor.
	OrderByLabel bool
}

var reportEntities = map[string]reportEntity{
	"contatos": {
		Label:      "Contatos",
		From:       `FROM contacts c LEFT JOIN users u ON u.id = c.owner_id LEFT JOIN companies co ON co.id = c.company_id`,
		DateColumn: "c.created_at",
		OwnerCol:   "c.owner_id",
		Metrics: map[string]reportMetric{
			"contagem": {Label: "Quantidade", Expr: "COUNT(*)"},
		},
		Dimensions: map[string]reportDimension{
			"dono":        {Label: "Dono", Expr: "COALESCE(u.name, 'Sem dono')", OrderByLabel: true},
			"estagio":     {Label: "Estágio", Expr: "c.lifecycle_stage", OrderByLabel: true},
			"origem":      {Label: "Origem", Expr: "COALESCE(NULLIF(c.source, ''), 'Sem origem')", OrderByLabel: true},
			"empresa":     {Label: "Empresa", Expr: "COALESCE(co.name, 'Sem empresa')", OrderByLabel: true},
			"mes_criacao": {Label: "Mês de criação", Expr: "TO_CHAR(c.created_at, 'YYYY-MM')"},
			"dia_criacao": {Label: "Dia de criação", Expr: "TO_CHAR(c.created_at, 'YYYY-MM-DD')"},
		},
	},
	"negocios": {
		Label: "Negócios",
		From: `FROM deals d
			LEFT JOIN users u ON u.id = d.owner_id
			LEFT JOIN pipeline_stages s ON s.id = d.stage_id
			LEFT JOIN pipelines p ON p.id = d.pipeline_id`,
		DateColumn: "d.created_at",
		OwnerCol:   "d.owner_id",
		Metrics: map[string]reportMetric{
			"contagem":    {Label: "Quantidade", Expr: "COUNT(*)"},
			"soma_valor":  {Label: "Valor total", Expr: "COALESCE(SUM(d.amount), 0)", Money: true},
			"media_valor": {Label: "Ticket médio", Expr: "COALESCE(AVG(d.amount), 0)", Money: true},
		},
		Dimensions: map[string]reportDimension{
			"dono":           {Label: "Dono", Expr: "COALESCE(u.name, 'Sem dono')", OrderByLabel: true},
			"etapa":          {Label: "Etapa", Expr: "COALESCE(s.name, 'Sem etapa')", OrderByLabel: true},
			"pipeline":       {Label: "Pipeline", Expr: "COALESCE(p.name, 'Sem pipeline')", OrderByLabel: true},
			"status":         {Label: "Situação", Expr: "d.status", OrderByLabel: true},
			"temperatura":    {Label: "Temperatura", Expr: "COALESCE(NULLIF(d.temperature, ''), 'sem')", OrderByLabel: true},
			"mes_criacao":    {Label: "Mês de criação", Expr: "TO_CHAR(d.created_at, 'YYYY-MM')"},
			"mes_fechamento": {Label: "Mês de fechamento", Expr: "TO_CHAR(d.closed_at, 'YYYY-MM')"},
		},
	},
	"tarefas": {
		Label:      "Tarefas",
		From:       `FROM tasks t LEFT JOIN users u ON u.id = t.owner_id`,
		DateColumn: "t.created_at",
		OwnerCol:   "t.owner_id",
		Metrics: map[string]reportMetric{
			"contagem": {Label: "Quantidade", Expr: "COUNT(*)"},
		},
		Dimensions: map[string]reportDimension{
			"dono":       {Label: "Dono", Expr: "COALESCE(u.name, 'Sem dono')", OrderByLabel: true},
			"tipo":       {Label: "Tipo", Expr: "t.type", OrderByLabel: true},
			"prioridade": {Label: "Prioridade", Expr: "t.priority", OrderByLabel: true},
			"situacao": {Label: "Situação",
				Expr:         "CASE WHEN t.completed_at IS NOT NULL THEN 'concluída' WHEN t.due_date < NOW() THEN 'atrasada' ELSE 'pendente' END",
				OrderByLabel: true},
			"mes_criacao": {Label: "Mês de criação", Expr: "TO_CHAR(t.created_at, 'YYYY-MM')"},
		},
	},
	"tickets": {
		Label:      "Tickets",
		From:       `FROM tickets tk LEFT JOIN users u ON u.id = tk.owner_id`,
		DateColumn: "tk.created_at",
		OwnerCol:   "tk.owner_id",
		Metrics: map[string]reportMetric{
			"contagem": {Label: "Quantidade", Expr: "COUNT(*)"},
		},
		Dimensions: map[string]reportDimension{
			"dono":        {Label: "Dono", Expr: "COALESCE(u.name, 'Sem dono')", OrderByLabel: true},
			"status":      {Label: "Situação", Expr: "tk.status", OrderByLabel: true},
			"prioridade":  {Label: "Prioridade", Expr: "tk.priority", OrderByLabel: true},
			"mes_criacao": {Label: "Mês de criação", Expr: "TO_CHAR(tk.created_at, 'YYYY-MM')"},
		},
	},
	"empresas": {
		Label:      "Empresas",
		From:       `FROM companies c LEFT JOIN users u ON u.id = c.owner_id`,
		DateColumn: "c.created_at",
		OwnerCol:   "c.owner_id",
		Metrics: map[string]reportMetric{
			"contagem": {Label: "Quantidade", Expr: "COUNT(*)"},
		},
		Dimensions: map[string]reportDimension{
			"dono":        {Label: "Dono", Expr: "COALESCE(u.name, 'Sem dono')", OrderByLabel: true},
			"cidade":      {Label: "Cidade", Expr: "COALESCE(NULLIF(c.city, ''), 'Sem cidade')", OrderByLabel: true},
			"estado":      {Label: "Estado", Expr: "COALESCE(NULLIF(c.state, ''), 'Sem estado')", OrderByLabel: true},
			"cliente":     {Label: "É cliente", Expr: "CASE WHEN c.is_client THEN 'cliente' ELSE 'prospect' END", OrderByLabel: true},
			"mes_criacao": {Label: "Mês de criação", Expr: "TO_CHAR(c.created_at, 'YYYY-MM')"},
		},
	},
}

var reportCharts = []string{"barras", "linha", "pizza", "tabela"}

// ReportFilters são os filtros aceitos (todos opcionais).
type ReportFilters struct {
	Days    int    `json:"days"`
	OwnerID int64  `json:"owner_id"`
	Status  string `json:"status"`
}

type Report struct {
	ID          int64         `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Entity      string        `json:"entity"`
	Metric      string        `json:"metric"`
	Dimension   string        `json:"dimension"`
	Filters     ReportFilters `json:"filters"`
	Chart       string        `json:"chart"`
	Shared      bool          `json:"shared"`
	Position    int           `json:"position"`
	CreatedBy   *int64        `json:"created_by"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// ReportRow é uma linha do resultado.
type ReportRow struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

// ReportResult é o relatório executado.
type ReportResult struct {
	Rows        []ReportRow `json:"rows"`
	Total       float64     `json:"total"`
	MetricLabel string      `json:"metric_label"`
	IsMoney     bool        `json:"is_money"`
	Dimension   string      `json:"dimension_label"`
}

// ReportCatalogEntry descreve uma entidade para a tela de montagem.
type ReportCatalogEntry struct {
	Key        string            `json:"key"`
	Label      string            `json:"label"`
	Metrics    map[string]string `json:"metrics"`
	Dimensions map[string]string `json:"dimensions"`
}

// ReportCatalog devolve o que a tela pode oferecer ao montar o relatório.
func ReportCatalog() []ReportCatalogEntry {
	order := []string{"negocios", "contatos", "empresas", "tarefas", "tickets"}
	catalog := make([]ReportCatalogEntry, 0, len(order))
	for _, key := range order {
		def := reportEntities[key]
		entry := ReportCatalogEntry{
			Key:        key,
			Label:      def.Label,
			Metrics:    map[string]string{},
			Dimensions: map[string]string{},
		}
		for k, m := range def.Metrics {
			entry.Metrics[k] = m.Label
		}
		for k, d := range def.Dimensions {
			entry.Dimensions[k] = d.Label
		}
		catalog = append(catalog, entry)
	}
	return catalog
}

// ValidateReport confere se entidade, métrica, agrupamento e gráfico existem.
func ValidateReport(r *Report) error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("informe o nome do relatório")
	}
	def, ok := reportEntities[r.Entity]
	if !ok {
		return fmt.Errorf("entidade inválida")
	}
	if _, ok := def.Metrics[r.Metric]; !ok {
		return fmt.Errorf("métrica inválida para %s", def.Label)
	}
	if _, ok := def.Dimensions[r.Dimension]; !ok {
		return fmt.Errorf("agrupamento inválido para %s", def.Label)
	}
	if r.Chart == "" {
		r.Chart = "barras"
	}
	if !slices.Contains(reportCharts, r.Chart) {
		return fmt.Errorf("tipo de gráfico inválido")
	}
	if r.Filters.Days < 0 || r.Filters.Days > 3650 {
		return fmt.Errorf("período inválido")
	}
	return nil
}

// maxReportRows evita gráfico com centenas de fatias.
const maxReportRows = 50

// RunReport executa o relatório. A consulta é montada só com as expressões do
// catálogo; o que vem do usuário entra como parâmetro.
func RunReport(db *sql.DB, r *Report) (*ReportResult, error) {
	if err := ValidateReport(r); err != nil {
		return nil, err
	}
	def := reportEntities[r.Entity]
	metric := def.Metrics[r.Metric]
	dimension := def.Dimensions[r.Dimension]

	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if r.Filters.Days > 0 {
		add(def.DateColumn+" >= NOW() - make_interval(days => $%d)", r.Filters.Days)
	}
	if r.Filters.OwnerID > 0 && def.OwnerCol != "" {
		add(def.OwnerCol+" = $%d", r.Filters.OwnerID)
	}
	if status := strings.TrimSpace(r.Filters.Status); status != "" {
		switch r.Entity {
		case "negocios":
			add("d.status = $%d", status)
		case "tickets":
			add("tk.status = $%d", status)
		case "contatos":
			add("c.lifecycle_stage = $%d", status)
		}
	}

	// Agrupamento por mês/dia ordena pelo próprio valor (cronológico).
	orderBy := "valor DESC"
	if !dimension.OrderByLabel {
		orderBy = "rotulo ASC"
	}

	query := fmt.Sprintf(`
		SELECT %s AS rotulo, %s AS valor
		%s
		WHERE %s
		GROUP BY rotulo
		ORDER BY %s
		LIMIT %d`,
		dimension.Expr, metric.Expr, def.From, strings.Join(where, " AND "), orderBy, maxReportRows)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &ReportResult{
		Rows:        []ReportRow{},
		MetricLabel: metric.Label,
		IsMoney:     metric.Money,
		Dimension:   dimension.Label,
	}
	for rows.Next() {
		var label sql.NullString
		var value float64
		if err := rows.Scan(&label, &value); err != nil {
			return nil, err
		}
		text := label.String
		if !label.Valid || strings.TrimSpace(text) == "" {
			text = "(sem valor)"
		}
		result.Rows = append(result.Rows, ReportRow{Label: text, Value: value})
		// Média não se soma: o total só faz sentido em contagem e soma.
		if r.Metric != "media_valor" {
			result.Total += value
		}
	}
	return result, rows.Err()
}

const reportSelect = `
	SELECT id, name, description, entity, metric, dimension, filters, chart,
	       shared, position, created_by, created_at, updated_at
	FROM reports`

func scanReport(row interface{ Scan(...any) error }) (*Report, error) {
	var r Report
	var filters []byte
	err := row.Scan(&r.ID, &r.Name, &r.Description, &r.Entity, &r.Metric, &r.Dimension,
		&filters, &r.Chart, &r.Shared, &r.Position, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(filters) > 0 {
		if err := json.Unmarshal(filters, &r.Filters); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

// ListReports devolve os relatórios compartilhados mais os do próprio usuário.
func ListReports(db *sql.DB, userID int64) ([]Report, error) {
	rows, err := db.Query(reportSelect+`
		WHERE shared = TRUE OR created_by = $1
		ORDER BY position, created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Report{}
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *r)
	}
	return list, rows.Err()
}

func ReportByID(db *sql.DB, id int64) (*Report, error) {
	return scanReport(db.QueryRow(reportSelect+` WHERE id = $1`, id))
}

func CreateReport(db *sql.DB, r *Report) error {
	filters, err := json.Marshal(r.Filters)
	if err != nil {
		return err
	}
	return db.QueryRow(`
		INSERT INTO reports (name, description, entity, metric, dimension, filters, chart,
		    shared, position, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,
		    COALESCE((SELECT MAX(position) + 1 FROM reports), 0), $9)
		RETURNING id, position, created_at, updated_at`,
		r.Name, r.Description, r.Entity, r.Metric, r.Dimension, filters, r.Chart,
		r.Shared, r.CreatedBy,
	).Scan(&r.ID, &r.Position, &r.CreatedAt, &r.UpdatedAt)
}

func UpdateReport(db *sql.DB, r *Report) error {
	filters, err := json.Marshal(r.Filters)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		UPDATE reports SET name = $1, description = $2, entity = $3, metric = $4,
		    dimension = $5, filters = $6, chart = $7, shared = $8, updated_at = NOW()
		WHERE id = $9`,
		r.Name, r.Description, r.Entity, r.Metric, r.Dimension, filters, r.Chart,
		r.Shared, r.ID)
	return err
}

func DeleteReport(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM reports WHERE id = $1`, id)
	return err
}
