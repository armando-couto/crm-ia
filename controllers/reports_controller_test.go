package controllers_test

import (
	"testing"
	"time"

	"fixpay/fix-crm/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

func reportRow(id int64, name, entity, metric, dimension, chart string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "name", "description", "entity", "metric", "dimension", "filters",
		"chart", "shared", "position", "created_by", "created_at", "updated_at",
	}).AddRow(id, name, "", entity, metric, dimension, []byte(`{"days":90}`),
		chart, true, 0, nil, now, now)
}

func TestListReportsIncludesCatalog(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM reports").
		WillReturnRows(reportRow(1, "Receita por dono", "negocios", "soma_valor", "dono", "barras"))

	resp := e.GET("/api/v1/reports").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("data").Array().Length().IsEqual(1)
	catalog := resp.Value("catalog").Array()
	catalog.NotEmpty()
	catalog.Value(0).Object().Value("key").IsEqual("negocios")
}

func TestCreateReport(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("INSERT INTO reports").
		WillReturnRows(sqlmock.NewRows([]string{"id", "position", "created_at", "updated_at"}).
			AddRow(1, 0, now, now))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))

	e.POST("/api/v1/reports").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name": "Receita por dono", "entity": "negocios",
			"metric": "soma_valor", "dimension": "dono", "chart": "barras",
		}).
		Expect().Status(iris.StatusCreated).
		JSON().Object().Value("name").IsEqual("Receita por dono")
}

// Métrica que não existe na entidade escolhida é recusada.
func TestCreateReportRejectsMetricFromAnotherEntity(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/reports").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name": "Impossível", "entity": "contatos",
			"metric": "soma_valor", "dimension": "dono",
		}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("étrica")
}

func TestCreateReportRejectsUnknownEntity(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/reports").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name": "x", "entity": "galaxias", "metric": "contagem", "dimension": "dono",
		}).
		Expect().Status(iris.StatusBadRequest)
}

// A prévia roda sem salvar nada.
func TestPreviewReport(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM deals").
		WillReturnRows(sqlmock.NewRows([]string{"rotulo", "valor"}).
			AddRow("Ana", 1500.0).AddRow("Bruno", 300.0))

	resp := e.POST("/api/v1/reports/preview").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"entity": "negocios", "metric": "soma_valor", "dimension": "dono",
		}).
		Expect().Status(iris.StatusOK).JSON().Object()

	result := resp.Value("result").Object()
	result.Value("total").IsEqual(1800)
	result.Value("is_money").IsEqual(true)
	result.Value("rows").Array().Length().IsEqual(2)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// O período pode ser trocado na hora de olhar, sem alterar o relatório salvo.
func TestRunReportAcceptsPeriodOverride(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM reports WHERE id").
		WithArgs(int64(1)).
		WillReturnRows(reportRow(1, "Contatos por origem", "contatos", "contagem", "origem", "pizza"))
	mock.ExpectQuery("FROM contacts").
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"rotulo", "valor"}).AddRow("site", 4.0))

	resp := e.GET("/api/v1/reports/1/run").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("days", 7).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("result").Object().Value("total").IsEqual(4)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Seller vê relatórios, mas não cria.
func TestReportPermissions(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 80, Email: "s@fixpay.com.br", Role: models.RoleSeller})

	e.POST("/api/v1/reports").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name": "x", "entity": "negocios", "metric": "contagem", "dimension": "dono",
		}).
		Expect().Status(iris.StatusForbidden)

	if !models.RoleCan(models.RoleSeller, models.PermReportsView) {
		t.Fatal("Seller deveria ver relatórios")
	}
	if models.RoleCan(models.RoleSeller, models.PermReportsManage) {
		t.Fatal("Seller não deveria criar relatórios por padrão")
	}
}
