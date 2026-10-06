package models

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func stageRow(id, pipelineID int64, name string, won, lost bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "pipeline_id", "name", "position", "probability", "is_won", "is_lost"}).
		AddRow(id, pipelineID, name, 0, 50, won, lost)
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
	mock.ExpectExec("UPDATE deals SET stage_id").
		WithArgs(int64(5), 2, DealAberto, nil, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

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

	mock.ExpectQuery("SELECT (.+) FROM pipeline_stages WHERE id").
		WithArgs(int64(9)).
		WillReturnRows(stageRow(9, 1, "Ganho", true, false))
	// Ao mover para etapa de ganho, o status deve virar "ganho" com closed_at preenchido.
	mock.ExpectExec("UPDATE deals SET stage_id").
		WithArgs(int64(9), 0, DealGanho, sqlmock.AnyArg(), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := MoveDealStage(db, 10, 9, 0); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
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
