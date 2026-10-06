package controllers_test

import (
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/middleware"
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/routes"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

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

	// Estado global entre testes: cache de usuários e contador de tentativas.
	middleware.ResetUserCache()
	services.ResetRateLimiter()

	app := iris.New()
	routes.Register(app)
	return httptest.New(t, app, httptest.LogLevel("disable")), mock, mailer
}

func userRow(id int64, email, hash, role string, active bool) *sqlmock.Rows {
	return userRowInvite(id, email, hash, role, active, false, nil)
}

// userRowInvite monta a linha de usuário incluindo o estado do convite.
func userRowInvite(id int64, email, hash, role string, active, mustChange bool, inviteExpires *time.Time) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "name", "email", "role", "active", "team_id", "team_name",
		"created_at", "updated_at", "must_change_password", "invite_expires_at",
		"password_changed_at", "password_hash",
	}).AddRow(id, "Usuária Teste", email, role, active, nil, "",
		now, now, mustChange, inviteExpires, now.Add(-time.Hour), hash)
}

func TestLoginSuccess(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("minha-senha")
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@exemplo.com.br").
		WillReturnRows(userRow(1, "ana@exemplo.com.br", hash, models.RoleAdmin, true))

	resp := e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "Ana@FixPay.com.br", "password": "minha-senha"}).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("token").String().NotEmpty()
	resp.Value("user").Object().Value("email").IsEqual("ana@exemplo.com.br")
}

func TestLoginWrongPassword(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("senha-correta")
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@exemplo.com.br").
		WillReturnRows(userRow(1, "ana@exemplo.com.br", hash, models.RoleAdmin, true))

	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "ana@exemplo.com.br", "password": "senha-errada"}).
		Expect().Status(iris.StatusUnauthorized)
}

func TestLoginInactiveUser(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("minha-senha")
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@exemplo.com.br").
		WillReturnRows(userRow(1, "ana@exemplo.com.br", hash, models.RoleAdmin, false))

	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "ana@exemplo.com.br", "password": "minha-senha"}).
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

// O middleware lê o usuário do banco a cada requisição (perfil e status atuais).
func TestProtectedRouteWithToken(t *testing.T) {
	e, mock, _ := newTestApp(t)

	user := &models.User{ID: 1, Name: "Ana", Email: "ana@exemplo.com.br", Role: models.RoleSeller}
	token, err := services.GenerateToken(user, utils.JWTSecret)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.id").
		WithArgs(int64(1)).
		WillReturnRows(userRow(1, "ana@exemplo.com.br", "hash", models.RoleSeller, true))

	e.GET("/api/v1/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("email").IsEqual("ana@exemplo.com.br")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminRouteForbiddenForVendedor(t *testing.T) {
	e, _, _ := newTestApp(t)

	token := tokenFor(t, &models.User{ID: 2, Email: "vend@exemplo.com.br", Role: models.RoleSeller})

	e.POST("/api/v1/users").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]string{"name": "Novo", "email": "novo@exemplo.com.br"}).
		Expect().Status(iris.StatusForbidden)
}

// Trocar o perfil no banco vale na requisição seguinte, sem esperar o token expirar.
func TestRoleChangeTakesEffectWithoutNewToken(t *testing.T) {
	e, mock, _ := newTestApp(t)

	user := &models.User{ID: 7, Name: "Bia", Email: "bia@exemplo.com.br", Role: models.RoleAdmin}
	token, _ := services.GenerateToken(user, utils.JWTSecret)

	// O token diz "admin", mas o banco já rebaixou o usuário para seller.
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.id").
		WithArgs(int64(7)).
		WillReturnRows(userRow(7, "bia@exemplo.com.br", "hash", models.RoleSeller, true))

	e.GET("/api/v1/permissions").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden)
}

// Desativar o usuário derruba a sessão mesmo com token válido.
func TestInactiveUserLosesSession(t *testing.T) {
	e, mock, _ := newTestApp(t)

	user := &models.User{ID: 8, Email: "off@exemplo.com.br", Role: models.RoleSeller}
	token, _ := services.GenerateToken(user, utils.JWTSecret)

	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.id").
		WithArgs(int64(8)).
		WillReturnRows(userRow(8, "off@exemplo.com.br", "hash", models.RoleSeller, false))

	e.GET("/api/v1/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusUnauthorized)
}

// Tokens emitidos antes da troca de senha param de valer.
func TestTokenIssuedBeforePasswordChangeIsRejected(t *testing.T) {
	e, mock, _ := newTestApp(t)

	user := &models.User{ID: 9, Email: "old@exemplo.com.br", Role: models.RoleSeller}
	token, _ := services.GenerateToken(user, utils.JWTSecret)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "name", "email", "role", "active", "team_id", "team_name",
		"created_at", "updated_at", "must_change_password", "invite_expires_at",
		"password_changed_at", "password_hash",
	}).AddRow(int64(9), "Velha Sessão", "old@exemplo.com.br", models.RoleSeller, true, nil, "",
		now, now, false, nil, now.Add(time.Hour), "hash")

	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.id").
		WithArgs(int64(9)).
		WillReturnRows(rows)

	e.GET("/api/v1/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusUnauthorized).
		JSON().Object().Value("error").String().Contains("senha")
}

// Com a senha temporária pendente, só /me e /me/permissions ficam liberados.
func TestMustChangePasswordBlocksOtherRoutes(t *testing.T) {
	e, _, _ := newTestApp(t)

	user := &models.User{ID: 10, Email: "novo@exemplo.com.br", Role: models.RoleAdmin, MustChangePassword: true}
	token := tokenFor(t, user)

	e.GET("/api/v1/contacts").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden).
		JSON().Object().Value("must_change_password").IsEqual(true)

	e.GET("/api/v1/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK)
}

func TestCreateUserAsAdminSendsWelcomeEmail(t *testing.T) {
	e, mock, mailer := newTestApp(t)

	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("INSERT INTO users").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(9, now, now))

	e.POST("/api/v1/users").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]string{"name": "Novo Vendedor", "email": "novo@exemplo.com.br", "role": "seller"}).
		Expect().Status(iris.StatusCreated).
		JSON().Object().Value("email").IsEqual("novo@exemplo.com.br")

	if len(mailer.sent) != 1 {
		t.Fatalf("esperado 1 e-mail de boas-vindas, enviados: %d", len(mailer.sent))
	}
}

// Após LoginMaxAttempts erros, o par IP+e-mail é bloqueado com 429.
func TestLoginRateLimit(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("senha-correta")
	for i := 0; i < services.LoginMaxAttempts; i++ {
		mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
			WithArgs("alvo@exemplo.com.br").
			WillReturnRows(userRow(1, "alvo@exemplo.com.br", hash, models.RoleAdmin, true))

		e.POST("/api/v1/auth/login").
			WithJSON(map[string]string{"email": "alvo@exemplo.com.br", "password": "chute-" + strconv.Itoa(i)}).
			Expect().Status(iris.StatusUnauthorized)
	}

	// A senha correta também é recusada enquanto o bloqueio estiver de pé.
	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "alvo@exemplo.com.br", "password": "senha-correta"}).
		Expect().Status(iris.StatusTooManyRequests).
		JSON().Object().Value("error").String().Contains("muitas tentativas")
}

// Login bem-sucedido zera o contador de tentativas.
func TestLoginSuccessClearsRateLimit(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("senha-correta")
	for i := 0; i < services.LoginMaxAttempts-1; i++ {
		mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
			WithArgs("ana@exemplo.com.br").
			WillReturnRows(userRow(1, "ana@exemplo.com.br", hash, models.RoleAdmin, true))
		e.POST("/api/v1/auth/login").
			WithJSON(map[string]string{"email": "ana@exemplo.com.br", "password": "errada"}).
			Expect().Status(iris.StatusUnauthorized)
	}

	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@exemplo.com.br").
		WillReturnRows(userRow(1, "ana@exemplo.com.br", hash, models.RoleAdmin, true))
	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "ana@exemplo.com.br", "password": "senha-correta"}).
		Expect().Status(iris.StatusOK)

	// Contador zerado: erra de novo e ainda recebe 401, não 429.
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@exemplo.com.br").
		WillReturnRows(userRow(1, "ana@exemplo.com.br", hash, models.RoleAdmin, true))
	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "ana@exemplo.com.br", "password": "errada"}).
		Expect().Status(iris.StatusUnauthorized)
}

// Convite vencido: a senha temporária não abre mais o sistema.
func TestLoginRejectsExpiredInvite(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("temporaria")
	expired := time.Now().Add(-24 * time.Hour)
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("convidado@exemplo.com.br").
		WillReturnRows(userRowInvite(11, "convidado@exemplo.com.br", hash, models.RoleSeller, true, true, &expired))

	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "convidado@exemplo.com.br", "password": "temporaria"}).
		Expect().Status(iris.StatusForbidden).
		JSON().Object().Value("error").String().Contains("expirou")
}

// Convite dentro do prazo entra, mas já sinalizando a troca obrigatória.
func TestLoginWithValidInviteAsksForNewPassword(t *testing.T) {
	e, mock, _ := newTestApp(t)

	hash, _ := services.HashPassword("temporaria")
	valid := time.Now().Add(48 * time.Hour)
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("convidado@exemplo.com.br").
		WillReturnRows(userRowInvite(12, "convidado@exemplo.com.br", hash, models.RoleSeller, true, true, &valid))

	e.POST("/api/v1/auth/login").
		WithJSON(map[string]string{"email": "convidado@exemplo.com.br", "password": "temporaria"}).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("must_change_password").IsEqual(true)
}

func TestForgotPasswordAlwaysOK(t *testing.T) {
	e, mock, _ := newTestApp(t)

	// E-mail inexistente: resposta 200 mesmo assim, sem vazar informação.
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("naoexiste@exemplo.com.br").
		WillReturnError(sql.ErrNoRows)

	e.POST("/api/v1/auth/forgot").
		WithJSON(map[string]string{"email": "naoexiste@exemplo.com.br"}).
		Expect().Status(iris.StatusOK)
}

func TestForgotPasswordSendsResetEmail(t *testing.T) {
	e, mock, mailer := newTestApp(t)

	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@exemplo.com.br").
		WillReturnRows(userRow(1, "ana@exemplo.com.br", "hash", models.RoleAdmin, true))
	mock.ExpectExec("INSERT INTO password_resets").
		WillReturnResult(sqlmock.NewResult(1, 1))

	e.POST("/api/v1/auth/forgot").
		WithJSON(map[string]string{"email": "ana@exemplo.com.br"}).
		Expect().Status(iris.StatusOK)

	if len(mailer.sent) != 1 {
		t.Fatalf("esperado 1 e-mail de redefinição, enviados: %d", len(mailer.sent))
	}
}
