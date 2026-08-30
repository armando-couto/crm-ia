package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	DealAberto  = "aberto"
	DealGanho   = "ganho"
	DealPerdido = "perdido"
)

type Deal struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Amount      float64    `json:"amount"`
	Currency    string     `json:"currency"`
	PipelineID  int64      `json:"pipeline_id"`
	StageID     int64      `json:"stage_id"`
	StageName   string     `json:"stage_name,omitempty"`
	ContactID   *int64     `json:"contact_id"`
	ContactName string     `json:"contact_name,omitempty"`
	CompanyID   *int64     `json:"company_id"`
	CompanyName string     `json:"company_name,omitempty"`
	OwnerID     *int64     `json:"owner_id"`
	OwnerName   string     `json:"owner_name,omitempty"`
	Status      string     `json:"status"`
	CloseDate   *time.Time `json:"close_date"`
	Position    int        `json:"position"`
	ClosedAt    *time.Time `json:"closed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type DealFilter struct {
	Search     string
	PipelineID int64
	StageID    int64
	OwnerID    int64
	Status     string
	ContactID  int64
	CompanyID  int64
	Pagination
}

const dealSelect = `
	SELECT d.id, d.name, d.amount, d.currency, d.pipeline_id, d.stage_id, s.name,
	       d.contact_id, COALESCE(ct.first_name || ' ' || COALESCE(ct.last_name,''), ''),
	       d.company_id, COALESCE(co.name,''),
	       d.owner_id, COALESCE(u.name,''),
	       d.status, d.close_date, d.position, d.closed_at, d.created_at, d.updated_at
	FROM deals d
	JOIN pipeline_stages s ON s.id = d.stage_id
	LEFT JOIN contacts ct ON ct.id = d.contact_id
	LEFT JOIN companies co ON co.id = d.company_id
	LEFT JOIN users u ON u.id = d.owner_id`

func scanDeal(row interface{ Scan(...any) error }) (*Deal, error) {
	var d Deal
	err := row.Scan(&d.ID, &d.Name, &d.Amount, &d.Currency, &d.PipelineID, &d.StageID, &d.StageName,
		&d.ContactID, &d.ContactName, &d.CompanyID, &d.CompanyName, &d.OwnerID, &d.OwnerName,
		&d.Status, &d.CloseDate, &d.Position, &d.ClosedAt, &d.CreatedAt, &d.UpdatedAt)
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
	cond := strings.Join(where, " AND ")

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM deals d WHERE `+cond, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(`%s WHERE %s ORDER BY d.updated_at DESC LIMIT $%d OFFSET $%d`,
		dealSelect, cond, len(args)-1, len(args)), args...)
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

// BoardDeals retorna os negócios abertos de um pipeline, ordenados para o kanban.
func BoardDeals(db *sql.DB, pipelineID int64) ([]Deal, error) {
	rows, err := db.Query(dealSelect+` WHERE d.pipeline_id = $1 AND d.status = 'aberto'
		ORDER BY d.stage_id, d.position, d.id`, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Deal{}
	for rows.Next() {
		d, err := scanDeal(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *d)
	}
	return list, rows.Err()
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
	return db.QueryRow(`
		INSERT INTO deals (name, amount, currency, pipeline_id, stage_id, contact_id, company_id, owner_id, status, close_date, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		        COALESCE((SELECT MAX(position) + 1 FROM deals WHERE stage_id = $5), 0))
		RETURNING id, position, created_at, updated_at`,
		d.Name, d.Amount, d.Currency, d.PipelineID, d.StageID, d.ContactID, d.CompanyID,
		d.OwnerID, d.Status, d.CloseDate,
	).Scan(&d.ID, &d.Position, &d.CreatedAt, &d.UpdatedAt)
}

func UpdateDeal(db *sql.DB, d *Deal) error {
	_, err := db.Exec(`
		UPDATE deals SET name = $1, amount = $2, currency = $3, contact_id = $4, company_id = $5,
		       owner_id = $6, close_date = $7, updated_at = NOW()
		WHERE id = $8`,
		d.Name, d.Amount, d.Currency, d.ContactID, d.CompanyID, d.OwnerID, d.CloseDate, d.ID)
	return err
}

// MoveDealStage move o negócio para outra etapa/posição (drag & drop do kanban).
// Se a etapa destino for de ganho/perda, o status é atualizado junto.
func MoveDealStage(db *sql.DB, dealID, stageID int64, position int) error {
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
	_, err = db.Exec(`
		UPDATE deals SET stage_id = $1, position = $2, status = $3, closed_at = $4, updated_at = NOW()
		WHERE id = $5`,
		stageID, position, status, closedAt, dealID)
	return err
}

// CloseDeal fecha um negócio como ganho ou perdido, movendo-o para a etapa correspondente do pipeline.
func CloseDeal(db *sql.DB, dealID int64, won bool) error {
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
	return err
}

func DeleteDeal(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM deals WHERE id = $1`, id)
	return err
}
