package controllers_test

import (
	"testing"

	"github.com/armando-couto/crm-ia/app/models"

	"github.com/kataras/iris/v12"
)

// Regras de permissão e validação da caixa de leads, dos bloqueios e das
// configurações de prospecção que o handler confere antes de ir ao banco.

func vendedorToken(t *testing.T) string {
	t.Helper()
	return tokenFor(t, &models.User{ID: 40, Name: "Vendedor", Email: "vendedor@exemplo.com.br", Role: models.RoleSeller})
}

func TestLeadsVendedorNaoVeTodosNemOutroDono(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := vendedorToken(t)

	e.GET("/api/v1/leads").WithQuery("fila", "todos").
		WithHeader("Authorization", "Bearer "+token).Expect().Status(iris.StatusForbidden)
	e.GET("/api/v1/leads").WithQuery("owner_id", 99).
		WithHeader("Authorization", "Bearer "+token).Expect().Status(iris.StatusForbidden)
	e.GET("/api/v1/leads").WithQuery("fila", "outra").
		WithHeader("Authorization", "Bearer "+token).Expect().Status(iris.StatusUnprocessableEntity)
	e.GET("/api/v1/leads").WithQuery("sort", "valor").
		WithHeader("Authorization", "Bearer "+token).Expect().Status(iris.StatusUnprocessableEntity)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLeadsVendedorNaoDistribui(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := vendedorToken(t)

	e.POST("/api/v1/leads/bulk").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"ids": []int64{1, 2}, "acao": "dono", "owner_id": 40}).
		Expect().Status(iris.StatusForbidden)
	e.POST("/api/v1/leads").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"titulo": "Padaria", "owner_id": 41}).
		Expect().Status(iris.StatusForbidden)
	// DELETE exige leads.assign (rota).
	e.DELETE("/api/v1/leads/1").WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusForbidden)
}

func TestLeadsBulkValida(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := adminToken(t)

	e.POST("/api/v1/leads/bulk").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"ids": []int64{}, "acao": "descartar", "motivo": "x"}).
		Expect().Status(iris.StatusUnprocessableEntity)
	e.POST("/api/v1/leads/bulk").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"ids": []int64{1}, "acao": "apagar"}).
		Expect().Status(iris.StatusUnprocessableEntity)
	e.POST("/api/v1/leads/bulk").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"ids": []int64{1}, "acao": "descartar"}).
		Expect().Status(iris.StatusUnprocessableEntity).
		JSON().Object().Value("error").IsEqual("informe o motivo do descarte")
	ids := make([]int64, models.MaxLeadsBulk+1)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	e.POST("/api/v1/leads/bulk").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"ids": ids, "acao": "temperatura", "temperatura": "quente"}).
		Expect().Status(iris.StatusUnprocessableEntity)
	e.POST("/api/v1/leads/7/descartar").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"motivo": "  "}).
		Expect().Status(iris.StatusUnprocessableEntity)
}

func TestBloqueiosValidam(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	e.GET("/api/v1/bloqueios").WithQuery("tipo", "whatsapp").
		WithHeader("Authorization", "Bearer "+token).Expect().Status(iris.StatusUnprocessableEntity)
	e.POST("/api/v1/bloqueios").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"tipo": "fax", "valor": "1"}).Expect().Status(iris.StatusUnprocessableEntity)
	e.POST("/api/v1/bloqueios").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"tipo": "telefone", "valor": "123"}).Expect().Status(iris.StatusUnprocessableEntity)
	e.POST("/api/v1/bloqueios").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"tipo": "cnpj", "valor": "11.222.333/0001-82", "motivo": "opt_out"}).
		Expect().Status(iris.StatusUnprocessableEntity).JSON().Object().Value("error").IsEqual("CNPJ inválido")
	e.POST("/api/v1/bloqueios").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"tipo": "email", "valor": "a@b.com", "motivo": "porque sim"}).
		Expect().Status(iris.StatusUnprocessableEntity)
	valores := make([]string, models.MaxBloqueiosImportacao+1)
	e.POST("/api/v1/bloqueios/importar").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"tipo": "telefone", "valores": valores}).Expect().Status(iris.StatusUnprocessableEntity)
	e.POST("/api/v1/bloqueios/importar").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"tipo": "telefone", "valores": []string{}}).Expect().Status(iris.StatusUnprocessableEntity)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigProspeccaoRecusaJanelaInvalida(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	// LoadConfigProspeccao lê app_settings; sem a linha valem os padrões.
	mock.ExpectQuery("app_settings").WillReturnError(sqlNoRowsErr())

	e.PUT("/api/v1/settings/prospeccao").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"janela_inicio": 6}).
		Expect().Status(iris.StatusUnprocessableEntity).
		JSON().Object().Value("error").String().Contains("janela de contato")
}

func TestEnviarAoCRMExigeGestao(t *testing.T) {
	e, _, _ := newTestApp(t)
	token := vendedorToken(t)
	e.POST("/api/v1/prospeccao/listas/1/enviar").WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"selecao": map[string]any{"todos": true}}).
		Expect().Status(iris.StatusForbidden)
}
