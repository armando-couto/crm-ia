package services

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func nowValue() time.Time {
	return time.Now()
}

const samplePayload = `[
  {
    "event": "inbound",
    "msg": {
      "from_email": "Carlos@BomPreco.com.br",
      "from_name": "Carlos Lima",
      "email": "vendas@exemplo.com.br",
      "subject": "Re: Proposta de adquirência",
      "text": "Podemos fechar com a taxa que conversamos?"
    }
  },
  {
    "event": "click",
    "msg": {}
  }
]`

func TestParseInboundEvents(t *testing.T) {
	events, err := ParseInboundEvents(samplePayload)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("esperados 2 eventos, obtidos %d", len(events))
	}
	if events[0].Event != "inbound" || events[0].Msg.FromEmail != "Carlos@BomPreco.com.br" {
		t.Fatalf("evento inesperado: %+v", events[0])
	}
	if events[0].Msg.Subject != "Re: Proposta de adquirência" {
		t.Fatalf("assunto inesperado: %s", events[0].Msg.Subject)
	}
}

func TestParseInboundEventsInvalid(t *testing.T) {
	if _, err := ParseInboundEvents("nao-e-json"); err == nil {
		t.Fatal("payload inválido deveria falhar")
	}
}

// ProcessInboundEvents deve ignorar eventos que não sejam "inbound" e agrupar
// a mensagem em uma conversa existente do mesmo remetente/assunto.
func TestProcessInboundEvents(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	events, err := ParseInboundEvents(samplePayload)
	if err != nil {
		t.Fatal(err)
	}

	// Conversa aberta já existe para o par (email, assunto normalizado).
	mock.ExpectQuery("SELECT id FROM conversations").
		WithArgs("carlos@bompreco.com.br", "Proposta de adquirência").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery("SELECT (.+) FROM conversations c").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "subject", "contact_id", "name", "peer_email", "status", "owner_id", "owner_name", "unread", "last_message_at", "created_at"}).
			AddRow(7, "Proposta de adquirência", 1, "Carlos Lima", "carlos@bompreco.com.br", "aberta", nil, "", false, nowValue(), nowValue()))
	mock.ExpectQuery("INSERT INTO conversation_messages").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(99, nowValue()))
	mock.ExpectExec("UPDATE conversations").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Atividade na timeline do contato vinculado.
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, nowValue()))

	created, err := ProcessInboundEvents(db, events)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if created != 1 {
		t.Fatalf("esperada 1 mensagem criada (evento click ignorado), obtidas %d", created)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
