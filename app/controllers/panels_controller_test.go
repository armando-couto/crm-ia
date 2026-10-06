package controllers_test

import (
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

func panelRow(id int64, name string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{"id", "name", "shared", "created_by", "created_at", "updated_at"}).
		AddRow(id, name, true, nil, now, now)
}

func TestListPanels(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM dashboards").WillReturnRows(panelRow(1, "Índice geral"))
	mock.ExpectQuery("FROM dashboard_items").
		WillReturnRows(sqlmock.NewRows([]string{
			"dashboard_id", "id", "report_id", "name", "position", "width",
		}).AddRow(int64(1), int64(9), int64(5), "Receita por dono", 0, "meio"))
	mock.ExpectQuery("SELECT workspace_dashboard_id").
		WillReturnRows(sqlmock.NewRows([]string{"workspace_dashboard_id"}).AddRow(int64(1)))

	resp := e.GET("/api/v1/panels").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	panel := resp.Value("data").Array().Value(0).Object()
	panel.Value("name").IsEqual("Índice geral")
	panel.Value("items").Array().Value(0).Object().Value("name").IsEqual("Receita por dono")
	resp.Value("workspace_dashboard_id").IsEqual(1)
}

func TestCreatePanel(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("INSERT INTO dashboards").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(1, now, now))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))

	e.POST("/api/v1/panels").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": "Índice geral"}).
		Expect().Status(iris.StatusCreated).
		JSON().Object().Value("name").IsEqual("Índice geral")
}

func TestCreatePanelRequiresName(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/panels").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": "   "}).
		Expect().Status(iris.StatusBadRequest)
}

// Pendurar um relatório inexistente é recusado antes de gravar.
func TestAddPanelItemRejectsUnknownReport(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM reports").WillReturnError(sqlNoRowsErr())

	e.POST("/api/v1/panels/1/items").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"report_id": 99}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("relatório")
}

// A escolha do painel do espaço de trabalho fica no perfil de quem escolheu.
func TestSetWorkspaceDashboard(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM dashboards").WillReturnRows(panelRow(2, "Vendas"))
	mock.ExpectQuery("FROM dashboard_items").
		WillReturnRows(sqlmock.NewRows([]string{"id", "report_id", "name", "position", "width"}))
	mock.ExpectExec("UPDATE users SET workspace_dashboard_id").
		WillReturnResult(sqlmock.NewResult(0, 1))

	e.PUT("/api/v1/workspace/dashboard").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"dashboard_id": 2}).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("workspace_dashboard_id").IsEqual(2)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPanelsRequirePermission(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 95, Email: "s@exemplo.com.br", Role: models.RoleSeller})

	e.POST("/api/v1/panels").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": "Meu painel"}).
		Expect().Status(iris.StatusForbidden)
}
