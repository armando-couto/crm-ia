package controllers_test

import (
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

func sequenceRow(id int64, name string, active bool) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "name", "description", "steps", "active", "dynamic",
		"exit_on_reply", "exit_on_meeting", "owner_id", "owner_name",
		"created_by", "created_at", "updated_at",
	}).AddRow(id, name, "", []byte(`[
		{"kind":"email_auto","delay_days":0,"subject":"Oi","body":"Olá"},
		{"kind":"task_call","delay_days":2,"title":"Ligar"}
	]`), active, false, true, true, nil, "", nil, now, now)
}

func expectMetrics(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("FROM sequence_members").
		WillReturnRows(sqlmock.NewRows([]string{"count", "active"}).AddRow(4, 2))
	mock.ExpectQuery("FROM email_messages").
		WillReturnRows(sqlmock.NewRows([]string{"sent", "opened", "replied"}).AddRow(10, 6, 2))
}

func TestListSequencesWithMetrics(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM sequences").WillReturnRows(sequenceRow(1, "Prospecção", true))
	expectMetrics(mock)

	resp := e.GET("/api/v1/sequences").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()

	seq := resp.Value("data").Array().Value(0).Object()
	seq.Value("enrolled").IsEqual(4)
	seq.Value("open_rate").IsEqual(60)
	seq.Value("reply_rate").IsEqual(20)
	resp.Value("steps").Object().ContainsKey("email_auto")
}

func TestCreateSequenceValidates(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	// E-mail automático sem conteúdo nem modelo.
	e.POST("/api/v1/sequences").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name":  "Vazia",
			"steps": []map[string]any{{"kind": "email_auto"}},
		}).
		Expect().Status(iris.StatusBadRequest)

	// Dinâmica sem etapa manual.
	e.POST("/api/v1/sequences").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name":    "Dinâmica",
			"dynamic": true,
			"steps":   []map[string]any{{"kind": "email_auto", "subject": "a", "body": "b"}},
		}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("manual")
}

func TestEnrollRejectsPausedSequence(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM sequences").WillReturnRows(sequenceRow(1, "Pausada", false))
	expectMetrics(mock)

	e.POST("/api/v1/sequences/1/enroll").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"contact_ids": []int64{5}}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("pausada")
}

func TestSequencesForbiddenForSeller(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 97, Email: "s@exemplo.com.br", Role: models.RoleSeller})

	e.POST("/api/v1/sequences").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": "X"}).
		Expect().Status(iris.StatusForbidden)
}
