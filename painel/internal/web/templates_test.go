package web

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/auth"
	"github.com/armando-couto/crm-ia/painel/internal/config"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/operacoes"
)

// Os templates são compilados no boot: um erro de sintaxe derrubaria o painel
// inteiro. Este teste compila todos e renderiza as páginas que não dependem
// de banco, com dados mínimos.
func servidorSemBanco(t *testing.T) *Servidor {
	t.Helper()
	cfg := config.Config{BasePath: "/painel", Dominio: "crmia.test", NomeProduto: "CRM IA", EmailSuporte: "contato@crmia.test"}
	s := &Servidor{cfg: cfg, sessoes: auth.NovoGerenciador(strings.Repeat("a", 32), strings.Repeat("b", 32), "/painel", false),
		tokens: map[string]tokenGuardado{}, senhas: map[string]senhaUnica{}}
	if err := s.calcularDigitais(); err != nil {
		t.Fatal(err)
	}
	if err := s.carregarTemplates(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTodosOsTemplatesCompilam(t *testing.T) {
	s := servidorSemBanco(t)
	esperados := []string{"pagina_login", "pagina_erro", "pagina_painel", "pagina_grupos", "pagina_grupo_form", "pagina_grupo_ficha",
		"pagina_clientes", "pagina_cliente_form", "pagina_cliente_ficha", "pagina_licencas", "pagina_licenca_form", "pagina_faturas",
		"pagina_planos", "pagina_plano_form", "pagina_versoes", "pagina_ambientes", "pagina_leads", "pagina_solicitacoes",
		"pagina_usuarios", "pagina_auditoria", "pagina_assinar"}
	for _, e := range esperados {
		if _, ok := s.templates[e]; !ok {
			t.Errorf("template %s não foi compilado", e)
		}
	}
}

func renderizar(t *testing.T, s *Servidor, pagina string, d dados) string {
	t.Helper()
	var buf bytes.Buffer
	if err := s.templates["pagina_"+pagina].ExecuteTemplate(&buf, "layout.html", d); err != nil {
		t.Fatalf("%s: %v", pagina, err)
	}
	return buf.String()
}

func TestPaginasPublicasRenderizam(t *testing.T) {
	s := servidorSemBanco(t)
	base := dados{Produto: "CRM IA", Dominio: "crmia.test", V: map[string]any{}}

	login := base
	login.Secao, login.Titulo = "login", "Entrar"
	html := renderizar(t, s, "login", login)
	if !strings.Contains(html, `action="/painel/login"`) || strings.Contains(html, "Fix") {
		t.Fatalf("login: %s", html)
	}

	lic := modelo.Licenca{ValorCentavos: 24900, UsuariosMax: 10, PlanoNome: "Profissional", PagamentoStatus: modelo.PagamentoAguardandoCartao, Fim: time.Now()}
	ck := base
	ck.Secao, ck.Titulo = "checkout", "Pagamento"
	ck.V = map[string]any{"Checkout": operacoes.Checkout{Licenca: lic, Pagador: "ACME Ltda", Documento: "11222333000181", Produto: "CRM IA · Profissional"},
		"Codigo": "abc", "Simulado": true, "EmailSuporte": "contato@crmia.test", "Form": map[string]string{}}
	html = renderizar(t, s, "assinar", ck)
	for _, esperado := range []string{"R$ 249,00", "ACME Ltda", "11.222.333/0001-81", `name="numero"`, `name="cvv"`, "Confirmar e ativar"} {
		if !strings.Contains(html, esperado) {
			t.Errorf("checkout sem %q", esperado)
		}
	}
	// A página de pagamento nunca cita quem processa.
	if strings.Contains(strings.ToLower(html), "fix pay") || strings.Contains(strings.ToLower(html), "fixpay") {
		t.Fatal("o checkout não pode citar o processador")
	}
	ck.V["Concluido"] = true
	html = renderizar(t, s, "assinar", ck)
	if !strings.Contains(html, "Tudo certo") {
		t.Fatal("checkout concluído deveria mostrar a confirmação")
	}
}

func TestFichaDoClienteRenderizaComSenhaUnica(t *testing.T) {
	s := servidorSemBanco(t)
	c := modelo.Cliente{ID: 7, GrupoID: 1, GrupoNome: "Rede", RazaoSocial: "ACME Ltda", NomeFantasia: "ACME", CNPJ: "11222333000181", Slug: "acme",
		AdminNome: "Ana", AdminEmail: "ana@acme.com", Status: modelo.StatusAtivo, CorPrimaria: "#6d5df6", UsuariosContratados: 5}
	d := dados{Produto: "CRM IA", Dominio: "crmia.test", Secao: "clientes", Titulo: "ACME",
		Sessao: auth.Sessao{Perfil: modelo.PerfilAdmin, Nome: "Admin"},
		V: map[string]any{"Cliente": c, "SenhaUnica": senhaUnica{Titulo: "Senha inicial", Senha: "Abc123xyz"},
			"Licenca": modelo.Licenca{PlanoNome: "Essencial", ValorCentavos: 9900, UsuariosMax: 3, Fim: time.Now().AddDate(0, 1, 0), PagamentoStatus: modelo.PagamentoAtivo, CartaoFinal: "1234", Cobranca: "recorrente"}}}
	html := renderizar(t, s, "cliente_ficha", d)
	for _, esperado := range []string{"Abc123xyz", "crmia.test/acme", "cartão •••• 1234", "/painel/clientes/7/suspender", "Zona de perigo"} {
		if !strings.Contains(html, esperado) {
			t.Errorf("ficha sem %q", esperado)
		}
	}
}
