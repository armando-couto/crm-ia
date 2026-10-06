package operacoes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/armando-couto/crm-ia/painel/internal/banco"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/provisionador"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
	"github.com/armando-couto/crm-ia/painel/internal/sign"
)

// Estes testes rodam contra um PostgreSQL de verdade (CRMIA_TEST_PG; padrão
// postgres://postgres:postgres@localhost:5432/postgres). Metade das regras de
// licença vive no SQL — índices parciais, baixa idempotente, extensão da
// vigência — e um mock devolveria o que mandássemos devolver. Sem banco a
// suíte é pulada com aviso.

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	admin := os.Getenv("CRMIA_TEST_PG")
	if admin == "" {
		admin = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelar()
	adm, err := pgxpool.New(ctx, admin)
	if err == nil {
		err = adm.Ping(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "AVISO: PostgreSQL de teste indisponível, pulando os testes de operações:", err)
		os.Exit(0)
	}
	_, _ = adm.Exec(ctx, `DROP DATABASE IF EXISTS crmia_test_painel`)
	if _, err := adm.Exec(ctx, `CREATE DATABASE crmia_test_painel`); err != nil {
		fmt.Fprintln(os.Stderr, "não consegui criar o banco de teste:", err)
		os.Exit(1)
	}
	adm.Close()
	url := strings.Replace(admin, "/postgres?", "/crmia_test_painel?", 1)
	if pool, err = banco.Conectar(ctx, url); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := banco.Migrar(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	codigo := m.Run()
	pool.Close()
	os.Exit(codigo)
}

// provisionadorFalso registra o que o painel pediu, sem Docker.
type provisionadorFalso struct {
	mu       sync.Mutex
	chamadas []string
	srv      *httptest.Server
}

func subirProvisionador(t *testing.T) *provisionadorFalso {
	f := &provisionadorFalso{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.chamadas = append(f.chamadas, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/tenants":
			var p map[string]any
			_ = json.NewDecoder(r.Body).Decode(&p)
			_ = json.NewEncoder(w).Encode(map[string]any{"slug": p["slug"], "status": "ativo", "url": "https://crmia.test/" + fmt.Sprint(p["slug"]), "versao": p["versao"], "senha_admin": "Senha123abc"})
		case strings.HasSuffix(r.URL.Path, "/token-interno"):
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"slug": "x", "status": "ativo", "versao": "1.0.0"})
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *provisionadorFalso) teve(pedaco string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.chamadas {
		if strings.Contains(c, pedaco) {
			return true
		}
	}
	return false
}

func servico(t *testing.T) (*Servico, *repo.Repo, *provisionadorFalso) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE faturas, licencas, plano_solicitacoes, eventos_ambiente, clientes, grupos, versoes, leads, auditoria, cobranca_eventos RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	r := repo.Novo(pool)
	f := subirProvisionador(t)
	s := Novo(r, provisionador.Novo(f.srv.URL, "token"), sign.Novo(sign.Config{Simulado: true}))
	s.URLCheckout = func(c string) string { return "https://crmia.test/painel/assinar/" + c }
	return s, r, f
}

func grupoECliente(t *testing.T, r *repo.Repo) (modelo.Grupo, modelo.Cliente) {
	ctx := context.Background()
	gid, err := r.SalvarGrupo(ctx, modelo.Grupo{Nome: "Rede ACME", CNPJResponsavel: "11222333000181", EmailFinanceiro: "fin@acme.com", CobrancaUnificada: true,
		CEP: "60000000", Logradouro: "Rua A", Numero: "10", Bairro: "Centro", Municipio: "Fortaleza", UF: "CE"})
	if err != nil {
		t.Fatal(err)
	}
	cid, err := r.CriarCliente(ctx, modelo.Cliente{GrupoID: gid, RazaoSocial: "ACME Ltda", NomeFantasia: "ACME", CNPJ: "11222333000181", Slug: "acme",
		AdminNome: "Ana", AdminEmail: "ana@acme.com", UsuariosContratados: 5, CorPrimaria: "#6d5df6", AutoAtualizar: true})
	if err != nil {
		t.Fatal(err)
	}
	g, _ := r.GrupoPorID(ctx, gid)
	c, _ := r.ClientePorID(ctx, cid)
	return g, c
}

func planoPorCodigo(t *testing.T, r *repo.Repo, codigo string) modelo.Plano {
	p, err := r.PlanoPorCodigo(context.Background(), codigo)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCalculoDoValor(t *testing.T) {
	s, r, _ := servico(t)
	ctx := context.Background()
	prof := planoPorCodigo(t, r, "profissional")
	casos := []struct {
		nome     string
		pedido   PedidoLicenca
		esperado int64
	}{
		{"mensal uma unidade", PedidoLicenca{PlanoID: prof.ID, Periodicidade: "mensal", Unidades: 1}, 24900},
		{"mensal três unidades", PedidoLicenca{PlanoID: prof.ID, Periodicidade: "mensal", Unidades: 3}, 74700},
		{"unidades zero conta como uma", PedidoLicenca{PlanoID: prof.ID, Periodicidade: "mensal"}, 24900},
		{"anual sem preço anual próprio = 12 meses", PedidoLicenca{PlanoID: prof.ID, Periodicidade: "anual", Unidades: 1}, 24900 * 12},
	}
	for _, c := range casos {
		valor, _, err := s.CalcularValor(ctx, c.pedido)
		if err != nil || valor != c.esperado {
			t.Errorf("%s: valor=%d err=%v (esperava %d)", c.nome, valor, err, c.esperado)
		}
	}
}

func TestProvisionarMarcaAtivoEDevolveSenha(t *testing.T) {
	s, r, f := servico(t)
	_, c := grupoECliente(t, r)
	res, err := s.Provisionar(context.Background(), c.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.SenhaAdmin != "Senha123abc" || res.URL != "https://crmia.test/acme" {
		t.Fatalf("resultado: %+v", res)
	}
	c, _ = r.ClientePorID(context.Background(), c.ID)
	if c.Status != modelo.StatusAtivo || c.URL == "" || c.Versao != "latest" {
		t.Fatalf("cliente depois de provisionar: %+v", c)
	}
	if !f.teve("POST /tenants") {
		t.Fatal("provisionador não foi chamado")
	}
	eventos, _ := r.EventosDoCliente(context.Background(), c.ID, 5)
	if len(eventos) == 0 || eventos[0].Tipo != "provisionado" {
		t.Fatalf("evento não registrado: %+v", eventos)
	}
}

func TestLicencaRecorrenteCheckoutEBaixa(t *testing.T) {
	s, r, _ := servico(t)
	ctx := context.Background()
	g, c := grupoECliente(t, r)
	prof := planoPorCodigo(t, r, "profissional")

	res, err := s.ContratarLicenca(ctx, PedidoLicenca{GrupoID: g.ID, PlanoID: prof.ID, Periodicidade: "mensal", Unidades: 1, Cobranca: "recorrente"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.URLCheckout, "https://crmia.test/painel/assinar/") {
		t.Fatalf("sem link de checkout: %+v", res)
	}
	l, _ := r.LicencaPorID(ctx, res.ID)
	if l.PagamentoStatus != modelo.PagamentoAguardandoCartao || l.AssinaturaID == "" || l.CodigoCheckout == "" {
		t.Fatalf("licença: %+v", l)
	}
	g, _ = r.GrupoPorID(ctx, g.ID)
	if g.PagadorID == "" {
		t.Fatal("o pagador do grupo deveria ter sido registrado")
	}
	abertas, _ := r.FaturasAbertasDaLicenca(ctx, l.ID)
	if len(abertas) != 1 || abertas[0].ValorCentavos != 24900 {
		t.Fatalf("primeira fatura: %+v", abertas)
	}

	// Anual não pode ser recorrente.
	if _, err := s.ContratarLicenca(ctx, PedidoLicenca{GrupoID: g.ID, PlanoID: prof.ID, Periodicidade: "anual", Cobranca: "recorrente"}); err == nil {
		t.Fatal("anual recorrente deveria ser recusado")
	}

	// Checkout: página e cartão recusado.
	ck, err := s.Checkout(ctx, l.CodigoCheckout)
	if err != nil || ck.Pagador != "Rede ACME" || ck.Documento != "11222333000181" {
		t.Fatalf("checkout: %+v %v", ck, err)
	}
	if _, err := s.Checkout(ctx, "nao-existe"); err == nil {
		t.Fatal("código inválido deveria falhar")
	}
	cartao := sign.Cartao{Titular: "Ana", Documento: "06300000000", Validade: "12/35", CVV: "123"}
	cartao.Numero = "4000000000000002"
	rc, err := s.CadastrarCartao(ctx, l.CodigoCheckout, cartao)
	if err != nil || rc.Aprovado || rc.Mensagem == "" {
		t.Fatalf("recusa: %+v %v", rc, err)
	}
	l, _ = r.LicencaPorID(ctx, l.ID)
	if l.PagamentoStatus != modelo.PagamentoRecusado {
		t.Fatalf("status após recusa: %s", l.PagamentoStatus)
	}

	// Cartão aprovado: recorrência ativa e primeira fatura paga, licença estendida.
	cartao.Numero = "4111111111111111"
	rc, err = s.CadastrarCartao(ctx, l.CodigoCheckout, cartao)
	if err != nil || !rc.Aprovado || rc.Final != "1111" {
		t.Fatalf("aprovação: %+v %v", rc, err)
	}
	l, _ = r.LicencaPorID(ctx, l.ID)
	if l.PagamentoStatus != modelo.PagamentoAtivo || l.CartaoFinal != "1111" || l.CartaoBandeira != "VISA" {
		t.Fatalf("licença após cartão: %+v", l)
	}
	abertas, _ = r.FaturasAbertasDaLicenca(ctx, l.ID)
	if len(abertas) != 0 {
		t.Fatalf("a fatura de abertura deveria estar paga: %+v", abertas)
	}

	// Meu plano enxerga tudo isso.
	pl, err := s.PlanoDaEmpresa(ctx, c.Slug)
	if err != nil || pl.Plano == nil || pl.Pagamento == nil || pl.Pagamento.Status != "ativo" || pl.Pagamento.CartaoFinal != "1111" || len(pl.Faturas) != 1 || pl.Faturas[0].Status != "paga" {
		t.Fatalf("plano da empresa: %+v %v", pl, err)
	}
	// Faturas do mês: idempotente.
	hoje := time.Now()
	n1, err := s.GerarFaturasDoMes(ctx, hoje.AddDate(0, 0, 25))
	if err != nil {
		t.Fatal(err)
	}
	n2, _ := s.GerarFaturasDoMes(ctx, hoje.AddDate(0, 0, 25))
	if n2 != 0 {
		t.Fatalf("segunda rodada criou %d faturas", n2)
	}
	_ = n1
	// Cancelar encerra licença e recorrência.
	if err := s.CancelarLicenca(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	l, _ = r.LicencaPorID(ctx, l.ID)
	if l.Status != "cancelada" || l.PagamentoStatus != modelo.PagamentoCancelado {
		t.Fatalf("após cancelar: %+v", l)
	}
	if _, err := s.Checkout(ctx, l.CodigoCheckout); err == nil {
		t.Fatal("checkout de licença cancelada deveria falhar")
	}
}

func TestPagamentoReativaSuspensoPorInadimplencia(t *testing.T) {
	s, r, f := servico(t)
	ctx := context.Background()
	g, c := grupoECliente(t, r)
	ess := planoPorCodigo(t, r, "essencial")
	if _, err := s.Provisionar(ctx, c.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	// Licença manual vencida há 40 dias, com fatura em aberto.
	inicio := time.Now().AddDate(0, -2, -10)
	res, err := s.ContratarLicenca(ctx, PedidoLicenca{GrupoID: g.ID, ClienteID: &c.ID, PlanoID: ess.ID, Periodicidade: "mensal", Inicio: inicio, Cobranca: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.MarcarLicencasVencidas(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	semLicenca, _ := r.ClientesSemLicencaVigente(ctx, time.Now(), 10)
	if len(semLicenca) != 1 || semLicenca[0].ID != c.ID {
		t.Fatalf("cliente sem licença vigente não detectado: %+v", semLicenca)
	}
	if err := s.SuspenderPorInadimplencia(ctx, c.ID, "teste"); err != nil {
		t.Fatal(err)
	}
	c, _ = r.ClientePorID(ctx, c.ID)
	if c.Status != modelo.StatusSuspenso || !c.SuspensoPorInadimplencia {
		t.Fatalf("cliente após suspensão: %+v", c)
	}
	// A fatura aberta mais antiga paga reativa (a vigência passa a cobrir hoje
	// porque a competência atual também é gerada e paga).
	_, _ = s.GerarFaturasDoMes(ctx, time.Now())
	abertas, _ := r.FaturasAbertasDaLicenca(ctx, res.ID)
	if len(abertas) < 1 {
		t.Fatal("deveria haver fatura em aberto")
	}
	var reativou bool
	for _, fa := range abertas {
		p, err := s.RegistrarPagamento(ctx, fa.ID, "manual", "teste-"+fmt.Sprint(fa.ID))
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Reativados) > 0 {
			reativou = true
		}
	}
	if !reativou {
		t.Fatal("o pagamento deveria ter reativado o ambiente")
	}
	c, _ = r.ClientePorID(ctx, c.ID)
	if c.Status != modelo.StatusAtivo {
		t.Fatalf("cliente após pagamento: %s", c.Status)
	}
	if !f.teve("/tenants/acme/reativar") || !f.teve("/tenants/acme/suspender") {
		t.Fatalf("provisionador não recebeu suspender/reativar: %v", f.chamadas)
	}
	// Segunda baixa da mesma fatura não faz nada.
	p, _ := s.RegistrarPagamento(ctx, abertas[0].ID, "manual", "x")
	if p.Baixada {
		t.Fatal("baixa deveria ser idempotente")
	}
}

func TestSolicitacaoDeUpgrade(t *testing.T) {
	s, r, _ := servico(t)
	ctx := context.Background()
	g, c := grupoECliente(t, r)
	ess := planoPorCodigo(t, r, "essencial")
	prof := planoPorCodigo(t, r, "profissional")
	if _, err := s.ContratarLicenca(ctx, PedidoLicenca{GrupoID: g.ID, PlanoID: ess.ID, Periodicidade: "mensal", Cobranca: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SolicitarPlano(ctx, c.Slug, "upgrade", &prof.ID, "preciso de mais usuários"); err != nil {
		t.Fatal(err)
	}
	if err := s.SolicitarPlano(ctx, c.Slug, "cancelamento", nil, ""); err != ErrSolicitacaoPendente {
		t.Fatalf("segunda solicitação deveria esperar a primeira: %v", err)
	}
	sols, _ := r.ListarSolicitacoes(ctx, "pendente")
	resposta, err := s.AprovarSolicitacao(ctx, sols[0].ID, "admin@crmia.test")
	if err != nil || !strings.Contains(resposta, "Profissional") {
		t.Fatalf("aprovar: %q %v", resposta, err)
	}
	lic, _ := r.LicencaVigenteDoCliente(ctx, c, time.Now())
	if lic.PlanoID != prof.ID || lic.UsuariosMax != prof.UsuariosMax || lic.ValorCentavos != prof.PrecoMensalCentavos {
		t.Fatalf("licença após upgrade: %+v", lic)
	}
}
