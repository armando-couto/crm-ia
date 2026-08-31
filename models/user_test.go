package models

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		"  Joao@FixPay.com.br ": "joao@fixpay.com.br",
		"ANA@TESTE.COM":         "ana@teste.com",
		"":                      "",
	}
	for in, want := range cases {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, esperado %q", in, got, want)
		}
	}
}

func TestValidRole(t *testing.T) {
	for _, r := range []string{RoleAdmin, RoleManager, RoleSeller} {
		if !ValidRole(r) {
			t.Errorf("%q deveria ser válido", r)
		}
	}
	if ValidRole("root") || ValidRole("") {
		t.Error("papéis desconhecidos não podem ser válidos")
	}
}

func userRows() *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "name", "email", "role", "active", "team_id", "team_name",
		"created_at", "updated_at", "must_change_password", "invite_expires_at",
		"password_changed_at", "password_hash",
	}).AddRow(1, "Ana", "ana@fixpay.com.br", RoleAdmin, true, nil, "",
		now, now, false, nil, now, "hash")
}

func TestUserByEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT (.+) FROM users u (.+) WHERE u.email").
		WithArgs("ana@fixpay.com.br").
		WillReturnRows(userRows())

	// E-mail com maiúsculas deve ser normalizado antes da consulta.
	u, err := UserByEmail(db, "  ANA@FixPay.com.br ")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if u.ID != 1 || u.Email != "ana@fixpay.com.br" {
		t.Fatalf("usuário inesperado: %+v", u)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery("INSERT INTO users").
		WithArgs("Ana", "ana@fixpay.com.br", "hash", RoleSeller, true, nil, false, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(7, now, now))

	u := &User{Name: "Ana", Email: "Ana@FixPay.com.br", PasswordHash: "hash", Role: RoleSeller, Active: true}
	if err := CreateUser(db, u); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if u.ID != 7 {
		t.Fatalf("ID esperado 7, obtido %d", u.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
