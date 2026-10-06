package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/provisioner/internal/config"
	"github.com/armando-couto/crm-ia/provisioner/internal/tenant"
)

func servidor(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DOCKER_FAKE_LOG", filepath.Join(dir, "docker.log"))
	t.Setenv("DOCKER_FAKE_SLUG", "acme")
	fake, _ := filepath.Abs("../tenant/testdata/docker")
	s, err := tenant.NovoServico(tenant.Config{Config: config.Config{
		Dominio: "crmia.test", RedePublica: "crmia_public", TenantsDir: filepath.Join(dir, "t"), BackupsDir: filepath.Join(dir, "b"),
		TemplatePath: filepath.Join("..", "..", "..", "infra", "tenant", "docker-compose.tenant.tmpl"),
		ImagemApp:    "crmia/app", ImagemDB: "postgres:16-alpine", ImagemRedis: "redis:7-alpine", ImagemSuspenso: "crmia/suspenso:latest",
		VersaoPadrao: "1.0.0", CPULimite: "1", MemLimite: "1g", DBMemLimite: "1g", RedisMemLimite: "128m", EsperaHealthcheck: time.Millisecond,
	}, Docker: fake})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Nova(s, "token-de-teste").Rotas())
	t.Cleanup(srv.Close)
	return srv
}

func chamar(t *testing.T, srv *httptest.Server, metodo, caminho, token, corpo string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(metodo, srv.URL+caminho, strings.NewReader(corpo))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestExigeToken(t *testing.T) {
	srv := servidor(t)
	if r := chamar(t, srv, "GET", "/tenants", "", ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sem token = %d", r.StatusCode)
	}
	if r := chamar(t, srv, "GET", "/tenants", "errado", ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("token errado = %d", r.StatusCode)
	}
	if r := chamar(t, srv, "GET", "/health", "", ""); r.StatusCode != http.StatusOK {
		t.Fatalf("health precisa ser público: %d", r.StatusCode)
	}
}

func TestCicloPelaAPI(t *testing.T) {
	srv := servidor(t)
	r := chamar(t, srv, "POST", "/tenants", "token-de-teste", `{"slug":"acme","nome":"ACME","cnpj":"11222333000181","usuarios_max":5}`)
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("criar = %d", r.StatusCode)
	}
	if r := chamar(t, srv, "POST", "/tenants", "token-de-teste", `{"slug":"PAINEL"}`); r.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("slug inválido = %d", r.StatusCode)
	}
	if r := chamar(t, srv, "GET", "/tenants/acme/token-interno", "token-de-teste", ""); r.StatusCode != http.StatusOK {
		t.Fatalf("token interno = %d", r.StatusCode)
	}
	if r := chamar(t, srv, "DELETE", "/tenants/acme", "token-de-teste", `{"confirmar":"outro"}`); r.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("remover sem confirmação = %d", r.StatusCode)
	}
	if r := chamar(t, srv, "DELETE", "/tenants/acme", "token-de-teste", `{"confirmar":"acme"}`); r.StatusCode != http.StatusOK {
		t.Fatalf("remover = %d", r.StatusCode)
	}
}
