package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/armando-couto/crm-ia/dados/internal/amostra"
	"github.com/armando-couto/crm-ia/dados/internal/banco"
	"github.com/armando-couto/crm-ia/dados/internal/busca"
	"github.com/armando-couto/crm-ia/dados/internal/carga"
	"github.com/armando-couto/crm-ia/dados/internal/testebanco"
)

// A API é testada de ponta a ponta sobre a base de amostra, num PostgreSQL
// de verdade (CRMIA_TEST_PG).

const tokenTeste = "0123456789abcdef0123456789abcdef"

var poolTeste *pgxpool.Pool

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if url, ok := testebanco.Criar("crmia_test_dados_api"); ok {
		ctx := context.Background()
		var err error
		poolTeste, err = banco.Conectar(ctx, url, 8, map[string]string{"jit": "off", "work_mem": "64MB", "statement_timeout": "4000"})
		if err == nil {
			err = banco.Migrar(ctx, poolTeste)
		}
		if err == nil {
			err = amostra.Garantir(ctx, poolTeste)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "não consegui preparar o banco de teste:", err)
			os.Exit(1)
		}
	}
	codigo := m.Run()
	if poolTeste != nil {
		poolTeste.Close()
	}
	os.Exit(codigo)
}

// cargasFalsas imita o agendador.
type cargasFalsas struct {
	amostra  bool
	emCurso  bool
	pedidas  []string
	situacao carga.EstadoCarga
}

func (c *cargasFalsas) Situacao(ctx context.Context) (carga.EstadoCarga, error) {
	if poolTeste != nil && c.situacao.Estado == "" {
		return carga.Situacao(ctx, poolTeste)
	}
	return c.situacao, nil
}

func (c *cargasFalsas) Iniciar(fonte string) error {
	switch {
	case c.amostra:
		return carga.ErrAmostra
	case c.emCurso:
		return carga.ErrEmCurso
	}
	c.pedidas = append(c.pedidas, fonte)
	c.emCurso = true
	return nil
}

func montar(t *testing.T, esquema string, cargas *cargasFalsas) *httptest.Server {
	t.Helper()
	if poolTeste == nil {
		t.Skip("sem PostgreSQL de teste (CRMIA_TEST_PG)")
	}
	srv := httptest.NewServer(Nova(busca.Novo(poolTeste, esquema, esquema == carga.EsquemaAmostra), cargas, tokenTeste).Rotas())
	t.Cleanup(srv.Close)
	return srv
}

func chamar(t *testing.T, srv *httptest.Server, metodo, caminho, token string, corpo any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	switch c := corpo.(type) {
	case nil:
	case string:
		r = strings.NewReader(c)
	default:
		j, _ := json.Marshal(c)
		r = bytes.NewReader(j)
	}
	req, _ := http.NewRequest(metodo, srv.URL+caminho, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var saida map[string]any
	dados, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(dados, &saida); err != nil {
		t.Fatalf("%s %s: resposta não é JSON: %s", metodo, caminho, dados)
	}
	return resp.StatusCode, saida
}

func TestSaudeSemTokenEDemaisRotasComToken(t *testing.T) {
	srv := montar(t, carga.EsquemaAmostra, &cargasFalsas{})
	if status, r := chamar(t, srv, "GET", "/saude", "", nil); status != 200 || r["status"] != "ok" {
		t.Fatalf("saúde: %d %v", status, r)
	}
	for _, rota := range [][2]string{{"GET", "/v1/situacao"}, {"GET", "/v1/catalogos/ufs"}, {"POST", "/v1/empresas/contagem"}, {"POST", "/v1/empresas/detalhes"},
		{"GET", "/v1/empresas/00000001000136"}, {"POST", "/v1/cargas"}} {
		for _, token := range []string{"", "errado" + tokenTeste, tokenTeste[:20]} {
			if status, r := chamar(t, srv, rota[0], rota[1], token, `{}`); status != 401 || r["erro"] != "não autorizado" {
				t.Fatalf("%s %s com token %q: %d %v", rota[0], rota[1], token, status, r)
			}
		}
	}
}

func TestSituacaoNoFormatoDoContrato(t *testing.T) {
	srv := montar(t, carga.EsquemaAmostra, &cargasFalsas{})
	status, r := chamar(t, srv, "GET", "/v1/situacao", tokenTeste, nil)
	if status != 200 {
		t.Fatalf("%d %v", status, r)
	}
	for _, chave := range []string{"pronta", "amostra", "referencia_cnpj", "referencia_pgfn", "versao", "empresas", "socios", "devedores", "atualizada_em", "espaco_bytes", "ufs", "carga"} {
		if _, ok := r[chave]; !ok {
			t.Errorf("falta %q em situacao: %v", chave, r)
		}
	}
	if r["pronta"] != true || r["amostra"] != true || !strings.HasPrefix(r["versao"].(string), "amostra-") || r["empresas"].(float64) < 2000 {
		t.Fatalf("situação: %v", r)
	}
	c := r["carga"].(map[string]any)
	for _, chave := range []string{"estado", "fonte", "referencia", "etapa", "percentual", "iniciada_em", "erro", "arquivos"} {
		if _, ok := c[chave]; !ok {
			t.Errorf("falta %q em carga: %v", chave, c)
		}
	}
	if c["estado"] != "ociosa" || c["arquivos"] == nil {
		t.Fatalf("carga: %v", c)
	}
}

func TestFluxoDeBuscaPelaAPI(t *testing.T) {
	srv := montar(t, carga.EsquemaAmostra, &cargasFalsas{})
	filtros := map[string]any{"ufs": []string{"CE"}, "com_celular": true}

	status, cont := chamar(t, srv, "POST", "/v1/empresas/contagem", tokenTeste, map[string]any{"filtros": filtros})
	if status != 200 || cont["total_modo"] != "exato" || cont["total"].(float64) < 100 {
		t.Fatalf("contagem: %d %v", status, cont)
	}
	total := int(cont["total"].(float64))
	if p := cont["parciais"].(map[string]any); int(p["com_celular"].(float64)) != total || p["com_email"] == nil || p["com_divida"] == nil {
		t.Fatalf("parciais: %v", p)
	}

	status, prev := chamar(t, srv, "POST", "/v1/empresas/previa", tokenTeste, map[string]any{"filtros": filtros, "limite": 5})
	if status != 200 || len(prev["itens"].([]any)) != 5 || prev["versao"] == "" || prev["ordem_aplicada"] != "relevancia" {
		t.Fatalf("prévia: %d %v", status, prev)
	}
	item := prev["itens"].([]any)[0].(map[string]any)
	for _, chave := range []string{"cnpj", "razao_social", "nome_fantasia", "cnae", "porte", "abertura", "capital", "municipio", "uf", "pessoa_fisica", "mei",
		"tem_celular", "tem_telefone", "tem_email", "tem_divida", "socios_qtd", "qualidade"} {
		if _, ok := item[chave]; !ok {
			t.Errorf("falta %q na prévia", chave)
		}
	}
	if len(item) != 17 {
		t.Errorf("a prévia tem campos além do contrato: %v", item)
	}
	if status, r := chamar(t, srv, "POST", "/v1/empresas/previa", tokenTeste, map[string]any{"filtros": filtros, "limite": 26}); status != 422 {
		t.Fatalf("prévia acima de 25: %d %v", status, r)
	}

	// candidatos em páginas, até o fim
	var cnpjs []string
	cursor := ""
	for i := 0; i < 50; i++ {
		status, r := chamar(t, srv, "POST", "/v1/empresas/candidatos", tokenTeste, map[string]any{"filtros": filtros, "ordem": "capital_desc", "limite": 150, "cursor": cursor})
		if status != 200 || r["ordem_aplicada"] != "capital_desc" || r["versao"] != prev["versao"] {
			t.Fatalf("candidatos: %d %v", status, r)
		}
		for _, c := range r["cnpjs"].([]any) {
			cnpjs = append(cnpjs, c.(string))
		}
		if r["fim"] == true {
			break
		}
		cursor = r["cursor"].(string)
	}
	if len(cnpjs) != total {
		t.Fatalf("candidatos devolveram %d, a contagem diz %d", len(cnpjs), total)
	}
	if status, r := chamar(t, srv, "POST", "/v1/empresas/candidatos", tokenTeste, map[string]any{"filtros": filtros, "limite": 5001}); status != 422 {
		t.Fatalf("candidatos acima de 5000: %d %v", status, r)
	}

	// detalhes e ficha única
	status, det := chamar(t, srv, "POST", "/v1/empresas/detalhes", tokenTeste, map[string]any{"cnpjs": append(cnpjs[:3:3], "11222333000181")})
	if status != 200 || len(det["itens"].([]any)) != 3 || len(det["nao_encontrados"].([]any)) != 1 {
		t.Fatalf("detalhes: %d %v", status, det)
	}
	ficha := det["itens"].([]any)[0].(map[string]any)
	for _, chave := range []string{"cnpj", "matriz", "razao_social", "nome_fantasia", "natureza", "pessoa_fisica", "porte", "capital", "abertura", "situacao", "situacao_desde",
		"simples", "mei", "cnae", "cnaes_secundarios", "endereco", "telefones", "email", "email_tipo", "socios", "divida", "qualidade", "referencia"} {
		if _, ok := ficha[chave]; !ok {
			t.Errorf("falta %q na ficha", chave)
		}
	}
	if len(ficha) != 23 || len(ficha["telefones"].([]any)) == 0 {
		t.Errorf("ficha fora do contrato: %v", ficha)
	}
	status, uma := chamar(t, srv, "GET", "/v1/empresas/"+cnpjs[0], tokenTeste, nil)
	if status != 200 || uma["cnpj"] != cnpjs[0] {
		t.Fatalf("ficha única: %d %v", status, uma)
	}
	if status, r := chamar(t, srv, "GET", "/v1/empresas/11222333000181", tokenTeste, nil); status != 404 || r["erro"] == "" {
		t.Fatalf("CNPJ fora da base: %d %v", status, r)
	}
	if status, r := chamar(t, srv, "GET", "/v1/empresas/123", tokenTeste, nil); status != 422 {
		t.Fatalf("CNPJ inválido: %d %v", status, r)
	}
}

func TestPessoaFisicaSoSaiComIncluirPF(t *testing.T) {
	srv := montar(t, carga.EsquemaAmostra, &cargasFalsas{})
	var pf string
	if err := poolTeste.QueryRow(context.Background(), `SELECT cnpj FROM prospec_amostra.base_ativa WHERE pf AND fone1 IS NOT NULL ORDER BY id LIMIT 1`).Scan(&pf); err != nil {
		t.Fatal(err)
	}
	_, sem := chamar(t, srv, "GET", "/v1/empresas/"+pf, tokenTeste, nil)
	if sem["pessoa_fisica"] != true || len(sem["telefones"].([]any)) != 0 || sem["email"] != "" || len(sem["socios"].([]any)) != 0 {
		t.Fatalf("pessoa física sem incluir_pf: %v", sem)
	}
	_, com := chamar(t, srv, "GET", "/v1/empresas/"+pf+"?incluir_pf=true", tokenTeste, nil)
	if len(com["telefones"].([]any)) == 0 {
		t.Fatalf("pessoa física com incluir_pf: %v", com)
	}
	_, det := chamar(t, srv, "POST", "/v1/empresas/detalhes", tokenTeste, map[string]any{"cnpjs": []string{pf}, "incluir_pf": true})
	if len(det["itens"].([]any)[0].(map[string]any)["telefones"].([]any)) == 0 {
		t.Fatalf("detalhes com incluir_pf: %v", det)
	}
}

func TestErrosDaBusca(t *testing.T) {
	srv := montar(t, carga.EsquemaAmostra, &cargasFalsas{})
	casos := []struct {
		caminho string
		corpo   any
		status  int
		trecho  string
	}{
		{"/v1/empresas/contagem", map[string]any{"filtros": map[string]any{}}, 422, "informe ao menos um filtro"},
		{"/v1/empresas/contagem", map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}, "mei": "sim"}}, 422, "para filtrar MEI, ligue 'incluir MEI e empresário individual'"},
		{"/v1/empresas/previa", map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}, "nome": "padaria"}}, 422, "exigem municipios"},
		{"/v1/empresas/candidatos", map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}}, "cursor": "xx"}, 422, "cursor inválido"},
		{"/v1/empresas/contagem", `{"filtros": `, 400, "corpo inválido"},
		{"/v1/empresas/detalhes", map[string]any{"cnpjs": []string{}}, 422, "informe cnpjs"},
		{"/v1/cargas", map[string]any{"fonte": "tudo mesmo"}, 422, "fonte deve ser"},
	}
	for _, c := range casos {
		status, r := chamar(t, srv, "POST", c.caminho, tokenTeste, c.corpo)
		if status != c.status || !strings.Contains(fmt.Sprint(r["erro"]), c.trecho) {
			t.Errorf("%s %v: %d %v, esperado %d com %q", c.caminho, c.corpo, status, r, c.status, c.trecho)
		}
	}
	if status, r := chamar(t, srv, "GET", "/v1/catalogos/cnaes?nivel=secao", tokenTeste, nil); status != 422 {
		t.Errorf("nível inválido: %d %v", status, r)
	}
}

func TestCatalogosPelaAPI(t *testing.T) {
	srv := montar(t, carga.EsquemaAmostra, &cargasFalsas{})
	status, r := chamar(t, srv, "GET", "/v1/catalogos/cnaes?q=padaria&limite=3", tokenTeste, nil)
	itens := r["itens"].([]any)
	if status != 200 || len(itens) != 3 {
		t.Fatalf("cnaes: %d %v", status, r)
	}
	if c := itens[0].(map[string]any); c["codigo"] != "47211" || c["nivel"] != "classe" || c["descricao"] == "" {
		t.Fatalf("item de cnae: %v", c)
	}
	_, r = chamar(t, srv, "GET", "/v1/catalogos/municipios?uf=CE&q=forta", tokenTeste, nil)
	if m := r["itens"].([]any)[0].(map[string]any); m["codigo"].(float64) != 1389 || m["nome"] != "FORTALEZA" || m["uf"] != "CE" || m["empresas"].(float64) == 0 {
		t.Fatalf("município: %v", m)
	}
	_, r = chamar(t, srv, "GET", "/v1/catalogos/naturezas", tokenTeste, nil)
	if n := r["itens"].([]any)[0].(map[string]any); n["codigo"] == nil || n["descricao"] == nil || n["pessoa_fisica"] == nil {
		t.Fatalf("natureza: %v", n)
	}
	_, r = chamar(t, srv, "GET", "/v1/catalogos/ufs", tokenTeste, nil)
	if u := r["itens"].([]any)[0].(map[string]any); u["uf"] == nil || u["nome"] == nil || u["empresas"] == nil {
		t.Fatalf("UF: %v", u)
	}
}

func TestBaseNaoCarregadaResponde503(t *testing.T) {
	srv := montar(t, carga.EsquemaServico, &cargasFalsas{}) // o esquema real não existe neste banco
	status, sit := chamar(t, srv, "GET", "/v1/situacao", tokenTeste, nil)
	if status != 200 || sit["pronta"] != false || sit["amostra"] != false {
		t.Fatalf("situação sem base: %d %v", status, sit)
	}
	for caminho, corpo := range map[string]any{
		"/v1/empresas/contagem":   map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}}},
		"/v1/empresas/previa":     map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}}},
		"/v1/empresas/candidatos": map[string]any{"filtros": map[string]any{"ufs": []string{"CE"}}},
		"/v1/empresas/detalhes":   map[string]any{"cnpjs": []string{"00000001000136"}},
	} {
		if status, r := chamar(t, srv, "POST", caminho, tokenTeste, corpo); status != 503 || r["erro"] != "a base ainda não foi carregada" {
			t.Errorf("%s: %d %v", caminho, status, r)
		}
	}
	if status, r := chamar(t, srv, "GET", "/v1/empresas/00000001000136", tokenTeste, nil); status != 503 {
		t.Errorf("ficha sem base: %d %v", status, r)
	}
}

func TestCargas(t *testing.T) {
	cargas := &cargasFalsas{situacao: carga.EstadoCarga{Estado: "ociosa", Arquivos: []carga.EstadoArquivo{}}}
	srv := montar(t, carga.EsquemaAmostra, cargas)
	if status, r := chamar(t, srv, "POST", "/v1/cargas", tokenTeste, map[string]string{"fonte": "pgfn"}); status != 202 || r["ok"] != true {
		t.Fatalf("carga aceita: %d %v", status, r)
	}
	if status, r := chamar(t, srv, "POST", "/v1/cargas", tokenTeste, map[string]string{"fonte": "tudo"}); status != 409 || r["erro"] != "já há uma carga em curso" {
		t.Fatalf("carga em curso: %d %v", status, r)
	}
	if len(cargas.pedidas) != 1 || cargas.pedidas[0] != "pgfn" {
		t.Fatalf("cargas pedidas: %v", cargas.pedidas)
	}
	srvAmostra := montar(t, carga.EsquemaAmostra, &cargasFalsas{amostra: true, situacao: cargas.situacao})
	if status, r := chamar(t, srvAmostra, "POST", "/v1/cargas", tokenTeste, map[string]string{"fonte": "tudo"}); status != 409 || r["erro"] != "serviço em modo amostra" {
		t.Fatalf("carga em modo amostra: %d %v", status, r)
	}
}

// Fica por último: troca a base.
func TestCursorAntigoResponde409(t *testing.T) {
	srv := montar(t, carga.EsquemaAmostra, &cargasFalsas{})
	corpo := map[string]any{"filtros": map[string]any{"ufs": []string{"SP"}}, "limite": 10}
	status, r := chamar(t, srv, "POST", "/v1/empresas/candidatos", tokenTeste, corpo)
	if status != 200 || r["cursor"] == "" {
		t.Fatalf("primeira página: %d %v", status, r)
	}
	if err := amostra.Montar(context.Background(), poolTeste); err != nil {
		t.Fatal(err)
	}
	corpo["cursor"] = r["cursor"]
	status, r2 := chamar(t, srv, "POST", "/v1/empresas/candidatos", tokenTeste, corpo)
	if status != 409 || r2["erro"] != "a base foi atualizada; refaça a busca" {
		t.Fatalf("cursor de versão antiga: %d %v", status, r2)
	}
	delete(corpo, "cursor")
	if status, r3 := chamar(t, srv, "POST", "/v1/empresas/candidatos", tokenTeste, corpo); status != 200 || r3["versao"] == r["versao"] {
		t.Fatalf("busca nova depois da troca: %d %v", status, r3)
	}
}
