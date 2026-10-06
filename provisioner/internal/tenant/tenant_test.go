package tenant

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/provisioner/internal/config"
)

func servicoDeTeste(t *testing.T) (*Servico, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "docker.log")
	t.Setenv("DOCKER_FAKE_LOG", log)
	fake, _ := filepath.Abs("testdata/docker")
	cfg := Config{Config: config.Config{
		Dominio: "crmia.test", RedePublica: "crmia_public", TenantsDir: filepath.Join(dir, "tenants"),
		TemplatePath: filepath.Join("..", "..", "..", "infra", "tenant", "docker-compose.tenant.tmpl"),
		BackupsDir:   filepath.Join(dir, "backups"), ImagemApp: "crmia/app", ImagemDB: "postgres:16-alpine",
		ImagemRedis: "redis:7-alpine", ImagemSuspenso: "crmia/suspenso:latest", VersaoPadrao: "1.0.0",
		CPULimite: "1.0", MemLimite: "768m", DBMemLimite: "512m", RedisMemLimite: "128m",
		EsperaHealthcheck: 5 * time.Millisecond, PainelURL: "http://painel:7894/painel",
	}, Docker: fake}
	s, err := NovoServico(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s, log
}

func TestValidarSlug(t *testing.T) {
	for _, ok := range []string{"locafesta", "minha-empresa", "ab", "x1"} {
		if err := ValidarSlug(ok); err != nil {
			t.Errorf("%q deveria ser aceito: %v", ok, err)
		}
	}
	for _, ruim := range []string{"", "A", "-abc", "abc-", "com espaço", "painel", "api", strings.Repeat("a", 41)} {
		if err := ValidarSlug(ruim); err == nil {
			t.Errorf("%q deveria ser recusado", ruim)
		}
	}
}

func TestCriarGeraComposeESegredosEDevolveSenhaUmaVez(t *testing.T) {
	s, log := servicoDeTeste(t)
	t.Setenv("DOCKER_FAKE_SLUG", "acme")
	ctx := context.Background()

	res, err := s.Criar(ctx, Pedido{Slug: "acme", Nome: `ACME "Ltda"`, CNPJ: "11222333000181", AdminEmail: "admin@acme.com", UsuariosMax: 7, CorPrimaria: "#123456"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != EstadoAtivo || res.URL != "https://crmia.test/acme" || res.Versao != "1.0.0" {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	if len(res.SenhaAdmin) != 12 {
		t.Fatalf("senha do admin deveria vir na primeira vez: %q", res.SenhaAdmin)
	}
	compose, err := os.ReadFile(s.caminhoCompose("acme"))
	if err != nil {
		t.Fatal(err)
	}
	texto := string(compose)
	for _, esperado := range []string{
		"name: ci-acme", "container_name: ci-acme-db", "container_name: ci-acme-redis", "container_name: ci-acme-app",
		`BASE_PATH: "/acme"`, `APP_URL: "https://crmia.test/acme"`, `LICENCA_USUARIOS_MAX: "7"`,
		`TEMA_COR_PRIMARIA: "#123456"`, `TENANT_NOME: "ACME \"Ltda\""`, "image: crmia/app:1.0.0",
		"redis://redis:6379/0", "PathPrefix(`/acme/`)", "!PathPrefix(`/acme/api/interno`)", "crmia.tenant=acme",
	} {
		if !strings.Contains(texto, esperado) {
			t.Errorf("compose sem %q", esperado)
		}
	}
	if strings.Contains(texto, "{{") {
		t.Error("compose com placeholder não renderizado")
	}
	chamadas, _ := os.ReadFile(log)
	if !strings.Contains(string(chamadas), "up -d --remove-orphans db redis app") {
		t.Errorf("docker compose up não foi chamado: %s", chamadas)
	}

	// Segunda vez: mesmos segredos, sem senha nova.
	res2, err := s.Criar(ctx, Pedido{Slug: "acme", Nome: "ACME", CNPJ: "11222333000181"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.SenhaAdmin != "" {
		t.Fatal("a senha do admin só pode ser devolvida uma vez")
	}
	seg1, _, _ := s.carregarOuGerarSegredos("acme")
	if !seg1.AdminEntregue || seg1.SenhaAdminPendente != "" || len(seg1.JWTSegredo) != 64 {
		t.Fatalf("segredos inconsistentes: %+v", seg1)
	}
	tok, err := s.TokenInterno("acme")
	if err != nil || tok != seg1.TokenInterno {
		t.Fatalf("token interno: %q %v", tok, err)
	}
}

func TestAtualizarRecriaSoOApp(t *testing.T) {
	s, log := servicoDeTeste(t)
	t.Setenv("DOCKER_FAKE_SLUG", "beta")
	ctx := context.Background()
	if _, err := s.Criar(ctx, Pedido{Slug: "beta", Nome: "Beta", CNPJ: "1"}); err != nil {
		t.Fatal(err)
	}
	max := 20
	res, err := s.Atualizar(ctx, "beta", Atualizacao{Versao: "2.0.0", UsuariosMax: &max})
	if err != nil {
		t.Fatal(err)
	}
	if res.Versao != "2.0.0" {
		t.Fatalf("versão não atualizada: %+v", res)
	}
	compose, _ := os.ReadFile(s.caminhoCompose("beta"))
	if !strings.Contains(string(compose), "crmia/app:2.0.0") || !strings.Contains(string(compose), `LICENCA_USUARIOS_MAX: "20"`) {
		t.Fatal("compose não refletiu a atualização")
	}
	chamadas, _ := os.ReadFile(log)
	if !strings.Contains(string(chamadas), "up -d --force-recreate app") {
		t.Fatalf("esperava recriar só o app: %s", chamadas)
	}
	if _, err := s.Atualizar(ctx, "inexistente", Atualizacao{}); err == nil {
		t.Fatal("atualizar cliente inexistente deveria falhar")
	}
}

func TestSuspenderExigeAmbienteEReativaDepois(t *testing.T) {
	s, log := servicoDeTeste(t)
	t.Setenv("DOCKER_FAKE_SLUG", "gama")
	ctx := context.Background()
	if err := s.Suspender(ctx, "gama"); err == nil {
		t.Fatal("suspender sem ambiente deveria falhar")
	}
	if _, err := s.Criar(ctx, Pedido{Slug: "gama", Nome: "Gama", CNPJ: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Suspender(ctx, "gama"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reativar(ctx, "gama"); err != nil {
		t.Fatal(err)
	}
	chamadas, _ := os.ReadFile(log)
	for _, c := range []string{"--profile suspenso up -d suspenso", "stop app", "up -d db redis app", "rm -sf suspenso"} {
		if !strings.Contains(string(chamadas), c) {
			t.Errorf("faltou a chamada %q", c)
		}
	}
	// Com a página de suspensão no ar, recriar é recusado.
	t.Setenv("DOCKER_FAKE_SUSPENSO", "1")
	if _, err := s.Criar(ctx, Pedido{Slug: "gama", Nome: "Gama", CNPJ: "1"}); err == nil || !strings.Contains(err.Error(), "suspenso") {
		t.Fatalf("recriar com suspensão ativa deveria ser recusado: %v", err)
	}
}

func TestRemoverFazBackupEApagaArquivos(t *testing.T) {
	s, log := servicoDeTeste(t)
	t.Setenv("DOCKER_FAKE_SLUG", "delta")
	ctx := context.Background()
	if _, err := s.Criar(ctx, Pedido{Slug: "delta", Nome: "Delta", CNPJ: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Remover(ctx, "delta"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.dirTenant("delta")); !os.IsNotExist(err) {
		t.Fatal("diretório do tenant deveria ter sido apagado")
	}
	backups, _ := os.ReadDir(s.cfg.BackupsDir)
	if len(backups) != 1 || !strings.HasPrefix(backups[0].Name(), "delta-") {
		t.Fatalf("backup não foi gerado: %v", backups)
	}
	chamadas, _ := os.ReadFile(log)
	if !strings.Contains(string(chamadas), "down --volumes --remove-orphans") {
		t.Fatal("down --volumes não foi chamado")
	}
	st, _ := s.Status(ctx, "delta")
	if st.Status != EstadoAusente {
		t.Fatalf("status depois de remover = %s", st.Status)
	}
}

func TestListarEVersoes(t *testing.T) {
	s, _ := servicoDeTeste(t)
	t.Setenv("DOCKER_FAKE_SLUG", "um")
	ctx := context.Background()
	if _, err := s.Criar(ctx, Pedido{Slug: "um", Nome: "Um", CNPJ: "1"}); err != nil {
		t.Fatal(err)
	}
	lista, err := s.Listar(ctx)
	if err != nil || len(lista) != 1 || lista[0].Slug != "um" {
		t.Fatalf("listar: %v %v", lista, err)
	}
	tags, err := s.VersoesDisponiveis(ctx)
	if err != nil || len(tags) != 2 {
		t.Fatalf("versões: %v %v", tags, err)
	}
}

func TestEscaparYAML(t *testing.T) {
	if got := escaparYAML(`a"b\c` + "\n"); got != `a\"b\\c\n` {
		t.Fatalf("escape = %q", got)
	}
}
