package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/armando-couto/crm-ia/painel/internal/auth"
	"github.com/armando-couto/crm-ia/painel/internal/banco"
	"github.com/armando-couto/crm-ia/painel/internal/basepublica"
	"github.com/armando-couto/crm-ia/painel/internal/config"
	"github.com/armando-couto/crm-ia/painel/internal/modelo"
	"github.com/armando-couto/crm-ia/painel/internal/operacoes"
	"github.com/armando-couto/crm-ia/painel/internal/provisionador"
	"github.com/armando-couto/crm-ia/painel/internal/repo"
	"github.com/armando-couto/crm-ia/painel/internal/sign"
)

// Testes de integração da API de prospecção: PostgreSQL de verdade
// (CRMIA_TEST_PG; padrão postgres://postgres:postgres@localhost:5432/postgres)
// e o serviço `dados` simulado por httptest. A cota vive no SQL (débito
// condicional, chave única do lote), então um banco de mentira não provaria
// nada. Sem banco, só estes testes são pulados; os de template seguem.

const bancoDeTeste = "crmia_test_painel_web"

var poolTeste *pgxpool.Pool

func TestMain(m *testing.M) {
	// Centenas de requisições por teste: o log de cada uma só atrapalha.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	admin := os.Getenv("CRMIA_TEST_PG")
	if admin == "" {
		admin = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	adm, err := pgxpool.New(ctx, admin)
	if err == nil {
		err = adm.Ping(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "AVISO: PostgreSQL de teste indisponível, pulando os testes de integração da prospecção:", err)
	} else {
		_, _ = adm.Exec(ctx, `DROP DATABASE IF EXISTS `+bancoDeTeste)
		if _, err := adm.Exec(ctx, `CREATE DATABASE `+bancoDeTeste); err != nil {
			fmt.Fprintln(os.Stderr, "não consegui criar o banco de teste:", err)
			os.Exit(1)
		}
		adm.Close()
		url := strings.Replace(admin, "/postgres?", "/"+bancoDeTeste+"?", 1)
		if poolTeste, err = banco.Conectar(ctx, url); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := banco.Migrar(ctx, poolTeste); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	cancelar()
	codigo := m.Run()
	if poolTeste != nil {
		poolTeste.Close()
	}
	os.Exit(codigo)
}

const tokenAmbiente = "token-interno-do-ambiente-de-teste"

// dadosFalso simula o serviço `dados`. Cada teste troca `responder` quando
// precisa de outro comportamento.
type dadosFalso struct {
	srv       *httptest.Server
	mu        sync.Mutex
	chamadas  []string
	corpos    [][]byte
	responder func(w http.ResponseWriter, r *http.Request, corpo []byte) bool
	pf        map[string]bool // CNPJs que a base trata como pessoa física
	ausentes  map[string]bool // CNPJs que a base não tem
}

func (f *dadosFalso) total(pedaco string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.chamadas {
		if strings.Contains(c, pedaco) {
			n++
		}
	}
	return n
}

func (f *dadosFalso) ultimoCorpo() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.corpos) == 0 {
		return nil
	}
	return f.corpos[len(f.corpos)-1]
}

func fichaFalsa(cnpj string, pf bool) map[string]any {
	return map[string]any{"cnpj": cnpj, "razao_social": "EMPRESA " + cnpj, "pessoa_fisica": pf,
		"telefones": []map[string]any{{"numero": "85900000001", "tipo": "celular"}}, "email": "contato@exemplo.invalid", "email_tipo": "corporativo",
		"socios": []map[string]any{{"nome": "SOCIO 1"}}}
}

func subirDados(t *testing.T) *dadosFalso {
	t.Helper()
	f := &dadosFalso{pf: map[string]bool{}, ausentes: map[string]bool{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corpo, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.chamadas = append(f.chamadas, r.Method+" "+r.URL.RequestURI())
		f.corpos = append(f.corpos, corpo)
		responder := f.responder
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer token-do-servico-de-dados-0001" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"erro":"não autorizado"}`))
			return
		}
		if responder != nil && responder(w, r, corpo) {
			return
		}
		switch {
		case r.URL.Path == "/v1/situacao":
			_ = json.NewEncoder(w).Encode(map[string]any{"pronta": true, "amostra": true, "referencia_cnpj": "2026-11", "referencia_pgfn": "2026-T3",
				"versao": "amostra-1", "empresas": 4321, "socios": 900, "devedores": 77, "espaco_bytes": 1 << 20, "ufs": []string{},
				"carga": map[string]any{"estado": "ociosa", "percentual": 0, "etapa": "", "erro": "", "arquivos": []any{}}})
		case strings.HasPrefix(r.URL.Path, "/v1/catalogos/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"itens": []map[string]any{{"codigo": "5611201", "descricao": "Restaurantes e similares", "nivel": "subclasse"}}, "consulta": r.URL.RawQuery})
		case r.URL.Path == "/v1/empresas/contagem":
			_, _ = w.Write([]byte(`{"total":1234,"total_modo":"exato","parciais":{"com_celular":800,"com_email":600,"com_divida":50}}`))
		case r.URL.Path == "/v1/empresas/previa":
			_, _ = w.Write([]byte(`{"itens":[{"cnpj":"00000001000191"}],"versao":"amostra-1","ordem_aplicada":"relevancia"}`))
		case r.URL.Path == "/v1/empresas/candidatos":
			_, _ = w.Write([]byte(`{"cnpjs":["00000001000191"],"cursor":"","fim":true,"versao":"amostra-1","ordem_aplicada":"relevancia"}`))
		case r.URL.Path == "/v1/empresas/detalhes":
			var in struct {
				CNPJs     []string `json:"cnpjs"`
				IncluirPF bool     `json:"incluir_pf"`
			}
			_ = json.Unmarshal(corpo, &in)
			itens, faltam := []map[string]any{}, []string{}
			for _, c := range in.CNPJs {
				if f.ausentes[c] {
					faltam = append(faltam, c)
					continue
				}
				itens = append(itens, fichaFalsa(c, f.pf[c]))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"itens": itens, "nao_encontrados": faltam})
		case strings.HasPrefix(r.URL.Path, "/v1/empresas/"):
			cnpj := strings.TrimPrefix(r.URL.Path, "/v1/empresas/")
			if f.ausentes[cnpj] {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"erro":"empresa não encontrada"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(fichaFalsa(cnpj, f.pf[cnpj]))
		case r.URL.Path == "/v1/cargas":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"erro":"rota desconhecida"}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

type ambiente struct {
	t       *testing.T
	srv     *Servidor
	rotas   http.Handler
	repo    *repo.Repo
	ops     *operacoes.Servico
	dados   *dadosFalso
	sessoes *auth.Gerenciador
	grupo   modelo.Grupo
	cliente modelo.Cliente
}

// montar sobe o painel de teste com um cliente "acme" sem licença. comDados
// falso = painel rodando sem o serviço `dados` configurado.
func montar(t *testing.T, comDados bool) *ambiente {
	t.Helper()
	if poolTeste == nil {
		t.Skip("sem PostgreSQL de teste (CRMIA_TEST_PG)")
	}
	ctx := context.Background()
	if _, err := poolTeste.Exec(ctx, `TRUNCATE faturas, licencas, plano_solicitacoes, eventos_ambiente, clientes, grupos, versoes, leads, auditoria, cobranca_eventos RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := poolTeste.Exec(ctx, `UPDATE configuracoes SET valor='300' WHERE chave='prospeccao.cota_sem_licenca'`); err != nil {
		t.Fatal(err)
	}
	prov := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/token-interno") {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": tokenAmbiente})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"slug": "acme", "status": "ativo"})
	}))
	t.Cleanup(prov.Close)

	a := &ambiente{t: t, repo: repo.Novo(poolTeste)}
	cliProv := provisionador.Novo(prov.URL, "token")
	a.ops = operacoes.Novo(a.repo, cliProv, sign.Novo(sign.Config{Simulado: true}))
	base := basepublica.Novo("", "")
	if comDados {
		a.dados = subirDados(t)
		base = basepublica.Novo(a.dados.srv.URL, "token-do-servico-de-dados-0001")
	}
	a.ops.BasePublica = base
	cfg := config.Config{BasePath: "/painel", Dominio: "crmia.test", NomeProduto: "CRM IA", EmailSuporte: "contato@crmia.test", ProvisionadorToken: "token"}
	a.sessoes = auth.NovoGerenciador(strings.Repeat("a", 32), strings.Repeat("b", 32), "/painel", false)
	var err error
	if a.srv, err = NovoServidor(cfg, a.repo, a.sessoes, a.ops, cliProv, nil, base); err != nil {
		t.Fatal(err)
	}
	a.rotas = a.srv.Rotas()

	gid, err := a.repo.SalvarGrupo(ctx, modelo.Grupo{Nome: "Rede ACME", CNPJResponsavel: "11222333000181", EmailFinanceiro: "fin@acme.com", CobrancaUnificada: true,
		CEP: "60000000", Logradouro: "Rua A", Numero: "10", Bairro: "Centro", Municipio: "Fortaleza", UF: "CE"})
	if err != nil {
		t.Fatal(err)
	}
	cid, err := a.repo.CriarCliente(ctx, modelo.Cliente{GrupoID: gid, RazaoSocial: "ACME Ltda", NomeFantasia: "ACME", CNPJ: "11222333000181", Slug: "acme",
		AdminNome: "Ana", AdminEmail: "ana@acme.com", UsuariosContratados: 5, CorPrimaria: "#6d5df6", AutoAtualizar: true})
	if err != nil {
		t.Fatal(err)
	}
	a.grupo, _ = a.repo.GrupoPorID(ctx, gid)
	a.cliente, _ = a.repo.ClientePorID(ctx, cid)
	return a
}

// licenciar dá ao grupo uma licença manual do plano, vigente por um mês.
func (a *ambiente) licenciar(codigo string) modelo.Licenca {
	a.t.Helper()
	ctx := context.Background()
	p, err := a.repo.PlanoPorCodigo(ctx, codigo)
	if err != nil {
		a.t.Fatal(err)
	}
	hoje := operacoes.DiaDaCota(time.Now())
	id, err := a.repo.CriarLicenca(ctx, modelo.Licenca{GrupoID: a.grupo.ID, PlanoID: p.ID, UsuariosMax: p.UsuariosMax, Periodicidade: "mensal",
		ValorCentavos: p.PrecoMensalCentavos, Inicio: hoje.AddDate(0, 0, -1), Fim: hoje.AddDate(0, 1, 0), Unidades: 1, Cobranca: "manual", PagamentoStatus: "manual"})
	if err != nil {
		a.t.Fatal(err)
	}
	l, err := a.repo.LicencaPorID(ctx, id)
	if err != nil {
		a.t.Fatal(err)
	}
	return l
}

func (a *ambiente) limitePlano(codigo string, limite int) {
	a.t.Helper()
	if _, err := poolTeste.Exec(context.Background(), `UPDATE planos SET prospeccao_leads_mes=$2 WHERE codigo=$1`, codigo, limite); err != nil {
		a.t.Fatal(err)
	}
	a.t.Cleanup(func() {
		_, _ = poolTeste.Exec(context.Background(), `UPDATE planos SET prospeccao_leads_mes = CASE codigo WHEN 'essencial' THEN 1000 WHEN 'profissional' THEN 5000 WHEN 'escala' THEN 20000 WHEN 'sob-medida' THEN 50000 ELSE 0 END`)
	})
}

func (a *ambiente) usado() int {
	a.t.Helper()
	leads, _, err := a.repo.UsoProspeccao(context.Background(), a.cliente.ID, operacoes.Competencia(time.Now()))
	if err != nil {
		a.t.Fatal(err)
	}
	return leads
}

type resposta struct {
	status int
	corpo  map[string]any
	cab    http.Header
	bruto  string
}

func (a *ambiente) pedir(metodo, caminho, token string, corpo any) resposta {
	a.t.Helper()
	var leitor io.Reader
	switch v := corpo.(type) {
	case nil:
	case string:
		leitor = strings.NewReader(v)
	default:
		b, _ := json.Marshal(v)
		leitor = bytes.NewReader(b)
	}
	req := httptest.NewRequest(metodo, "/painel/api/empresas/acme/prospeccao"+caminho, leitor)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.rotas.ServeHTTP(w, req)
	r := resposta{status: w.Code, cab: w.Header(), bruto: w.Body.String()}
	_ = json.Unmarshal(w.Body.Bytes(), &r.corpo)
	return r
}

func (a *ambiente) api(metodo, caminho string, corpo any) resposta {
	a.t.Helper()
	return a.pedir(metodo, caminho, tokenAmbiente, corpo)
}

// exigirErro confere o formato de erro da API: as duas chaves de mensagem,
// iguais e não vazias, e o código.
func exigirErro(t *testing.T, r resposta, status int, codigo string) {
	t.Helper()
	if r.status != status {
		t.Fatalf("status %d (esperava %d): %s", r.status, status, r.bruto)
	}
	erro, _ := r.corpo["erro"].(string)
	error2, _ := r.corpo["error"].(string)
	if erro == "" || erro != error2 {
		t.Fatalf("erro sem as duas chaves iguais: %s", r.bruto)
	}
	if r.corpo["codigo"] != codigo {
		t.Fatalf("codigo %v (esperava %q): %s", r.corpo["codigo"], codigo, r.bruto)
	}
}

// cnpjs gera n CNPJs fictícios distintos a partir de um prefixo.
func cnpjs(prefixo, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%02d%06d000100", prefixo, i)
	}
	return out
}

func cotaDe(t *testing.T, corpo map[string]any) operacoes.Cota {
	t.Helper()
	var c operacoes.Cota
	b, _ := json.Marshal(corpo["cota"])
	if err := json.Unmarshal(b, &c); err != nil || c.Mes == "" {
		t.Fatalf("resposta sem cota: %v", corpo)
	}
	return c
}

// ---------------------------------------------------------------------------

func TestMigracaoDaProspeccao(t *testing.T) {
	a := montar(t, false)
	ctx := context.Background()
	for codigo, esperado := range map[string]int{"essencial": 1000, "profissional": 5000, "escala": 20000, "sob-medida": 50000} {
		p, err := a.repo.PlanoPorCodigo(ctx, codigo)
		if err != nil || p.ProspeccaoLeadsMes != esperado {
			t.Errorf("plano %s: cota %d err=%v (esperava %d)", codigo, p.ProspeccaoLeadsMes, err, esperado)
		}
	}
	if _, err := poolTeste.Exec(ctx, `UPDATE planos SET prospeccao_leads_mes=-1 WHERE codigo='essencial'`); err == nil {
		t.Fatal("cota negativa deveria ser recusada pelo CHECK")
	}
	// Salvar o plano pelo repositório grava a cota.
	p, _ := a.repo.PlanoPorCodigo(ctx, "essencial")
	p.ProspeccaoLeadsMes = 1234
	if _, err := a.repo.SalvarPlano(ctx, p); err != nil {
		t.Fatal(err)
	}
	if p, _ = a.repo.PlanoPorCodigo(ctx, "essencial"); p.ProspeccaoLeadsMes != 1234 {
		t.Fatalf("cota não foi gravada: %d", p.ProspeccaoLeadsMes)
	}
	p.ProspeccaoLeadsMes = 1000
	_, _ = a.repo.SalvarPlano(ctx, p)
}

func TestProspeccaoExigeTokenDoAmbiente(t *testing.T) {
	a := montar(t, true)
	exigirErro(t, a.pedir("GET", "", "", nil), http.StatusUnauthorized, "nao_autorizado")
	exigirErro(t, a.pedir("POST", "/contagem", "token-errado", `{"filtros":{"ufs":["CE"]}}`), http.StatusUnauthorized, "nao_autorizado")
	exigirErro(t, a.pedir("POST", "/extracoes", "", `{"pedido_id":"p","cnpjs":["00000001000191"]}`), http.StatusUnauthorized, "nao_autorizado")
	if a.dados.total("/v1/") != 0 {
		t.Fatal("pedido sem credencial não pode chegar ao serviço de dados")
	}
	// Ambiente autenticado, mas sem cadastro no painel.
	req := httptest.NewRequest("GET", "/painel/api/empresas/fantasma/prospeccao", nil)
	req.Header.Set("Authorization", "Bearer "+tokenAmbiente)
	w := httptest.NewRecorder()
	a.rotas.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("slug desconhecido: %d %s", w.Code, w.Body.String())
	}
}

func TestResumoDaProspeccao(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	r := a.api("GET", "", nil)
	if r.status != http.StatusOK || r.corpo["habilitada"] != true || r.corpo["motivo"] != "" {
		t.Fatalf("resumo: %d %s", r.status, r.bruto)
	}
	cota := cotaDe(t, r.corpo)
	if cota.Limite != 5000 || cota.Usado != 0 || cota.Restante != 5000 || cota.Mes != operacoes.Competencia(time.Now()).Format("2006-01") {
		t.Fatalf("cota: %+v", cota)
	}
	base, _ := r.corpo["base"].(map[string]any)
	carga, _ := base["carga"].(map[string]any)
	if base["pronta"] != true || base["amostra"] != true || base["referencia_cnpj"] != "2026-11" || base["referencia_pgfn"] != "2026-T3" ||
		base["versao"] != "amostra-1" || base["empresas"] != float64(4321) || carga["estado"] != "ociosa" {
		t.Fatalf("base: %s", r.bruto)
	}
	for _, chave := range []string{"percentual", "etapa", "erro"} {
		if _, ok := carga[chave]; !ok {
			t.Errorf("carga sem %q: %s", chave, r.bruto)
		}
	}

	// Plano sem prospecção: desabilitada, com o motivo.
	a.limitePlano("profissional", 0)
	r = a.api("GET", "", nil)
	if r.corpo["habilitada"] != false || r.corpo["motivo"] != "seu plano não inclui prospecção" {
		t.Fatalf("plano com cota zero: %s", r.bruto)
	}

	// Serviço fora do ar: desabilitada, sem erro HTTP.
	a.limitePlano("profissional", 5000)
	a.dados.srv.Close()
	r = a.api("GET", "", nil)
	if r.status != http.StatusOK || r.corpo["habilitada"] != false || r.corpo["motivo"] == "" {
		t.Fatalf("serviço fora do ar: %d %s", r.status, r.bruto)
	}
	if cotaDe(t, r.corpo).Limite != 5000 {
		t.Fatal("a cota deve vir mesmo com o serviço fora do ar")
	}
}

func TestPainelFuncionaSemOServicoDeDados(t *testing.T) {
	a := montar(t, false)
	a.licenciar("essencial")
	r := a.api("GET", "", nil)
	if r.status != http.StatusOK || r.corpo["habilitada"] != false || !strings.Contains(fmt.Sprint(r.corpo["motivo"]), "não está disponível") {
		t.Fatalf("resumo sem serviço: %d %s", r.status, r.bruto)
	}
	if cotaDe(t, r.corpo).Limite != 1000 {
		t.Fatalf("cota sem serviço: %s", r.bruto)
	}
	exigirErro(t, a.api("POST", "/contagem", `{"filtros":{"ufs":["CE"]}}`), http.StatusServiceUnavailable, "base_em_carga")
	exigirErro(t, a.api("GET", "/catalogos/ufs", nil), http.StatusServiceUnavailable, "base_em_carga")
	exigirErro(t, a.api("POST", "/extracoes", map[string]any{"pedido_id": "p1", "cnpjs": cnpjs(1, 2)}), http.StatusServiceUnavailable, "base_em_carga")
	exigirErro(t, a.api("GET", "/cnpj/00000001000191", nil), http.StatusServiceUnavailable, "base_em_carga")
	if a.usado() != 0 {
		t.Fatal("nada pode ser descontado sem o serviço")
	}

	// O "Meu plano" segue respondendo, com a cota.
	req := httptest.NewRequest("GET", "/painel/api/empresas/acme/plano", nil)
	req.Header.Set("Authorization", "Bearer "+tokenAmbiente)
	w := httptest.NewRecorder()
	a.rotas.ServeHTTP(w, req)
	var plano struct {
		Prospeccao operacoes.Cota `json:"prospeccao"`
		Planos     []struct {
			Nome               string `json:"nome"`
			ProspeccaoLeadsMes int    `json:"prospeccao_leads_mes"`
		} `json:"planos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &plano); err != nil || w.Code != http.StatusOK {
		t.Fatalf("meu plano: %d %s", w.Code, w.Body.String())
	}
	if plano.Prospeccao.Limite != 1000 || plano.Prospeccao.Restante != 1000 || plano.Prospeccao.Mes == "" {
		t.Fatalf("meu plano sem a cota de prospecção: %s", w.Body.String())
	}
	if len(plano.Planos) == 0 || plano.Planos[0].ProspeccaoLeadsMes != 1000 {
		t.Fatalf("planos disponíveis sem a cota: %s", w.Body.String())
	}
}

func TestRepassesDaProspeccao(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")

	r := a.api("POST", "/contagem", `{"filtros":{"ufs":["CE"],"cnae_prefixos":["56"]}}`)
	if r.status != http.StatusOK || r.corpo["total"] != float64(1234) || r.corpo["total_modo"] != "exato" {
		t.Fatalf("contagem: %d %s", r.status, r.bruto)
	}
	if got := string(a.dados.ultimoCorpo()); got != `{"filtros":{"ufs":["CE"],"cnae_prefixos":["56"]}}` {
		t.Fatalf("o corpo da contagem deve chegar como veio: %s", got)
	}

	r = a.api("POST", "/previa", `{"filtros":{"ufs":["CE"]},"ordem":"recentes"}`)
	if r.status != http.StatusOK || r.corpo["ordem_aplicada"] != "relevancia" || len(r.corpo["itens"].([]any)) != 1 {
		t.Fatalf("prévia: %d %s", r.status, r.bruto)
	}

	r = a.api("GET", "/catalogos/cnaes?q=restaurante&codigos=56,5611201&limite=20", nil)
	if r.status != http.StatusOK || !strings.Contains(fmt.Sprint(r.corpo["consulta"]), "q=restaurante") || !strings.Contains(fmt.Sprint(r.corpo["consulta"]), "codigos=56%2C5611201") {
		t.Fatalf("catálogo: a consulta deve ser repassada: %d %s", r.status, r.bruto)
	}
	exigirErro(t, a.api("GET", "/catalogos/senhas", nil), http.StatusNotFound, "nao_encontrado")
	exigirErro(t, a.api("POST", "/contagem", `[1,2]`), http.StatusUnprocessableEntity, "filtro_invalido")

	// candidatos exige o pedido_id da lista e não o repassa ao serviço.
	exigirErro(t, a.api("POST", "/candidatos", `{"filtros":{"ufs":["CE"]},"limite":100}`), http.StatusUnprocessableEntity, "filtro_invalido")
	antes := a.dados.total("/v1/empresas/candidatos")
	r = a.api("POST", "/candidatos", `{"pedido_id":"3f2b","filtros":{"ufs":["CE"]},"limite":100}`)
	if r.status != http.StatusOK || r.corpo["fim"] != true {
		t.Fatalf("candidatos: %d %s", r.status, r.bruto)
	}
	if a.dados.total("/v1/empresas/candidatos") != antes+1 || strings.Contains(string(a.dados.ultimoCorpo()), "pedido_id") || !strings.Contains(string(a.dados.ultimoCorpo()), `"limite":100`) {
		t.Fatalf("corpo enviado ao serviço: %s", a.dados.ultimoCorpo())
	}
	if a.usado() != 0 {
		t.Fatal("contar, pré-visualizar e listar candidatos é grátis")
	}
}

func TestErrosDoServicoViramErrosComAsDuasChaves(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	casos := []struct {
		nome          string
		status        int
		corpo         string
		esperado      int
		codigo, texto string
	}{
		{"filtro inválido", 422, `{"erro":"informe ao menos UF, município ou CNAE"}`, 422, "filtro_invalido", "informe ao menos UF, município ou CNAE"},
		{"requisição ruim", 400, `{"erro":"corpo inválido"}`, 422, "filtro_invalido", "corpo inválido"},
		{"cursor de versão antiga", 409, `{"erro":"a base foi atualizada; refaça a busca"}`, 409, "base_atualizada", "a base foi atualizada; refaça a busca"},
		{"base não carregada", 503, `{"erro":"a base ainda não foi carregada"}`, 503, "base_em_carga", "a base ainda não foi carregada"},
		{"erro interno do serviço", 500, `quebrou`, 503, "base_em_carga", ""},
		{"token do serviço errado", 401, `{"erro":"não autorizado"}`, 503, "base_em_carga", ""},
	}
	for _, c := range casos {
		a.dados.mu.Lock()
		a.dados.responder = func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.corpo))
			return true
		}
		a.dados.mu.Unlock()
		r := a.api("POST", "/candidatos", `{"pedido_id":"p","filtros":{"ufs":["CE"]},"cursor":"velho"}`)
		exigirErro(t, r, c.esperado, c.codigo)
		if c.texto != "" && r.corpo["error"] != c.texto {
			t.Errorf("%s: mensagem %q (esperava %q)", c.nome, r.corpo["error"], c.texto)
		}
		if strings.Contains(r.bruto, "quebrou") {
			t.Errorf("%s: o corpo cru do serviço não pode vazar", c.nome)
		}
	}
	// Extração com a base fora do ar: 503 e nada descontado.
	exigirErro(t, a.api("POST", "/extracoes", map[string]any{"pedido_id": "p1", "cnpjs": cnpjs(1, 3)}), http.StatusServiceUnavailable, "base_em_carga")
	a.dados.srv.Close()
	exigirErro(t, a.api("POST", "/contagem", `{"filtros":{"ufs":["CE"]}}`), http.StatusServiceUnavailable, "base_em_carga")
	exigirErro(t, a.api("POST", "/extracoes", map[string]any{"pedido_id": "p1", "cnpjs": cnpjs(1, 3)}), http.StatusServiceUnavailable, "base_em_carga")
	if a.usado() != 0 {
		t.Fatal("falha do serviço não pode descontar da cota")
	}
}

func TestLimiteDeRequisicoesPorCliente(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	for i := 0; i < limitePrevia; i++ {
		if r := a.api("POST", "/previa", `{"filtros":{"ufs":["CE"]}}`); r.status != http.StatusOK {
			t.Fatalf("prévia %d: %d %s", i, r.status, r.bruto)
		}
	}
	r := a.api("POST", "/previa", `{"filtros":{"ufs":["CE"]}}`)
	exigirErro(t, r, http.StatusTooManyRequests, "limite_requisicoes")
	if r.cab.Get("Retry-After") == "" || r.corpo["tentar_em_segundos"] == nil {
		t.Fatalf("429 de limite sem Retry-After: %v %s", r.cab, r.bruto)
	}
	if _, temCota := r.corpo["cota"]; temCota {
		t.Fatal("429 de limite de requisições não é 429 de cota")
	}
	if a.dados.total("/v1/empresas/previa") != limitePrevia {
		t.Fatalf("o pedido barrado não pode chegar ao serviço: %d", a.dados.total("/v1/empresas/previa"))
	}
	// O limite é por rota: a contagem segue livre.
	if r := a.api("POST", "/contagem", `{"filtros":{"ufs":["CE"]}}`); r.status != http.StatusOK {
		t.Fatalf("contagem depois do limite da prévia: %d", r.status)
	}
	// candidatos: 30 por minuto.
	for i := 0; i < limiteCandidatos; i++ {
		if r := a.api("POST", "/candidatos", `{"pedido_id":"p","filtros":{"ufs":["CE"]}}`); r.status != http.StatusOK {
			t.Fatalf("candidatos %d: %d", i, r.status)
		}
	}
	exigirErro(t, a.api("POST", "/candidatos", `{"pedido_id":"p","filtros":{"ufs":["CE"]}}`), http.StatusTooManyRequests, "limite_requisicoes")
}

func TestExtracaoDescontaEIdempotente(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	lote := cnpjs(1, 200)
	a.dados.ausentes[lote[7]] = true
	a.dados.ausentes[lote[9]] = true

	r := a.api("POST", "/extracoes", map[string]any{"pedido_id": "lista-1", "cnpjs": lote})
	if r.status != http.StatusOK {
		t.Fatalf("extração: %d %s", r.status, r.bruto)
	}
	if n := len(r.corpo["itens"].([]any)); n != 198 {
		t.Fatalf("itens: %d", n)
	}
	if n := len(r.corpo["nao_encontrados"].([]any)); n != 2 {
		t.Fatalf("não encontrados: %d", n)
	}
	// Desconta o que foi DEVOLVIDO, não o que foi pedido.
	if c := cotaDe(t, r.corpo); c.Usado != 198 || c.Restante != 5000-198 || c.Limite != 5000 {
		t.Fatalf("cota depois da extração: %+v", c)
	}

	// Mesmo pedido, mesmo lote (em outra ordem e com máscara): entrega de novo e não desconta.
	embaralhado := append([]string{}, lote[100:]...)
	embaralhado = append(embaralhado, lote[:100]...)
	embaralhado[0] = embaralhado[0][:2] + "." + embaralhado[0][2:5] + "." + embaralhado[0][5:8] + "/" + embaralhado[0][8:12] + "-" + embaralhado[0][12:]
	r = a.api("POST", "/extracoes", map[string]any{"pedido_id": "lista-1", "cnpjs": embaralhado})
	if r.status != http.StatusOK || len(r.corpo["itens"].([]any)) != 198 {
		t.Fatalf("repetição: %d %s", r.status, r.bruto)
	}
	if c := cotaDe(t, r.corpo); c.Usado != 198 {
		t.Fatalf("a repetição descontou de novo: %+v", c)
	}

	// Outro lote do mesmo pedido desconta; o mesmo lote em OUTRO pedido também.
	r = a.api("POST", "/extracoes", map[string]any{"pedido_id": "lista-1", "cnpjs": cnpjs(2, 50)})
	if c := cotaDe(t, r.corpo); c.Usado != 248 {
		t.Fatalf("segundo lote: %+v", c)
	}
	r = a.api("POST", "/extracoes", map[string]any{"pedido_id": "lista-2", "cnpjs": cnpjs(2, 50)})
	if c := cotaDe(t, r.corpo); c.Usado != 298 {
		t.Fatalf("outro pedido: %+v", c)
	}

	// Nenhuma empresa encontrada: nada é gravado.
	fantasmas := cnpjs(3, 3)
	for _, c := range fantasmas {
		a.dados.ausentes[c] = true
	}
	r = a.api("POST", "/extracoes", map[string]any{"pedido_id": "lista-3", "cnpjs": fantasmas})
	if r.status != http.StatusOK || len(r.corpo["itens"].([]any)) != 0 || len(r.corpo["nao_encontrados"].([]any)) != 3 || cotaDe(t, r.corpo).Usado != 298 {
		t.Fatalf("lote vazio: %d %s", r.status, r.bruto)
	}
	var registros, soma int
	if err := poolTeste.QueryRow(context.Background(), `SELECT count(*), coalesce(sum(quantidade),0) FROM prospeccao_extracoes WHERE cliente_id=$1`, a.cliente.ID).Scan(&registros, &soma); err != nil {
		t.Fatal(err)
	}
	if registros != 3 || soma != 298 || a.usado() != 298 {
		t.Fatalf("registros=%d soma=%d usado=%d", registros, soma, a.usado())
	}

	// incluir_pf é repassado ao serviço.
	_ = a.api("POST", "/extracoes", map[string]any{"pedido_id": "lista-4", "cnpjs": cnpjs(4, 1), "incluir_pf": true})
	if !strings.Contains(string(a.dados.ultimoCorpo()), `"incluir_pf":true`) {
		t.Fatalf("incluir_pf não chegou ao serviço: %s", a.dados.ultimoCorpo())
	}
}

func TestExtracaoValidaOPedido(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	casos := map[string]any{
		"sem pedido_id":      map[string]any{"cnpjs": cnpjs(1, 2)},
		"sem cnpjs":          map[string]any{"pedido_id": "p"},
		"mais de 500":        map[string]any{"pedido_id": "p", "cnpjs": cnpjs(1, 501)},
		"cnpj curto":         map[string]any{"pedido_id": "p", "cnpjs": []string{"123"}},
		"cnpj com dv letras": map[string]any{"pedido_id": "p", "cnpjs": []string{"12ABC34501DEAB"}},
		"corpo ilegível":     `{"pedido_id":`,
	}
	for nome, corpo := range casos {
		r := a.api("POST", "/extracoes", corpo)
		if r.status != http.StatusUnprocessableEntity || r.corpo["error"] == nil || r.corpo["erro"] == nil {
			t.Errorf("%s: %d %s", nome, r.status, r.bruto)
		}
	}
	if a.dados.total("/v1/empresas/detalhes") != 0 || a.usado() != 0 {
		t.Fatal("pedido inválido não chega ao serviço nem desconta")
	}
	// CNPJ alfanumérico (em produção desde julho de 2026) é aceito, em maiúsculas.
	r := a.api("POST", "/extracoes", map[string]any{"pedido_id": "p", "cnpjs": []string{"12.abc.345/01de-35"}})
	if r.status != http.StatusOK || !strings.Contains(string(a.dados.ultimoCorpo()), `"12ABC34501DE35"`) {
		t.Fatalf("CNPJ alfanumérico: %d %s / %s", r.status, r.bruto, a.dados.ultimoCorpo())
	}
}

func TestExtracaoSemSaldoNaoEntregaNada(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	a.limitePlano("profissional", 100)

	if r := a.api("POST", "/extracoes", map[string]any{"pedido_id": "l1", "cnpjs": cnpjs(1, 80)}); r.status != http.StatusOK {
		t.Fatalf("primeiro lote: %d %s", r.status, r.bruto)
	}
	// Restam 20 e o lote tem 50: 429 com a cota, sem itens e sem desconto.
	r := a.api("POST", "/extracoes", map[string]any{"pedido_id": "l1", "cnpjs": cnpjs(2, 50)})
	exigirErro(t, r, http.StatusTooManyRequests, "cota")
	if c := cotaDe(t, r.corpo); c.Limite != 100 || c.Usado != 80 || c.Restante != 20 {
		t.Fatalf("cota no 429: %+v", c)
	}
	if _, temItens := r.corpo["itens"]; temItens || r.cab.Get("Retry-After") != "" {
		t.Fatalf("429 de cota não entrega itens nem manda Retry-After: %s", r.bruto)
	}
	if a.usado() != 80 {
		t.Fatalf("lote recusado descontou: %d", a.usado())
	}
	// O app pede um lote do tamanho do que resta: cabe.
	r = a.api("POST", "/extracoes", map[string]any{"pedido_id": "l1", "cnpjs": cnpjs(2, 20)})
	if r.status != http.StatusOK || cotaDe(t, r.corpo).Restante != 0 {
		t.Fatalf("lote do tamanho do saldo: %d %s", r.status, r.bruto)
	}
	// Cota zerada: lote novo é recusado sem nem consultar a base...
	antes := a.dados.total("/v1/empresas/detalhes")
	exigirErro(t, a.api("POST", "/extracoes", map[string]any{"pedido_id": "l1", "cnpjs": cnpjs(3, 1)}), http.StatusTooManyRequests, "cota")
	if a.dados.total("/v1/empresas/detalhes") != antes {
		t.Fatal("sem saldo nenhum, a base não deveria ser consultada")
	}
	// ...mas a repetição de um lote já pago continua sendo entregue.
	r = a.api("POST", "/extracoes", map[string]any{"pedido_id": "l1", "cnpjs": cnpjs(1, 80)})
	if r.status != http.StatusOK || len(r.corpo["itens"].([]any)) != 80 || a.usado() != 100 {
		t.Fatalf("repetição com a cota esgotada: %d usado=%d", r.status, a.usado())
	}
	// E continua valendo depois da virada do mês: o registro é de outra competência.
	if _, err := poolTeste.Exec(context.Background(), `UPDATE prospeccao_extracoes SET competencia = competencia - interval '1 month' WHERE cliente_id=$1;`, a.cliente.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := poolTeste.Exec(context.Background(), `UPDATE prospeccao_uso SET competencia = competencia - interval '1 month' WHERE cliente_id=$1`, a.cliente.ID); err != nil {
		t.Fatal(err)
	}
	r = a.api("POST", "/extracoes", map[string]any{"pedido_id": "l1", "cnpjs": cnpjs(1, 80)})
	if r.status != http.StatusOK || len(r.corpo["itens"].([]any)) != 80 {
		t.Fatalf("repetição no mês seguinte: %d %s", r.status, r.bruto)
	}
	if c := cotaDe(t, r.corpo); c.Usado != 0 || c.Restante != 100 {
		t.Fatalf("mês novo começa zerado e a repetição não desconta: %+v", c)
	}
}

func TestExtracoesSimultaneas(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	a.limitePlano("profissional", 100)

	// 12 chamadas idênticas ao mesmo tempo: todas entregam, uma só desconta.
	var grupo sync.WaitGroup
	var entregas atomic.Int32
	lote := cnpjs(1, 30)
	for i := 0; i < 12; i++ {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			if r := a.api("POST", "/extracoes", map[string]any{"pedido_id": "igual", "cnpjs": lote}); r.status == http.StatusOK && len(r.corpo["itens"].([]any)) == 30 {
				entregas.Add(1)
			}
		}()
	}
	grupo.Wait()
	if entregas.Load() != 12 || a.usado() != 30 {
		t.Fatalf("chamadas idênticas: entregas=%d usado=%d (esperava 12 e 30)", entregas.Load(), a.usado())
	}

	// 20 lotes diferentes de 10 disputando os 70 que restam: passam exatamente 7.
	var aceitos, recusados atomic.Int32
	for i := 0; i < 20; i++ {
		grupo.Add(1)
		go func(i int) {
			defer grupo.Done()
			r := a.api("POST", "/extracoes", map[string]any{"pedido_id": "disputa", "cnpjs": cnpjs(10+i, 10)})
			switch r.status {
			case http.StatusOK:
				aceitos.Add(1)
			case http.StatusTooManyRequests:
				recusados.Add(1)
			}
		}(i)
	}
	grupo.Wait()
	if aceitos.Load() != 7 || recusados.Load() != 13 || a.usado() != 100 {
		t.Fatalf("disputa pela cota: aceitos=%d recusados=%d usado=%d", aceitos.Load(), recusados.Load(), a.usado())
	}
	var soma int
	_ = poolTeste.QueryRow(context.Background(), `SELECT coalesce(sum(quantidade),0) FROM prospeccao_extracoes WHERE cliente_id=$1`, a.cliente.ID).Scan(&soma)
	if soma != 100 {
		t.Fatalf("os registros de extração não fecham com o uso: %d", soma)
	}
}

func TestRegrasDaCota(t *testing.T) {
	a := montar(t, true)
	ctx := context.Background()

	// Sem licença alguma: vale a configuração (padrão 300).
	if c, err := a.ops.CotaProspeccao(ctx, a.cliente); err != nil || c.Limite != 300 {
		t.Fatalf("sem licença: %+v %v", c, err)
	}
	_ = a.repo.SalvarConfiguracao(ctx, "prospeccao.cota_sem_licenca", "0")
	r := a.api("GET", "", nil)
	if r.corpo["habilitada"] != false || cotaDe(t, r.corpo).Limite != 0 {
		t.Fatalf("configuração 0 deve desabilitar: %s", r.bruto)
	}
	_ = a.repo.SalvarConfiguracao(ctx, "prospeccao.cota_sem_licenca", "lixo")
	if c, _ := a.ops.CotaProspeccao(ctx, a.cliente); c.Limite != operacoes.CotaSemLicencaPadrao {
		t.Fatalf("configuração inválida cai no padrão: %+v", c)
	}

	// Licença do grupo: cada CNPJ tem a cota inteira do plano.
	lic := a.licenciar("escala")
	if c, _ := a.ops.CotaProspeccao(ctx, a.cliente); c.Limite != 20000 {
		t.Fatalf("licença vigente: %+v", c)
	}
	// Carência: licença vencida (ainda não suspensa) mantém a cota do plano.
	if _, err := poolTeste.Exec(ctx, `UPDATE licencas SET inicio = current_date - 40, fim = current_date - 5, status='vencida' WHERE id=$1`, lic.ID); err != nil {
		t.Fatal(err)
	}
	if c, _ := a.ops.CotaProspeccao(ctx, a.cliente); c.Limite != 20000 {
		t.Fatalf("licença vencida deveria manter a cota do plano: %+v", c)
	}
	// Licença cancelada não conta: volta à regra de quem não tem licença.
	if _, err := poolTeste.Exec(ctx, `UPDATE licencas SET status='cancelada' WHERE id=$1`, lic.ID); err != nil {
		t.Fatal(err)
	}
	if c, _ := a.ops.CotaProspeccao(ctx, a.cliente); c.Limite != operacoes.CotaSemLicencaPadrao {
		t.Fatalf("licença cancelada: %+v", c)
	}
	// Uso acima do limite (plano rebaixado no meio do mês): restante nunca fica negativo.
	_ = a.repo.SalvarConfiguracao(ctx, "prospeccao.cota_sem_licenca", "10")
	if _, err := poolTeste.Exec(ctx, `INSERT INTO prospeccao_uso (cliente_id, competencia, leads) VALUES ($1, $2::date, 25)`, a.cliente.ID, operacoes.Competencia(time.Now()).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	if c, _ := a.ops.CotaProspeccao(ctx, a.cliente); c.Limite != 10 || c.Usado != 25 || c.Restante != 0 {
		t.Fatalf("uso acima do limite: %+v", c)
	}
}

func TestFichaAvulsaDoCNPJ(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	const pj, pf, some = "00000001000191", "00000002000172", "00000003000153"
	a.dados.pf[pf] = true
	a.dados.ausentes[some] = true

	r := a.api("GET", "/cnpj/00.000.001-0001.91", nil)
	if r.status != http.StatusOK || r.corpo["cnpj"] != pj || len(r.corpo["telefones"].([]any)) != 1 || r.corpo["email"] == "" {
		t.Fatalf("ficha de pessoa jurídica: %d %s", r.status, r.bruto)
	}
	// Pessoa física sai sem contatos e sem sócios, mesmo que o serviço os mande.
	r = a.api("GET", "/cnpj/"+pf, nil)
	if r.status != http.StatusOK || r.corpo["pessoa_fisica"] != true || len(r.corpo["telefones"].([]any)) != 0 || r.corpo["email"] != "" || len(r.corpo["socios"].([]any)) != 0 {
		t.Fatalf("ficha de pessoa física deveria sair sem contatos: %s", r.bruto)
	}
	if strings.Contains(r.bruto, "85900000001") || strings.Contains(r.bruto, "exemplo.invalid") || strings.Contains(r.bruto, "SOCIO 1") {
		t.Fatalf("contato de pessoa física vazou: %s", r.bruto)
	}
	exigirErro(t, a.api("GET", "/cnpj/"+some, nil), http.StatusNotFound, "nao_encontrado")
	exigirErro(t, a.api("GET", "/cnpj/123", nil), http.StatusUnprocessableEntity, "filtro_invalido")

	// A ficha avulsa não toca na cota de leads; conta em "consultas".
	leads, consultas, _ := a.repo.UsoProspeccao(context.Background(), a.cliente.ID, operacoes.Competencia(time.Now()))
	if leads != 0 || consultas != 3 {
		t.Fatalf("uso depois das fichas: leads=%d consultas=%d", leads, consultas)
	}

	// Limite diário: 300 fichas por cliente.
	if _, err := poolTeste.Exec(context.Background(), `UPDATE prospeccao_uso SET consultas_no_dia=$2 WHERE cliente_id=$1`, a.cliente.ID, limiteCNPJDia); err != nil {
		t.Fatal(err)
	}
	antes := a.dados.total("GET /v1/empresas/")
	r = a.api("GET", "/cnpj/"+pj, nil)
	exigirErro(t, r, http.StatusTooManyRequests, "limite_requisicoes")
	if r.cab.Get("Retry-After") == "" || a.dados.total("GET /v1/empresas/") != antes {
		t.Fatalf("limite diário: Retry-After=%q", r.cab.Get("Retry-After"))
	}
	// No dia seguinte o contador recomeça.
	if _, err := poolTeste.Exec(context.Background(), `UPDATE prospeccao_uso SET consultas_dia = consultas_dia - 1 WHERE cliente_id=$1`, a.cliente.ID); err != nil {
		t.Fatal(err)
	}
	if r := a.api("GET", "/cnpj/"+pj, nil); r.status != http.StatusOK {
		t.Fatalf("dia novo: %d %s", r.status, r.bruto)
	}
	var noDia int
	_ = poolTeste.QueryRow(context.Background(), `SELECT consultas_no_dia FROM prospeccao_uso WHERE cliente_id=$1`, a.cliente.ID).Scan(&noDia)
	if noDia != 1 {
		t.Fatalf("contador do dia novo: %d", noDia)
	}
}

func TestLimitePorMinutoDaFichaAvulsa(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	for i := 0; i < limiteCNPJ; i++ {
		if r := a.api("GET", "/cnpj/00000001000191", nil); r.status != http.StatusOK {
			t.Fatalf("ficha %d: %d", i, r.status)
		}
	}
	exigirErro(t, a.api("GET", "/cnpj/00000001000191", nil), http.StatusTooManyRequests, "limite_requisicoes")
	// O pedido barrado pelo limite por minuto não gasta o limite do dia.
	if _, consultas, _ := a.repo.UsoProspeccao(context.Background(), a.cliente.ID, operacoes.Competencia(time.Now())); consultas != limiteCNPJ {
		t.Fatalf("consultas contadas: %d", consultas)
	}
}

func TestUsoPorClienteNaPagina(t *testing.T) {
	a := montar(t, true)
	a.licenciar("profissional")
	if r := a.api("POST", "/extracoes", map[string]any{"pedido_id": "l1", "cnpjs": cnpjs(1, 40)}); r.status != http.StatusOK {
		t.Fatalf("extração: %d", r.status)
	}
	uso, err := a.repo.UsoProspeccaoPorCliente(context.Background(), operacoes.Competencia(time.Now()))
	if err != nil || len(uso) != 1 || uso[0].Leads != 40 || uso[0].ClienteSlug != "acme" || uso[0].UltimaExtracao == nil {
		t.Fatalf("uso por cliente: %+v %v", uso, err)
	}
	// A página exige sessão: sem ela, vai para o login.
	w := httptest.NewRecorder()
	a.rotas.ServeHTTP(w, httptest.NewRequest("GET", "/painel/base-de-dados", nil))
	if w.Code != http.StatusSeeOther || !strings.HasSuffix(w.Header().Get("Location"), "/painel/login") {
		t.Fatalf("página sem sessão: %d %s", w.Code, w.Header().Get("Location"))
	}

	admin := a.entrar(modelo.PerfilAdmin)
	w = a.comSessao(admin, "GET", "/painel/base-de-dados", "")
	html := w.Body.String()
	for _, esperado := range []string{"Base de dados", "4.321", "2026-11", "2026-T3", "demonstração", "Verificar e carregar agora", "ACME", "5.000", ">40<", "Profissional"} {
		if w.Code != http.StatusOK || !strings.Contains(html, esperado) {
			t.Errorf("página (%d) sem %q", w.Code, esperado)
		}
	}

	// Administrador dispara a verificação; o serviço recebe o pedido.
	w = a.comSessao(admin, "POST", "/painel/base-de-dados/carregar", "fonte=tudo")
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "ok=") || a.dados.total("POST /v1/cargas") != 1 {
		t.Fatalf("carregar: %d %s", w.Code, w.Header().Get("Location"))
	}
	if !strings.Contains(string(a.dados.ultimoCorpo()), `"fonte":"tudo"`) {
		t.Fatalf("corpo da carga: %s", a.dados.ultimoCorpo())
	}
	// Carga já em curso (ou modo amostra): 409 vira aviso, não erro.
	a.dados.mu.Lock()
	a.dados.responder = func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		if r.URL.Path != "/v1/cargas" {
			return false
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"erro":"serviço em modo amostra"}`))
		return true
	}
	a.dados.mu.Unlock()
	w = a.comSessao(admin, "POST", "/painel/base-de-dados/carregar", "fonte=cnpj")
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "aviso=") {
		t.Fatalf("carga em conflito: %d %s", w.Code, w.Header().Get("Location"))
	}
	// Perfil comercial vê a página, mas não dispara carga.
	comercial := a.entrar(modelo.PerfilComercial)
	if w = a.comSessao(comercial, "GET", "/painel/base-de-dados", ""); w.Code != http.StatusOK || strings.Contains(w.Body.String(), "Verificar e carregar agora") {
		t.Fatalf("comercial: %d", w.Code)
	}
	if w = a.comSessao(comercial, "POST", "/painel/base-de-dados/carregar", "fonte=tudo"); w.Code != http.StatusForbidden || a.dados.total("POST /v1/cargas") != 2 {
		t.Fatalf("comercial não pode carregar: %d", w.Code)
	}

	// Serviço fora do ar: a página abre com o aviso e segue mostrando o uso.
	a.dados.srv.Close()
	w = a.comSessao(admin, "GET", "/painel/base-de-dados", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Não consegui consultar o serviço de dados") || !strings.Contains(w.Body.String(), "ACME") {
		t.Fatalf("página com o serviço fora do ar: %d", w.Code)
	}
	// A ficha do cliente mostra a linha de uso.
	w = a.comSessao(admin, "GET", fmt.Sprintf("/painel/clientes/%d", a.cliente.ID), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Prospecção no mês:") || !strings.Contains(w.Body.String(), "40 de 5.000 leads") {
		t.Fatalf("ficha do cliente sem a linha de prospecção: %d", w.Code)
	}
}

func TestPaginaDaBaseSemOServico(t *testing.T) {
	a := montar(t, false)
	admin := a.entrar(modelo.PerfilAdmin)
	w := a.comSessao(admin, "GET", "/painel/base-de-dados", "")
	html := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(html, "não está configurado") || strings.Contains(html, "Verificar e carregar agora") {
		t.Fatalf("página sem o serviço: %d %s", w.Code, html)
	}
	w = a.comSessao(admin, "POST", "/painel/base-de-dados/carregar", "fonte=tudo")
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "erro=") {
		t.Fatalf("carregar sem o serviço: %d %s", w.Code, w.Header().Get("Location"))
	}
	// O formulário de plano grava a cota de prospecção.
	p, _ := a.repo.PlanoPorCodigo(context.Background(), "essencial")
	w = a.comSessao(admin, "GET", fmt.Sprintf("/painel/planos/%d/editar", p.ID), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `name="prospeccao_leads_mes" min="0" step="1" value="1000"`) {
		t.Fatalf("formulário do plano sem o campo: %d", w.Code)
	}
	form := "codigo=essencial&nome=Essencial&usuarios_min=1&usuarios_max=3&preco_mensal=99,00&preco_anual=0&ordem=1&ativo=on&prospeccao_leads_mes=1500"
	if w = a.comSessao(admin, "POST", fmt.Sprintf("/painel/planos/%d/editar", p.ID), form); w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), "ok=") {
		t.Fatalf("salvar plano: %d %s", w.Code, w.Header().Get("Location"))
	}
	if p, _ = a.repo.PlanoPorCodigo(context.Background(), "essencial"); p.ProspeccaoLeadsMes != 1500 || p.PrecoMensalCentavos != 9900 {
		t.Fatalf("plano salvo: %+v", p)
	}
	w = a.comSessao(admin, "GET", "/painel/planos", "")
	if !strings.Contains(w.Body.String(), "Leads de prospecção/mês") || !strings.Contains(w.Body.String(), "1.500") {
		t.Fatal("lista de planos sem a coluna de prospecção")
	}
	p.ProspeccaoLeadsMes = 1000
	_, _ = a.repo.SalvarPlano(context.Background(), p)
}

// entrar cria um usuário do painel com o perfil e devolve os cookies da sessão.
func (a *ambiente) entrar(perfil modelo.Perfil) []*http.Cookie {
	a.t.Helper()
	ctx := context.Background()
	email := string(perfil) + "@crmia.test"
	_, _ = poolTeste.Exec(ctx, `DELETE FROM usuarios WHERE email=$1`, email)
	id, err := a.repo.CriarUsuario(ctx, modelo.Usuario{Nome: "Teste " + string(perfil), Email: email, SenhaHash: "hash-" + string(perfil), Perfil: perfil, Ativo: true})
	if err != nil {
		a.t.Fatal(err)
	}
	u, err := a.repo.UsuarioPorID(ctx, id)
	if err != nil {
		a.t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if err := a.sessoes.Iniciar(w, u); err != nil {
		a.t.Fatal(err)
	}
	return w.Result().Cookies()
}

func (a *ambiente) comSessao(cookies []*http.Cookie, metodo, caminho, form string) *httptest.ResponseRecorder {
	a.t.Helper()
	var req *http.Request
	if metodo == http.MethodPost {
		for _, c := range cookies {
			if c.Name == auth.CookieCSRF {
				form += "&" + auth.CampoCSRF + "=" + url.QueryEscape(c.Value)
			}
		}
		req = httptest.NewRequest(metodo, caminho, strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(metodo, caminho, nil)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	a.rotas.ServeHTTP(w, req)
	return w
}
