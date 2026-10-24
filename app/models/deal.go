package models

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	DealAberto  = "aberto"
	DealGanho   = "ganho"
	DealPerdido = "perdido"
)

type Deal struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	PipelineID  int64   `json:"pipeline_id"`
	StageID     int64   `json:"stage_id"`
	StageName   string  `json:"stage_name,omitempty"`
	ContactID   *int64  `json:"contact_id"`
	ContactName string  `json:"contact_name,omitempty"`
	CompanyID   *int64  `json:"company_id"`
	CompanyName string  `json:"company_name,omitempty"`
	OwnerID     *int64  `json:"owner_id"`
	OwnerName   string  `json:"owner_name,omitempty"`
	Status      string  `json:"status"`
	Temperature string  `json:"temperature"`
	// Categoria de previsão (Sales Forecast): a chance de fechar no período.
	ForecastCategory string     `json:"forecast_category"`
	CloseDate        *time.Time `json:"close_date"`
	Position         int        `json:"position"`
	ClosedAt         *time.Time `json:"closed_at"`
	LastActivityAt   *time.Time `json:"last_activity_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// Campos do kanban (preenchidos por BoardDeals): quando entrou na etapa
	// atual, há quantos dias está nela, o prazo saudável da etapa e há quantos
	// dias não tem atividade. Zerados nas demais consultas.
	StageEnteredAt   *time.Time `json:"stage_entered_at"`
	DiasNaEtapa      int        `json:"dias_na_etapa"`
	PrazoDias        *int       `json:"prazo_dias"`
	DiasSemAtividade int        `json:"dias_sem_atividade"`
}

func ValidDealTemperature(t string) bool {
	return t == "" || t == "fria" || t == "media" || t == "quente"
}

type DealFilter struct {
	Search       string
	PipelineID   int64
	StageID      int64
	OwnerID      int64
	Status       string
	ContactID    int64
	CompanyID    int64
	Temperature  string
	CreatedDays  int
	InactiveDays int
	CloseWindow  string // fecham_mes | previsao_vencida
	SortBy       string
	SortDir      string
	Advanced     *AdvancedFilters
	Pagination
}

// dealOrderBy valida a ordenação contra uma lista fechada de colunas.
func dealOrderBy(sortBy, sortDir string) string {
	column := map[string]string{
		"name":          "LOWER(d.name)",
		"amount":        "d.amount",
		"created_at":    "d.created_at",
		"updated_at":    "d.updated_at",
		"close_date":    "d.close_date",
		"last_activity": "last_activity_at",
	}[sortBy]
	if column == "" {
		column = "d.updated_at"
	}
	dir := "DESC"
	if strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	return column + " " + dir + " NULLS LAST"
}

const dealColumns = `d.id, d.name, d.amount, d.currency, d.pipeline_id, d.stage_id, s.name,
	       d.contact_id, COALESCE(ct.first_name || ' ' || COALESCE(ct.last_name,''), ''),
	       d.company_id, COALESCE(co.name,''),
	       d.owner_id, COALESCE(u.name,''),
	       d.status, COALESCE(d.temperature,''), COALESCE(d.forecast_category,'pipeline'),
	       d.close_date, d.position, d.closed_at,
	       (SELECT MAX(a.created_at) FROM activities a WHERE a.deal_id = d.id) AS last_activity_at,
	       d.created_at, d.updated_at`

const dealFrom = `
	FROM deals d
	JOIN pipeline_stages s ON s.id = d.stage_id
	LEFT JOIN contacts ct ON ct.id = d.contact_id
	LEFT JOIN companies co ON co.id = d.company_id
	LEFT JOIN users u ON u.id = d.owner_id`

const dealSelect = `
	SELECT ` + dealColumns + dealFrom

// boardSelect acrescenta ao dealSelect o que o kanban precisa para o selo de
// tempo na etapa: a última entrada na etapa atual e o prazo da etapa.
const boardSelect = `
	SELECT ` + dealColumns + `,
	       (SELECT MAX(h.entered_at) FROM deal_stage_history h
	         WHERE h.deal_id = d.id AND h.stage_id = d.stage_id) AS stage_entered_at,
	       s.prazo_dias` + dealFrom

func scanDeal(row interface{ Scan(...any) error }) (*Deal, error) {
	var d Deal
	err := row.Scan(&d.ID, &d.Name, &d.Amount, &d.Currency, &d.PipelineID, &d.StageID, &d.StageName,
		&d.ContactID, &d.ContactName, &d.CompanyID, &d.CompanyName, &d.OwnerID, &d.OwnerName,
		&d.Status, &d.Temperature, &d.ForecastCategory, &d.CloseDate, &d.Position, &d.ClosedAt, &d.LastActivityAt,
		&d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	d.ContactName = strings.TrimSpace(d.ContactName)
	return &d, nil
}

func ListDeals(db *sql.DB, f DealFilter) ([]Deal, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Search != "" {
		add("LOWER(d.name) LIKE $%d", "%"+strings.ToLower(f.Search)+"%")
	}
	if f.PipelineID > 0 {
		add("d.pipeline_id = $%d", f.PipelineID)
	}
	if f.StageID > 0 {
		add("d.stage_id = $%d", f.StageID)
	}
	if f.OwnerID > 0 {
		add("d.owner_id = $%d", f.OwnerID)
	}
	if f.Status != "" {
		add("d.status = $%d", f.Status)
	}
	if f.ContactID > 0 {
		add("d.contact_id = $%d", f.ContactID)
	}
	if f.CompanyID > 0 {
		add("d.company_id = $%d", f.CompanyID)
	}
	if f.Temperature != "" {
		add("d.temperature = $%d", f.Temperature)
	}
	if f.CreatedDays > 0 {
		add("d.created_at >= NOW() - make_interval(days => $%d)", f.CreatedDays)
	}
	if f.InactiveDays > 0 {
		add("NOT EXISTS (SELECT 1 FROM activities a WHERE a.deal_id = d.id AND a.created_at >= NOW() - make_interval(days => $%d))", f.InactiveDays)
	}
	switch f.CloseWindow {
	case "fecham_mes":
		where = append(where, "d.close_date >= date_trunc('month', NOW()) AND d.close_date < date_trunc('month', NOW()) + INTERVAL '1 month'")
	case "previsao_vencida":
		where = append(where, "d.status = 'aberto' AND d.close_date < CURRENT_DATE")
	}
	if adv := BuildAdvancedWhere(f.Advanced, DealFilterFieldsSpec, &args); adv != "" {
		where = append(where, adv)
	}
	cond := strings.Join(where, " AND ")

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM deals d WHERE `+cond, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(`%s WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`,
		dealSelect, cond, dealOrderBy(f.SortBy, f.SortDir), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Deal{}
	for rows.Next() {
		d, err := scanDeal(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *d)
	}
	return list, total, rows.Err()
}

// BoardFilter restringe o kanban (busca, dono, temperatura, datas e filtros avançados).
type BoardFilter struct {
	PipelineID   int64
	OwnerID      int64
	Search       string
	Temperature  string
	CreatedDays  int    // criados nos últimos N dias
	InactiveDays int    // sem atividade há N dias (ou nunca)
	CloseWindow  string // fecham_mes | previsao_vencida
	Advanced     *AdvancedFilters
	// IncludeClosed inclui negócios ganhos/perdidos fechados recentemente.
	ClosedDays int
}

// BoardDeals retorna os negócios do pipeline para o kanban: os abertos e,
// opcionalmente, os fechados nos últimos N dias (colunas de ganho/perda).
func BoardDeals(db *sql.DB, f BoardFilter) ([]Deal, error) {
	where := []string{"d.pipeline_id = $1"}
	args := []any{f.PipelineID}
	if f.ClosedDays > 0 {
		args = append(args, f.ClosedDays)
		where = append(where, fmt.Sprintf(
			"(d.status = 'aberto' OR d.closed_at >= NOW() - make_interval(days => $%d))", len(args)))
	} else {
		where = append(where, "d.status = 'aberto'")
	}
	if f.OwnerID > 0 {
		args = append(args, f.OwnerID)
		where = append(where, fmt.Sprintf("d.owner_id = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		where = append(where, fmt.Sprintf("LOWER(d.name) LIKE $%d", len(args)))
	}
	if f.Temperature != "" {
		args = append(args, f.Temperature)
		where = append(where, fmt.Sprintf("d.temperature = $%d", len(args)))
	}
	if f.CreatedDays > 0 {
		args = append(args, f.CreatedDays)
		where = append(where, fmt.Sprintf("d.created_at >= NOW() - make_interval(days => $%d)", len(args)))
	}
	if f.InactiveDays > 0 {
		args = append(args, f.InactiveDays)
		where = append(where, fmt.Sprintf(
			"NOT EXISTS (SELECT 1 FROM activities a WHERE a.deal_id = d.id AND a.created_at >= NOW() - make_interval(days => $%d))",
			len(args)))
	}
	switch f.CloseWindow {
	case "fecham_mes":
		where = append(where, "d.close_date >= date_trunc('month', NOW()) AND d.close_date < date_trunc('month', NOW()) + INTERVAL '1 month'")
	case "previsao_vencida":
		where = append(where, "d.status = 'aberto' AND d.close_date < CURRENT_DATE")
	}
	if adv := BuildAdvancedWhere(f.Advanced, DealFilterFieldsSpec, &args); adv != "" {
		where = append(where, adv)
	}

	rows, err := db.Query(boardSelect+` WHERE `+strings.Join(where, " AND ")+`
		ORDER BY d.stage_id, d.position, d.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	list := []Deal{}
	for rows.Next() {
		d, err := scanBoardDeal(rows, now)
		if err != nil {
			return nil, err
		}
		list = append(list, *d)
	}
	return list, rows.Err()
}

// scanBoardDeal lê a linha do boardSelect e calcula os dias na etapa e sem
// atividade a partir de "now" (parâmetro para os testes serem determinísticos).
func scanBoardDeal(row interface{ Scan(...any) error }, now time.Time) (*Deal, error) {
	var d Deal
	err := row.Scan(&d.ID, &d.Name, &d.Amount, &d.Currency, &d.PipelineID, &d.StageID, &d.StageName,
		&d.ContactID, &d.ContactName, &d.CompanyID, &d.CompanyName, &d.OwnerID, &d.OwnerName,
		&d.Status, &d.Temperature, &d.ForecastCategory, &d.CloseDate, &d.Position, &d.ClosedAt, &d.LastActivityAt,
		&d.CreatedAt, &d.UpdatedAt, &d.StageEnteredAt, &d.PrazoDias)
	if err != nil {
		return nil, err
	}
	d.ContactName = strings.TrimSpace(d.ContactName)
	if d.StageEnteredAt == nil {
		// Negócios anteriores ao histórico: a entrada na etapa é a criação.
		entered := d.CreatedAt
		d.StageEnteredAt = &entered
	}
	d.DiasNaEtapa = diasDesde(*d.StageEnteredAt, now)
	ultima := d.CreatedAt
	if d.LastActivityAt != nil && d.LastActivityAt.After(ultima) {
		ultima = *d.LastActivityAt
	}
	d.DiasSemAtividade = diasDesde(ultima, now)
	return &d, nil
}

// diasDesde conta dias corridos inteiros entre dois instantes (nunca negativo).
func diasDesde(desde, ate time.Time) int {
	if !ate.After(desde) {
		return 0
	}
	return int(ate.Sub(desde).Hours() / 24)
}

func DealByID(db *sql.DB, id int64) (*Deal, error) {
	row := db.QueryRow(dealSelect+` WHERE d.id = $1`, id)
	return scanDeal(row)
}

func CreateDeal(db *sql.DB, d *Deal) error {
	if d.Currency == "" {
		d.Currency = "BRL"
	}
	if d.Status == "" {
		d.Status = DealAberto
	}
	err := db.QueryRow(`
		INSERT INTO deals (name, amount, currency, pipeline_id, stage_id, contact_id, company_id, owner_id, status, temperature, close_date, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
		        COALESCE((SELECT MAX(position) + 1 FROM deals WHERE stage_id = $5), 0))
		RETURNING id, position, created_at, updated_at`,
		d.Name, d.Amount, d.Currency, d.PipelineID, d.StageID, d.ContactID, d.CompanyID,
		d.OwnerID, d.Status, d.Temperature, d.CloseDate,
	).Scan(&d.ID, &d.Position, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return err
	}
	// Histórico de etapas: alimenta a conversão de funil e o tempo por etapa.
	_, err = db.Exec(`INSERT INTO deal_stage_history (deal_id, stage_id) VALUES ($1, $2)`,
		d.ID, d.StageID)
	return err
}

// ===== Rastro de mudanças (deal_changes) =====

// Campos rastreados em deal_changes: são os que o funil usa para reconstruir a
// movimentação de um período (waterfall, adiamentos, variação em 7 dias).
const (
	DealChangeAmount    = "amount"
	DealChangeCloseDate = "close_date"
	DealChangeStage     = "stage_id"
	DealChangeOwner     = "owner_id"
	DealChangeStatus    = "status"
)

// DealChange é uma linha de deal_changes: o valor anterior e o novo como texto
// (datas em AAAA-MM-DD, nulos como "").
type DealChange struct {
	ID        int64     `json:"id"`
	DealID    int64     `json:"deal_id"`
	Campo     string    `json:"campo"`
	De        string    `json:"de"`
	Para      string    `json:"para"`
	UserID    *int64    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Conversões para o texto gravado em deal_changes.
func changeAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
func changeInt(v int64) string      { return strconv.FormatInt(v, 10) }
func changeIntPtr(v *int64) string {
	if v == nil {
		return ""
	}
	return changeInt(*v)
}
func changeDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// recordDealChange grava uma linha em deal_changes quando o valor mudou.
func recordDealChange(db *sql.DB, dealID int64, campo, de, para string, userID *int64) error {
	if de == para {
		return nil
	}
	_, err := db.Exec(`INSERT INTO deal_changes (deal_id, campo, de, para, user_id) VALUES ($1, $2, $3, $4, $5)`,
		dealID, campo, de, para, userID)
	return err
}

func UpdateDeal(db *sql.DB, d *Deal) error {
	return UpdateDealBy(db, d, nil)
}

// UpdateDealBy grava a edição do negócio e registra em deal_changes o que
// mudou em valor, previsão de fechamento e dono, atribuído a userID.
func UpdateDealBy(db *sql.DB, d *Deal, userID *int64) error {
	if d.ForecastCategory == "" {
		d.ForecastCategory = ForecastPipeline
	}
	// Valores anteriores, para o rastro de mudanças.
	var antes struct {
		amount    float64
		closeDate *time.Time
		ownerID   *int64
	}
	err := db.QueryRow(`SELECT amount, close_date, owner_id FROM deals WHERE id = $1`, d.ID).
		Scan(&antes.amount, &antes.closeDate, &antes.ownerID)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		UPDATE deals SET name = $1, amount = $2, currency = $3, contact_id = $4, company_id = $5,
		       owner_id = $6, temperature = $7, close_date = $8, forecast_category = $9, updated_at = NOW()
		WHERE id = $10`,
		d.Name, d.Amount, d.Currency, d.ContactID, d.CompanyID, d.OwnerID, d.Temperature,
		d.CloseDate, d.ForecastCategory, d.ID)
	if err != nil {
		return err
	}

	changes := []struct{ campo, de, para string }{
		{DealChangeAmount, changeAmount(antes.amount), changeAmount(d.Amount)},
		{DealChangeCloseDate, changeDate(antes.closeDate), changeDate(d.CloseDate)},
		{DealChangeOwner, changeIntPtr(antes.ownerID), changeIntPtr(d.OwnerID)},
	}
	for _, c := range changes {
		if err := recordDealChange(db, d.ID, c.campo, c.de, c.para, userID); err != nil {
			return err
		}
	}
	return nil
}

// MoveDealStage move o negócio para outra etapa/posição (drag & drop do kanban).
// Se a etapa destino for de ganho/perda, o status é atualizado junto.
func MoveDealStage(db *sql.DB, dealID, stageID int64, position int) error {
	return MoveDealStageBy(db, dealID, stageID, position, nil)
}

// MoveDealStageBy é MoveDealStage atribuindo a mudança a userID em deal_changes.
func MoveDealStageBy(db *sql.DB, dealID, stageID int64, position int, userID *int64) error {
	stage, err := StageByID(db, stageID)
	if err != nil {
		return err
	}
	status := DealAberto
	var closedAt any
	switch {
	case stage.IsWon:
		status = DealGanho
		closedAt = time.Now()
	case stage.IsLost:
		status = DealPerdido
		closedAt = time.Now()
	default:
		closedAt = nil
	}
	var previousStage int64
	var previousStatus string
	err = db.QueryRow(`SELECT stage_id, status FROM deals WHERE id = $1`, dealID).
		Scan(&previousStage, &previousStatus)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		UPDATE deals SET stage_id = $1, position = $2, status = $3, closed_at = $4, updated_at = NOW()
		WHERE id = $5`,
		stageID, position, status, closedAt, dealID)
	if err != nil {
		return err
	}
	if previousStage != stageID {
		// A entrada na etapa nova entra no histórico do funil.
		if _, err := db.Exec(`INSERT INTO deal_stage_history (deal_id, stage_id) VALUES ($1, $2)`,
			dealID, stageID); err != nil {
			return err
		}
		if err := recordDealChange(db, dealID, DealChangeStage,
			changeInt(previousStage), changeInt(stageID), userID); err != nil {
			return err
		}
	}
	return recordDealChange(db, dealID, DealChangeStatus, previousStatus, status, userID)
}

// CloseDeal fecha um negócio como ganho ou perdido, movendo-o para a etapa correspondente do pipeline.
func CloseDeal(db *sql.DB, dealID int64, won bool) error {
	return CloseDealBy(db, dealID, won, nil)
}

// CloseDealBy é CloseDeal atribuindo a mudança a userID em deal_changes.
func CloseDealBy(db *sql.DB, dealID int64, won bool, userID *int64) error {
	deal, err := DealByID(db, dealID)
	if err != nil {
		return err
	}
	column := "is_lost"
	status := DealPerdido
	if won {
		column = "is_won"
		status = DealGanho
	}
	var stageID int64
	err = db.QueryRow(fmt.Sprintf(
		`SELECT id FROM pipeline_stages WHERE pipeline_id = $1 AND %s = TRUE ORDER BY position LIMIT 1`, column),
		deal.PipelineID).Scan(&stageID)
	if err == sql.ErrNoRows {
		stageID = deal.StageID
		err = nil
	}
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		UPDATE deals SET status = $1, stage_id = $2, closed_at = NOW(), updated_at = NOW() WHERE id = $3`,
		status, stageID, dealID)
	if err != nil {
		return err
	}
	if stageID != deal.StageID {
		// Fechar também é mudar de etapa: entra no histórico como no kanban.
		if _, err := db.Exec(`INSERT INTO deal_stage_history (deal_id, stage_id) VALUES ($1, $2)`,
			dealID, stageID); err != nil {
			return err
		}
		if err := recordDealChange(db, dealID, DealChangeStage,
			changeInt(deal.StageID), changeInt(stageID), userID); err != nil {
			return err
		}
	}
	return recordDealChange(db, dealID, DealChangeStatus, deal.Status, status, userID)
}

func DeleteDeal(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM deals WHERE id = $1`, id)
	return err
}
