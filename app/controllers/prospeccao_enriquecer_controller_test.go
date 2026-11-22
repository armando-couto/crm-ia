package controllers_test

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/armando-couto/crm-ia/app/middleware"
	"github.com/armando-couto/crm-ia/app/migrations"
	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/routes"
	"github.com/armando-couto/crm-ia/app/services"
	"github.com/armando-couto/crm-ia/app/utils"

	"github.com/iris-contrib/httpexpect/v2"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
	_ "github.com/lib/pq"
)

// As rotas do enriquecimento em lote fazem várias consultas de seleção e de
// uso mensal: são testadas contra Postgres de verdade (pulam sem
// CRMIA_TEST_DSN), com o mesmo lock consultivo dos demais testes de integração.

func enriqAppComBanco(t *testing.T) (*httpexpect.Expect, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("CRMIA_TEST_DSN")
	if dsn == "" {
		t.Skip("CRMIA_TEST_DSN não definido: pulando teste de integração")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("banco de teste inacessível: %v", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`SELECT pg_advisory_lock($1)`, 918273); err != nil {
		t.Fatal(err)
	}
	anterior := utils.DB
	segredo := utils.JWTSecret
	cfgSegredo := utils.Cfg.JWTSecret
	t.Cleanup(func() {
		db.Exec(`DELETE FROM app_settings WHERE key = $1`, models.SettingEnrichment)
		db.Exec(`SELECT pg_advisory_unlock($1)`, 918273)
		db.Close()
		utils.DB, utils.JWTSecret, utils.Cfg.JWTSecret = anterior, segredo, cfgSegredo
	})
	if err := migrations.Run(db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if _, err := db.Exec(`TRUNCATE prospeccao_jobs, prospeccao_itens, prospeccao_listas RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM app_settings WHERE key = $1`, models.SettingEnrichment); err != nil {
		t.Fatal(err)
	}
	utils.DB = db
	utils.JWTSecret = "segredo-de-teste"
	utils.Cfg.JWTSecret = "segredo-de-teste"
	middleware.ResetUserCache()
	services.ResetRateLimiter()
	app := iris.New()
	routes.Register(app)
	return httptest.New(t, app, httptest.LogLevel("disable")), db
}

func enriqListaComItens(t *testing.T, db *sql.DB, n int) int64 {
	t.Helper()
	l := &models.ProspLista{Nome: "Lista de teste", Filtros: models.ProspFiltros{UFs: []string{"CE"}}, QuantidadePedida: n, Status: models.ListaPronta}
	if err := models.CreateProspLista(db, l); err != nil {
		t.Fatal(err)
	}
	itens := make([]models.ProspItem, n)
	for i := range itens {
		base := fmt.Sprintf("%08d0001", 90000+i)
		c := base + models.CNPJDigitosVerificadores(base)
		itens[i] = models.ProspItem{CNPJ: c, RazaoSocial: "EMPRESA " + c, UF: "CE", Porte: models.PorteME}
	}
	if _, err := models.InsertProspItens(db, l.ID, itens); err != nil {
		t.Fatal(err)
	}
	return l.ID
}

func enriqConfigurar(t *testing.T, db *sql.DB, mudar func(*models.EnrichmentSettings)) {
	t.Helper()
	cfg := models.DefaultEnrichmentSettings()
	mudar(&cfg)
	if err := models.SetSetting(db, models.SettingEnrichment, cfg.ParaGravar(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestEnriquecerProspRotas(t *testing.T) {
	e, db := enriqAppComBanco(t)
	// O job guarda criado_por (chave estrangeira): o admin precisa existir.
	var uid int64
	if err := db.QueryRow(`INSERT INTO users (name, email, password_hash, role, active)
		VALUES ('Admin Enriquecer', 'admin.enriquecer@exemplo.com.br', 'x', 'admin', TRUE)
		ON CONFLICT (email) DO UPDATE SET active = TRUE RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM prospeccao_jobs WHERE criado_por = $1`, uid)
		db.Exec(`DELETE FROM users WHERE id = $1`, uid)
	})
	token := tokenFor(t, &models.User{ID: uid, Name: "Admin Enriquecer", Email: "admin.enriquecer@exemplo.com.br", Role: models.RoleAdmin})
	listaID := enriqListaComItens(t, db, 2)
	url := func(sufixo string) string {
		return "/api/v1/prospeccao/listas/" + strconv.FormatInt(listaID, 10) + "/enriquecer" + sufixo
	}
	todos := map[string]any{"selecao": map[string]any{"todos": true}}

	e.POST("/api/v1/prospeccao/listas/999999/enriquecer/estimativa").WithHeader("Authorization", "Bearer "+token).
		WithJSON(todos).Expect().Status(iris.StatusNotFound)
	e.POST(url("/estimativa")).WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"selecao": map[string]any{}}).Expect().Status(iris.StatusUnprocessableEntity)

	// Provedor desligado: a estimativa explica e o pedido é recusado.
	est := e.POST(url("/estimativa")).WithHeader("Authorization", "Bearer "+token).
		WithJSON(todos).Expect().Status(iris.StatusOK).JSON().Object()
	est.Value("selecionados").IsEqual(2)
	est.Value("a_enriquecer").IsEqual(2)
	est.Value("ja_enriquecidos").IsEqual(0)
	est.Value("saldo").IsNull()
	prov := est.Value("provedor").Object()
	prov.Value("codigo").IsEqual("wholead")
	prov.Value("pronto").IsEqual(false)
	prov.Value("motivo").String().NotEmpty()
	est.Value("uso_mes").Object().ContainsKey("limite_centavos")
	e.POST(url("")).WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"selecao": map[string]any{"todos": true}, "custo_confirmado_centavos": 0}).
		Expect().Status(iris.StatusUnprocessableEntity)

	// Conta real configurada (o pedido não consulta o provedor): custo
	// confirmado abaixo da estimativa → 409 custo_mudou com a estimativa nova.
	cifrado, err := utils.Cifrar("segredo")
	if err != nil {
		t.Fatal(err)
	}
	enriqConfigurar(t, db, func(c *models.EnrichmentSettings) {
		c.WhoLead.Habilitado, c.WhoLead.ClientID, c.WhoLead.ClientSecretCifrado = true, "cid", cifrado
		c.WhoLead.BaseURL = "http://127.0.0.1:1" // nunca chamado nestas rotas sem saldo
	})
	r := e.POST(url("")).WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"selecao": map[string]any{"todos": true}, "custo_confirmado_centavos": 80}).
		Expect().Status(iris.StatusConflict).JSON().Object()
	r.Value("codigo").IsEqual(services.CodigoCustoMudou)
	r.Value("estimativa").Object().Value("custo_estimado_centavos").IsEqual(160)

	// Limite mensal menor que a estimativa → 429.
	enriqConfigurar(t, db, func(c *models.EnrichmentSettings) {
		c.WhoLead.Habilitado, c.WhoLead.ClientID, c.WhoLead.ClientSecretCifrado = true, "cid", cifrado
		c.LimiteMensalCentavos = 100
	})
	e.POST(url("")).WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"selecao": map[string]any{"todos": true}, "custo_confirmado_centavos": 160}).
		Expect().Status(iris.StatusTooManyRequests).JSON().Object().Value("codigo").IsEqual("limite_mensal")

	// Dentro do limite: 202 com o job; o segundo pedido encontra o job ativo.
	enriqConfigurar(t, db, func(c *models.EnrichmentSettings) {
		c.WhoLead.Habilitado, c.WhoLead.ClientID, c.WhoLead.ClientSecretCifrado = true, "cid", cifrado
	})
	job := e.POST(url("")).WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"selecao": map[string]any{"todos": true}, "custo_confirmado_centavos": 160}).
		Expect().Status(iris.StatusAccepted).JSON().Object().Value("job").Object()
	job.Value("tipo").IsEqual(models.JobEnriquecer)
	job.Value("status").IsEqual(models.JobPendente)
	job.Value("total").IsEqual(2)
	job.NotContainsKey("parametros")
	ativo := e.POST(url("")).WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"selecao": map[string]any{"todos": true}, "custo_confirmado_centavos": 160}).
		Expect().Status(iris.StatusConflict).JSON().Object()
	ativo.Value("codigo").IsEqual(services.CodigoJobAtivo)
	ativo.Value("job").Object().Value("id").IsEqual(job.Value("id").Raw())

	// Seleção sem nada a enriquecer → 422.
	if _, err := db.Exec(`UPDATE prospeccao_jobs SET status = 'concluido'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE prospeccao_itens SET estado = 'enviado'`); err != nil {
		t.Fatal(err)
	}
	e.POST(url("")).WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"selecao": map[string]any{"todos": true}, "custo_confirmado_centavos": 0}).
		Expect().Status(iris.StatusUnprocessableEntity)
}
