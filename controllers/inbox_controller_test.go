package controllers_test

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

// conversationListRow monta a linha do ListConversations (com dono e prévia).
func conversationListRow(id int64, subject string, ownerID any, ownerName string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "subject", "contact_id", "contact_name", "peer_email", "status",
		"owner_id", "owner_name", "unread", "last_message_at", "preview", "created_at",
	}).AddRow(id, subject, nil, "", "cliente@empresa.com.br", "aberta",
		ownerID, ownerName, true, now, "Olá, tudo bem?", now)
}

// conversationRow monta a linha do ConversationByID.
func conversationRow(id int64, subject string, ownerID any, ownerName string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "subject", "contact_id", "contact_name", "peer_email", "status",
		"owner_id", "owner_name", "unread", "last_message_at", "created_at",
	}).AddRow(id, subject, nil, "", "cliente@empresa.com.br", "aberta",
		ownerID, ownerName, false, now, now)
}

func TestListConversationsQueueMineWithCounters(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	// A fila "minhas" filtra pelo dono = usuário logado.
	mock.ExpectQuery("FROM conversations c").
		WithArgs("aberta", int64(1)).
		WillReturnRows(conversationListRow(7, "Proposta Fix Pay", int64(1), "Admin"))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM conversations WHERE unread").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery("FILTER").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"unassigned", "mine", "open", "closed"}).
			AddRow(42, 9, 118, 20))

	resp := e.GET("/api/v1/conversations").
		WithQuery("status", "aberta").WithQuery("queue", "minhas").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("data").Array().Value(0).Object().Value("owner_name").IsEqual("Admin")
	counters := resp.Value("counters").Object()
	counters.Value("unassigned").IsEqual(42)
	counters.Value("mine").IsEqual(9)
	counters.Value("open").IsEqual(118)
	counters.Value("closed").IsEqual(20)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssignConversation(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectExec("UPDATE conversations SET owner_id").
		WithArgs(int64(3), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM conversations c").
		WithArgs(int64(7)).
		WillReturnRows(conversationRow(7, "Proposta Fix Pay", int64(3), "Bia"))

	resp := e.PATCH("/api/v1/conversations/7/owner").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"owner_id": 3}).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("owner_id").IsEqual(3)
	resp.Value("owner_name").IsEqual("Bia")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUnassignConversation(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	// owner_id nulo devolve a conversa para a fila "Não atribuído".
	mock.ExpectExec("UPDATE conversations SET owner_id").
		WithArgs(nil, int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM conversations c").
		WithArgs(int64(7)).
		WillReturnRows(conversationRow(7, "Proposta Fix Pay", nil, ""))

	e.PATCH("/api/v1/conversations/7/owner").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"owner_id": nil}).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("owner_id").IsNull()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddConversationComment(t *testing.T) {
	e, mock, mailer := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("FROM conversations c").
		WithArgs(int64(7)).
		WillReturnRows(conversationRow(7, "Proposta Fix Pay", nil, ""))
	mock.ExpectQuery("INSERT INTO conversation_messages").
		WithArgs(int64(7), "comentario", "", "", "Proposta Fix Pay",
			"Cliente pediu desconto, posso aprovar 10%?", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(99, now))
	mock.ExpectExec("UPDATE conversations").
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp := e.POST("/api/v1/conversations/7/comments").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"body": "  Cliente pediu desconto, posso aprovar 10%?  "}).
		Expect().Status(iris.StatusCreated).JSON().Object()

	resp.Value("direction").IsEqual("comentario")
	resp.Value("body").IsEqual("Cliente pediu desconto, posso aprovar 10%?")

	// Comentário interno jamais dispara e-mail para o contato.
	if len(mailer.sent) != 0 {
		t.Fatalf("comentário interno enviou e-mail: %v", mailer.sent)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAddConversationCommentRequiresBody(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/conversations/7/comments").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"body": "   "}).
		Expect().Status(iris.StatusBadRequest)
}
