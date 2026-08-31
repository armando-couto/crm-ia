package models_test

import (
	"testing"
	"time"

	"fixpay/fix-crm/models"
)

func TestSaveAndLoadGoals(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	if _, err := db.Exec(`DELETE FROM sales_goals`); err != nil {
		t.Fatal(err)
	}

	ana := createTestUser(t, db, "ana.meta@fixpay.com.br")
	period := time.Now().Format("2006-01")

	// Meta individual e meta da equipe convivem no mesmo mês.
	if err := models.SaveGoal(db, &ana, period, 50000); err != nil {
		t.Fatal(err)
	}
	if err := models.SaveGoal(db, nil, period, 120000); err != nil {
		t.Fatal(err)
	}

	goals, err := models.ListGoals(db, period)
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 2 {
		t.Fatalf("esperava 2 metas (equipe + individual), veio %d", len(goals))
	}

	// Salvar de novo sobrescreve em vez de duplicar.
	if err := models.SaveGoal(db, &ana, period, 60000); err != nil {
		t.Fatal(err)
	}
	goals, _ = models.ListGoals(db, period)
	if len(goals) != 2 {
		t.Fatalf("a meta deveria ser sobrescrita, veio %d linhas", len(goals))
	}

	// Meta zero apaga.
	if err := models.SaveGoal(db, &ana, period, 0); err != nil {
		t.Fatal(err)
	}
	goals, _ = models.ListGoals(db, period)
	if len(goals) != 1 {
		t.Fatalf("meta zero deveria apagar a linha, veio %d", len(goals))
	}
}

func TestSaveGoalRejectsBadInput(t *testing.T) {
	db := testDB(t)

	if err := models.SaveGoal(db, nil, "2026-13-99", 1000); err == nil {
		t.Fatal("período inválido deveria ser recusado")
	}
	if err := models.SaveGoal(db, nil, "2026-09", -5); err == nil {
		t.Fatal("meta negativa deveria ser recusada")
	}
}

// A previsão separa o que já foi ganho do que está comprometido no mês.
func TestForecastSplitsWonAndCommitted(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	db.Exec(`DELETE FROM sales_goals`)

	ana := createTestUser(t, db, "ana.prev@fixpay.com.br")
	var stageID, pipelineID int64
	if err := db.QueryRow(
		`SELECT id, pipeline_id FROM pipeline_stages WHERE NOT is_won AND NOT is_lost
		 ORDER BY position LIMIT 1`).Scan(&stageID, &pipelineID); err != nil {
		t.Fatal(err)
	}
	// Probabilidade conhecida para conferir o ponderado.
	if _, err := db.Exec(`UPDATE pipeline_stages SET probability = 50 WHERE id = $1`, stageID); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	period := now.Format("2006-01")
	fechamento := time.Date(now.Year(), now.Month(), 15, 0, 0, 0, 0, time.Local)

	// Um aberto com previsão no mês e um já ganho no mês.
	aberto := &models.Deal{Name: "Aberto", Amount: 10000, PipelineID: pipelineID,
		StageID: stageID, OwnerID: &ana, CloseDate: &fechamento}
	if err := models.CreateDeal(db, aberto); err != nil {
		t.Fatal(err)
	}
	ganho := &models.Deal{Name: "Ganho", Amount: 4000, PipelineID: pipelineID,
		StageID: stageID, OwnerID: &ana}
	if err := models.CreateDeal(db, ganho); err != nil {
		t.Fatal(err)
	}
	if err := models.CloseDeal(db, ganho.ID, true); err != nil {
		t.Fatal(err)
	}

	if err := models.SaveGoal(db, &ana, period, 20000); err != nil {
		t.Fatal(err)
	}

	forecast, err := models.LoadForecast(db, period, 0)
	if err != nil {
		t.Fatal(err)
	}
	if forecast.Won != 4000 {
		t.Fatalf("ganho esperado 4000, veio %v", forecast.Won)
	}
	if forecast.Committed != 10000 {
		t.Fatalf("comprometido esperado 10000, veio %v", forecast.Committed)
	}
	// 10000 × 50% = 5000.
	if forecast.Weighted != 5000 {
		t.Fatalf("ponderado esperado 5000, veio %v", forecast.Weighted)
	}
	if forecast.Projected != 9000 {
		t.Fatalf("projeção esperada 9000 (4000 + 5000), veio %v", forecast.Projected)
	}
	if forecast.TeamGoal != 20000 {
		t.Fatalf("sem meta da equipe, deveria somar as individuais: %v", forecast.TeamGoal)
	}
	if forecast.Gap != 11000 {
		t.Fatalf("faltante esperado 11000, veio %v", forecast.Gap)
	}
	if len(forecast.Rows) != 1 || forecast.Rows[0].Attainment != 20 {
		t.Fatalf("atingimento esperado 20%% (4000/20000), veio %+v", forecast.Rows)
	}
}

// Negócio com fechamento fora do mês não entra na previsão do mês.
func TestForecastIgnoresOtherMonths(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	db.Exec(`DELETE FROM sales_goals`)

	var stageID, pipelineID int64
	db.QueryRow(`SELECT id, pipeline_id FROM pipeline_stages ORDER BY position LIMIT 1`).
		Scan(&stageID, &pipelineID)

	outroMes := time.Now().AddDate(0, 3, 0)
	deal := &models.Deal{Name: "Longe", Amount: 9999, PipelineID: pipelineID,
		StageID: stageID, CloseDate: &outroMes}
	if err := models.CreateDeal(db, deal); err != nil {
		t.Fatal(err)
	}

	forecast, err := models.LoadForecast(db, time.Now().Format("2006-01"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if forecast.Committed != 0 {
		t.Fatalf("negócio de outro mês não deveria entrar: %v", forecast.Committed)
	}
}

func TestSalesAnalytics(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	var stageID, pipelineID int64
	db.QueryRow(`SELECT id, pipeline_id FROM pipeline_stages WHERE NOT is_won AND NOT is_lost
		ORDER BY position LIMIT 1`).Scan(&stageID, &pipelineID)

	// Três negócios: dois ganhos, um perdido.
	for i, won := range []bool{true, true, false} {
		deal := &models.Deal{Name: "N", Amount: float64(1000 * (i + 1)),
			PipelineID: pipelineID, StageID: stageID}
		if err := models.CreateDeal(db, deal); err != nil {
			t.Fatal(err)
		}
		if err := models.CloseDeal(db, deal.ID, won); err != nil {
			t.Fatal(err)
		}
	}

	a, err := models.LoadSalesAnalytics(db, 90, 0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Won != 2 || a.Lost != 1 {
		t.Fatalf("esperava 2 ganhos e 1 perdido, veio %d/%d", a.Won, a.Lost)
	}
	// 2 de 3 fechados.
	if int(a.WinRate) != 66 {
		t.Fatalf("taxa de ganho esperada ~66%%, veio %v", a.WinRate)
	}
	// (1000 + 2000) / 2
	if a.AvgTicket != 1500 {
		t.Fatalf("ticket médio esperado 1500, veio %v", a.AvgTicket)
	}
	if a.WonAmount != 3000 {
		t.Fatalf("receita ganha esperada 3000, veio %v", a.WonAmount)
	}
	if len(a.Funnel) == 0 {
		t.Fatal("o funil deveria trazer as etapas do pipeline")
	}
}
