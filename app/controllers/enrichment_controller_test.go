package controllers_test

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/app/services/enriquecimento"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kataras/iris/v12"
)

// companyRowCNPJ monta a linha completa de empresa com o CNPJ informado.
func companyRowCNPJ(id int64, name, cnpj string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "name", "domain", "phone", "industry", "city", "state", "owner_id",
		"owner_name", "created_at", "updated_at",
		"ec_number", "economic_group", "cnpj", "accredited_at", "representative",
		"instagram", "products", "machines_count", "is_client", "anticipation_mode",
		"validator", "do_not_disturb",
		"is_target", "target_tier", "target_notes", "target_since",
		"legal_name", "trade_name", "cnae_code", "cnae_description", "legal_nature", "founded_at", "capital",
		"company_size", "registration_status", "registration_status_at", "simples", "mei", "street",
		"street_number", "complement", "neighborhood", "zip", "email", "website", "employees_range",
		"revenue_range", "intel", "enriched_at",
	}).AddRow(id, name, "", "", "", "", "", nil, "", now, now,
		"", "", cnpj, nil, "", "", []byte("{}"), 0, false, "", false, false,
		false, 0, "", nil,
		"", "", "", "", "", nil, nil, "", "", nil, nil, nil, "", "", "", "", "", "", "", "", "",
		[]byte("{}"), nil)
}

const cnpjAcme = "11.222.333/0001-81"

const fixtureBrasilAPIAcme = `{"cnpj":"11222333000181","razao_social":"ACME VENDAS LTDA","nome_fantasia":"ACME",
 "descricao_situacao_cadastral":"ATIVA","data_situacao_cadastral":"2015-03-10","data_inicio_atividade":"2015-03-10",
 "cnae_fiscal":6201501,"cnae_fiscal_descricao":"Desenvolvimento de programas","natureza_juridica":"Sociedade Empresária Limitada",
 "porte":"MICRO EMPRESA","capital_social":100000,"logradouro":"DAS FLORES","descricao_tipo_de_logradouro":"RUA","numero":"100",
 "bairro":"CENTRO","cep":"60000000","uf":"CE","municipio":"FORTALEZA","ddd_telefone_1":"8533334444","email":"contato@acme.com.br",
 "opcao_pelo_simples":true,"opcao_pelo_mei":false,"qsa":[]}`

// servidorReceitaFalso responde a BrasilAPI e deixa o CEP sem resultado.
func servidorReceitaFalso(t *testing.T) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/cnpj/v1/11222333000181" {
			_, _ = w.Write([]byte(fixtureBrasilAPIAcme))
			return
		}
		w.WriteHeader(404)
	}))
	antigos := []string{enriquecimento.BaseBrasilAPI, enriquecimento.BaseOpenCNPJ, enriquecimento.BaseCNPJWS, enriquecimento.BaseViaCEP}
	enriquecimento.BaseBrasilAPI, enriquecimento.BaseOpenCNPJ, enriquecimento.BaseCNPJWS, enriquecimento.BaseViaCEP = ts.URL, ts.URL, ts.URL, ts.URL
	// O cache de sessão do middleware também mora em utils.Cache: só limpamos
	// as chaves do enriquecimento, sem trocar o store.
	utils.Cache.DelPrefixo("enriq:")
	t.Cleanup(func() {
		ts.Close()
		enriquecimento.BaseBrasilAPI, enriquecimento.BaseOpenCNPJ, enriquecimento.BaseCNPJWS, enriquecimento.BaseViaCEP = antigos[0], antigos[1], antigos[2], antigos[3]
		utils.Cache.DelPrefixo("enriq:")
	})
}

func enrichmentCols() []string {
	return []string{"id", "company_id", "provedor", "status", "mensagem", "datasets", "demo", "custo_centavos",
		"perfil", "bruto", "aplicado_em", "campos_aplicados", "user_id", "user_name", "created_at"}
}

// Prévia pela Receita grava a consulta e devolve o diff; o apply grava só os
// campos escolhidos, as fontes, o intel e a atividade na timeline.
func TestEnrichPreviewEApply(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	servidorReceitaFalso(t)
	now := time.Now()

	// --- prévia ---
	mock.ExpectQuery("FROM companies").WillReturnRows(companyRowCNPJ(3, "Acme (digitado)", cnpjAcme))
	mock.ExpectQuery("SELECT value FROM app_settings").WillReturnError(sqlNoRowsErr())
	mock.ExpectQuery("FROM company_enrichments").WillReturnError(sqlNoRowsErr()) // nenhuma consulta recente
	mock.ExpectQuery("INSERT INTO company_enrichments").
		WithArgs(int64(3), "receita", "ok", nil, nil, false, 0, sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(41, now))

	resp := e.POST("/api/v1/companies/3/enrich/preview").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"provedores": []string{"receita"}}).
		Expect().Status(iris.StatusOK).JSON().Object()

	resp.Value("enrichment_ids").Array().IsEqual([]any{41})
	resp.Value("custo_centavos").Number().IsEqual(0)
	consulta := resp.Value("consultas").Array().Value(0).Object()
	consulta.Value("provedor").IsEqual("receita")
	consulta.Value("status").IsEqual("ok")
	consulta.Value("cache").IsEqual(false)
	consulta.Value("demo").IsEqual(false)
	perfil := resp.Value("perfil").Object()
	perfil.Value("provedor").IsEqual("receita")
	perfil.Value("campos").Object().Value("legal_name").IsEqual("ACME VENDAS LTDA")
	perfil.Value("sinais").Array().NotEmpty()

	diffPor := map[string]map[string]any{}
	for _, item := range resp.Value("diff").Array().Iter() {
		d := item.Object().Raw()
		diffPor[d["campo"].(string)] = d
	}
	if d := diffPor["legal_name"]; d == nil || d["sugerido"] != true || d["label"] != "Razão social" || d["provedor"] != "receita" || d["proposto"] != "ACME VENDAS LTDA" {
		t.Fatalf("diff legal_name = %+v", d)
	}
	if d := diffPor["name"]; d == nil || d["sugerido"] != false || d["atual"] != "Acme (digitado)" {
		t.Fatalf("name preenchido não pode ser sugerido: %+v", d)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	// --- apply ---
	perfilJSON, _ := json.Marshal(perfil.Raw())
	linha := func() *sqlmock.Rows {
		return sqlmock.NewRows(enrichmentCols()).AddRow(41, 3, "receita", "ok", "", "", false, 0, perfilJSON, nil, nil, nil, 1, "Admin", now)
	}
	mock.ExpectQuery("FROM companies").WillReturnRows(companyRowCNPJ(3, "Acme (digitado)", cnpjAcme))
	mock.ExpectQuery("FROM company_enrichments").WithArgs(int64(3), sqlmock.AnyArg()).WillReturnRows(linha())
	mock.ExpectExec(`UPDATE companies SET city = \$1, legal_name = \$2, updated_at = NOW\(\) WHERE id = \$3`).
		WithArgs("Fortaleza", "ACME VENDAS LTDA", int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO company_field_sources").
		WithArgs(int64(3), "legal_name", "receita", int64(41), "city", "receita", int64(41)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery("DISTINCT ON").WillReturnRows(linha())
	mock.ExpectExec("UPDATE companies SET intel").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE company_enrichments SET aplicado_em").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO activities").
		WithArgs("sistema", "Dados enriquecidos pela Receita Federal: 2 campos", nil, int64(1), nil, int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(9, now))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("FROM companies").WillReturnRows(companyRowCNPJ(3, "Acme (digitado)", cnpjAcme))

	aplicado := e.POST("/api/v1/companies/3/enrich/apply").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"enrichment_ids": []int64{41}, "campos": []string{"legal_name", "city", "xpto", "name_errado"}}).
		Expect().Status(iris.StatusOK).JSON().Object()
	aplicado.Value("aplicados").Array().IsEqual([]any{"legal_name", "city"})
	aplicado.Value("company").Object().Value("id").IsEqual(3)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Consulta que não pertence à empresa é recusada.
func TestEnrichApplyRejeitaConsultaDeOutraEmpresa(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	mock.ExpectQuery("FROM companies").WillReturnRows(companyRowCNPJ(3, "Acme", cnpjAcme))
	mock.ExpectQuery("FROM company_enrichments").WillReturnRows(sqlmock.NewRows(enrichmentCols()))

	e.POST("/api/v1/companies/3/enrich/apply").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"enrichment_ids": []int64{99}, "campos": []string{"legal_name"}}).
		Expect().Status(iris.StatusUnprocessableEntity).JSON().Object().Value("error").String().Contains("não pertence")
}

// Sem CNPJ válido não há o que enriquecer; fonte paga sem configuração dá 422
// com a mensagem combinada.
func TestEnrichPreviewValidacoes(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)

	mock.ExpectQuery("FROM companies").WillReturnRows(companyRowCNPJ(3, "Acme", ""))
	e.POST("/api/v1/companies/3/enrich/preview").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"provedores": []string{"receita"}}).
		Expect().Status(iris.StatusUnprocessableEntity)

	mock.ExpectQuery("FROM companies").WillReturnRows(companyRowCNPJ(3, "Acme", cnpjAcme))
	mock.ExpectQuery("SELECT value FROM app_settings").WillReturnError(sqlNoRowsErr())
	e.POST("/api/v1/companies/3/enrich/preview").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"provedores": []string{"receita", "bigdatacorp"}}).
		Expect().Status(iris.StatusUnprocessableEntity).JSON().Object().Value("error").String().
		IsEqual("BigDataCorp não configurado. Peça ao administrador para ativar em Configurações → Integrações.")

	e.GET("/api/v1/cnpj/11222333000182").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusUnprocessableEntity)

	ids := make([]int64, 101)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	e.POST("/api/v1/companies/enrich/bulk").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"ids": ids, "provedores": []string{"receita"}}).
		Expect().Status(iris.StatusBadRequest)
	e.POST("/api/v1/companies/enrich/bulk").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"ids": []int64{1}, "provedores": []string{"serasa"}}).
		Expect().Status(iris.StatusBadRequest)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Limite mensal: o custo estimado da fonte paga não cabe → 429.
func TestEnrichPreviewLimiteMensal(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	utils.Cfg.JWTSecret = utils.JWTSecret
	cifrado, err := utils.Cifrar("token-real")
	if err != nil {
		t.Fatal(err)
	}
	settings := `{"limite_mensal_centavos":20000,"bigdatacorp":{"habilitado":true,"token_id":"tid","access_token_cifrado":"` + cifrado + `"}}`

	mock.ExpectQuery("FROM companies").WillReturnRows(companyRowCNPJ(3, "Acme", cnpjAcme))
	mock.ExpectQuery("SELECT value FROM app_settings").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(settings))
	mock.ExpectQuery("FROM company_enrichments").WillReturnRows(sqlmock.NewRows([]string{"provedor", "count", "sum"}).AddRow("bigdatacorp", 700, 19990))

	resp := e.POST("/api/v1/companies/3/enrich/preview").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"provedores": []string{"bigdatacorp"}}).
		Expect().Status(iris.StatusTooManyRequests).JSON().Object()
	resp.Value("error").String().Contains("Limite mensal")
	resp.Value("uso_mes").Object().Value("custo_centavos").IsEqual(19990)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// argumentoSemSegredo confere que o JSON gravado leva o token cifrado, nunca em claro.
type argumentoSemSegredo struct{ claro string }

func (a argumentoSemSegredo) Match(v driver.Value) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	return !strings.Contains(s, a.claro) && strings.Contains(s, `"access_token_cifrado":"`) &&
		!strings.Contains(s, `"access_token":`)
}

// Configurações: o segredo é cifrado ao gravar e nunca volta na resposta.
func TestEnrichmentSettingsNaoVazamSegredo(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	utils.Cfg.JWTSecret = utils.JWTSecret

	mock.ExpectQuery("SELECT value FROM app_settings").WillReturnError(sqlNoRowsErr())
	mock.ExpectExec("INSERT INTO app_settings").
		WithArgs("enrichment", argumentoSemSegredo{claro: "super-secreto"}, int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO audit_log").WillReturnResult(sqlmock.NewResult(1, 1))

	resp := e.PUT("/api/v1/settings/enrichment").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"auto_receita": true, "sobrescrever": "vazios", "limite_mensal_centavos": 15000, "prazo_cache_dias": 30,
			"bigdatacorp": map[string]any{"habilitado": true, "token_id": "tid", "access_token": "super-secreto",
				"datasets_padrao": []string{"basic_data", "relationships"}},
			"serasa": map[string]any{"habilitado": false, "ambiente": "uat"},
		}).
		Expect().Status(iris.StatusOK)
	corpo := resp.Body().Raw()
	if strings.Contains(corpo, "super-secreto") || strings.Contains(corpo, "cifrado") {
		t.Fatalf("resposta vazou o segredo: %s", corpo)
	}
	settings := resp.JSON().Object().Value("settings").Object()
	settings.Value("bigdatacorp").Object().Value("has_secret").IsEqual(true)
	settings.Value("bigdatacorp").Object().Value("datasets_padrao").Array().IsEqual([]any{"basic_data", "relationships"})
	settings.Value("limite_mensal_centavos").IsEqual(15000)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	// Fonte paga habilitada sem credencial nem demo: recusada.
	mock.ExpectQuery("SELECT value FROM app_settings").WillReturnError(sqlNoRowsErr())
	e.PUT("/api/v1/settings/enrichment").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"sobrescrever": "vazios", "prazo_cache_dias": 30,
			"serasa": map[string]any{"habilitado": true, "ambiente": "prod", "client_id": "x"}}).
		Expect().Status(iris.StatusBadRequest)

	// GET devolve catálogo com as três fontes e o uso do mês.
	mock.ExpectQuery("SELECT value FROM app_settings").WillReturnError(sqlNoRowsErr())
	mock.ExpectQuery("FROM company_enrichments").WillReturnRows(sqlmock.NewRows([]string{"provedor", "count", "sum"}))
	get := e.GET("/api/v1/settings/enrichment").
		WithHeader("Authorization", "Bearer "+token).
		Expect().Status(iris.StatusOK).JSON().Object()
	get.Value("fontes").Array().Length().IsEqual(3)
	get.Value("settings").Object().Value("sobrescrever").IsEqual("vazios")
	get.Value("uso_mes").Object().Value("limite_centavos").IsEqual(20000)
}

// Criar empresa com CNPJ válido: a Receita preenche o que está vazio de forma
// síncrona, registrando consulta, fontes, intel e atividade. Sem resposta da
// Receita a criação segue normalmente.
func TestCreateCompanyAutoReceita(t *testing.T) {
	e, mock, _ := newTestApp(t)
	token := adminToken(t)
	servidorReceitaFalso(t)
	now := time.Now()

	mock.ExpectQuery("SELECT value FROM app_settings").WillReturnError(sqlNoRowsErr())
	mock.ExpectQuery("INSERT INTO companies").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(10, now, now))
	mock.ExpectQuery("INSERT INTO activities").WithArgs("sistema", "Empresa criada", nil, int64(1), nil, int64(10), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, now))
	mock.ExpectQuery("INSERT INTO company_enrichments").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(42, now))
	mock.ExpectExec("INSERT INTO company_field_sources").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE company_enrichments SET aplicado_em").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE companies SET intel").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(2, now))
	mock.ExpectQuery("FROM companies").WillReturnRows(companyRowCNPJ(10, "Acme", cnpjAcme))

	e.POST("/api/v1/companies").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": "Acme", "cnpj": cnpjAcme}).
		Expect().Status(iris.StatusCreated).JSON().Object().Value("id").IsEqual(10)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	// Receita sem resultado (CNPJ válido, mas desconhecido do servidor falso):
	// a empresa é criada do mesmo jeito, sem histórico.
	mock.ExpectQuery("SELECT value FROM app_settings").WillReturnError(sqlNoRowsErr())
	mock.ExpectQuery("INSERT INTO companies").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(11, now, now))
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(3, now))
	e.POST("/api/v1/companies").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": "Banco", "cnpj": "00.000.000/0001-91"}).
		Expect().Status(iris.StatusCreated)

	// auto_receita: false no corpo desliga a consulta.
	mock.ExpectQuery("SELECT value FROM app_settings").WillReturnError(sqlNoRowsErr())
	mock.ExpectQuery("INSERT INTO companies").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(12, now, now))
	mock.ExpectQuery("INSERT INTO activities").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(4, now))
	e.POST("/api/v1/companies").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": "Acme 2", "cnpj": cnpjAcme, "auto_receita": false}).
		Expect().Status(iris.StatusCreated)

	// CNPJ com dígitos errados é recusado.
	e.POST("/api/v1/companies").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"name": "Errada", "cnpj": "11.222.333/0001-82"}).
		Expect().Status(iris.StatusBadRequest)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
