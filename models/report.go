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
	Days       int    `json:"days"`
	OwnerID    int64  `json:"owner_id"`
	Status     string `json:"status"`
	PipelineID int64  `json:"pipeline_id"`
}

type Report struct {
	ID          int64         `json:"id"`
	Kind        string        `json:"kind"`
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

// ReportRow é uma linha do resultado. Percent só é preenchido na conversão de
// funil (taxa contra a etapa anterior).
type ReportRow struct {
	Label   string  `json:"label"`
	Value   float64 `json:"value"`
	Percent float64 `json:"percent,omitempty"`
}

// ReportResult é o relatório executado.
type ReportResult struct {
	Rows []ReportRow `json:"rows"`
	// Séries mensais, usadas só pelo relatório de progresso.
	Series      []ReportSeries `json:"series,omitempty"`
	Total       float64        `json:"total"`
	MetricLabel string         `json:"metric_label"`
	IsMoney     bool           `json:"is_money"`
	Dimension   string         `json:"dimension_label"`
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
	if !ValidReportKind(r.Kind) {
		return fmt.Errorf("tipo de relatório inválido")
	}
	if r.Kind == "" {
		r.Kind = ReportKindAggregate
	}
	// As análises prontas são sempre de negócios e não usam métrica/agrupamento.
	if r.Kind != ReportKindAggregate {
		r.Entity = "negocios"
		if r.Filters.Days < 0 || r.Filters.Days > 3650 {
			return fmt.Errorf("período inválido")
		}
		if r.Chart == "" {
			r.Chart = "barras"
		}
		return nil
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
	if r.Kind != "" && r.Kind != ReportKindAggregate {
		return RunReportByKind(db, r)
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
	SELECT id, COALESCE(kind, 'agregado'), name, description, entity, metric, dimension,
	       filters, chart, shared, position, created_by, created_at, updated_at
	FROM reports`

func scanReport(row interface{ Scan(...any) error }) (*Report, error) {
	var r Report
	var filters []byte
	err := row.Scan(&r.ID, &r.Kind, &r.Name, &r.Description, &r.Entity, &r.Metric, &r.Dimension,
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
		INSERT INTO reports (kind, name, description, entity, metric, dimension, filters, chart,
		    shared, position, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,
		    COALESCE((SELECT MAX(position) + 1 FROM reports), 0), $10)
		RETURNING id, position, created_at, updated_at`,
		r.Kind, r.Name, r.Description, r.Entity, r.Metric, r.Dimension, filters, r.Chart,
		r.Shared, r.CreatedBy,
	).Scan(&r.ID, &r.Position, &r.CreatedAt, &r.UpdatedAt)
}

func UpdateReport(db *sql.DB, r *Report) error {
	filters, err := json.Marshal(r.Filters)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		UPDATE reports SET kind = $1, name = $2, description = $3, entity = $4, metric = $5,
		    dimension = $6, filters = $7, chart = $8, shared = $9, updated_at = NOW()
		WHERE id = $10`,
		r.Kind, r.Name, r.Description, r.Entity, r.Metric, r.Dimension, filters, r.Chart,
		r.Shared, r.ID)
	return err
}

func DeleteReport(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM reports WHERE id = $1`, id)
	return err
}

// ===== Tipos de relatório especializados (negócios) =====

// Tipos disponíveis. O agregado é o construtor métrica × agrupamento; os demais
// são análises prontas de negócio, no estilo do Insights.
const (
	ReportKindAggregate  = "agregado"
	ReportKindConversion = "conversao"
	ReportKindDuration   = "duracao"
	ReportKindProgress   = "progresso"
)

var reportKinds = []string{
	ReportKindAggregate, ReportKindConversion, ReportKindDuration, ReportKindProgress,
}

// ReportKindLabels alimenta o modal de criação em dois passos.
var ReportKindLabels = map[string]string{
	ReportKindAggregate:  "Desempenho",
	ReportKindConversion: "Conversão de funil",
	ReportKindDuration:   "Duração do negócio",
	ReportKindProgress:   "Progresso",
}

func ValidReportKind(kind string) bool {
	return kind == "" || slices.Contains(reportKinds, kind)
}

// ReportSeries é uma linha do gráfico de série temporal (progresso).
type ReportSeries struct {
	Name   string      `json:"name"`
	Points []ReportRow `json:"points"`
}

// LoadFunnelConversion mede a conversão etapa a etapa a partir do histórico:
// quantos negócios entraram em cada etapa no período, a taxa contra a etapa
// anterior e contra o topo, mais a taxa de ganho de quem fechou.
func LoadFunnelConversion(db *sql.DB, pipelineID int64, days int) (*ReportResult, error) {
	if days <= 0 || days > 3650 {
		days = 365
	}

	args := []any{days}
	pipeFilter := ""
	if pipelineID > 0 {
		args = append(args, pipelineID)
		pipeFilter = fmt.Sprintf(" AND s.pipeline_id = $%d", len(args))
	}

	rows, err := db.Query(fmt.Sprintf(`
		SELECT s.id, s.name,
		       COUNT(DISTINCT h.deal_id) FILTER (
		           WHERE h.entered_at >= NOW() - make_interval(days => $1))
		FROM pipeline_stages s
		LEFT JOIN deal_stage_history h ON h.stage_id = s.id
		WHERE NOT s.is_won AND NOT s.is_lost%s
		GROUP BY s.id, s.name, s.position
		ORDER BY s.position`, pipeFilter), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &ReportResult{
		Rows:        []ReportRow{},
		MetricLabel: "Negócios que entraram",
		Dimension:   "Etapa",
	}
	for rows.Next() {
		var id int64
		var row ReportRow
		if err := rows.Scan(&id, &row.Label, &row.Value); err != nil {
			return nil, err
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Percent = conversão contra a etapa anterior (100% na primeira).
	for i := range result.Rows {
		if i == 0 {
			result.Rows[i].Percent = 100
			continue
		}
		if prev := result.Rows[i-1].Value; prev > 0 {
			result.Rows[i].Percent = result.Rows[i].Value / prev * 100
		}
	}

	// A taxa de ganho do período fecha o funil.
	var won, lost float64
	wonArgs := []any{days}
	wonFilter := ""
	if pipelineID > 0 {
		wonArgs = append(wonArgs, pipelineID)
		wonFilter = fmt.Sprintf(" AND pipeline_id = $%d", len(wonArgs))
	}
	if err := db.QueryRow(fmt.Sprintf(`
		SELECT COUNT(*) FILTER (WHERE status = 'ganho'),
		       COUNT(*) FILTER (WHERE status = 'perdido')
		FROM deals
		WHERE closed_at >= NOW() - make_interval(days => $1)%s`, wonFilter), wonArgs...,
	).Scan(&won, &lost); err != nil {
		return nil, err
	}
	winRate := 0.0
	if won+lost > 0 {
		winRate = won / (won + lost) * 100
	}
	result.Rows = append(result.Rows, ReportRow{Label: "Ganho", Value: won, Percent: winRate})
	result.Total = winRate

	return result, nil
}

// LoadStageDuration mede quantos dias, em média, o negócio passa em cada etapa
// (da entrada nela até a entrada na etapa seguinte; a etapa atual conta até
// agora ou até o fechamento).
func LoadStageDuration(db *sql.DB, pipelineID int64, days int) (*ReportResult, error) {
	if days <= 0 || days > 3650 {
		days = 365
	}

	args := []any{days}
	pipeFilter := ""
	if pipelineID > 0 {
		args = append(args, pipelineID)
		pipeFilter = fmt.Sprintf(" AND s.pipeline_id = $%d", len(args))
	}

	rows, err := db.Query(fmt.Sprintf(`
		WITH spans AS (
		    SELECT h.stage_id, h.deal_id,
		           COALESCE(
		               LEAD(h.entered_at) OVER (PARTITION BY h.deal_id ORDER BY h.entered_at),
		               d.closed_at, NOW()
		           ) - h.entered_at AS dur
		    FROM deal_stage_history h
		    JOIN deals d ON d.id = h.deal_id
		    WHERE h.entered_at >= NOW() - make_interval(days => $1)
		)
		SELECT s.name, COALESCE(AVG(EXTRACT(EPOCH FROM sp.dur) / 86400), 0)
		FROM pipeline_stages s
		LEFT JOIN spans sp ON sp.stage_id = s.id
		WHERE NOT s.is_won AND NOT s.is_lost%s
		GROUP BY s.id, s.name, s.position
		ORDER BY s.position`, pipeFilter), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &ReportResult{
		Rows:        []ReportRow{},
		MetricLabel: "Dias em média",
		Dimension:   "Etapa",
	}
	for rows.Next() {
		var row ReportRow
		if err := rows.Scan(&row.Label, &row.Value); err != nil {
			return nil, err
		}
		result.Rows = append(result.Rows, row)
		result.Total += row.Value
	}
	return result, rows.Err()
}

// LoadDealProgress monta a série mensal de criados, ganhos e perdidos — o
// gráfico de progresso segmentado por status.
func LoadDealProgress(db *sql.DB, pipelineID int64, months int) (*ReportResult, error) {
	if months <= 0 || months > 36 {
		months = 12
	}

	args := []any{months}
	pipeFilter := ""
	if pipelineID > 0 {
		args = append(args, pipelineID)
		pipeFilter = fmt.Sprintf(" AND pipeline_id = $%d", len(args))
	}

	// Um SELECT por série, sempre ancorado na mesma grade de meses.
	query := fmt.Sprintf(`
		WITH meses AS (
		    SELECT date_trunc('month', NOW()) - (n || ' months')::interval AS inicio
		    FROM generate_series($1::int - 1, 0, -1) AS n
		)
		SELECT TO_CHAR(m.inicio, 'YYYY-MM'),
		       COUNT(d.id) FILTER (WHERE date_trunc('month', d.created_at) = m.inicio),
		       COUNT(d.id) FILTER (WHERE d.status = 'ganho'
		           AND date_trunc('month', d.closed_at) = m.inicio),
		       COUNT(d.id) FILTER (WHERE d.status = 'perdido'
		           AND date_trunc('month', d.closed_at) = m.inicio)
		FROM meses m
		LEFT JOIN deals d ON TRUE%s
		GROUP BY m.inicio
		ORDER BY m.inicio`, pipeFilter)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	criados := ReportSeries{Name: "Criados", Points: []ReportRow{}}
	ganhos := ReportSeries{Name: "Ganhos", Points: []ReportRow{}}
	perdidos := ReportSeries{Name: "Perdidos", Points: []ReportRow{}}
	for rows.Next() {
		var mes string
		var c, g, p float64
		if err := rows.Scan(&mes, &c, &g, &p); err != nil {
			return nil, err
		}
		criados.Points = append(criados.Points, ReportRow{Label: mes, Value: c})
		ganhos.Points = append(ganhos.Points, ReportRow{Label: mes, Value: g})
		perdidos.Points = append(perdidos.Points, ReportRow{Label: mes, Value: p})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &ReportResult{
		Rows:        []ReportRow{},
		Series:      []ReportSeries{criados, ganhos, perdidos},
		MetricLabel: "Negócios por mês",
		Dimension:   "Mês",
	}, nil
}

// RunReportByKind despacha para a análise certa conforme o tipo salvo.
func RunReportByKind(db *sql.DB, r *Report) (*ReportResult, error) {
	switch r.Kind {
	case ReportKindConversion:
		return LoadFunnelConversion(db, r.Filters.PipelineID, r.Filters.Days)
	case ReportKindDuration:
		return LoadStageDuration(db, r.Filters.PipelineID, r.Filters.Days)
	case ReportKindProgress:
		months := r.Filters.Days / 30
		return LoadDealProgress(db, r.Filters.PipelineID, months)
	}
	return RunReport(db, r)
}
