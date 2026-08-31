package models_test

import (
	"testing"

	"fixpay/fix-crm/models"
)

func TestValidateReport(t *testing.T) {
	r := &models.Report{Name: "Receita por dono", Entity: "negocios",
		Metric: "soma_valor", Dimension: "dono"}
	if err := models.ValidateReport(r); err != nil {
		t.Fatalf("relatório válido foi recusado: %v", err)
	}
	// Sem gráfico informado, cai no padrão.
	if r.Chart != "barras" {
		t.Fatalf("gráfico padrão deveria ser barras, veio %q", r.Chart)
	}
}

func TestValidateReportRejectsBadCombinations(t *testing.T) {
	cases := map[string]*models.Report{
		"sem nome":            {Entity: "negocios", Metric: "contagem", Dimension: "dono"},
		"entidade inválida":   {Name: "x", Entity: "planetas", Metric: "contagem", Dimension: "dono"},
		"métrica inexistente": {Name: "x", Entity: "contatos", Metric: "soma_valor", Dimension: "dono"},
		"agrupamento errado":  {Name: "x", Entity: "contatos", Metric: "contagem", Dimension: "etapa"},
		"gráfico inválido":    {Name: "x", Entity: "negocios", Metric: "contagem", Dimension: "dono", Chart: "3d"},
		"período absurdo": {Name: "x", Entity: "negocios", Metric: "contagem", Dimension: "dono",
			Filters: models.ReportFilters{Days: 99999}},
	}
	for name, r := range cases {
		if err := models.ValidateReport(r); err == nil {
			t.Errorf("%s: deveria ser recusado", name)
		}
	}
}

// O catálogo alimenta a tela de montagem.
func TestReportCatalog(t *testing.T) {
	catalog := models.ReportCatalog()
	if len(catalog) != 5 {
		t.Fatalf("esperava 5 entidades no catálogo, veio %d", len(catalog))
	}
	if catalog[0].Key != "negocios" {
		t.Fatalf("negócios deveria abrir o catálogo, veio %s", catalog[0].Key)
	}
	if catalog[0].Metrics["soma_valor"] == "" {
		t.Fatal("negócios deveria oferecer a soma do valor")
	}
	// Contatos não têm valor para somar.
	for _, entry := range catalog {
		if entry.Key == "contatos" && entry.Metrics["soma_valor"] != "" {
			t.Fatal("contatos não deveriam oferecer soma de valor")
		}
	}
}

// A consulta gerada roda de verdade e soma o que deveria.
func TestRunReportSumsByOwner(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	ana := createTestUser(t, db, "ana.relatorio@fixpay.com.br")
	bruno := createTestUser(t, db, "bruno.relatorio@fixpay.com.br")

	var stageID, pipelineID int64
	if err := db.QueryRow(
		`SELECT id, pipeline_id FROM pipeline_stages ORDER BY position LIMIT 1`).
		Scan(&stageID, &pipelineID); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct {
		owner  int64
		amount float64
	}{{ana, 1000}, {ana, 500}, {bruno, 300}} {
		owner := spec.owner
		if err := models.CreateDeal(db, &models.Deal{
			Name: "Negócio", Amount: spec.amount, PipelineID: pipelineID,
			StageID: stageID, OwnerID: &owner,
		}); err != nil {
			t.Fatal(err)
		}
	}

	report := &models.Report{Name: "Receita por dono", Entity: "negocios",
		Metric: "soma_valor", Dimension: "dono", Chart: "barras"}
	result, err := models.RunReport(db, report)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("esperava 2 donos, veio %+v", result.Rows)
	}
	// Ordenado do maior para o menor.
	if result.Rows[0].Value != 1500 || result.Rows[1].Value != 300 {
		t.Fatalf("valores inesperados: %+v", result.Rows)
	}
	if result.Total != 1800 {
		t.Fatalf("total esperado 1800, veio %v", result.Total)
	}
	if !result.IsMoney {
		t.Fatal("soma de valor deveria ser marcada como dinheiro")
	}
}

// Contagem por estágio agrupa e conta certo.
func TestRunReportCountsByLifecycle(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	for _, stage := range []string{"lead", "lead", "cliente"} {
		if err := models.CreateContact(db, &models.Contact{
			FirstName: "C", LifecycleStage: stage,
		}); err != nil {
			t.Fatal(err)
		}
	}

	result, err := models.RunReport(db, &models.Report{
		Name: "Contatos por estágio", Entity: "contatos",
		Metric: "contagem", Dimension: "estagio", Chart: "pizza",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 || result.Rows[0].Value != 2 {
		t.Fatalf("agrupamento inesperado: %+v", result.Rows)
	}
	if result.IsMoney {
		t.Fatal("contagem não é dinheiro")
	}
}

// O filtro de dono restringe o resultado.
func TestRunReportFiltersByOwner(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	ana := createTestUser(t, db, "ana.filtro@fixpay.com.br")
	bruno := createTestUser(t, db, "bruno.filtro@fixpay.com.br")
	for _, owner := range []int64{ana, ana, bruno} {
		id := owner
		if err := models.CreateContact(db, &models.Contact{FirstName: "C", OwnerID: &id}); err != nil {
			t.Fatal(err)
		}
	}

	result, err := models.RunReport(db, &models.Report{
		Name: "Só da Ana", Entity: "contatos", Metric: "contagem", Dimension: "dono",
		Filters: models.ReportFilters{OwnerID: ana},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0].Value != 2 {
		t.Fatalf("o filtro de dono não foi aplicado: %+v", result.Rows)
	}
}

// Média não soma no total (somar médias não significa nada).
func TestRunReportAverageHasNoTotal(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	var stageID, pipelineID int64
	db.QueryRow(`SELECT id, pipeline_id FROM pipeline_stages ORDER BY position LIMIT 1`).
		Scan(&stageID, &pipelineID)
	for _, amount := range []float64{100, 300} {
		models.CreateDeal(db, &models.Deal{Name: "N", Amount: amount,
			PipelineID: pipelineID, StageID: stageID})
	}

	result, err := models.RunReport(db, &models.Report{
		Name: "Ticket médio", Entity: "negocios", Metric: "media_valor", Dimension: "status",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 0 {
		t.Fatalf("média não deveria acumular total, veio %v", result.Total)
	}
	if len(result.Rows) != 1 || result.Rows[0].Value != 200 {
		t.Fatalf("média esperada 200, veio %+v", result.Rows)
	}
}

// Um relatório inválido nunca chega ao banco.
func TestRunReportRejectsInvalid(t *testing.T) {
	db := testDB(t)

	if _, err := models.RunReport(db, &models.Report{
		Name: "x", Entity: "negocios", Metric: "contagem", Dimension: "cidade",
	}); err == nil {
		t.Fatal("agrupamento de outra entidade deveria ser recusado")
	}
}

// Rótulo vazio ganha um texto legível em vez de sair em branco.
func TestRunReportLabelsEmptyValues(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	if err := models.CreateContact(db, &models.Contact{FirstName: "Sem dono"}); err != nil {
		t.Fatal(err)
	}
	result, err := models.RunReport(db, &models.Report{
		Name: "Por dono", Entity: "contatos", Metric: "contagem", Dimension: "dono",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0].Label != "Sem dono" {
		t.Fatalf("rótulo vazio deveria virar 'Sem dono': %+v", result.Rows)
	}
}

// A conversão de funil conta quem passou por cada etapa e a taxa entre elas.
func TestFunnelConversion(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	// Etapas do pipeline padrão, na ordem.
	stageRows, err := db.Query(`SELECT id, pipeline_id FROM pipeline_stages
		WHERE NOT is_won AND NOT is_lost ORDER BY position LIMIT 2`)
	if err != nil {
		t.Fatal(err)
	}
	type st struct{ id, pipe int64 }
	stages := []st{}
	for stageRows.Next() {
		var s st
		stageRows.Scan(&s.id, &s.pipe)
		stages = append(stages, s)
	}
	stageRows.Close()
	if len(stages) < 2 {
		t.Skip("pipeline de teste sem duas etapas")
	}

	// 4 negócios entram na etapa 1; 2 avançam para a etapa 2; 1 é ganho.
	ids := []int64{}
	for i := 0; i < 4; i++ {
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
	if err := models.CloseDeal(db, ids[0], true); err != nil {
		t.Fatal(err)
	}

	result, err := models.LoadFunnelConversion(db, stages[0].pipe, 90)
	if err != nil {
		t.Fatal(err)
	}

	// Primeira etapa: 4 entraram (100%). Segunda: 2 (50% da anterior). Ganho: 1.
	if result.Rows[0].Value != 4 || result.Rows[0].Percent != 100 {
		t.Fatalf("topo do funil inesperado: %+v", result.Rows[0])
	}
	if result.Rows[1].Value != 2 || result.Rows[1].Percent != 50 {
		t.Fatalf("conversão da segunda etapa inesperada: %+v", result.Rows[1])
	}
	ganho := result.Rows[len(result.Rows)-1]
	if ganho.Label != "Ganho" || ganho.Value != 1 || ganho.Percent != 100 {
		t.Fatalf("fechamento do funil inesperado: %+v (1 ganho, 0 perdidos = 100%%)", ganho)
	}
}

// A duração mede o tempo médio por etapa a partir do histórico.
func TestStageDuration(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	var stageID, pipelineID int64
	db.QueryRow(`SELECT id, pipeline_id FROM pipeline_stages WHERE NOT is_won AND NOT is_lost
		ORDER BY position LIMIT 1`).Scan(&stageID, &pipelineID)

	deal := &models.Deal{Name: "N", Amount: 100, PipelineID: pipelineID, StageID: stageID}
	if err := models.CreateDeal(db, deal); err != nil {
		t.Fatal(err)
	}
	// Entrou na etapa há 10 dias e fechou há 4: passou 6 dias nela.
	if _, err := db.Exec(`UPDATE deal_stage_history SET entered_at = NOW() - INTERVAL '10 days'
		WHERE deal_id = $1`, deal.ID); err != nil {
		t.Fatal(err)
	}
	if err := models.CloseDeal(db, deal.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE deals SET closed_at = NOW() - INTERVAL '4 days'
		WHERE id = $1`, deal.ID); err != nil {
		t.Fatal(err)
	}

	result, err := models.LoadStageDuration(db, pipelineID, 90)
	if err != nil {
		t.Fatal(err)
	}
	// A primeira etapa deve marcar ~6 dias.
	if result.Rows[0].Value < 5.5 || result.Rows[0].Value > 6.5 {
		t.Fatalf("duração esperada ~6 dias, veio %v", result.Rows[0].Value)
	}
}

// O progresso monta a grade mensal com criados, ganhos e perdidos.
func TestDealProgress(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	var stageID, pipelineID int64
	db.QueryRow(`SELECT id, pipeline_id FROM pipeline_stages ORDER BY position LIMIT 1`).
		Scan(&stageID, &pipelineID)

	for _, won := range []bool{true, false} {
		deal := &models.Deal{Name: "N", Amount: 100, PipelineID: pipelineID, StageID: stageID}
		if err := models.CreateDeal(db, deal); err != nil {
			t.Fatal(err)
		}
		if err := models.CloseDeal(db, deal.ID, won); err != nil {
			t.Fatal(err)
		}
	}

	result, err := models.LoadDealProgress(db, 0, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Series) != 3 {
		t.Fatalf("esperava 3 séries (criados/ganhos/perdidos), veio %d", len(result.Series))
	}
	// A grade tem exatamente 6 meses em todas as séries.
	for _, serie := range result.Series {
		if len(serie.Points) != 6 {
			t.Fatalf("série %s deveria ter 6 meses, veio %d", serie.Name, len(serie.Points))
		}
	}
	// O mês atual (último ponto) registra os fechamentos.
	last := len(result.Series[0].Points) - 1
	if result.Series[0].Points[last].Value != 2 { // criados
		t.Fatalf("criados no mês atual deveria ser 2: %+v", result.Series[0].Points[last])
	}
	if result.Series[1].Points[last].Value != 1 || result.Series[2].Points[last].Value != 1 {
		t.Fatalf("ganhos/perdidos no mês atual deveriam ser 1/1")
	}
}

// Um relatório salvo com tipo especializado roda pela mesma porta de execução.
func TestRunReportDispatchesByKind(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)

	report := &models.Report{Name: "Funil", Kind: models.ReportKindConversion,
		Filters: models.ReportFilters{Days: 90}}
	result, err := models.RunReport(db, report)
	if err != nil {
		t.Fatal(err)
	}
	if result.Dimension != "Etapa" {
		t.Fatalf("o dispatch pelo tipo não rodou a conversão: %+v", result)
	}
	// A entidade é forçada para negócios na validação.
	if report.Entity != "negocios" {
		t.Fatalf("tipo especializado deveria fixar a entidade: %s", report.Entity)
	}
}
