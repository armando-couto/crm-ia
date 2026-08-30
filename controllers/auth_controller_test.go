package controllers_test

import (
	"database/sql"
	"testing"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/routes"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/iris-contrib/httpexpect/v2"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
)

// fakeMailer captura os envios de e-mail nos testes.
type fakeMailer struct {
	sent []string
}

func (f *fakeMailer) Send(toEmail, toName, subject, html string) error {
	f.sent = append(f.sent, toEmail+"|"+subject)
	return nil
}

func newTestApp(t *testing.T) (*httpexpect.Expect, sqlmock.Sqlmock, *fakeMailer) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	utils.DB = db
	utils.JWTSecret = "segredo-de-teste"
	utils.AppURL = "http://localhost:9000"

	mailer := &fakeMailer{}
	services.Mail = mailer
	t.Cleanup(func() { services.Mail = nil })

	app := iris.New()
	routes.Register(app)
	return httptest.New(t, app, httptest.LogLevel("disable")), mock, mailer
}

func userRow(id int64, email, hash, role string, active bool) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{"id", "name", "email", "role", "active", "team_id", "team_name", "created_at", "updated_at", "password_hash"}).
		AddRow(id, "Usuária Teste", email, role, active, nil, "", now, now, hash)
}

func TestLoginSuccess(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("minha-senha")
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@fixpay.com.br").
		WillReturnRows(userRow(1, "ana@fixpay.com.br", hash, models.RoleAdmin, true))

	resp := e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "Ana@FixPay.com.br", "password": "minha-senha"}).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("token").String().NotEmpty()
	resp.Value("user").Object().Value("email").IsEqual("ana@fixpay.com.br")
}

func TestLoginWrongPassword(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("senha-correta")
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@fixpay.com.br").
		WillReturnRows(userRow(1, "ana@fixpay.com.br", hash, models.RoleAdmin, true))

	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "ana@fixpay.com.br", "password": "senha-errada"}).
		Expect().Status(iris.StatusUnauthorized)
}

func TestLoginInactiveUser(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("minha-senha")
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@fixpay.com.br").
		WillReturnRows(userRow(1, "ana@fixpay.com.br", hash, models.RoleAdmin, false))

	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "ana@fixpay.com.br", "password": "minha-senha"}).
		Expect().Status(iris.StatusUnauthorized)
}

func TestLoginMissingFields(t *testing.T) {
	e, _, _ := newTestApp(t)

	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "", "password": ""}).
		Expect().Status(iris.StatusBadRequest)
}

func TestProtectedRouteWithoutToken(t *testing.T) {
	e, _, _ := newTestApp(t)

	e.GET("/api/v1/me").Expect().Status(iris.StatusUnauthorized)
	e.GET("/api/v1/contacts").Expect().Status(iris.StatusUnauthorized)
}

func TestProtectedRouteWithToken(t *testing.T) {
	e, mock, _ := newTestApp(t)

	user := &models.User{ID: 1, Name: "Ana", Email: "ana@fixpay.com.br", Role: models.RoleSeller}
	token, err := services.GenerateToken(user, utils.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.id").
		WithArgs(int64(1)).
		WillReturnRows(userRow(1, "ana@fixpay.com.br", "hash", models.RoleSeller, true))

	e.GET("/api/v1/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("email").IsEqual("ana@fixpay.com.br")
}

func TestAdminRouteForbiddenForVendedor(t *testing.T) {
	e, _, _ := newTestApp(t)

	user := &models.User{ID: 2, Email: "vend@fixpay.com.br", Role: models.RoleSeller}
	token, _ := services.GenerateToken(user, utils.JWTSecret)

	e.POST("/api/v1/users").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]string{"name": "Novo", "email": "novo@fixpay.com.br"}).
		Expect().Status(iris.StatusForbidden)
}

func TestCreateUserAsAdminSendsWelcomeEmail(t *testing.T) {
	e, mock, mailer := newTestApp(t)

	admin := &models.User{ID: 1, Email: "admin@fixpay.com.br", Role: models.RoleAdmin}
	token, _ := services.GenerateToken(admin, utils.JWTSecret)

	now := time.Now()
	mock.ExpectQuery("INSERT INTO users").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(9, now, now))

	e.POST("/api/v1/users").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]string{"name": "Novo Vendedor", "email": "novo@fixpay.com.br", "role": "seller"}).
		Expect().Status(iris.StatusCreated).
		JSON().Object().Value("email").IsEqual("novo@fixpay.com.br")

	if len(mailer.sent) != 1 {
		t.Fatalf("esperado 1 e-mail de boas-vindas, enviados: %d", len(mailer.sent))
	}
}

func TestForgotPasswordAlwaysOK(t *testing.T) {
	e, mock, _ := newTestApp(t)

	// E-mail inexistente: resposta 200 mesmo assim, sem vazar informação.
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("naoexiste@fixpay.com.br").
		WillReturnError(sql.ErrNoRows)

	e.POST("/api/v1/auth/forgot").
		WithJSON(map[string]string{"email": "naoexiste@fixpay.com.br"}).
		Expect().Status(iris.StatusOK)
}

func TestForgotPasswordSendsResetEmail(t *testing.T) {
	e, mock, mailer := newTestApp(t)

	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@fixpay.com.br").
		WillReturnRows(userRow(1, "ana@fixpay.com.br", "hash", models.RoleAdmin, true))
	mock.ExpectExec("INSERT INTO password_resets").
		WillReturnResult(sqlmock.NewResult(1, 1))

	e.POST("/api/v1/auth/forgot").
		WithJSON(map[string]string{"email": "ana@fixpay.com.br"}).
		Expect().Status(iris.StatusOK)

	if len(mailer.sent) != 1 {
		t.Fatalf("esperado 1 e-mail de redefinição, enviados: %d", len(mailer.sent))
	}
}
