package models

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func stageRow(id, pipelineID int64, name string, won, lost bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "pipeline_id", "name", "position", "probability", "is_won", "is_lost", "prazo_dias"}).
		AddRow(id, pipelineID, name, 0, 50, won, lost, nil)
}

func TestMoveDealStageOpen(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT (.+) FROM pipeline_stages WHERE id").
		WithArgs(int64(5)).
		WillReturnRows(stageRow(5, 1, "Negociação", false, false))
	mock.ExpectQuery("SELECT stage_id, status FROM deals").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"stage_id", "status"}).AddRow(int64(4), DealAberto))
	mock.ExpectExec("UPDATE deals SET stage_id").
		WithArgs(int64(5), 2, DealAberto, nil, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Mudou de etapa: histórico do funil + rastro da mudança. Status igual: sem linha.
	mock.ExpectExec("INSERT INTO deal_stage_history").
		WithArgs(int64(10), int64(5)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO deal_changes").
		WithArgs(int64(10), DealChangeStage, "4", "5", nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := MoveDealStage(db, 10, 5, 2); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMoveDealStageToWon(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	usuario := int64(7)
	mock.ExpectQuery("SELECT (.+) FROM pipeline_stages WHERE id").
		WithArgs(int64(9)).
		WillReturnRows(stageRow(9, 1, "Ganho", true, false))
	mock.ExpectQuery("SELECT stage_id, status FROM deals").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"stage_id", "status"}).AddRow(int64(4), DealAberto))
	// Ao mover para etapa de ganho, o status deve virar "ganho" com closed_at preenchido.
	mock.ExpectExec("UPDATE deals SET stage_id").
		WithArgs(int64(9), 0, DealGanho, sqlmock.AnyArg(), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deal_stage_history").
		WithArgs(int64(10), int64(9)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO deal_changes").
		WithArgs(int64(10), DealChangeStage, "4", "9", &usuario).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO deal_changes").
		WithArgs(int64(10), DealChangeStatus, DealAberto, DealGanho, &usuario).
		WillReturnResult(sqlmock.NewResult(2, 1))

	if err := MoveDealStageBy(db, 10, 9, 0, &usuario); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Mover dentro da mesma etapa (só reordenar) não gera histórico nem rastro.
func TestMoveDealStageSameStage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT (.+) FROM pipeline_stages WHERE id").
		WithArgs(int64(5)).
		WillReturnRows(stageRow(5, 1, "Negociação", false, false))
	mock.ExpectQuery("SELECT stage_id, status FROM deals").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"stage_id", "status"}).AddRow(int64(5), DealAberto))
	mock.ExpectExec("UPDATE deals SET stage_id").
		WithArgs(int64(5), 3, DealAberto, nil, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := MoveDealStage(db, 10, 5, 3); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// UpdateDeal grava uma linha em deal_changes por campo alterado entre valor,
// previsão de fechamento e dono (e nenhuma para o que não mudou).
func TestUpdateDealRecordsChanges(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	antes := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	depois := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	donoAntigo := int64(2)
	donoNovo := int64(3)
	usuario := int64(9)

	mock.ExpectQuery("SELECT amount, close_date, owner_id FROM deals").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"amount", "close_date", "owner_id"}).
			AddRow(1000.0, antes, donoAntigo))
	mock.ExpectExec("UPDATE deals SET name").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deal_changes").
		WithArgs(int64(10), DealChangeAmount, "1000", "1500.5", &usuario).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO deal_changes").
		WithArgs(int64(10), DealChangeCloseDate, "2026-10-10", "2026-10-24", &usuario).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectExec("INSERT INTO deal_changes").
		WithArgs(int64(10), DealChangeOwner, "2", "3", &usuario).
		WillReturnResult(sqlmock.NewResult(3, 1))

	d := &Deal{ID: 10, Name: "Venda", Amount: 1500.5, Currency: "BRL", OwnerID: &donoNovo, CloseDate: &depois}
	if err := UpdateDealBy(db, d, &usuario); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Sem alteração nos campos rastreados, nada vai para deal_changes; limpar a
// previsão de fechamento grava "" como valor novo.
func TestUpdateDealWithoutTrackedChanges(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	antes := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT amount, close_date, owner_id FROM deals").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"amount", "close_date", "owner_id"}).
			AddRow(1000.0, antes, nil))
	mock.ExpectExec("UPDATE deals SET name").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deal_changes").
		WithArgs(int64(10), DealChangeCloseDate, "2026-10-10", "", nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	d := &Deal{ID: 10, Name: "Venda renomeada", Amount: 1000}
	if err := UpdateDeal(db, d); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// CloseDeal move para a etapa de ganho e, agora, grava o histórico de etapas
// (antes só o status mudava e o funil perdia a passagem).
func TestCloseDealWritesHistoryAndChanges(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM deals d").WithArgs(int64(10)).WillReturnRows(dealRow(10, 1, 3, DealAberto))
	mock.ExpectQuery("SELECT id FROM pipeline_stages WHERE pipeline_id = (.+) AND is_won = TRUE").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectExec("UPDATE deals SET status").
		WithArgs(DealGanho, int64(9), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deal_stage_history").
		WithArgs(int64(10), int64(9)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO deal_changes").
		WithArgs(int64(10), DealChangeStage, "3", "9", nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO deal_changes").
		WithArgs(int64(10), DealChangeStatus, DealAberto, DealGanho, nil).
		WillReturnResult(sqlmock.NewResult(2, 1))

	if err := CloseDeal(db, 10, true); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// dealRow monta a linha no formato do dealSelect.
func dealRow(id, pipelineID, stageID int64, status string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "name", "amount", "currency", "pipeline_id", "stage_id", "stage_name",
		"contact_id", "contact_name", "company_id", "company_name", "owner_id", "owner_name",
		"status", "temperature", "forecast_category", "close_date", "position", "closed_at",
		"last_activity_at", "created_at", "updated_at",
	}).AddRow(id, "Negócio", 1000.0, "BRL", pipelineID, stageID, "Proposta",
		nil, "", nil, "", nil, "", status, "", "pipeline", nil, 0, nil, nil, now, now)
}

func TestCreateDealDefaults(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("INSERT INTO deals").
		WillReturnRows(sqlmock.NewRows([]string{"id", "position", "created_at", "updated_at"}).
			AddRow(3, 0, time.Now(), time.Now()))
	// A criação também grava a entrada no histórico de etapas.
	mock.ExpectExec("INSERT INTO deal_stage_history").
		WillReturnResult(sqlmock.NewResult(1, 1))

	d := &Deal{Name: "Venda POS", PipelineID: 1, StageID: 2}
	if err := CreateDeal(db, d); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if d.Currency != "BRL" {
		t.Fatalf("moeda padrão deveria ser BRL, obtido %s", d.Currency)
	}
	if d.Status != DealAberto {
		t.Fatalf("status padrão deveria ser aberto, obtido %s", d.Status)
	}
}

// O kanban calcula dias na etapa e sem atividade a partir das colunas extras,
// caindo para created_at quando o negócio não tem histórico nem atividade.
func TestBoardDealsFillsStageAge(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	criado := now.AddDate(0, 0, -20)
	entrou := now.AddDate(0, 0, -5)
	atividade := now.AddDate(0, 0, -2)
	prazo := 10
	cols := []string{
		"id", "name", "amount", "currency", "pipeline_id", "stage_id", "stage_name",
		"contact_id", "contact_name", "company_id", "company_name", "owner_id", "owner_name",
		"status", "temperature", "forecast_category", "close_date", "position", "closed_at",
		"last_activity_at", "created_at", "updated_at", "stage_entered_at", "prazo_dias",
	}
	rows := sqlmock.NewRows(cols).
		AddRow(int64(1), "Com histórico", 100.0, "BRL", int64(1), int64(2), "Proposta",
			nil, "", nil, "", nil, "", DealAberto, "", "pipeline", nil, 0, nil, atividade, criado, criado, entrou, prazo).
		AddRow(int64(2), "Sem histórico", 100.0, "BRL", int64(1), int64(2), "Proposta",
			nil, "", nil, "", nil, "", DealAberto, "", "pipeline", nil, 1, nil, nil, criado, criado, nil, nil)
	mock.ExpectQuery("stage_entered_at").WithArgs(int64(1)).WillReturnRows(rows)

	rs, err := db.Query(boardSelect+` WHERE d.pipeline_id = $1`, int64(1))
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	var deals []Deal
	for rs.Next() {
		d, err := scanBoardDeal(rs, now)
		if err != nil {
			t.Fatal(err)
		}
		deals = append(deals, *d)
	}
	if len(deals) != 2 {
		t.Fatalf("esperava 2 negócios, veio %d", len(deals))
	}
	if deals[0].DiasNaEtapa != 5 || deals[0].DiasSemAtividade != 2 || deals[0].PrazoDias == nil || *deals[0].PrazoDias != 10 {
		t.Fatalf("negócio com histórico: %+v", deals[0])
	}
	if deals[1].DiasNaEtapa != 20 || deals[1].DiasSemAtividade != 20 || deals[1].PrazoDias != nil ||
		deals[1].StageEnteredAt == nil || !deals[1].StageEnteredAt.Equal(criado) {
		t.Fatalf("negócio sem histórico deveria usar created_at: %+v", deals[1])
	}
}

// A ordenação da lista de negócios vem da URL: whitelist obrigatória.
func TestDealOrderBy(t *testing.T) {
	cases := []struct{ sortBy, sortDir, want string }{
		{"", "", "d.updated_at DESC NULLS LAST"},
		{"amount", "desc", "d.amount DESC NULLS LAST"},
		{"close_date", "asc", "d.close_date ASC NULLS LAST"},
		{"last_activity", "", "last_activity_at DESC NULLS LAST"},
		{"amount; DROP TABLE deals", "asc", "d.updated_at ASC NULLS LAST"},
	}
	for _, c := range cases {
		if got := dealOrderBy(c.sortBy, c.sortDir); got != c.want {
			t.Errorf("dealOrderBy(%q, %q) = %q, esperado %q", c.sortBy, c.sortDir, got, c.want)
		}
	}
}
