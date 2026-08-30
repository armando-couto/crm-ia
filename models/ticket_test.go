package models

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestValidTicketStatus(t *testing.T) {
	for _, s := range TicketStatuses {
		if !ValidTicketStatus(s) {
			t.Errorf("%q deveria ser válido", s)
		}
	}
	if ValidTicketStatus("cancelado") || ValidTicketStatus("") {
		t.Error("status desconhecidos não podem ser válidos")
	}
}

func TestCreateTicketDefaults(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery("INSERT INTO tickets").
		WithArgs("POS travado", "", "aberto", "media", nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(1, now, now))

	ticket := &Ticket{Subject: "POS travado"}
	if err := CreateTicket(db, ticket); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if ticket.Status != "aberto" || ticket.Priority != "media" {
		t.Fatalf("padrões esperados aberto/media, obtidos %s/%s", ticket.Status, ticket.Priority)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Ao mudar para resolvido, o closed_at deve ser preenchido (flag $8 = true).
func TestUpdateTicketClosesResolved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE tickets SET").
		WithArgs("POS travado", "", "resolvido", "media", nil, nil, nil, true, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ticket := &Ticket{ID: 1, Subject: "POS travado", Status: "resolvido", Priority: "media"}
	if err := UpdateTicket(db, ticket); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidMeetingAndProjectAndCall(t *testing.T) {
	if !ValidMeetingStatus("agendada") || ValidMeetingStatus("x") {
		t.Error("validação de status de reunião incorreta")
	}
	if !ValidProjectStatus("ativo") || ValidProjectStatus("x") {
		t.Error("validação de status de projeto incorreta")
	}
	if !ValidCallOutcome("conectada") || ValidCallOutcome("x") {
		t.Error("validação de resultado de chamada incorreta")
	}
}
