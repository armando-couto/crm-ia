package controllers_test

import (
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

// companyRow monta a linha de empresa no formato lido pelo model
// (colunas base + as de negócio da Fix Pay + as de conta-alvo).
func companyRow(id int64, name string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "name", "domain", "phone", "industry", "city", "state", "owner_id",
		"owner_name", "created_at", "updated_at",
		"ec_number", "economic_group", "cnpj", "accredited_at", "representative",
		"instagram", "products", "machines_count", "is_client", "anticipation_mode",
		"validator", "do_not_disturb",
		"is_target", "target_tier", "target_notes", "target_since",
	}).AddRow(id, name, "", "", "", "", "", nil, "", now, now,
		"", "", "", nil, "", "", []byte("{}"), 0, false, "", false, false,
		false, 0, "", nil)
}

// ===== Contas-alvo =====

func TestSetTargetAccount(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM companies").
		WillReturnRows(companyRow(3, "Mercado Central"))
	mock.ExpectExec("UPDATE companies").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, time.Now()))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("FROM companies").
		WillReturnRows(companyRow(3, "Mercado Central"))

	e.PUT("/api/v1/companies/3/target").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"is_target": true, "target_tier": 1, "target_notes": "conta estratégica"}).
		Expect().Status(iris.StatusOK)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Tier fora de 1–3 é recusado pelo model.
func TestSetTargetAccountRejectsBadTier(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM companies").WillReturnRows(companyRow(3, "Mercado Central"))

	e.PUT("/api/v1/companies/3/target").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"is_target": true, "target_tier": 9}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("prioridade")
}

// ===== Previsão e metas =====

func TestForecastHandler(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM deals").
		WillReturnRows(sqlmock.NewRows([]string{
			"owner_id", "owner_name", "won", "committed", "weighted", "open_deals",
		}).AddRow(int64(1), "Ana", 4000.0, 10000.0, 5000.0, 2))
	mock.ExpectQuery("FROM sales_goals").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount"}).AddRow(int64(1), 20000.0))

	resp := e.GET("/api/v1/forecast").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("period", "2026-09").
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("won").IsEqual(4000)
	resp.Value("projected").IsEqual(9000)
	resp.Value("team_goal").IsEqual(20000)
	resp.Value("rows").Array().Value(0).Object().Value("attainment").IsEqual(20)
}

func TestForecastRejectsBadPeriod(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.GET("/api/v1/forecast").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("period", "setembro").
		Expect().Status(iris.StatusBadRequest)
}

func TestSaveGoalRequiresPermission(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 90, Email: "s@exemplo.com.br", Role: models.RoleSeller})

	e.PUT("/api/v1/goals").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"period": "2026-09", "amount": 1000}).
		Expect().Status(iris.StatusForbidden)

	if !models.RoleCan(models.RoleSeller, models.PermForecastView) {
		t.Fatal("Seller deveria ver a previsão")
	}
	if models.RoleCan(models.RoleSeller, models.PermGoalsManage) {
		t.Fatal("Seller não deveria definir metas")
	}
}

// ===== Espaço de trabalho =====

func TestWorkspaceHandler(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	// Tarefas (atrasadas e de hoje).
	mock.ExpectQuery("FROM tasks").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "type", "due_date", "contact_id", "company_id", "deal_id", "atrasada",
		}).AddRow(1, "Ligar para o cliente", "ligacao", now, nil, nil, nil, true).
			AddRow(2, "Enviar proposta", "email", now, nil, nil, nil, false))
	// Reuniões de hoje.
	mock.ExpectQuery("FROM meetings").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "contact", "starts_at", "contact_id", "company_id", "deal_id",
		}))
	// Negócios parados.
	mock.ExpectQuery("FROM deals").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "stage", "close_date", "amount", "contact_id", "company_id", "deal_id", "dias",
		}).AddRow(5, "Maquininhas", "Proposta", nil, 8000.0, nil, nil, int64(5), 21))
	// Fechando em breve.
	mock.ExpectQuery("FROM deals").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "stage", "close_date", "amount", "contact_id", "company_id", "deal_id",
		}))
	// Leads sem contato.
	mock.ExpectQuery("FROM contacts").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "email", "created_at", "contact_id", "company_id", "deal_id",
		}))
	// Números do topo.
	mock.ExpectQuery("SELECT \\(SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{
			"open_deals", "open_amount", "won_month", "tasks", "meetings", "targets",
		}).AddRow(4, 32000.0, 12000.0, 7, 3, 2))
	// Meta pessoal.
	mock.ExpectQuery("SELECT COALESCE").
		WillReturnRows(sqlmock.NewRows([]string{"goal"}).AddRow(20000.0))

	resp := e.GET("/api/v1/workspace").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("overdue_tasks").Array().Length().IsEqual(1)
	resp.Value("today_tasks").Array().Length().IsEqual(1)
	// O negócio parado explica por que está na fila.
	resp.Value("stale_deals").Array().Value(0).Object().
		Value("reason").IsEqual("sem interação há 21 dias")
	resp.Value("open_amount").IsEqual(32000)
	resp.Value("goal_month").IsEqual(20000)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Ver o dia de outra pessoa exige permissão de gestão.
func TestWorkspaceOfAnotherUserIsRestricted(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 91, Email: "s@exemplo.com.br", Role: "desconhecido"})

	e.GET("/api/v1/workspace").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("user_id", 1).
		Expect().Status(iris.StatusForbidden)
}

// ===== Atividades =====

// O feed geral dispensa o filtro por registro; sem ele, segue obrigatório.
func TestActivitiesFeed(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	e.GET("/api/v1/activities").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusBadRequest)

	mock.ExpectQuery("FROM activities").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "kind", "content", "metadata", "user_id", "user_name",
			"contact_id", "company_id", "deal_id", "ticket_id", "created_at",
			"contact_name", "company_name", "deal_name",
		}).AddRow(1, "ligacao", "Falei com o cliente", nil, int64(1), "Ana",
			int64(3), nil, nil, nil, time.Now(), "Ana Silva", "", ""))

	resp := e.GET("/api/v1/activities").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("feed", true).WithQuery("days", 30).
		Expect().Status(iris.StatusOK).JSON().Object()

	item := resp.Value("data").Array().Value(0).Object()
	item.Value("kind").IsEqual("ligacao")
	item.Value("contact_name").IsEqual("Ana Silva")
}

// ===== Sales Forecast: categorias e envio =====

func TestCategoryForecastHandler(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM deals").
		WillReturnRows(sqlmock.NewRows([]string{
			"owner_id", "owner_name", "pipeline", "best_case", "committed", "closed",
		}).AddRow(int64(1), "Ana", 1000.0, 2000.0, 4000.0, 3000.0))
	mock.ExpectQuery("FROM sales_goals").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount"}).AddRow(int64(1), 10000.0))
	mock.ExpectQuery("FROM forecast_submissions").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "note"}).
			AddRow(int64(1), 7500.0, "pipeline forte"))

	resp := e.GET("/api/v1/forecast/categories").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("period", "2026-09").
		Expect().Status(iris.StatusOK).JSON().Object()

	f := resp.Value("forecast").Object()
	f.Value("closed").IsEqual(3000)
	f.Value("gap").IsEqual(7000)
	f.Value("submitted").IsEqual(7500)
	resp.Value("categories").Object().ContainsKey("melhor_caso")
}

func TestSubmitForecast(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectExec("INSERT INTO forecast_submissions").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("FROM forecast_submissions").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "period", "amount", "note", "updated_at",
		}).AddRow(1, int64(1), time.Now(), 7500.0, "pipeline forte", time.Now()))

	e.PUT("/api/v1/forecast/submission").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"period": "2026-09", "amount": 7500, "note": "pipeline forte"}).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("submission").Object().Value("amount").IsEqual(7500)
}

func TestSubmitForecastRejectsNegative(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.PUT("/api/v1/forecast/submission").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"period": "2026-09", "amount": -50}).
		Expect().Status(iris.StatusBadRequest)
}

func TestSetDealForecastCategoryValidates(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM deals").WillReturnRows(dealRowForecast(5))

	e.PATCH("/api/v1/deals/5/forecast").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"forecast_category": "chute"}).
		Expect().Status(iris.StatusBadRequest)
}

// dealRowForecast monta a linha do negócio no formato do dealSelect.
func dealRowForecast(id int64) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "name", "amount", "currency", "pipeline_id", "stage_id", "stage_name",
		"contact_id", "contact_name", "company_id", "company_name", "owner_id", "owner_name",
		"status", "temperature", "forecast_category", "close_date", "position", "closed_at",
		"last_activity_at", "created_at", "updated_at",
	}).AddRow(id, "Negócio", 1000.0, "BRL", int64(1), int64(1), "Proposta",
		nil, "", nil, "", nil, "", "aberto", "", "pipeline", nil, 0, nil, nil, now, now)
}
