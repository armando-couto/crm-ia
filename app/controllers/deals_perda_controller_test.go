package controllers_test

import (
	"testing"

	"github.com/armando-couto/crm-ia/app/models"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

// Com o motivo obrigatório (padrão), perder sem motivo responde 422 e nada é
// gravado: nem pelo fechamento, nem soltando o cartão numa etapa de perda.
func TestPerderNegocioSemMotivo422(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM deals").WillReturnRows(dealRowForecast(5))
	mock.ExpectQuery("FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}))
	e.PATCH("/api/v1/deals/5/close").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"won": false}).
		Expect().Status(iris.StatusUnprocessableEntity).
		JSON().Object().Value("codigo").IsEqual("motivo_obrigatorio")

	mock.ExpectQuery("FROM deals").WillReturnRows(dealRowForecast(5))
	mock.ExpectQuery("FROM pipeline_stages").WillReturnRows(sqlmock.NewRows([]string{
		"id", "pipeline_id", "name", "position", "probability", "is_won", "is_lost", "prazo_dias",
	}).AddRow(int64(6), int64(1), "Perdido", 5, 0, false, true, nil))
	mock.ExpectQuery("FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}))
	e.PATCH("/api/v1/deals/5/stage").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"stage_id": 6, "position": 0}).
		Expect().Status(iris.StatusUnprocessableEntity).
		JSON().Object().Value("codigo").IsEqual("motivo_obrigatorio")

	// Motivo fora da lista configurada também é 422.
	mock.ExpectQuery("FROM deals").WillReturnRows(dealRowForecast(5))
	mock.ExpectQuery("FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}))
	e.PATCH("/api/v1/deals/5/close").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"won": false, "motivo": "Não gostei da cor"}).
		Expect().Status(iris.StatusUnprocessableEntity).
		JSON().Object().Value("codigo").IsEqual("motivo_invalido")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// GET /settings/motivos-perda só exige autenticação: o vendedor lê a lista.
func TestMotivosPerdaPadrao(t *testing.T) {
	e, mock, _ := newTestApp(t)
	mock.ExpectQuery("FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}))
	obj := e.GET("/api/v1/settings/motivos-perda").
		WithHeader("Authorization", "Bearer "+tokenFor(t, &models.User{ID: 91, Email: "v@exemplo.com.br", Role: models.RoleSeller})).
		Expect().Status(iris.StatusOK).JSON().Object()
	obj.Value("exigir").IsEqual(true)
	obj.Value("motivos").Array().Value(0).IsEqual("Preço")
}
