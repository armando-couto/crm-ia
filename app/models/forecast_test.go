package models_test

import (
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

func TestSaveAndLoadGoals(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	if _, err := db.Exec(`DELETE FROM sales_goals`); err != nil {
		t.Fatal(err)
	}

	ana := createTestUser(t, db, "ana.meta@exemplo.com.br")
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

	ana := createTestUser(t, db, "ana.prev@exemplo.com.br")
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

// A visão por categoria agrupa nos baldes certos e respeita o "excluído".
func TestCategoryForecastBuckets(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	db.Exec(`DELETE FROM sales_goals`)
	db.Exec(`DELETE FROM forecast_submissions`)

	ana := createTestUser(t, db, "ana.cat@exemplo.com.br")
	var stageID, pipelineID int64
	db.QueryRow(`SELECT id, pipeline_id FROM pipeline_stages WHERE NOT is_won AND NOT is_lost
		ORDER BY position LIMIT 1`).Scan(&stageID, &pipelineID)

	now := time.Now()
	fechamento := time.Date(now.Year(), now.Month(), 15, 0, 0, 0, 0, time.Local)
	period := now.Format("2006-01")

	// Um negócio em cada balde, mais um excluído e um ganho.
	for _, spec := range []struct {
		amount   float64
		category string
	}{
		{1000, models.ForecastPipeline},
		{2000, models.ForecastBestCase},
		{4000, models.ForecastCommitted},
		{9999, models.ForecastExcluded},
	} {
		deal := &models.Deal{Name: "N", Amount: spec.amount, PipelineID: pipelineID,
			StageID: stageID, OwnerID: &ana, CloseDate: &fechamento}
		if err := models.CreateDeal(db, deal); err != nil {
			t.Fatal(err)
		}
		deal.ForecastCategory = spec.category
		if err := models.UpdateDeal(db, deal); err != nil {
			t.Fatal(err)
		}
	}
	ganho := &models.Deal{Name: "Ganho", Amount: 3000, PipelineID: pipelineID,
		StageID: stageID, OwnerID: &ana}
	if err := models.CreateDeal(db, ganho); err != nil {
		t.Fatal(err)
	}
	if err := models.CloseDeal(db, ganho.ID, true); err != nil {
		t.Fatal(err)
	}

	if err := models.SaveGoal(db, &ana, period, 10000); err != nil {
		t.Fatal(err)
	}
	if err := models.SaveSubmission(db, ana, period, 7500, "pipeline forte"); err != nil {
		t.Fatal(err)
	}

	forecast, err := models.LoadCategoryForecast(db, period, 0)
	if err != nil {
		t.Fatal(err)
	}
	if forecast.Pipeline != 1000 || forecast.BestCase != 2000 || forecast.Committed != 4000 {
		t.Fatalf("baldes errados: %+v", forecast)
	}
	if forecast.Closed != 3000 {
		t.Fatalf("o ganho deveria contar como fechado: %v", forecast.Closed)
	}
	// O excluído (9999) não aparece em lugar nenhum.
	total := forecast.Pipeline + forecast.BestCase + forecast.Committed + forecast.Closed
	if total != 10000 {
		t.Fatalf("o excluído vazou para os números: %v", total)
	}
	if forecast.Gap != 7000 {
		t.Fatalf("lacuna esperada 7000 (10000 - 3000), veio %v", forecast.Gap)
	}
	if len(forecast.Rows) != 1 || forecast.Rows[0].Submitted != 7500 ||
		forecast.Rows[0].SubmittedNote != "pipeline forte" {
		t.Fatalf("o envio do vendedor deveria aparecer na linha: %+v", forecast.Rows)
	}
	if forecast.Submitted != 7500 {
		t.Fatalf("a soma dos envios deveria ser 7500: %v", forecast.Submitted)
	}
}

// O envio sobrescreve o anterior do mesmo mês (um por pessoa por período).
func TestSubmissionUpsert(t *testing.T) {
	db := testDB(t)
	db.Exec(`DELETE FROM forecast_submissions`)

	ana := createTestUser(t, db, "ana.sub@exemplo.com.br")
	period := time.Now().Format("2006-01")

	if err := models.SaveSubmission(db, ana, period, 5000, "primeira"); err != nil {
		t.Fatal(err)
	}
	if err := models.SaveSubmission(db, ana, period, 8000, "revisada"); err != nil {
		t.Fatal(err)
	}

	sub, err := models.MySubmission(db, ana, period)
	if err != nil {
		t.Fatal(err)
	}
	if sub == nil || sub.Amount != 8000 || sub.Note != "revisada" {
		t.Fatalf("o envio deveria ser sobrescrito: %+v", sub)
	}

	var total int
	db.QueryRow(`SELECT COUNT(*) FROM forecast_submissions WHERE user_id = $1`, ana).Scan(&total)
	if total != 1 {
		t.Fatalf("deveria existir um envio só, veio %d", total)
	}

	// Sem envio no período seguinte, volta nil.
	proximo := time.Now().AddDate(0, 1, 0).Format("2006-01")
	if sub, _ := models.MySubmission(db, ana, proximo); sub != nil {
		t.Fatalf("mês sem envio deveria voltar nil: %+v", sub)
	}
}
