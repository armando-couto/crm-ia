package models

import (
	"database/sql"
)

type StageMetric struct {
	StageID   int64   `json:"stage_id"`
	StageName string  `json:"stage_name"`
	Count     int     `json:"count"`
	Amount    float64 `json:"amount"`
}

type MonthMetric struct {
	Month  string  `json:"month"` // YYYY-MM
	Count  int     `json:"count"`
	Amount float64 `json:"amount"`
}

type OwnerMetric struct {
	OwnerID   int64   `json:"owner_id"`
	OwnerName string  `json:"owner_name"`
	Won       int     `json:"won"`
	Amount    float64 `json:"amount"`
}

type Dashboard struct {
	TotalContacts   int           `json:"total_contacts"`
	TotalCompanies  int           `json:"total_companies"`
	OpenDeals       int           `json:"open_deals"`
	OpenAmount      float64       `json:"open_amount"`
	ForecastAmount  float64       `json:"forecast_amount"`
	WonThisMonth    int           `json:"won_this_month"`
	WonAmountMonth  float64       `json:"won_amount_month"`
	LostThisMonth   int           `json:"lost_this_month"`
	TasksPending    int           `json:"tasks_pending"`
	TasksOverdue    int           `json:"tasks_overdue"`
	DealsByStage    []StageMetric `json:"deals_by_stage"`
	WonByMonth      []MonthMetric `json:"won_by_month"`
	RankingOwners   []OwnerMetric `json:"ranking_owners"`
	NewContactsWeek int           `json:"new_contacts_week"`
}

// LoadDashboard consolida as métricas principais do CRM em uma única resposta.
func LoadDashboard(db *sql.DB, pipelineID int64) (*Dashboard, error) {
	d := &Dashboard{
		DealsByStage:  []StageMetric{},
		WonByMonth:    []MonthMetric{},
		RankingOwners: []OwnerMetric{},
	}

	err := db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM contacts),
		       (SELECT COUNT(*) FROM companies),
		       (SELECT COUNT(*) FROM contacts WHERE created_at >= NOW() - INTERVAL '7 days'),
		       (SELECT COUNT(*) FROM tasks WHERE completed_at IS NULL),
		       (SELECT COUNT(*) FROM tasks WHERE completed_at IS NULL AND due_date < NOW())`,
	).Scan(&d.TotalContacts, &d.TotalCompanies, &d.NewContactsWeek, &d.TasksPending, &d.TasksOverdue)
	if err != nil {
		return nil, err
	}

	err = db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(amount), 0),
		       COALESCE(SUM(amount * s.probability / 100.0), 0)
		FROM deals d
		JOIN pipeline_stages s ON s.id = d.stage_id
		WHERE d.status = 'aberto' AND ($1 = 0 OR d.pipeline_id = $1)`, pipelineID,
	).Scan(&d.OpenDeals, &d.OpenAmount, &d.ForecastAmount)
	if err != nil {
		return nil, err
	}

	err = db.QueryRow(`
		SELECT COUNT(*) FILTER (WHERE status = 'ganho'),
		       COALESCE(SUM(amount) FILTER (WHERE status = 'ganho'), 0),
		       COUNT(*) FILTER (WHERE status = 'perdido')
		FROM deals
		WHERE closed_at >= date_trunc('month', NOW()) AND ($1 = 0 OR pipeline_id = $1)`, pipelineID,
	).Scan(&d.WonThisMonth, &d.WonAmountMonth, &d.LostThisMonth)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT s.id, s.name, COUNT(d.id), COALESCE(SUM(d.amount), 0)
		FROM pipeline_stages s
		LEFT JOIN deals d ON d.stage_id = s.id AND d.status = 'aberto'
		WHERE s.is_won = FALSE AND s.is_lost = FALSE AND ($1 = 0 OR s.pipeline_id = $1)
		GROUP BY s.id, s.name, s.position
		ORDER BY s.position`, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m StageMetric
		if err := rows.Scan(&m.StageID, &m.StageName, &m.Count, &m.Amount); err != nil {
			return nil, err
		}
		d.DealsByStage = append(d.DealsByStage, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	monthRows, err := db.Query(`
		SELECT to_char(date_trunc('month', closed_at), 'YYYY-MM'), COUNT(*), COALESCE(SUM(amount), 0)
		FROM deals
		WHERE status = 'ganho' AND closed_at >= date_trunc('month', NOW()) - INTERVAL '5 months'
		      AND ($1 = 0 OR pipeline_id = $1)
		GROUP BY 1 ORDER BY 1`, pipelineID)
	if err != nil {
		return nil, err
	}
	defer monthRows.Close()
	for monthRows.Next() {
		var m MonthMetric
		if err := monthRows.Scan(&m.Month, &m.Count, &m.Amount); err != nil {
			return nil, err
		}
		d.WonByMonth = append(d.WonByMonth, m)
	}
	if err := monthRows.Err(); err != nil {
		return nil, err
	}

	ownerRows, err := db.Query(`
		SELECT u.id, u.name, COUNT(d.id), COALESCE(SUM(d.amount), 0)
		FROM users u
		JOIN deals d ON d.owner_id = u.id AND d.status = 'ganho'
		     AND d.closed_at >= date_trunc('month', NOW()) AND ($1 = 0 OR d.pipeline_id = $1)
		GROUP BY u.id, u.name
		ORDER BY 4 DESC
		LIMIT 5`, pipelineID)
	if err != nil {
		return nil, err
	}
	defer ownerRows.Close()
	for ownerRows.Next() {
		var m OwnerMetric
		if err := ownerRows.Scan(&m.OwnerID, &m.OwnerName, &m.Won, &m.Amount); err != nil {
			return nil, err
		}
		d.RankingOwners = append(d.RankingOwners, m)
	}
	return d, ownerRows.Err()
}

// SearchResult é um item da busca global (contato, empresa ou negócio).
type SearchResult struct {
	Type  string `json:"type"` // contato | empresa | negocio
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Sub   string `json:"sub"`
}

// GlobalSearch procura em contatos, empresas e negócios pelo termo informado.
func GlobalSearch(db *sql.DB, term string) ([]SearchResult, error) {
	q := "%" + NormalizeEmail(term) + "%"
	rows, err := db.Query(`
		(SELECT 'contato', id, first_name || ' ' || COALESCE(last_name,''), COALESCE(email,'')
		 FROM contacts
		 WHERE LOWER(first_name || ' ' || COALESCE(last_name,'')) LIKE $1 OR LOWER(COALESCE(email,'')) LIKE $1
		 LIMIT 5)
		UNION ALL
		(SELECT 'empresa', id, name, COALESCE(domain,'')
		 FROM companies WHERE LOWER(name) LIKE $1 LIMIT 5)
		UNION ALL
		(SELECT 'negocio', id, name, status
		 FROM deals WHERE LOWER(name) LIKE $1 LIMIT 5)`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []SearchResult{}
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.Type, &r.ID, &r.Title, &r.Sub); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}
