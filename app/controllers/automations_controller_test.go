package controllers_test

import (
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

func TestCreateAutomation(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("INSERT INTO automations").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(1, now, now))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))

	e.POST("/api/v1/automations").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name":         "Boas-vindas",
			"trigger_kind": "contato_criado",
			"actions": []map[string]any{
				{"kind": "enviar_email", "config": map[string]any{"subject": "Oi", "body": "Bem-vindo"}},
			},
		}).
		Expect().Status(iris.StatusCreated).
		JSON().Object().Value("name").IsEqual("Boas-vindas")
}

func TestCreateAutomationRejectsUnknownTrigger(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/automations").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name":         "Inválida",
			"trigger_kind": "quando_der_vontade",
			"actions":      []map[string]any{{"kind": "adicionar_nota"}},
		}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("gatilho")
}

func TestCreateAutomationRejectsUnknownAction(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/automations").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name":         "Inválida",
			"trigger_kind": "contato_criado",
			"actions":      []map[string]any{{"kind": "invocar_demonio"}},
		}).
		Expect().Status(iris.StatusBadRequest)
}

// Terminar em espera deixaria a sequência parada sem fazer nada.
func TestCreateAutomationRejectsTrailingWait(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/automations").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name":         "Só espera",
			"trigger_kind": "contato_criado",
			"actions": []map[string]any{
				{"kind": "adicionar_nota", "config": map[string]any{"content": "x"}},
				{"kind": "aguardar", "config": map[string]any{"days": 3}},
			},
		}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("última ação")
}

func TestCreateAutomationRequiresActions(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/automations").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name": "Vazia", "trigger_kind": "contato_criado", "actions": []any{},
		}).
		Expect().Status(iris.StatusBadRequest)
}

// A listagem devolve o catálogo que a tela de montagem usa.
func TestListAutomationsIncludesCatalog(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("FROM automations").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "description", "trigger_kind", "trigger_config", "actions",
			"active", "runs", "last_run_at", "created_by", "created_at", "updated_at",
		}).AddRow(1, "Boas-vindas", "", "contato_criado", []byte(`{}`),
			[]byte(`[{"kind":"enviar_email"}]`), true, 5, now, nil, now, now))
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	resp := e.GET("/api/v1/automations").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("data").Array().Length().IsEqual(1)
	resp.Value("triggers").Object().ContainsKey("contato_criado")
	resp.Value("actions").Object().ContainsKey("enviar_email")
	resp.Value("time_triggers").Array().NotEmpty()
	// Contatos no meio de sequência aparecem por automação.
	resp.Value("pending").Object().Value("1").IsEqual(2)
}

func TestAutomationsForbiddenForSeller(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 70, Email: "s@exemplo.com.br", Role: models.RoleSeller})

	e.GET("/api/v1/automations").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden)

	if models.RoleCan(models.RoleSeller, models.PermAutomationsManage) {
		t.Fatal("Seller não deveria configurar automações por padrão")
	}
	if !models.RoleCan(models.RoleManager, models.PermAutomationsManage) {
		t.Fatal("Manager deveria configurar automações")
	}
}
