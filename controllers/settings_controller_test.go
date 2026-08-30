package controllers_test

import (
	"testing"
	"time"

	"fixpay/fix-crm/models"
	"fixpay/fix-crm/services"
	"fixpay/fix-crm/utils"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

// Sem configuração salva, o formulário volta ao padrão.
func TestGetContactFormDefault(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("SELECT value FROM app_settings").
		WillReturnError(sqlNoRowsErr())

	resp := e.GET("/api/v1/settings/contact-form").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("fields").Array().Length().IsEqual(len(models.DefaultContactForm()))
}

func TestUpdateContactFormRejectsInvalid(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	// Nome oculto: configuração inválida.
	e.PUT("/api/v1/settings/contact-form").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"fields": []map[string]any{
			{"key": "first_name", "visible": false, "required": true},
		}}).
		Expect().Status(iris.StatusBadRequest)
}

func TestUpdateContactFormForbiddenForVendedor(t *testing.T) {
	e, _, _ := newTestApp(t)

	vendedor := &models.User{ID: 3, Email: "v@fixpay.com.br", Role: models.RoleVendedor}
	token, _ := services.GenerateToken(vendedor, utils.JWTSecret)

	e.PUT("/api/v1/settings/contact-form").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"fields": models.DefaultContactForm()}).
		Expect().Status(iris.StatusForbidden)
}

func TestCreateSavedView(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("INSERT INTO saved_views").
		WillReturnRows(sqlmock.NewRows([]string{"id", "position", "created_at"}).AddRow(1, 0, time.Now()))

	e.POST("/api/v1/views").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"entity":  "companies",
			"name":    "Clientes ativos",
			"filters": map[string]any{"criado_dias": 90, "sort": "created_at"},
		}).
		Expect().Status(iris.StatusCreated).
		JSON().Object().Value("name").IsEqual("Clientes ativos")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateSavedViewValidation(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	// Entidade inválida
	e.POST("/api/v1/views").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"entity": "tickets", "name": "X", "filters": map[string]any{}}).
		Expect().Status(iris.StatusBadRequest)

	// Nome vazio
	e.POST("/api/v1/views").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"entity": "contacts", "name": "  ", "filters": map[string]any{}}).
		Expect().Status(iris.StatusBadRequest)
}
