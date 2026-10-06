package models

import (
	"database/sql"
	"fmt"
	"time"
)

// SalesGoal é a meta de um mês: da equipe (UserID nulo) ou de uma pessoa.
type SalesGoal struct {
	ID     int64   `json:"id"`
	UserID *int64  `json:"user_id"`
	Period string  `json:"period"` // AAAA-MM
	Amount float64 `json:"amount"`
}

// ForecastRow é a previsão de um vendedor no período.
type ForecastRow struct {
	OwnerID   *int64 `json:"owner_id"`
	OwnerName string `json:"owner_name"`
	// Ganho: já fechado no período.
	Won float64 `json:"won"`
	// Comprometido: aberto com previsão de fechamento dentro do período.
	Committed float64 `json:"committed"`
	// Ponderado: comprometido × probabilidade da etapa.
	Weighted  float64 `json:"weighted"`
	OpenDeals int     `json:"open_deals"`
	Goal      float64 `json:"goal"`
	// Atingimento do ganho sobre a meta, em porcentagem.
	Attainment float64 `json:"attainment"`
}

// Forecast é a previsão do período inteiro.
type Forecast struct {
	Period    string        `json:"period"`
	Rows      []ForecastRow `json:"rows"`
	TeamGoal  float64       `json:"team_goal"`
	Won       float64       `json:"won"`
	Committed float64       `json:"committed"`
	Weighted  float64       `json:"weighted"`
	// Projeção = já ganho + ponderado do que ainda está aberto.
	Projected float64 `json:"projected"`
	Gap       float64 `json:"gap"`
}

// parsePeriod aceita "AAAA-MM" e devolve o primeiro dia do mês.
func parsePeriod(period string) (time.Time, error) {
	if period == "" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local), nil
	}
	t, err := time.ParseInLocation("2006-01", period, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("período inválido (use AAAA-MM)")
	}
	return t, nil
}

// LoadForecast monta a previsão do mês por vendedor, comparada com as metas.
func LoadForecast(db *sql.DB, period string, pipelineID int64) (*Forecast, error) {
	start, err := parsePeriod(period)
	if err != nil {
		return nil, err
	}
	end := start.AddDate(0, 1, 0)

	pipelineFilter := ""
	args := []any{start, end}
	if pipelineID > 0 {
		args = append(args, pipelineID)
		pipelineFilter = fmt.Sprintf(" AND d.pipeline_id = $%d", len(args))
	}

	// Uma passada só: ganho no período, comprometido e ponderado do que está
	// aberto com previsão de fechamento dentro do mês.
	query := fmt.Sprintf(`
		SELECT d.owner_id, COALESCE(u.name, 'Sem dono'),
		       COALESCE(SUM(d.amount) FILTER (
		           WHERE d.status = 'ganho' AND d.closed_at >= $1 AND d.closed_at < $2), 0) AS won,
		       COALESCE(SUM(d.amount) FILTER (
		           WHERE d.status = 'aberto' AND d.close_date >= $1 AND d.close_date < $2), 0) AS committed,
		       COALESCE(SUM(d.amount * COALESCE(s.probability, 0) / 100.0) FILTER (
		           WHERE d.status = 'aberto' AND d.close_date >= $1 AND d.close_date < $2), 0) AS weighted,
		       COUNT(*) FILTER (
		           WHERE d.status = 'aberto' AND d.close_date >= $1 AND d.close_date < $2) AS open_deals
		FROM deals d
		LEFT JOIN users u ON u.id = d.owner_id
		LEFT JOIN pipeline_stages s ON s.id = d.stage_id
		WHERE ((d.status = 'ganho' AND d.closed_at >= $1 AND d.closed_at < $2)
		    OR (d.status = 'aberto' AND d.close_date >= $1 AND d.close_date < $2))%s
		GROUP BY d.owner_id, u.name
		ORDER BY won DESC, committed DESC`, pipelineFilter)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	forecast := &Forecast{Period: start.Format("2006-01"), Rows: []ForecastRow{}}
	for rows.Next() {
		var r ForecastRow
		if err := rows.Scan(&r.OwnerID, &r.OwnerName, &r.Won, &r.Committed,
			&r.Weighted, &r.OpenDeals); err != nil {
			return nil, err
		}
		forecast.Rows = append(forecast.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	goals, teamGoal, err := loadGoals(db, start)
	if err != nil {
		return nil, err
	}
	forecast.TeamGoal = teamGoal

	for i := range forecast.Rows {
		row := &forecast.Rows[i]
		if row.OwnerID != nil {
			row.Goal = goals[*row.OwnerID]
		}
		if row.Goal > 0 {
			row.Attainment = row.Won / row.Goal * 100
		}
		forecast.Won += row.Won
		forecast.Committed += row.Committed
		forecast.Weighted += row.Weighted
	}
	forecast.Projected = forecast.Won + forecast.Weighted
	forecast.Gap = forecast.TeamGoal - forecast.Projected

	return forecast, nil
}

// loadGoals devolve as metas individuais do mês e a meta da equipe. Sem meta da
// equipe cadastrada, ela vira a soma das individuais.
func loadGoals(db *sql.DB, start time.Time) (map[int64]float64, float64, error) {
	rows, err := db.Query(`SELECT user_id, amount FROM sales_goals WHERE period = $1`, start)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	goals := map[int64]float64{}
	var teamGoal, sumIndividual float64
	for rows.Next() {
		var userID *int64
		var amount float64
		if err := rows.Scan(&userID, &amount); err != nil {
			return nil, 0, err
		}
		if userID == nil {
			teamGoal = amount
			continue
		}
		goals[*userID] = amount
		sumIndividual += amount
	}
	if teamGoal == 0 {
		teamGoal = sumIndividual
	}
	return goals, teamGoal, rows.Err()
}

func ListGoals(db *sql.DB, period string) ([]SalesGoal, error) {
	start, err := parsePeriod(period)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT id, user_id, period, amount FROM sales_goals WHERE period = $1 ORDER BY user_id NULLS FIRST`,
		start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []SalesGoal{}
	for rows.Next() {
		var g SalesGoal
		var p time.Time
		if err := rows.Scan(&g.ID, &g.UserID, &p, &g.Amount); err != nil {
			return nil, err
		}
		g.Period = p.Format("2006-01")
		list = append(list, g)
	}
	return list, rows.Err()
}

// SaveGoal grava a meta do mês (sobrescreve a anterior). Meta zero apaga.
func SaveGoal(db *sql.DB, userID *int64, period string, amount float64) error {
	start, err := parsePeriod(period)
	if err != nil {
		return err
	}
	if amount < 0 {
		return fmt.Errorf("a meta não pode ser negativa")
	}

	if amount == 0 {
		if userID == nil {
			_, err := db.Exec(`DELETE FROM sales_goals WHERE period = $1 AND user_id IS NULL`, start)
			return err
		}
		_, err := db.Exec(`DELETE FROM sales_goals WHERE period = $1 AND user_id = $2`, start, *userID)
		return err
	}

	// Os índices únicos são parciais, então o upsert vai por caminhos diferentes
	// para a meta da equipe e a individual.
	if userID == nil {
		_, err := db.Exec(`
			INSERT INTO sales_goals (user_id, period, amount) VALUES (NULL, $1, $2)
			ON CONFLICT (period) WHERE user_id IS NULL
			DO UPDATE SET amount = EXCLUDED.amount, updated_at = NOW()`, start, amount)
		return err
	}
	_, err = db.Exec(`
		INSERT INTO sales_goals (user_id, period, amount) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, period) WHERE user_id IS NOT NULL
		DO UPDATE SET amount = EXCLUDED.amount, updated_at = NOW()`, *userID, start, amount)
	return err
}

// ===== Análise de vendas =====

// StageConversion mostra quantos negócios passaram por cada etapa e quanto
// seguiu adiante.
type StageConversion struct {
	StageID   int64   `json:"stage_id"`
	StageName string  `json:"stage_name"`
	Count     int     `json:"count"`
	Amount    float64 `json:"amount"`
	// Porcentagem em relação à primeira etapa do funil.
	Rate float64 `json:"rate"`
}

// SalesAnalytics reúne os números de desempenho do período.
type SalesAnalytics struct {
	Days           int               `json:"days"`
	Created        int               `json:"created"`
	Won            int               `json:"won"`
	Lost           int               `json:"lost"`
	WinRate        float64           `json:"win_rate"`
	AvgTicket      float64           `json:"avg_ticket"`
	AvgCycleDays   float64           `json:"avg_cycle_days"`
	WonAmount      float64           `json:"won_amount"`
	Funnel         []StageConversion `json:"funnel"`
	ActivityByKind []ReportRow       `json:"activity_by_kind"`
}

// LoadSalesAnalytics calcula taxa de ganho, ticket médio, ciclo de vendas,
// funil por etapa e o volume de interações do período.
func LoadSalesAnalytics(db *sql.DB, days int, pipelineID int64) (*SalesAnalytics, error) {
	if days <= 0 || days > 3650 {
		days = 90
	}
	a := &SalesAnalytics{Days: days, Funnel: []StageConversion{}, ActivityByKind: []ReportRow{}}

	pipeFilter := ""
	args := []any{days}
	if pipelineID > 0 {
		args = append(args, pipelineID)
		pipeFilter = fmt.Sprintf(" AND pipeline_id = $%d", len(args))
	}

	// Criados, ganhos, perdidos, ticket médio e ciclo, tudo no mesmo período.
	err := db.QueryRow(fmt.Sprintf(`
		SELECT COUNT(*) FILTER (WHERE created_at >= NOW() - make_interval(days => $1)),
		       COUNT(*) FILTER (WHERE status = 'ganho' AND closed_at >= NOW() - make_interval(days => $1)),
		       COUNT(*) FILTER (WHERE status = 'perdido' AND closed_at >= NOW() - make_interval(days => $1)),
		       COALESCE(AVG(amount) FILTER (
		           WHERE status = 'ganho' AND closed_at >= NOW() - make_interval(days => $1)), 0),
		       COALESCE(SUM(amount) FILTER (
		           WHERE status = 'ganho' AND closed_at >= NOW() - make_interval(days => $1)), 0),
		       COALESCE(AVG(EXTRACT(EPOCH FROM (closed_at - created_at)) / 86400) FILTER (
		           WHERE status = 'ganho' AND closed_at >= NOW() - make_interval(days => $1)), 0)
		FROM deals WHERE TRUE%s`, pipeFilter), args...,
	).Scan(&a.Created, &a.Won, &a.Lost, &a.AvgTicket, &a.WonAmount, &a.AvgCycleDays)
	if err != nil {
		return nil, err
	}
	if fechados := a.Won + a.Lost; fechados > 0 {
		a.WinRate = float64(a.Won) / float64(fechados) * 100
	}

	// Funil: negócios abertos por etapa, na ordem do pipeline.
	funnelArgs := []any{}
	funnelFilter := ""
	if pipelineID > 0 {
		funnelArgs = append(funnelArgs, pipelineID)
		funnelFilter = " AND s.pipeline_id = $1"
	}
	rows, err := db.Query(fmt.Sprintf(`
		SELECT s.id, s.name, COUNT(d.id), COALESCE(SUM(d.amount), 0)
		FROM pipeline_stages s
		LEFT JOIN deals d ON d.stage_id = s.id AND d.status = 'aberto'
		WHERE NOT s.is_won AND NOT s.is_lost%s
		GROUP BY s.id, s.name, s.position
		ORDER BY s.position`, funnelFilter), funnelArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var c StageConversion
		if err := rows.Scan(&c.StageID, &c.StageName, &c.Count, &c.Amount); err != nil {
			return nil, err
		}
		a.Funnel = append(a.Funnel, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// A taxa compara cada etapa com a primeira (topo do funil).
	if len(a.Funnel) > 0 && a.Funnel[0].Count > 0 {
		first := float64(a.Funnel[0].Count)
		for i := range a.Funnel {
			a.Funnel[i].Rate = float64(a.Funnel[i].Count) / first * 100
		}
	}

	// Volume de interações por tipo: mostra se a equipe está trabalhando.
	actRows, err := db.Query(`
		SELECT kind, COUNT(*) FROM activities
		WHERE created_at >= NOW() - make_interval(days => $1)
		GROUP BY kind ORDER BY COUNT(*) DESC`, days)
	if err != nil {
		return nil, err
	}
	defer actRows.Close()

	for actRows.Next() {
		var row ReportRow
		if err := actRows.Scan(&row.Label, &row.Value); err != nil {
			return nil, err
		}
		a.ActivityByKind = append(a.ActivityByKind, row)
	}
	return a, actRows.Err()
}

// ===== Categorias de previsão (Sales Forecast) =====

// Categorias, da menos à mais certa. "excluido" fica fora dos números.
const (
	ForecastExcluded  = "excluido"
	ForecastPipeline  = "pipeline"
	ForecastBestCase  = "melhor_caso"
	ForecastCommitted = "comprometido"
	ForecastClosed    = "fechado"
)

var forecastCategories = []string{
	ForecastExcluded, ForecastPipeline, ForecastBestCase, ForecastCommitted, ForecastClosed,
}

// ForecastCategoryLabels alimenta o seletor no negócio e as colunas da tela.
var ForecastCategoryLabels = map[string]string{
	ForecastExcluded:  "Excluído",
	ForecastPipeline:  "Pipeline",
	ForecastBestCase:  "Melhor caso",
	ForecastCommitted: "Comprometido",
	ForecastClosed:    "Fechado",
}

func ValidForecastCategory(c string) bool {
	if c == "" {
		return true
	}
	for _, v := range forecastCategories {
		if v == c {
			return true
		}
	}
	return false
}

// CategoryRow é a linha de um vendedor na visão por categoria: quanto ele tem
// em cada balde no período, mais o número que ele mesmo submeteu.
type CategoryRow struct {
	OwnerID   *int64  `json:"owner_id"`
	OwnerName string  `json:"owner_name"`
	Pipeline  float64 `json:"pipeline"`
	BestCase  float64 `json:"best_case"`
	Committed float64 `json:"committed"`
	Closed    float64 `json:"closed"`
	Goal      float64 `json:"goal"`
	// Envio de previsão do próprio vendedor (0 quando não enviou).
	Submitted     float64 `json:"submitted"`
	SubmittedNote string  `json:"submitted_note,omitempty"`
}

// CategoryForecast é a visão por categoria do período inteiro.
type CategoryForecast struct {
	Period    string        `json:"period"`
	Rows      []CategoryRow `json:"rows"`
	Pipeline  float64       `json:"pipeline"`
	BestCase  float64       `json:"best_case"`
	Committed float64       `json:"committed"`
	Closed    float64       `json:"closed"`
	Submitted float64       `json:"submitted"`
	TeamGoal  float64       `json:"team_goal"`
	// Lacuna: meta menos o que já fechou.
	Gap float64 `json:"gap"`
}

// LoadCategoryForecast agrega os negócios do período pelos baldes de categoria.
// Ganhos contam como "fechado" independentemente da categoria marcada; abertos
// entram no balde escolhido pelo vendedor (excluído fica de fora).
func LoadCategoryForecast(db *sql.DB, period string, pipelineID int64) (*CategoryForecast, error) {
	start, err := parsePeriod(period)
	if err != nil {
		return nil, err
	}
	end := start.AddDate(0, 1, 0)

	pipelineFilter := ""
	args := []any{start, end}
	if pipelineID > 0 {
		args = append(args, pipelineID)
		pipelineFilter = fmt.Sprintf(" AND d.pipeline_id = $%d", len(args))
	}

	query := fmt.Sprintf(`
		SELECT d.owner_id, COALESCE(u.name, 'Sem dono'),
		       COALESCE(SUM(d.amount) FILTER (
		           WHERE d.status = 'aberto' AND d.forecast_category = 'pipeline'), 0),
		       COALESCE(SUM(d.amount) FILTER (
		           WHERE d.status = 'aberto' AND d.forecast_category = 'melhor_caso'), 0),
		       COALESCE(SUM(d.amount) FILTER (
		           WHERE d.status = 'aberto' AND d.forecast_category = 'comprometido'), 0),
		       COALESCE(SUM(d.amount) FILTER (WHERE d.status = 'ganho'), 0)
		FROM deals d
		LEFT JOIN users u ON u.id = d.owner_id
		WHERE ((d.status = 'ganho' AND d.closed_at >= $1 AND d.closed_at < $2)
		    OR (d.status = 'aberto' AND d.close_date >= $1 AND d.close_date < $2
		        AND d.forecast_category <> 'excluido'))%s
		GROUP BY d.owner_id, u.name
		ORDER BY 6 DESC, 5 DESC`, pipelineFilter)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	forecast := &CategoryForecast{Period: start.Format("2006-01"), Rows: []CategoryRow{}}
	for rows.Next() {
		var r CategoryRow
		if err := rows.Scan(&r.OwnerID, &r.OwnerName, &r.Pipeline, &r.BestCase,
			&r.Committed, &r.Closed); err != nil {
			return nil, err
		}
		forecast.Rows = append(forecast.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	goals, teamGoal, err := loadGoals(db, start)
	if err != nil {
		return nil, err
	}
	forecast.TeamGoal = teamGoal

	submissions, err := loadSubmissions(db, start)
	if err != nil {
		return nil, err
	}

	for i := range forecast.Rows {
		row := &forecast.Rows[i]
		if row.OwnerID != nil {
			row.Goal = goals[*row.OwnerID]
			if sub, ok := submissions[*row.OwnerID]; ok {
				row.Submitted = sub.Amount
				row.SubmittedNote = sub.Note
			}
		}
		forecast.Pipeline += row.Pipeline
		forecast.BestCase += row.BestCase
		forecast.Committed += row.Committed
		forecast.Closed += row.Closed
		forecast.Submitted += row.Submitted
	}
	forecast.Gap = forecast.TeamGoal - forecast.Closed

	return forecast, nil
}

// ===== Envio de previsão =====

// ForecastSubmission é o número que o vendedor submete para o período.
type ForecastSubmission struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	Period    string    `json:"period"`
	Amount    float64   `json:"amount"`
	Note      string    `json:"note"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveSubmission grava (ou atualiza) o envio do vendedor para o mês.
func SaveSubmission(db *sql.DB, userID int64, period string, amount float64, note string) error {
	start, err := parsePeriod(period)
	if err != nil {
		return err
	}
	if amount < 0 {
		return fmt.Errorf("o valor previsto não pode ser negativo")
	}
	_, err = db.Exec(`
		INSERT INTO forecast_submissions (user_id, period, amount, note)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, period)
		DO UPDATE SET amount = EXCLUDED.amount, note = EXCLUDED.note, updated_at = NOW()`,
		userID, start, amount, note)
	return err
}

// loadSubmissions devolve os envios do mês indexados por vendedor.
func loadSubmissions(db *sql.DB, start time.Time) (map[int64]ForecastSubmission, error) {
	rows, err := db.Query(`
		SELECT user_id, amount, note FROM forecast_submissions WHERE period = $1`, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]ForecastSubmission{}
	for rows.Next() {
		var s ForecastSubmission
		if err := rows.Scan(&s.UserID, &s.Amount, &s.Note); err != nil {
			return nil, err
		}
		out[s.UserID] = s
	}
	return out, rows.Err()
}

// MySubmission devolve o envio do próprio vendedor no período (nil sem envio).
func MySubmission(db *sql.DB, userID int64, period string) (*ForecastSubmission, error) {
	start, err := parsePeriod(period)
	if err != nil {
		return nil, err
	}
	var s ForecastSubmission
	var p time.Time
	err = db.QueryRow(`
		SELECT id, user_id, period, amount, note, updated_at
		FROM forecast_submissions WHERE user_id = $1 AND period = $2`,
		userID, start,
	).Scan(&s.ID, &s.UserID, &p, &s.Amount, &s.Note, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Period = p.Format("2006-01")
	return &s, nil
}
