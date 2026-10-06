package controllers_test

import (
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"

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

func TestUpdateContactFormForbiddenForSeller(t *testing.T) {
	e, _, _ := newTestApp(t)

	token := tokenFor(t, &models.User{ID: 3, Email: "v@exemplo.com.br", Role: models.RoleSeller})

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

// O Seller não pode acessar rotas protegidas por permissão de configuração.
func TestSellerBlockedByPermission(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 4, Email: "s@exemplo.com.br", Role: models.RoleSeller})

	e.GET("/api/v1/permissions").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden)

	e.POST("/api/v1/properties").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"entity": "deals", "label": "X", "field_type": "texto"}).
		Expect().Status(iris.StatusForbidden)
}

// O Manager pode gerenciar pipelines, mas não usuários (padrão da matriz).
func TestManagerPermissionBoundaries(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 5, Email: "m@exemplo.com.br", Role: models.RoleManager})

	e.POST("/api/v1/users").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": "X", "email": "x@exemplo.com.br"}).
		Expect().Status(iris.StatusForbidden)

	// Pipelines: passa pela permissão (erro de payload, não 403).
	e.POST("/api/v1/pipelines").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": ""}).
		Expect().Status(iris.StatusBadRequest)
}

// O usuário logado consulta as próprias permissões.
func TestMyPermissions(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 6, Email: "s2@exemplo.com.br", Role: models.RoleSeller})

	resp := e.GET("/api/v1/me/permissions").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("role").IsEqual("seller")
	resp.Value("permissions").Object().Value("contacts.view").IsEqual(true)
	resp.Value("permissions").Object().Value("settings.users").IsEqual(false)
}
