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
