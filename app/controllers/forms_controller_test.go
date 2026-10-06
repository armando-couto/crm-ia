package controllers_test

import (
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

// formRow monta a linha do formulário no formato do formSelect.
func formRow(id int64, slug string, active bool) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "slug", "name", "headline", "description", "fields", "submit_label",
		"success_message", "redirect_url", "owner_id", "owner_name", "list_id", "list_name",
		"lifecycle_stage", "source", "active", "submissions", "created_by", "created_at", "updated_at",
	}).AddRow(id, slug, "Contato do site", "Fale conosco", "", []byte(`[
		{"key":"first_name","label":"Nome","type":"texto","required":true},
		{"key":"email","label":"E-mail","type":"email","required":true},
		{"key":"message","label":"Mensagem","type":"textarea"}
	]`), "Enviar", "Recebemos seus dados.", "", nil, "", nil, "",
		"lead", "formulario", active, 3, nil, now, now)
}

func TestCreatePublicForm(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	now := time.Now()
	mock.ExpectQuery("INSERT INTO public_forms").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(1, now, now))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))

	resp := e.POST("/api/v1/forms").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name": "Contato do Site",
			"fields": []map[string]any{
				{"key": "first_name", "label": "Nome", "type": "texto", "required": true},
				{"key": "email", "label": "E-mail", "type": "email", "required": true},
			},
		}).
		Expect().Status(iris.StatusCreated).JSON().Object()

	// O slug sai do nome quando não é informado.
	resp.Value("slug").IsEqual("contato-do-site")
	resp.Value("active").IsEqual(true)
}

func TestCreatePublicFormRequiresEmailField(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/forms").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"name":   "Sem e-mail",
			"fields": []map[string]any{{"key": "nome", "label": "Nome", "type": "texto"}},
		}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("email")
}

// A página pública só recebe o necessário para renderizar o formulário.
func TestGetPublicFormHidesInternals(t *testing.T) {
	e, mock, _ := newTestApp(t)

	mock.ExpectQuery("FROM public_forms").
		WithArgs("contato-do-site").
		WillReturnRows(formRow(1, "contato-do-site", true))

	resp := e.GET("/api/public/forms/contato-do-site").
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("name").IsEqual("Contato do site")
	resp.Value("fields").Array().Length().IsEqual(3)
	resp.NotContainsKey("owner_id")
	resp.NotContainsKey("list_id")
	resp.NotContainsKey("submissions")
}

func TestGetPublicFormPausedIsNotFound(t *testing.T) {
	e, mock, _ := newTestApp(t)

	mock.ExpectQuery("FROM public_forms").
		WithArgs("pausado").
		WillReturnRows(formRow(2, "pausado", false))

	e.GET("/api/public/forms/pausado").Expect().Status(iris.StatusNotFound)
}

func TestSubmitPublicFormCreatesContact(t *testing.T) {
	e, mock, _ := newTestApp(t)
	services.ResetRateLimiter()

	now := time.Now()
	mock.ExpectQuery("FROM public_forms").
		WithArgs("contato-do-site").
		WillReturnRows(formRow(1, "contato-do-site", true))
	// Contato ainda não existe: é criado.
	mock.ExpectQuery("FROM contacts").
		WillReturnError(sqlNoRowsErr())
	mock.ExpectQuery("INSERT INTO contacts").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(11, now, now))
	mock.ExpectExec("INSERT INTO form_submissions").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE public_forms").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(30, now))

	e.POST("/api/public/forms/contato-do-site").
		WithJSON(map[string]string{
			"first_name": "Ana", "email": "ana@cliente.com", "message": "quero uma maquininha",
		}).
		Expect().Status(iris.StatusOK).
		JSON().Object().Value("message").String().Contains("Recebemos")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSubmitPublicFormRequiresRequiredFields(t *testing.T) {
	e, mock, _ := newTestApp(t)
	services.ResetRateLimiter()

	mock.ExpectQuery("FROM public_forms").
		WithArgs("contato-do-site").
		WillReturnRows(formRow(1, "contato-do-site", true))

	e.POST("/api/public/forms/contato-do-site").
		WithJSON(map[string]string{"email": "ana@cliente.com"}).
		Expect().Status(iris.StatusBadRequest).
		JSON().Object().Value("error").String().Contains("Nome")
}

func TestSubmitPublicFormRejectsInvalidEmail(t *testing.T) {
	e, mock, _ := newTestApp(t)
	services.ResetRateLimiter()

	mock.ExpectQuery("FROM public_forms").
		WithArgs("contato-do-site").
		WillReturnRows(formRow(1, "contato-do-site", true))

	e.POST("/api/public/forms/contato-do-site").
		WithJSON(map[string]string{"first_name": "Ana", "email": "sem-arroba"}).
		Expect().Status(iris.StatusBadRequest)
}

// O campo isca responde sucesso mas não grava nada: robô não descobre o bloqueio.
func TestSubmitPublicFormHoneypot(t *testing.T) {
	e, mock, _ := newTestApp(t)
	services.ResetRateLimiter()

	mock.ExpectQuery("FROM public_forms").
		WithArgs("contato-do-site").
		WillReturnRows(formRow(1, "contato-do-site", true))

	e.POST("/api/public/forms/contato-do-site").
		WithJSON(map[string]string{
			"first_name": "Robô", "email": "spam@spam.com", "_gotcha": "preenchido",
		}).
		Expect().Status(iris.StatusOK)

	// Nenhum INSERT esperado além da leitura do formulário.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSubmitPublicFormRateLimited(t *testing.T) {
	e, mock, _ := newTestApp(t)
	services.ResetRateLimiter()

	// Estoura o limite da janela pública para o IP do teste (o handler lê o
	// X-Forwarded-For antes do RemoteAddr, como em produção atrás do proxy).
	const ip = "203.0.113.9"
	for i := 0; i <= services.PublicMaxAttempts; i++ {
		services.PublicRateLimited(ip)
	}

	// Nem chega a consultar o formulário.
	e.POST("/api/public/forms/contato-do-site").
		WithHeader("X-Forwarded-For", ip).
		WithJSON(map[string]string{"first_name": "Ana", "email": "ana@cliente.com"}).
		Expect().Status(iris.StatusTooManyRequests)

	_ = mock
}

func TestFormsForbiddenWithoutPermission(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := tokenFor(t, &models.User{ID: 50, Email: "s@exemplo.com.br", Role: models.RoleSeller})

	e.GET("/api/v1/forms").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden)
}
