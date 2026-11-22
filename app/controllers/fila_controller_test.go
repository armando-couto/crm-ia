package controllers_test

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

// Pedido inválido responde 422 antes de abrir a transação.
func TestResultadoLeadValida(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	casos := []map[string]any{
		{"resultado": "talvez"},
		{"resultado": "retornar"},
		{"resultado": "sem_interesse"},
		{"resultado": "numero_errado"},
		{"resultado": "reuniao", "reuniao": map[string]any{"inicio": "amanhã às 10"}},
	}
	for _, corpo := range casos {
		mock.ExpectQuery("FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}))
		e.POST("/api/v1/leads/7/resultado").
			WithHeader("Authorization", "Bearer "+token).
			WithJSON(corpo).
			Expect().Status(iris.StatusUnprocessableEntity).
			JSON().Object().ContainsKey("error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgendaDisponibilidadeValidaDias(t *testing.T) {
	e, _, _ := newTestApp(t)
	e.GET("/api/v1/agenda/disponibilidade").
		WithHeader("Authorization", "Bearer "+adminToken(t)).
		WithQuery("dias", 30).
		Expect().Status(iris.StatusUnprocessableEntity)
}
