package models

import (
	"database/sql"
	"strconv"
	"time"
)

// WorkspaceItem é uma linha da fila de trabalho: o que fazer e em qual registro.
type WorkspaceItem struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Subtitle  string     `json:"subtitle"`
	Due       *time.Time `json:"due"`
	Amount    float64    `json:"amount,omitempty"`
	ContactID *int64     `json:"contact_id,omitempty"`
	CompanyID *int64     `json:"company_id,omitempty"`
	DealID    *int64     `json:"deal_id,omitempty"`
	// Motivo pelo qual o item está na fila (ex.: "sem interação há 21 dias").
	Reason string `json:"reason,omitempty"`
}

// Workspace é o dia do vendedor: o que está atrasado, o que é de hoje e o que
// está pedindo atenção.
type Workspace struct {
	OverdueTasks   []WorkspaceItem `json:"overdue_tasks"`
	TodayTasks     []WorkspaceItem `json:"today_tasks"`
	TodayMeetings  []WorkspaceItem `json:"today_meetings"`
	StaleDeals     []WorkspaceItem `json:"stale_deals"`
	ClosingSoon    []WorkspaceItem `json:"closing_soon"`
	UntouchedLeads []WorkspaceItem `json:"untouched_leads"`

	OpenDeals     int     `json:"open_deals"`
	OpenAmount    float64 `json:"open_amount"`
	WonMonth      float64 `json:"won_month"`
	GoalMonth     float64 `json:"goal_month"`
	TasksPending  int     `json:"tasks_pending"`
	MeetingsWeek  int     `json:"meetings_week"`
	TargetAccount int     `json:"target_accounts"`
}

// staleDealDays é o tempo sem interação que joga o negócio na fila de atenção.
const staleDealDays = 14

// LoadWorkspace monta o painel de trabalho do vendedor.
func LoadWorkspace(db *sql.DB, userID int64) (*Workspace, error) {
	w := &Workspace{
		OverdueTasks:   []WorkspaceItem{},
		TodayTasks:     []WorkspaceItem{},
		TodayMeetings:  []WorkspaceItem{},
		StaleDeals:     []WorkspaceItem{},
		ClosingSoon:    []WorkspaceItem{},
		UntouchedLeads: []WorkspaceItem{},
	}

	// Tarefas atrasadas e de hoje.
	taskRows, err := db.Query(`
		SELECT t.id, t.title, COALESCE(t.type,''), t.due_date, t.contact_id, t.company_id, t.deal_id,
		       t.due_date::date < CURRENT_DATE AS atrasada
		FROM tasks t
		WHERE t.owner_id = $1 AND t.completed_at IS NULL AND t.due_date IS NOT NULL
		  AND t.due_date::date <= CURRENT_DATE
		ORDER BY t.due_date
		LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	for taskRows.Next() {
		var item WorkspaceItem
		var overdue bool
		if err := taskRows.Scan(&item.ID, &item.Title, &item.Subtitle, &item.Due,
			&item.ContactID, &item.CompanyID, &item.DealID, &overdue); err != nil {
			taskRows.Close()
			return nil, err
		}
		if overdue {
			w.OverdueTasks = append(w.OverdueTasks, item)
		} else {
			w.TodayTasks = append(w.TodayTasks, item)
		}
	}
	taskRows.Close()
	if err := taskRows.Err(); err != nil {
		return nil, err
	}

	// Reuniões de hoje.
	if err := collect(db, &w.TodayMeetings, `
		SELECT m.id, m.title, COALESCE(TRIM(c.first_name || ' ' || c.last_name), ''), m.starts_at,
		       m.contact_id, m.company_id, m.deal_id
		FROM meetings m
		LEFT JOIN contacts c ON c.id = m.contact_id
		WHERE m.user_id = $1 AND m.status = 'agendada'
		  AND m.starts_at::date = CURRENT_DATE
		ORDER BY m.starts_at
		LIMIT 20`, userID); err != nil {
		return nil, err
	}

	// Negócios abertos sem interação recente.
	staleRows, err := db.Query(`
		SELECT d.id, d.name, COALESCE(s.name,''), d.close_date, d.amount,
		       d.contact_id, d.company_id, d.id,
		       COALESCE(EXTRACT(DAY FROM NOW() - GREATEST(
		           d.created_at, (SELECT MAX(a.created_at) FROM activities a WHERE a.deal_id = d.id)
		       )), 0)::int AS dias
		FROM deals d
		LEFT JOIN pipeline_stages s ON s.id = d.stage_id
		WHERE d.owner_id = $1 AND d.status = 'aberto'
		  AND NOT EXISTS (
		      SELECT 1 FROM activities a
		      WHERE a.deal_id = d.id AND a.created_at >= NOW() - make_interval(days => $2))
		  AND d.created_at < NOW() - make_interval(days => $2)
		ORDER BY d.amount DESC
		LIMIT 20`, userID, staleDealDays)
	if err != nil {
		return nil, err
	}
	for staleRows.Next() {
		var item WorkspaceItem
		var days int
		if err := staleRows.Scan(&item.ID, &item.Title, &item.Subtitle, &item.Due, &item.Amount,
			&item.ContactID, &item.CompanyID, &item.DealID, &days); err != nil {
			staleRows.Close()
			return nil, err
		}
		item.Reason = pluralDays(days)
		w.StaleDeals = append(w.StaleDeals, item)
	}
	staleRows.Close()
	if err := staleRows.Err(); err != nil {
		return nil, err
	}

	// Negócios com fechamento previsto nos próximos 7 dias.
	if err := collectDeals(db, &w.ClosingSoon, `
		SELECT d.id, d.name, COALESCE(s.name,''), d.close_date, d.amount,
		       d.contact_id, d.company_id, d.id
		FROM deals d
		LEFT JOIN pipeline_stages s ON s.id = d.stage_id
		WHERE d.owner_id = $1 AND d.status = 'aberto'
		  AND d.close_date IS NOT NULL
		  AND d.close_date BETWEEN CURRENT_DATE AND CURRENT_DATE + 7
		ORDER BY d.close_date
		LIMIT 20`, userID); err != nil {
		return nil, err
	}

	// Leads da carteira que ainda não receberam nenhuma interação.
	if err := collect(db, &w.UntouchedLeads, `
		SELECT c.id, TRIM(c.first_name || ' ' || c.last_name), COALESCE(c.email,''),
		       c.created_at, c.id, c.company_id, NULL
		FROM contacts c
		WHERE c.owner_id = $1 AND c.lifecycle_stage = 'lead'
		  AND NOT EXISTS (SELECT 1 FROM activities a WHERE a.contact_id = c.id AND a.kind <> 'sistema')
		ORDER BY c.created_at DESC
		LIMIT 20`, userID); err != nil {
		return nil, err
	}

	// Números do topo.
	if err := db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM deals WHERE owner_id = $1 AND status = 'aberto'),
		       (SELECT COALESCE(SUM(amount),0) FROM deals WHERE owner_id = $1 AND status = 'aberto'),
		       (SELECT COALESCE(SUM(amount),0) FROM deals
		         WHERE owner_id = $1 AND status = 'ganho'
		           AND closed_at >= date_trunc('month', CURRENT_DATE)),
		       (SELECT COUNT(*) FROM tasks WHERE owner_id = $1 AND completed_at IS NULL),
		       (SELECT COUNT(*) FROM meetings WHERE user_id = $1 AND status = 'agendada'
		           AND starts_at BETWEEN NOW() AND NOW() + INTERVAL '7 days'),
		       (SELECT COUNT(*) FROM companies WHERE owner_id = $1 AND is_target)`,
		userID,
	).Scan(&w.OpenDeals, &w.OpenAmount, &w.WonMonth, &w.TasksPending,
		&w.MeetingsWeek, &w.TargetAccount); err != nil {
		return nil, err
	}

	// Meta pessoal do mês, quando houver.
	start := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
	if err := db.QueryRow(`
		SELECT COALESCE((SELECT amount FROM sales_goals WHERE user_id = $1 AND period = $2), 0)`,
		userID, start).Scan(&w.GoalMonth); err != nil {
		return nil, err
	}

	return w, nil
}

// collect roda a consulta e enche a fila (7 colunas, sem valor).
func collect(db *sql.DB, target *[]WorkspaceItem, query string, args ...any) error {
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var item WorkspaceItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Subtitle, &item.Due,
			&item.ContactID, &item.CompanyID, &item.DealID); err != nil {
			return err
		}
		*target = append(*target, item)
	}
	return rows.Err()
}

// collectDeals é o mesmo que collect, mas com a coluna de valor do negócio.
func collectDeals(db *sql.DB, target *[]WorkspaceItem, query string, args ...any) error {
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var item WorkspaceItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Subtitle, &item.Due, &item.Amount,
			&item.ContactID, &item.CompanyID, &item.DealID); err != nil {
			return err
		}
		*target = append(*target, item)
	}
	return rows.Err()
}

func pluralDays(days int) string {
	if days == 1 {
		return "sem interação há 1 dia"
	}
	return "sem interação há " + strconv.Itoa(days) + " dias"
}
