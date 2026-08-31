package controllers_test

import (
	"testing"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/utils"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

func TestListAuditLogAsAdmin(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM audit_log").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT (.+) FROM audit_log").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "user_name", "action", "entity", "entity_id",
			"summary", "details", "ip", "created_at",
		}).AddRow(1, int64(1), "Admin", models.AuditExport, "contato", nil,
			"exportou 120 contatos em CSV", nil, "10.0.0.1", time.Now()))

	resp := e.GET("/api/v1/audit").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("total").IsEqual(1)
	resp.Value("data").Array().Value(0).Object().Value("summary").
		String().Contains("exportou 120 contatos")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A trilha é restrita: Seller e Manager não enxergam por padrão.
func TestListAuditLogForbiddenForSeller(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 20, Email: "s@fixpay.com.br", Role: models.RoleSeller})

	e.GET("/api/v1/audit").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden)
}

// Exportar contatos precisa deixar rastro de quem levou a base.
func TestExportContactsIsAudited(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM contacts").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.+) FROM contacts").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "first_name", "last_name", "email", "phone", "job_title",
			"lifecycle_stage", "source", "company_id", "company_name", "owner_id",
			"owner_name", "last_activity_at", "created_at", "updated_at",
		}))
	mock.ExpectExec("INSERT INTO audit_log").
		WillReturnResult(sqlmock.NewResult(1, 1))

	e.GET("/api/v1/contacts/export").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Sem a chave configurada, o webhook do Mandrill recusa tudo.
func TestMandrillWebhookRejectsWithoutKey(t *testing.T) {
	e, _, _ := newTestApp(t)

	original := utils.MandrillWebhookKey
	utils.MandrillWebhookKey = ""
	t.Cleanup(func() { utils.MandrillWebhookKey = original })

	e.POST("/api/webhooks/mandrill/inbound").
		WithFormField("mandrill_events", "[]").
		Expect().Status(iris.StatusUnauthorized)
}

func TestMandrillWebhookRejectsBadSignature(t *testing.T) {
	e, _, _ := newTestApp(t)
	withWebhookKey(t, map[string]string{"mandrill_events": "[]"})

	e.POST("/api/webhooks/mandrill/inbound").
		WithHeader("X-Mandrill-Signature", "assinatura-falsa").
		WithFormField("mandrill_events", "[]").
		Expect().Status(iris.StatusUnauthorized)
}

// Assinatura válida para um payload, reaproveitada em outro: precisa falhar.
func TestMandrillWebhookRejectsReplayedSignature(t *testing.T) {
	e, _, _ := newTestApp(t)
	signature := withWebhookKey(t, map[string]string{"mandrill_events": "[]"})

	e.POST("/api/webhooks/mandrill/inbound").
		WithHeader("X-Mandrill-Signature", signature).
		WithFormField("mandrill_events", `[{"event":"inbound"}]`).
		Expect().Status(iris.StatusUnauthorized)
}
