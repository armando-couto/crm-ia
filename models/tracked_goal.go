package models

import (
	"database/sql"
	"fmt"
	"slices"
	"time"
)

// Tipos de meta acompanhada.
const (
	GoalWon      = "ganho"
	GoalAdded    = "adicionado"
	GoalProgress = "progresso"
	GoalActivity = "atividade"
)

var trackedGoalKinds = []string{GoalWon, GoalAdded, GoalProgress, GoalActivity}

// GoalKindLabels alimenta o modal de criação (com a pergunta de cada tipo).
var GoalKindLabels = map[string]string{
	GoalWon:      "Negócios ganhos",
	GoalAdded:    "Negócios adicionados",
	GoalProgress: "Negócios em progresso",
	GoalActivity: "Atividades realizadas",
}

// Métricas de rastreamento.
const (
	GoalMetricValue = "valor"
	GoalMetricCount = "numero"
)

// TrackedGoal é a meta com tipo, métrica, funil e duração.
type TrackedGoal struct {
	ID           int64     `json:"id"`
	Kind         string    `json:"kind"`
	Metric       string    `json:"metric"`
	UserID       *int64    `json:"user_id"`
	UserName     string    `json:"user_name,omitempty"`
	PipelineID   *int64    `json:"pipeline_id"`
	PipelineName string    `json:"pipeline_name,omitempty"`
	StageID      *int64    `json:"stage_id"`
	StageName    string    `json:"stage_name,omitempty"`
	ActivityKind string    `json:"activity_kind"`
	Amount       float64   `json:"amount"`
	StartPeriod  string    `json:"start_period"` // AAAA-MM
	EndPeriod    string    `json:"end_period"`   // AAAA-MM ou vazio (sem fim)
	CreatedBy    *int64    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Preenchidos na listagem: a meta já terminou? e o mês corrente.
	Finished     bool    `json:"finished"`
	CurrentValue float64 `json:"current_value"`
	Attainment   float64 `json:"attainment"`
}

// GoalPoint é um mês do acompanhamento: realizado contra o alvo.
type GoalPoint struct {
	Month      string  `json:"month"`
	Actual     float64 `json:"actual"`
	Target     float64 `json:"target"`
	Attainment float64 `json:"attainment"`
}

// ValidateTrackedGoal confere tipo, métrica e coerência dos campos.
func ValidateTrackedGoal(g *TrackedGoal) error {
	if !slices.Contains(trackedGoalKinds, g.Kind) {
		return fmt.Errorf("tipo de meta inválido")
	}
	if g.Metric == "" {
		g.Metric = GoalMetricValue
	}
	if g.Metric != GoalMetricValue && g.Metric != GoalMetricCount {
		return fmt.Errorf("métrica inválida (valor ou numero)")
	}
	// Atividade não tem valor em dinheiro.
	if g.Kind == GoalActivity {
		g.Metric = GoalMetricCount
		if g.ActivityKind != "" && g.ActivityKind != ActivityLigacao &&
			g.ActivityKind != ActivityReuniao && g.ActivityKind != ActivityEmail &&
			g.ActivityKind != ActivityNota {
			return fmt.Errorf("tipo de atividade inválido")
		}
	}
	if g.Kind == GoalProgress && g.StageID == nil {
		return fmt.Errorf("a meta de progresso precisa da etapa observada")
	}
	if g.Amount <= 0 {
		return fmt.Errorf("o alvo da meta deve ser maior que zero")
	}
	if _, err := parsePeriod(g.StartPeriod); err != nil {
		return fmt.Errorf("início inválido (use AAAA-MM)")
	}
	if g.EndPeriod != "" {
		start, _ := parsePeriod(g.StartPeriod)
		end, err := parsePeriod(g.EndPeriod)
		if err != nil {
			return fmt.Errorf("término inválido (use AAAA-MM)")
		}
		if end.Before(start) {
			return fmt.Errorf("o término não pode vir antes do início")
		}
	}
	return nil
}

const trackedGoalSelect = `
	SELECT g.id, g.kind, g.metric, g.user_id, COALESCE(u.name,''),
	       g.pipeline_id, COALESCE(p.name,''), g.stage_id, COALESCE(s.name,''),
	       g.activity_kind, g.amount, g.start_period, g.end_period,
	       g.created_by, g.created_at, g.updated_at
	FROM tracked_goals g
	LEFT JOIN users u ON u.id = g.user_id
	LEFT JOIN pipelines p ON p.id = g.pipeline_id
	LEFT JOIN pipeline_stages s ON s.id = g.stage_id`

func scanTrackedGoal(row interface{ Scan(...any) error }) (*TrackedGoal, error) {
	var g TrackedGoal
	var start time.Time
	var end *time.Time
	err := row.Scan(&g.ID, &g.Kind, &g.Metric, &g.UserID, &g.UserName,
		&g.PipelineID, &g.PipelineName, &g.StageID, &g.StageName,
		&g.ActivityKind, &g.Amount, &start, &end,
		&g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	g.StartPeriod = start.Format("2006-01")
	if end != nil {
		g.EndPeriod = end.Format("2006-01")
		monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
		g.Finished = end.Before(monthStart)
	}
	return &g, nil
}

func ListTrackedGoals(db *sql.DB) ([]TrackedGoal, error) {
	rows, err := db.Query(trackedGoalSelect + ` ORDER BY g.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []TrackedGoal{}
	for rows.Next() {
		g, err := scanTrackedGoal(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// O realizado do mês corrente dá a leitura rápida da listagem.
	current := time.Now().Format("2006-01")
	for i := range list {
		value, err := goalActual(db, &list[i], current)
		if err != nil {
			return nil, err
		}
		list[i].CurrentValue = value
		if list[i].Amount > 0 {
			list[i].Attainment = value / list[i].Amount * 100
		}
	}
	return list, nil
}

func TrackedGoalByID(db *sql.DB, id int64) (*TrackedGoal, error) {
	return scanTrackedGoal(db.QueryRow(trackedGoalSelect+` WHERE g.id = $1`, id))
}

func CreateTrackedGoal(db *sql.DB, g *TrackedGoal) error {
	start, _ := parsePeriod(g.StartPeriod)
	var end any
	if g.EndPeriod != "" {
		e, _ := parsePeriod(g.EndPeriod)
		end = e
	}
	return db.QueryRow(`
		INSERT INTO tracked_goals (kind, metric, user_id, pipeline_id, stage_id,
		    activity_kind, amount, start_period, end_period, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at, updated_at`,
		g.Kind, g.Metric, g.UserID, g.PipelineID, g.StageID,
		g.ActivityKind, g.Amount, start, end, g.CreatedBy,
	).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
}

func UpdateTrackedGoal(db *sql.DB, g *TrackedGoal) error {
	start, _ := parsePeriod(g.StartPeriod)
	var end any
	if g.EndPeriod != "" {
		e, _ := parsePeriod(g.EndPeriod)
		end = e
	}
	_, err := db.Exec(`
		UPDATE tracked_goals SET kind = $1, metric = $2, user_id = $3, pipeline_id = $4,
		    stage_id = $5, activity_kind = $6, amount = $7, start_period = $8,
		    end_period = $9, updated_at = NOW()
		WHERE id = $10`,
		g.Kind, g.Metric, g.UserID, g.PipelineID, g.StageID,
		g.ActivityKind, g.Amount, start, end, g.ID)
	return err
}

func DeleteTrackedGoal(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM tracked_goals WHERE id = $1`, id)
	return err
}

// goalActual mede o realizado da meta em um mês (AAAA-MM), conforme o tipo,
// a métrica e os filtros de dono e funil.
func goalActual(db *sql.DB, g *TrackedGoal, month string) (float64, error) {
	start, err := parsePeriod(month)
	if err != nil {
		return 0, err
	}
	end := start.AddDate(0, 1, 0)

	sel := "COUNT(*)"
	if g.Metric == GoalMetricValue {
		sel = "COALESCE(SUM(d.amount), 0)"
	}

	where := []string{}
	args := []any{start, end}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}

	var query string
	switch g.Kind {
	case GoalWon:
		where = append(where, "d.status = 'ganho'", "d.closed_at >= $1 AND d.closed_at < $2")
		if g.UserID != nil {
			add("d.owner_id = $%d", *g.UserID)
		}
		if g.PipelineID != nil {
			add("d.pipeline_id = $%d", *g.PipelineID)
		}
		query = fmt.Sprintf(`SELECT %s FROM deals d WHERE %s`, sel, joinAnd(where))

	case GoalAdded:
		where = append(where, "d.created_at >= $1 AND d.created_at < $2")
		if g.UserID != nil {
			add("d.owner_id = $%d", *g.UserID)
		}
		if g.PipelineID != nil {
			add("d.pipeline_id = $%d", *g.PipelineID)
		}
		query = fmt.Sprintf(`SELECT %s FROM deals d WHERE %s`, sel, joinAnd(where))

	case GoalProgress:
		// Entradas na etapa observada, direto do histórico do funil.
		where = append(where, "h.entered_at >= $1 AND h.entered_at < $2")
		add("h.stage_id = $%d", *g.StageID)
		if g.UserID != nil {
			add("d.owner_id = $%d", *g.UserID)
		}
		if g.Metric == GoalMetricValue {
			sel = "COALESCE(SUM(d.amount), 0)"
		} else {
			sel = "COUNT(DISTINCT h.deal_id)"
		}
		query = fmt.Sprintf(`
			SELECT %s FROM deal_stage_history h
			JOIN deals d ON d.id = h.deal_id
			WHERE %s`, sel, joinAnd(where))

	case GoalActivity:
		where = append(where, "a.created_at >= $1 AND a.created_at < $2")
		if g.ActivityKind != "" {
			add("a.kind = $%d", g.ActivityKind)
		} else {
			where = append(where, "a.kind <> 'sistema'")
		}
		if g.UserID != nil {
			add("a.user_id = $%d", *g.UserID)
		}
		query = fmt.Sprintf(`SELECT COUNT(*) FROM activities a WHERE %s`, joinAnd(where))

	default:
		return 0, fmt.Errorf("tipo de meta inválido")
	}

	var value float64
	err = db.QueryRow(query, args...).Scan(&value)
	return value, err
}

func joinAnd(conds []string) string {
	out := conds[0]
	for _, c := range conds[1:] {
		out += " AND " + c
	}
	return out
}

// TrackedGoalProgress monta a série mês a mês da meta, do início ao término
// (ou até o mês corrente quando a meta não tem fim).
func TrackedGoalProgress(db *sql.DB, g *TrackedGoal) ([]GoalPoint, error) {
	start, err := parsePeriod(g.StartPeriod)
	if err != nil {
		return nil, err
	}
	last := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
	if g.EndPeriod != "" {
		if end, err := parsePeriod(g.EndPeriod); err == nil {
			last = end
		}
	}

	points := []GoalPoint{}
	for month := start; !month.After(last) && len(points) < 36; month = month.AddDate(0, 1, 0) {
		key := month.Format("2006-01")
		actual, err := goalActual(db, g, key)
		if err != nil {
			return nil, err
		}
		point := GoalPoint{Month: key, Actual: actual, Target: g.Amount}
		if g.Amount > 0 {
			point.Attainment = actual / g.Amount * 100
		}
		points = append(points, point)
	}
	return points, nil
}
