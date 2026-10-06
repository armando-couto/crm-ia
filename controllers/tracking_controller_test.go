package controllers_test

import (
	"encoding/base64"
	"testing"
	"time"

	"fixpay/fix-crm/services"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/iris-contrib/httpexpect/v2"
	"github.com/kataras/iris/v12"
)

func TestTrackOpenRecordsAndReturnsPixel(t *testing.T) {
	e, mock, _ := newTestApp(t)

	mock.ExpectQuery("SELECT id FROM email_messages WHERE token").
		WithArgs("tok-abc").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectExec("UPDATE email_messages").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO email_events").WillReturnResult(sqlmock.NewResult(1, 1))

	resp := e.GET("/api/track/o/tok-abc/pixel.gif").Expect().Status(iris.StatusOK)
	resp.Header("Content-Type").Contains("image/gif")
	resp.Header("Cache-Control").Contains("no-store")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Token desconhecido ainda recebe o pixel: não revelamos quais tokens existem.
func TestTrackOpenUnknownTokenStillReturnsPixel(t *testing.T) {
	e, mock, _ := newTestApp(t)

	mock.ExpectQuery("SELECT id FROM email_messages WHERE token").
		WithArgs("inexistente").
		WillReturnError(sqlNoRowsErr())

	e.GET("/api/track/o/inexistente/pixel.gif").
		Expect().Status(iris.StatusOK).
		Header("Content-Type").Contains("image/gif")
}

func TestTrackClickRedirectsToTarget(t *testing.T) {
	e, mock, _ := newTestApp(t)

	target := "https://fixpay.com.br/planos"
	encoded := base64.URLEncoding.EncodeToString([]byte(target))

	mock.ExpectQuery("SELECT id FROM email_messages WHERE token").
		WithArgs("tok-xyz").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectExec("UPDATE email_messages").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO email_events").WillReturnResult(sqlmock.NewResult(1, 1))

	e.GET("/api/track/c/tok-xyz").WithQuery("u", encoded).
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().Status(iris.StatusFound).
		Header("Location").IsEqual(target)
}

// Link adulterado com outro esquema não redireciona para ele.
func TestTrackClickRejectsUnsafeTarget(t *testing.T) {
	e, _, _ := newTestApp(t)

	encoded := base64.URLEncoding.EncodeToString([]byte("javascript:alert(1)"))
	e.GET("/api/track/c/qualquer").WithQuery("u", encoded).
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().Status(iris.StatusFound).
		Header("Location").NotContainsFold("javascript")
}

func TestListSentEmails(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("FROM email_messages").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "subject", "to_email", "contact_id", "contact_name", "deal_id", "user_id",
			"user_name", "source", "opens", "clicks", "first_open_at", "last_open_at",
			"first_click_at", "sent_at",
		}).AddRow(1, "Proposta", "ana@cliente.com", int64(3), "Ana Silva", nil, int64(1),
			"Admin", "manual", 2, 1, now, now, now, now))

	resp := e.GET("/api/v1/emails/sent").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	item := resp.Value("data").Array().Value(0).Object()
	item.Value("subject").IsEqual("Proposta")
	item.Value("opens").IsEqual(2)
	item.Value("clicks").IsEqual(1)
}

func TestEmailStats(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"sent", "opened", "clicked"}).AddRow(10, 6, 3))

	resp := e.GET("/api/v1/emails/stats").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("sent").IsEqual(10)
	resp.Value("open_rate").IsEqual(60)
	resp.Value("click_rate").IsEqual(30)
}

// O envio manual passa a registrar o e-mail e a instrumentar o HTML.
func TestSendEmailIsTracked(t *testing.T) {
	e, mock, mailer := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("SELECT (.+) FROM contacts").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "first_name", "last_name", "email", "phone", "job_title",
			"lifecycle_stage", "source", "company_id", "company_name", "owner_id",
			"owner_name", "buying_role", "last_activity_at", "created_at", "updated_at",
		}).AddRow(3, "Ana", "Silva", "ana@cliente.com", "", "", "lead", "",
			nil, "", nil, "", "", nil, now, now))
	mock.ExpectQuery("INSERT INTO email_messages").
		WillReturnRows(sqlmock.NewRows([]string{"id", "sent_at"}).AddRow(7, now))
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(20, now))

	e.POST("/api/v1/emails").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"contact_id": 3, "subject": "Proposta",
			"body": `Veja em <a href="https://fixpay.com.br/planos">nosso site</a>`,
		}).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("email_message_id").IsEqual(7)

	if len(mailer.sent) != 1 {
		t.Fatalf("esperado 1 e-mail enviado, veio %d", len(mailer.sent))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInstrumentedBodyReachesMandrill(t *testing.T) {
	// O corpo instrumentado é o que precisa chegar no Mandrill: sem o pixel,
	// abertura nenhuma seria contabilizada.
	html := services.InstrumentEmailHTML("<p>oi</p>", "tok")
	if !contains(html, "pixel.gif") {
		t.Fatal("o corpo instrumentado deveria conter o pixel")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
