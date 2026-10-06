package models_test

import (
	"testing"
	"time"

	"fixpay/fix-crm/models"
)

func TestValidateTrackedGoal(t *testing.T) {
	valida := &models.TrackedGoal{Kind: models.GoalWon, Amount: 1000, StartPeriod: "2026-08"}
	if err := models.ValidateTrackedGoal(valida); err != nil {
		t.Fatalf("meta válida recusada: %v", err)
	}
	if valida.Metric != models.GoalMetricValue {
		t.Fatal("a métrica padrão deveria ser valor")
	}

	casos := map[string]*models.TrackedGoal{
		"tipo inválido":       {Kind: "loteria", Amount: 100, StartPeriod: "2026-08"},
		"alvo zero":           {Kind: models.GoalWon, Amount: 0, StartPeriod: "2026-08"},
		"início inválido":     {Kind: models.GoalWon, Amount: 100, StartPeriod: "agosto"},
		"término antes":       {Kind: models.GoalWon, Amount: 100, StartPeriod: "2026-08", EndPeriod: "2026-05"},
		"progresso sem etapa": {Kind: models.GoalProgress, Amount: 100, StartPeriod: "2026-08"},
	}
	for nome, g := range casos {
		if err := models.ValidateTrackedGoal(g); err == nil {
			t.Errorf("%s: deveria ser recusado", nome)
		}
	}

	// Atividade força a métrica para número.
	atividade := &models.TrackedGoal{Kind: models.GoalActivity, Metric: "valor",
		Amount: 30, StartPeriod: "2026-08"}
	if err := models.ValidateTrackedGoal(atividade); err != nil {
		t.Fatal(err)
	}
	if atividade.Metric != models.GoalMetricCount {
		t.Fatal("meta de atividade deveria virar contagem")
	}
}

// O realizado da meta de ganho respeita métrica, dono e mês.
func TestTrackedGoalActuals(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	db.Exec(`DELETE FROM tracked_goals`)

	ana := createTestUser(t, db, "ana.meta2@fixpay.com.br")
	bruno := createTestUser(t, db, "bruno.meta2@fixpay.com.br")
	var stageID, pipelineID int64
	db.QueryRow(`SELECT id, pipeline_id FROM pipeline_stages WHERE NOT is_won AND NOT is_lost
		ORDER BY position LIMIT 1`).Scan(&stageID, &pipelineID)

	// Ana ganha 2 negócios (1000 + 500); Bruno ganha 1 (300).
	for _, spec := range []struct {
		owner  int64
		amount float64
	}{{ana, 1000}, {ana, 500}, {bruno, 300}} {
		owner := spec.owner
		deal := &models.Deal{Name: "N", Amount: spec.amount, PipelineID: pipelineID,
			StageID: stageID, OwnerID: &owner}
		if err := models.CreateDeal(db, deal); err != nil {
			t.Fatal(err)
		}
		if err := models.CloseDeal(db, deal.ID, true); err != nil {
			t.Fatal(err)
		}
	}

	month := time.Now().Format("2006-01")

	equipe := &models.TrackedGoal{Kind: models.GoalWon, Metric: models.GoalMetricValue,
		Amount: 2000, StartPeriod: month}
	if err := models.CreateTrackedGoal(db, equipe); err != nil {
		t.Fatal(err)
	}
	daAna := &models.TrackedGoal{Kind: models.GoalWon, Metric: models.GoalMetricCount,
		UserID: &ana, Amount: 5, StartPeriod: month}
	if err := models.CreateTrackedGoal(db, daAna); err != nil {
		t.Fatal(err)
	}

	goals, err := models.ListTrackedGoals(db)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]models.TrackedGoal{}
	for _, g := range goals {
		byID[g.ID] = g
	}

	if got := byID[equipe.ID].CurrentValue; got != 1800 {
		t.Fatalf("equipe em valor deveria somar 1800, veio %v", got)
	}
	if got := byID[equipe.ID].Attainment; int(got) != 90 {
		t.Fatalf("atingimento esperado 90%%, veio %v", got)
	}
	if got := byID[daAna.ID].CurrentValue; got != 2 {
		t.Fatalf("Ana em número deveria contar 2, veio %v", got)
	}
}

// A meta de progresso conta as entradas na etapa pelo histórico.
func TestTrackedGoalProgressKind(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	db.Exec(`DELETE FROM tracked_goals`)

	stageRows, _ := db.Query(`SELECT id, pipeline_id FROM pipeline_stages
		WHERE NOT is_won AND NOT is_lost ORDER BY position LIMIT 2`)
	type st struct{ id, pipe int64 }
	stages := []st{}
	for stageRows.Next() {
		var s st
		stageRows.Scan(&s.id, &s.pipe)
		stages = append(stages, s)
	}
	stageRows.Close()

	// 3 negócios nascem na etapa 1; 2 avançam para a etapa 2.
	ids := []int64{}
	for i := 0; i < 3; i++ {
		deal := &models.Deal{Name: "N", Amount: 100, PipelineID: stages[0].pipe, StageID: stages[0].id}
		if err := models.CreateDeal(db, deal); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, deal.ID)
	}
	for _, id := range ids[:2] {
		if err := models.MoveDealStage(db, id, stages[1].id, 0); err != nil {
			t.Fatal(err)
		}
	}

	month := time.Now().Format("2006-01")
	meta := &models.TrackedGoal{Kind: models.GoalProgress, Metric: models.GoalMetricCount,
		StageID: &stages[1].id, Amount: 4, StartPeriod: month}
	if err := models.CreateTrackedGoal(db, meta); err != nil {
		t.Fatal(err)
	}

	goals, err := models.ListTrackedGoals(db)
	if err != nil {
		t.Fatal(err)
	}
	if goals[0].CurrentValue != 2 {
		t.Fatalf("2 negócios entraram na etapa, veio %v", goals[0].CurrentValue)
	}
}

// A série mensal cobre do início ao término, com o alvo em cada ponto.
func TestTrackedGoalSeries(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	db.Exec(`DELETE FROM tracked_goals`)

	// Âncora no primeiro dia do mês: AddDate sobre dia 31 pula meses curtos.
	base := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
	inicio := base.AddDate(0, -2, 0).Format("2006-01")
	fim := base.Format("2006-01")
	meta := &models.TrackedGoal{Kind: models.GoalAdded, Metric: models.GoalMetricCount,
		Amount: 3, StartPeriod: inicio, EndPeriod: fim}
	if err := models.CreateTrackedGoal(db, meta); err != nil {
		t.Fatal(err)
	}

	var stageID, pipelineID int64
	db.QueryRow(`SELECT id, pipeline_id FROM pipeline_stages ORDER BY position LIMIT 1`).
		Scan(&stageID, &pipelineID)
	for i := 0; i < 2; i++ {
		if err := models.CreateDeal(db, &models.Deal{Name: "N", Amount: 100,
			PipelineID: pipelineID, StageID: stageID}); err != nil {
			t.Fatal(err)
		}
	}

	saved, err := models.TrackedGoalByID(db, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	points, err := models.TrackedGoalProgress(db, saved)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 3 {
		t.Fatalf("esperava 3 meses na série, veio %d", len(points))
	}
	last := points[len(points)-1]
	if last.Actual != 2 || last.Target != 3 {
		t.Fatalf("mês corrente deveria marcar 2 de 3: %+v", last)
	}
	if int(last.Attainment) != 66 {
		t.Fatalf("atingimento esperado ~66%%, veio %v", last.Attainment)
	}
	if points[0].Actual != 0 {
		t.Fatalf("mês antigo deveria estar zerado: %+v", points[0])
	}
}

// Meta encerrada aparece como passada.
func TestTrackedGoalFinished(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	db.Exec(`DELETE FROM tracked_goals`)

	base := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
	antigo := base.AddDate(0, -3, 0).Format("2006-01")
	fim := base.AddDate(0, -1, 0).Format("2006-01")
	meta := &models.TrackedGoal{Kind: models.GoalWon, Amount: 100,
		StartPeriod: antigo, EndPeriod: fim}
	if err := models.CreateTrackedGoal(db, meta); err != nil {
		t.Fatal(err)
	}

	saved, err := models.TrackedGoalByID(db, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Finished {
		t.Fatalf("meta com término no mês passado deveria constar encerrada: %+v", saved)
	}
}
