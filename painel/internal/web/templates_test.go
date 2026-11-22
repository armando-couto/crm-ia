package web

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/armando-couto/crm-ia/painel/internal/auth"
	"github.com/armando-couto/crm-ia/painel/internal/basepublica"
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
		"pagina_usuarios", "pagina_auditoria", "pagina_assinar", "pagina_dados"}
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
	// Sem a cota no contexto (erro ao calcular), a ficha abre sem a linha.
	if strings.Contains(html, "Prospecção no mês") {
		t.Error("a linha de prospecção só aparece quando há cota")
	}
	d.V["Prospeccao"] = operacoes.Cota{Mes: "2026-11", Limite: 5000, Usado: 1250, Restante: 3750}
	if html = renderizar(t, s, "cliente_ficha", d); !strings.Contains(html, "<strong>Prospecção no mês:</strong> 1.250 de 5.000 leads") || !strings.Contains(html, "restam 3.750 em 2026-11") {
		t.Errorf("linha de prospecção da ficha: %s", html)
	}
	d.V["Prospeccao"] = operacoes.Cota{Mes: "2026-11"}
	if html = renderizar(t, s, "cliente_ficha", d); !strings.Contains(html, "0 de 0 leads") || !strings.Contains(html, "o plano não inclui prospecção") {
		t.Error("ficha de cliente com plano sem prospecção")
	}
}

func TestPaginaBaseDeDadosRenderiza(t *testing.T) {
	s := servidorSemBanco(t)
	inicio := time.Date(2026, 11, 22, 9, 30, 0, 0, time.UTC)
	sit := basepublica.Situacao{Pronta: true, ReferenciaCNPJ: "2026-10", ReferenciaPGFN: "2026-T3", Versao: "v7", Empresas: 28700000, Socios: 25000000,
		Devedores: 1234567, EspacoBytes: 13 << 30, AtualizadaEm: &inicio,
		Carga: basepublica.Carga{Estado: "baixando", Fonte: "cnpj", Referencia: "2026-11", Etapa: "Estabelecimentos3.zip", Percentual: 37.6, IniciadaEm: &inicio,
			Arquivos: []basepublica.Arquivo{{Nome: "Empresas0.zip", Estado: "carregado", Bytes: 400 << 20, TotalBytes: 400 << 20, Linhas: 4494860},
				{Nome: "Estabelecimentos3.zip", Estado: "baixando", Bytes: 100 << 20, TotalBytes: 400 << 20}}}}
	d := dados{Produto: "CRM IA", Dominio: "crmia.test", Secao: "dados", Titulo: "Base de dados", CSRF: "tok",
		Sessao: auth.Sessao{Perfil: modelo.PerfilSuporte, Nome: "Suporte"},
		V: map[string]any{"Configurada": true, "Situacao": sit, "Mes": "11/2026",
			"Uso": []modelo.UsoProspeccao{{ClienteID: 7, ClienteNome: "ACME", ClienteSlug: "acme", Leads: 1250, Consultas: 12, Limite: 5000, Plano: "Profissional"},
				{ClienteID: 8, ClienteNome: "Beta", ClienteSlug: "beta", Leads: 300, Limite: 300}}}}
	html := renderizar(t, s, "dados", d)
	for _, esperado := range []string{"28.700.000", "1.234.567", "13,0 GB", "2026-10", "2026-T3", "baixando", "Estabelecimentos3.zip", `aria-valuenow="37"`, "width: 37%",
		"width: 25%", "4.494.860", "location.reload", "A base anterior segue respondendo", `action="/painel/base-de-dados/carregar"`, "Verificar e carregar agora",
		`href="/painel/clientes/7"`, "1.250", "5.000", "Profissional", "sem licença", "barra--cheia", `class="ativo">Base de dados</a>`, "Brasil inteiro"} {
		if !strings.Contains(html, esperado) {
			t.Errorf("página da base sem %q", esperado)
		}
	}

	// Carga com erro: mostra a mensagem, não recarrega sozinha.
	sit.Carga = basepublica.Carga{Estado: "erro", Erro: "espaço em disco insuficiente"}
	d.V["Situacao"] = sit
	html = renderizar(t, s, "dados", d)
	if !strings.Contains(html, "espaço em disco insuficiente") || !strings.Contains(html, "indicador--alerta") || strings.Contains(html, "location.reload") {
		t.Error("página com carga em erro")
	}

	// Serviço ausente: aviso, sem botão, sem quebrar (Situacao não existe).
	d.Sessao = auth.Sessao{Perfil: modelo.PerfilAdmin}
	d.V = map[string]any{"Configurada": false, "Mes": "11/2026"}
	html = renderizar(t, s, "dados", d)
	if !strings.Contains(html, "não está configurado") || !strings.Contains(html, "Nenhum cliente usou a prospecção neste mês.") || strings.Contains(html, "Verificar e carregar agora") {
		t.Errorf("página sem o serviço: %s", html)
	}
	// Serviço fora do ar.
	d.V = map[string]any{"Configurada": true, "SituacaoErro": "a base de empresas não está respondendo", "Mes": "11/2026"}
	if html = renderizar(t, s, "dados", d); !strings.Contains(html, "Não consegui consultar o serviço de dados") {
		t.Error("página com o serviço fora do ar")
	}
	// Quem não provisiona não vê o botão de carga.
	d.Sessao = auth.Sessao{Perfil: modelo.PerfilComercial}
	d.V = map[string]any{"Configurada": true, "Situacao": basepublica.Situacao{Amostra: true}, "Mes": "11/2026"}
	html = renderizar(t, s, "dados", d)
	if strings.Contains(html, "Verificar e carregar agora") || !strings.Contains(html, "demonstração") || !strings.Contains(html, "ainda não foi carregada") {
		t.Error("página para o perfil comercial, em modo amostra e sem base pronta")
	}
}

func TestPlanoMostraACotaDeProspeccao(t *testing.T) {
	s := servidorSemBanco(t)
	d := dados{Produto: "CRM IA", Secao: "planos", Titulo: "Editar plano", Sessao: auth.Sessao{Perfil: modelo.PerfilAdmin},
		V: map[string]any{"Plano": modelo.Plano{ID: 2, Codigo: "profissional", Nome: "Profissional", UsuariosMin: 4, UsuariosMax: 10, PrecoMensalCentavos: 24900, Ativo: true, ProspeccaoLeadsMes: 5000}}}
	html := renderizar(t, s, "plano_form", d)
	if !strings.Contains(html, "Leads de prospecção por mês") || !strings.Contains(html, `name="prospeccao_leads_mes" min="0" step="1" value="5000"`) {
		t.Errorf("formulário de plano sem o campo de prospecção: %s", html)
	}
	d.Titulo = "Planos e preços"
	d.V = map[string]any{"Planos": []modelo.Plano{{ID: 2, Codigo: "profissional", Nome: "Profissional", UsuariosMin: 4, UsuariosMax: 10, PrecoMensalCentavos: 24900, Ativo: true, ProspeccaoLeadsMes: 20000},
		{ID: 3, Codigo: "basico", Nome: "Básico", UsuariosMin: 1, UsuariosMax: 2, PrecoMensalCentavos: 4900, Ativo: true}}}
	html = renderizar(t, s, "planos", d)
	if !strings.Contains(html, "Leads de prospecção/mês") || !strings.Contains(html, `<td class="num">20.000</td>`) || !strings.Contains(html, `<td class="num">—</td>`) {
		t.Errorf("lista de planos sem a coluna de prospecção: %s", html)
	}
}

func TestFormatadores(t *testing.T) {
	for entrada, esperado := range map[int64]string{0: "0", 999: "999", 1000: "1.000", 28700000: "28.700.000", -1234: "-1.234"} {
		if got := milhar(entrada); got != esperado {
			t.Errorf("milhar(%d) = %q", entrada, got)
		}
	}
	if milhar(5000) != "5.000" || milhar("x") != "x" {
		t.Error("milhar com int e com tipo desconhecido")
	}
	for entrada, esperado := range map[int64]string{0: "0 B", 512: "512 B", 2048: "2 KB", 5 << 20: "5,0 MB", 13 << 30: "13,0 GB"} {
		if got := tamanhoLegivel(entrada); got != esperado {
			t.Errorf("tamanhoLegivel(%d) = %q", entrada, got)
		}
	}
}
