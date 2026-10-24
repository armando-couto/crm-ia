package models_test

import (
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// Ponta a ponta contra o Postgres (pula sem CRMIA_TEST_DSN): cria negócios,
// move, edita e fecha pelos models, e confere que LoadFunil enxerga o rastro
// (deal_stage_history + deal_changes) que essas operações gravam.
func TestLoadFunilEndToEnd(t *testing.T) {
	db := testDB(t)
	cleanTables(t, db)
	db.Exec(`DELETE FROM sales_goals`)

	ana := createTestUser(t, db, "ana.funil@exemplo.com.br")
	bia := createTestUser(t, db, "bia.funil@exemplo.com.br")

	pipelines, err := models.ListPipelines(db)
	if err != nil || len(pipelines) == 0 {
		t.Fatalf("pipeline semente: %v", err)
	}
	pipe := pipelines[0]
	var normais []models.PipelineStage
	var ganhoStage, perdidoStage models.PipelineStage
	for _, s := range pipe.Stages {
		switch {
		case s.IsWon:
			ganhoStage = s
		case s.IsLost:
			perdidoStage = s
		default:
			normais = append(normais, s)
		}
	}
	if len(normais) < 3 || ganhoStage.ID == 0 || perdidoStage.ID == 0 {
		t.Fatalf("pipeline semente inesperado: %+v", pipe.Stages)
	}
	// Prazo curto na 2ª etapa para provar o estagnado.
	prazo := 5
	normais[1].PrazoDias = &prazo
	if err := models.UpdateStage(db, &normais[1]); err != nil {
		t.Fatal(err)
	}
	if s, _ := models.StageByID(db, normais[1].ID); s.PrazoDias == nil || *s.PrazoDias != 5 {
		t.Fatalf("prazo_dias não foi gravado: %+v", s)
	}

	novo := func(nome string, valor float64, dono int64) *models.Deal {
		d := &models.Deal{Name: nome, Amount: valor, PipelineID: pipe.ID, StageID: normais[0].ID, OwnerID: &dono}
		if err := models.CreateDeal(db, d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	// envelhece empurra criação e histórico para trás, simulando um negócio antigo.
	envelhece := func(id int64, dias int) {
		if _, err := db.Exec(`UPDATE deals SET created_at = NOW() - make_interval(days => $2) WHERE id = $1`, id, dias); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE deal_stage_history SET entered_at = NOW() - make_interval(days => $2) WHERE deal_id = $1`, id, dias); err != nil {
			t.Fatal(err)
		}
	}

	// Antigo (existia no início do período): valor sobe de 1000 para 1500 e segue aberto.
	antigo := novo("Antigo", 1000, ana)
	envelhece(antigo.ID, 60)
	antigo.Amount = 1500
	if err := models.UpdateDealBy(db, antigo, &ana); err != nil {
		t.Fatal(err)
	}
	// Ganho: criado no período, passa por 3 etapas e fecha como ganho.
	ganho := novo("Ganho", 800, ana)
	if err := models.MoveDealStageBy(db, ganho.ID, normais[1].ID, 0, &ana); err != nil {
		t.Fatal(err)
	}
	if err := models.MoveDealStageBy(db, ganho.ID, normais[2].ID, 0, &ana); err != nil {
		t.Fatal(err)
	}
	if err := models.CloseDealBy(db, ganho.ID, true, &ana); err != nil {
		t.Fatal(err)
	}
	// Perdido: criado no período, perdido pelo CloseDeal.
	perdido := novo("Perdido", 300, bia)
	if err := models.CloseDeal(db, perdido.ID, false); err != nil {
		t.Fatal(err)
	}
	// Estagnado: na 2ª etapa há 10 dias (prazo 5) e com a previsão adiada.
	estagnado := novo("Estagnado", 2000, bia)
	if err := models.MoveDealStage(db, estagnado.ID, normais[1].ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE deal_stage_history SET entered_at = NOW() - INTERVAL '10 days'
		WHERE deal_id = $1 AND stage_id = $2`, estagnado.ID, normais[1].ID); err != nil {
		t.Fatal(err)
	}
	primeira := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	depois := primeira.AddDate(0, 0, 14)
	estagnado.CloseDate = &primeira
	if err := models.UpdateDeal(db, estagnado); err != nil {
		t.Fatal(err)
	}
	estagnado.CloseDate = &depois
	if err := models.UpdateDeal(db, estagnado); err != nil {
		t.Fatal(err)
	}
	// Fechado antes do período: não pode aparecer em nada.
	velho := novo("Velho", 9999, ana)
	if err := models.CloseDeal(db, velho.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE deals SET created_at = NOW() - INTERVAL '90 days', closed_at = NOW() - INTERVAL '60 days' WHERE id = $1`, velho.ID); err != nil {
		t.Fatal(err)
	}

	// O rastro: CloseDeal passou a gravar o histórico da etapa de ganho.
	assertCount(t, db, `SELECT COUNT(*) FROM deal_stage_history WHERE deal_id = $1`, ganho.ID, 4)
	assertCount(t, db, `SELECT COUNT(*) FROM deal_changes WHERE deal_id = $1 AND campo = 'amount'`, antigo.ID, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM deal_changes WHERE deal_id = $1 AND campo = 'status'`, ganho.ID, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM deal_changes WHERE deal_id = $1 AND campo = 'stage_id'`, ganho.ID, 3)
	assertCount(t, db, `SELECT COUNT(*) FROM deal_changes WHERE deal_id = $1 AND campo = 'close_date'`, estagnado.ID, 2)
	assertCount(t, db, `SELECT COUNT(*) FROM deal_changes WHERE deal_id = $1 AND user_id IS NOT NULL`, antigo.ID, 1)

	periodo := time.Now().Format("2006-01")
	if err := models.SaveGoal(db, &ana, periodo, 31000); err != nil {
		t.Fatal(err)
	}

	funil, err := models.LoadFunil(db, models.FunilFilter{PipelineID: pipe.ID, Periodo: "mes", Modo: models.FunilModoFluxo})
	if err != nil {
		t.Fatalf("LoadFunil: %v", err)
	}

	if funil.Pipeline.ID != pipe.ID || len(funil.Pipeline.Stages) != len(pipe.Stages) {
		t.Fatalf("pipeline: %+v", funil.Pipeline)
	}
	if funil.Periodo.Rotulo != "Este mês" || funil.Periodo.Modo != "fluxo" {
		t.Fatalf("período: %+v", funil.Periodo)
	}
	r := funil.Resumo
	if r.Abertos != 2 || r.ValorAberto != 3500 || r.Ganhos != 1 || r.ValorGanho != 800 || r.Perdidos != 1 || r.ValorPerdido != 300 {
		t.Fatalf("resumo: %+v", r)
	}
	// Em risco: o estagnado (prazo + adiado) e o antigo (60 dias parado, sem atividade).
	if r.Meta != 31000 || r.Gap != 30200 || r.EmRisco != 2 || r.ValorEmRisco != 3500 {
		t.Fatalf("meta/risco: %+v", r)
	}

	m := funil.Movimentacao
	soma := m.Inicio.Valor + m.Criados.Valor + m.Aumentos.Valor + m.Reducoes.Valor + m.Ganhos.Valor + m.Perdidos.Valor
	if soma != m.Fim.Valor || m.Fim.Valor != 3500 {
		t.Fatalf("waterfall não fecha: %+v", m)
	}
	// O "Antigo" valia 1000 no início (reconstruído pelo rastro) e subiu 500.
	if m.Inicio.Qtd != 1 || m.Inicio.Valor != 1000 || m.Aumentos.Qtd != 1 || m.Aumentos.Valor != 500 {
		t.Fatalf("início/aumentos: %+v", m)
	}
	if m.Criados.Qtd != 3 || m.Adiados.Qtd != 1 || m.Adiados.Valor != 2000 || m.Ganhos.Valor != -800 || m.Perdidos.Valor != -300 {
		t.Fatalf("criados/adiados/ganhos/perdidos: %+v", m)
	}

	// Funil em fluxo: quem se movimentou no período (ganho, perdido, estagnado;
	// o antigo só mudou de valor). Ganho conta em todas as normais; o
	// estagnado chegou à 2ª.
	porEtapa := map[int64]models.FunilEtapa{}
	for _, e := range funil.Funil {
		porEtapa[e.StageID] = e
	}
	if porEtapa[normais[0].ID].Alcancaram != 3 || porEtapa[normais[1].ID].Alcancaram != 2 || porEtapa[normais[2].ID].Alcancaram != 1 {
		t.Fatalf("alcançaram: %+v", funil.Funil)
	}
	if porEtapa[ganhoStage.ID].Alcancaram != 1 || porEtapa[perdidoStage.ID].Alcancaram != 1 {
		t.Fatalf("etapas finais: %+v", funil.Funil)
	}
	if porEtapa[normais[1].ID].Estagnados != 1 || porEtapa[normais[1].ID].AbertosAgora != 1 || porEtapa[normais[0].ID].Estagnados != 1 {
		t.Fatalf("estagnados por etapa: %+v", funil.Funil)
	}

	if len(funil.Negocios) != 4 {
		t.Fatalf("negócios: %d, esperado 4 (o velho fica de fora)", len(funil.Negocios))
	}
	for _, n := range funil.Negocios {
		switch n.ID {
		case estagnado.ID:
			if n.DiasNaEtapa != 10 || n.PrazoDias == nil || *n.PrazoDias != 5 || n.Adiamentos != 1 {
				t.Fatalf("estagnado: %+v", n)
			}
			// Criado agora: ainda não está "sem atividade".
			if !contem(n.Motivos, "estagnado") || !contem(n.Motivos, "adiado") || contem(n.Motivos, "sem_atividade") {
				t.Fatalf("motivos do estagnado: %v", n.Motivos)
			}
			if n.Variacao7d == nil || n.Variacao7d.CloseDateDias != 14 {
				t.Fatalf("variação do estagnado: %+v", n.Variacao7d)
			}
			if n.CloseDate == nil || *n.CloseDate != "2026-11-15" {
				t.Fatalf("close_date: %v", n.CloseDate)
			}
		case ganho.ID:
			if n.Status != "ganho" || n.StageID != ganhoStage.ID || len(n.Motivos) != 0 || n.Variacao7d == nil || n.Variacao7d.Etapa == "" {
				t.Fatalf("ganho: %+v", n)
			}
		case antigo.ID:
			if n.Variacao7d == nil || n.Variacao7d.Valor != 500 || n.OwnerName != "ana.funil" {
				t.Fatalf("antigo: %+v", n)
			}
		}
	}

	if len(funil.PorDono) != 2 || funil.PorDono[0].Nome != "ana.funil" || funil.PorDono[0].Meta != 31000 || funil.PorDono[0].Ganhos != 1 {
		t.Fatalf("por dono: %+v", funil.PorDono)
	}

	// Coorte: só os criados no período (o antigo sai) e nada existia no início.
	coorte, err := models.LoadFunil(db, models.FunilFilter{PipelineID: pipe.ID, Periodo: "30", Modo: models.FunilModoCoorte})
	if err != nil {
		t.Fatal(err)
	}
	if len(coorte.Negocios) != 3 || coorte.Movimentacao.Inicio.Qtd != 0 || coorte.Resumo.Abertos != 1 {
		t.Fatalf("coorte: %d negócios, %+v", len(coorte.Negocios), coorte.Movimentacao)
	}

	// Filtro por dono: a Bia só tem o perdido e o estagnado.
	daBia, err := models.LoadFunil(db, models.FunilFilter{PipelineID: pipe.ID, Periodo: "30", OwnerID: &bia})
	if err != nil {
		t.Fatal(err)
	}
	if len(daBia.Negocios) != 2 || daBia.Resumo.Perdidos != 1 || daBia.Resumo.Meta != 0 {
		t.Fatalf("funil da Bia: %+v", daBia.Resumo)
	}

	// Kanban: o selo de tempo na etapa vem preenchido.
	board, err := models.BoardDeals(db, models.BoardFilter{PipelineID: pipe.ID, ClosedDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range board {
		if d.ID == estagnado.ID && (d.DiasNaEtapa != 10 || d.PrazoDias == nil || *d.PrazoDias != 5 || d.StageEnteredAt == nil) {
			t.Fatalf("board do estagnado: %+v", d)
		}
	}

	if _, err := models.LoadFunil(db, models.FunilFilter{PipelineID: pipe.ID, Periodo: "15"}); err != models.ErrFunilPeriodoInvalido {
		t.Fatalf("período inválido deveria falhar, veio %v", err)
	}
}

func contem(lista []string, s string) bool {
	for _, v := range lista {
		if v == s {
			return true
		}
	}
	return false
}
