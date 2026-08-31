package controllers_test

import (
	"testing"
	"time"

	"fixpay/fix-crm/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

func TestCreateTrackedGoal(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("INSERT INTO tracked_goals").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(1, now, now))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))

	e.POST("/api/v1/tracked-goals").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"kind": "ganho", "metric": "valor", "amount": 200000,
			"start_period": "2026-08", "end_period": "2026-09",
		}).
		Expect().Status(iris.StatusCreated).
		JSON().Object().Value("kind").IsEqual("ganho")
}

func TestCreateTrackedGoalValidates(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	// Progresso sem etapa observada.
	e.POST("/api/v1/tracked-goals").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"kind": "progresso", "amount": 10, "start_period": "2026-08",
		}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("etapa")

	// Término antes do início.
	e.POST("/api/v1/tracked-goals").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"kind": "ganho", "amount": 10, "start_period": "2026-08", "end_period": "2026-01",
		}).
		Expect().Status(iris.StatusBadRequest)
}

func TestTrackedGoalsRequirePermission(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 99, Email: "s@fixpay.com.br", Role: models.RoleSeller})

	// Seller vê as metas…
	if !models.RoleCan(models.RoleSeller, models.PermForecastView) {
		t.Fatal("Seller deveria ver as metas")
	}
	// …mas não cria.
	e.POST("/api/v1/tracked-goals").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"kind": "ganho", "amount": 10, "start_period": "2026-08"}).
		Expect().Status(iris.StatusForbidden)
}
