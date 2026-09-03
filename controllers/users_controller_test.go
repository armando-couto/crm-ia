package controllers_test

import (
	"testing"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

// TestResendUserInvite: gera senha nova, marca troca obrigatória e manda o e-mail.
func TestResendUserInvite(t *testing.T) {
	e, mock, mailer := newTestApp(t)
	token := adminToken(t)

	hash, _ := services.HashPassword("antiga")
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.id").
		WithArgs(int64(7)).
		WillReturnRows(userRow(7, "eliseu@fixpay.com.br", hash, models.RoleAdmin, true))
	mock.ExpectExec("UPDATE users SET password_hash").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_log").
		WillReturnResult(sqlmock.NewResult(1, 1))

	e.POST("/api/v1/users/7/resend-invite").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("message").String().Contains("eliseu@fixpay.com.br")

	if len(mailer.sent) != 1 || mailer.sent[0] != "eliseu@fixpay.com.br|Bem-vindo ao Fix CRM" {
		t.Fatalf("e-mail não enviado como esperado: %v", mailer.sent)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Usuário desativado não recebe senha nova: primeiro reative o acesso.
func TestResendUserInviteRecusaInativo(t *testing.T) {
	e, mock, mailer := newTestApp(t)
	token := adminToken(t)

	hash, _ := services.HashPassword("antiga")
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.id").
		WithArgs(int64(8)).
		WillReturnRows(userRow(8, "saiu@fixpay.com.br", hash, models.RoleSeller, false))

	e.POST("/api/v1/users/8/resend-invite").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusBadRequest)

	if len(mailer.sent) != 0 {
		t.Fatalf("não devia enviar e-mail: %v", mailer.sent)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Sem serviço de e-mail a senha não pode ser trocada às cegas.
func TestResendUserInviteSemMailer(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	services.Mail = nil

	hash, _ := services.HashPassword("antiga")
	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.id").
		WithArgs(int64(9)).
		WillReturnRows(userRow(9, "ana@fixpay.com.br", hash, models.RoleSeller, true))

	e.POST("/api/v1/users/9/resend-invite").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusBadRequest)

	// Nenhum UPDATE deve ter acontecido.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Vendedor não reenvia senha de ninguém.
func TestResendUserInviteExigePermissao(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 20, Email: "v@fixpay.com.br", Role: models.RoleSeller})

	e.POST("/api/v1/users/7/resend-invite").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden)
}
