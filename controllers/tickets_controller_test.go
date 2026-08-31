package controllers_test

import (
	"database/sql"
	"testing"
	"time"

	"fixpay/fix-crm/middleware"
	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

func sqlNoRowsErr() error {
	return sql.ErrNoRows
}

func adminToken(t *testing.T) string {
	t.Helper()
	return tokenFor(t, &models.User{ID: 1, Name: "Admin", Email: "admin@fixpay.com.br", Role: models.RoleAdmin})
}

// tokenFor emite o JWT e deixa o usuário no cache do middleware, que hoje lê
// papel e status do banco a cada requisição.
func tokenFor(t *testing.T, user *models.User) string {
	t.Helper()
	user.Active = true
	token, err := services.GenerateToken(user, utils.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	middleware.CacheUser(user)
	return token
}

func TestCreateTicket(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("INSERT INTO tickets").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(5, now, now))
	// Atividade "Ticket criado" na timeline.
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, now))

	e.POST("/api/v1/tickets").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"subject": "Maquininha não liga", "priority": "alta"}).
		Expect().Status(iris.StatusCreated).
		JSON().Object().Value("subject").IsEqual("Maquininha não liga")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTicketWithoutSubject(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/tickets").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"subject": "  "}).
		Expect().Status(iris.StatusBadRequest)
}

func TestCreateTicketInvalidStatus(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/tickets").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"subject": "Teste", "status": "cancelado"}).
		Expect().Status(iris.StatusBadRequest)
}

func TestTicketsRequireAuth(t *testing.T) {
	e, _, _ := newTestApp(t)
	e.GET("/api/v1/tickets").Expect().Status(iris.StatusUnauthorized)
}

// withWebhookKey configura a chave do webhook e devolve a assinatura esperada
// para os campos informados.
func withWebhookKey(t *testing.T, params map[string]string) string {
	t.Helper()
	originalKey, originalURL := utils.MandrillWebhookKey, utils.MandrillWebhookURL
	utils.MandrillWebhookKey = "chave-do-webhook"
	utils.MandrillWebhookURL = "http://localhost:9000/api/webhooks/mandrill/inbound"
	t.Cleanup(func() {
		utils.MandrillWebhookKey, utils.MandrillWebhookURL = originalKey, originalURL
	})
	return services.MandrillSignature(utils.MandrillWebhookKey, utils.MandrillWebhookURL, params)
}

// O webhook do Mandrill é público (sem token), mas exige assinatura válida:
// o ping de validação vem sem campos.
func TestMandrillWebhookValidationPing(t *testing.T) {
	e, _, _ := newTestApp(t)
	signature := withWebhookKey(t, map[string]string{})

	e.POST("/api/webhooks/mandrill/inbound").
		WithHeader("X-Mandrill-Signature", signature).
		Expect().Status(iris.StatusOK)
}

func TestMandrillWebhookInbound(t *testing.T) {
	e, mock, _ := newTestApp(t)

	now := time.Now()
	// Sem conversa aberta: cria uma nova, sem contato correspondente.
	mock.ExpectQuery("SELECT id FROM conversations").
		WillReturnError(sqlNoRowsErr())
	mock.ExpectQuery("SELECT id FROM contacts WHERE email").
		WillReturnError(sqlNoRowsErr())
	mock.ExpectQuery("INSERT INTO conversations").
		WillReturnRows(sqlmock.NewRows([]string{"id", "last_message_at", "created_at"}).AddRow(3, now, now))
	mock.ExpectQuery("INSERT INTO conversation_messages").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(9, now))
	mock.ExpectExec("UPDATE conversations").
		WillReturnResult(sqlmock.NewResult(0, 1))

	payload := `[{"event":"inbound","msg":{"from_email":"novo@cliente.com","email":"vendas@fixpay.com.br","subject":"Quero contratar","text":"Olá, quero saber mais."}}]`
	signature := withWebhookKey(t, map[string]string{"mandrill_events": payload})

	e.POST("/api/webhooks/mandrill/inbound").
		WithHeader("X-Mandrill-Signature", signature).
		WithFormField("mandrill_events", payload).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("created").IsEqual(1)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
