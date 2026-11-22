package carga

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/encoding/charmap"

	"github.com/armando-couto/crm-ia/dados/internal/banco"
	"github.com/armando-couto/crm-ia/dados/internal/config"
	"github.com/armando-couto/crm-ia/dados/internal/testebanco"
	"github.com/armando-couto/crm-ia/dados/internal/texto"
)

var urlBancoTeste string

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	urlBancoTeste, _ = testebanco.Criar("crmia_test_dados_carga")
	os.Exit(m.Run())
}

// fontes imita a Receita (WebDAV) e a PGFN (índice HTML) num servidor só.
type fontes struct {
	mu       sync.Mutex
	arquivos map[string][]byte // "/cnpj/2026-11/Cnaes.zip", "/pgfn/2026_trimestre_03/Dados_abertos_FGTS.zip"
	versoes  map[string]int
	gets     map[string]int
}

func (f *fontes) publicar(caminho string, dados []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.arquivos[caminho] = dados
	f.versoes[caminho]++
}

func (f *fontes) etag(caminho string) string {
	return fmt.Sprintf(`"%s-v%d"`, path.Base(caminho), f.versoes[caminho])
}

func (f *fontes) filhos(pasta string) (pastas, arquivos []string) {
	vistos := map[string]bool{}
	for c := range f.arquivos {
		if !strings.HasPrefix(c, pasta) {
			continue
		}
		resto := strings.TrimPrefix(c, pasta)
		if i := strings.IndexByte(resto, '/'); i >= 0 {
			if !vistos[resto[:i]] {
				vistos[resto[:i]] = true
				pastas = append(pastas, resto[:i])
			}
		} else {
			arquivos = append(arquivos, resto)
		}
	}
	sort.Strings(pastas)
	sort.Strings(arquivos)
	return
}

func (f *fontes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	caminho := r.URL.Path
	switch {
	case r.Method == "PROPFIND":
		pastas, arquivos := f.filhos(caminho)
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`)
		fmt.Fprintf(&b, `<d:response><d:href>%s</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>`, caminho)
		for _, p := range pastas {
			fmt.Fprintf(&b, `<d:response><d:href>%s%s/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>`, caminho, p)
		}
		for _, a := range arquivos {
			fmt.Fprintf(&b, `<d:response><d:href>%s%s</d:href><d:propstat><d:prop><d:getcontentlength>%d</d:getcontentlength><d:getetag>%s</d:getetag><d:getlastmodified>Mon, 16 Nov 2026 10:00:23 GMT</d:getlastmodified><d:resourcetype/></d:prop></d:propstat></d:response>`,
				caminho, a, len(f.arquivos[caminho+a]), strings.ReplaceAll(f.etag(caminho+a), `"`, "&quot;"))
		}
		b.WriteString(`</d:multistatus>`)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(b.String()))
	case strings.HasSuffix(caminho, "/"):
		pastas, arquivos := f.filhos(caminho)
		var b strings.Builder
		b.WriteString(`<html><body><ul><li><a href="/"> Parent Directory</a></li>`)
		for _, p := range pastas {
			fmt.Fprintf(&b, `<li><a href="%s/"> %s/</a></li>`, p, p)
		}
		for _, a := range arquivos {
			fmt.Fprintf(&b, `<li><a href="%s"> %s</a></li>`, a, a)
		}
		b.WriteString(`</ul></body></html>`)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(b.String()))
	default:
		dados, ok := f.arquivos[caminho]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", f.etag(caminho))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Length", strconv.Itoa(len(dados)))
		if r.Method == http.MethodHead {
			return
		}
		f.gets[caminho]++
		_, _ = w.Write(dados)
	}
}

func zipCom(t *testing.T, membros map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	nomes := make([]string, 0, len(membros))
	for n := range membros {
		nomes = append(nomes, n)
	}
	sort.Strings(nomes)
	for _, n := range nomes {
		w, err := z.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		latin1, err := charmap.ISO8859_1.NewEncoder().String(membros[n])
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(latin1))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func linhaCSV(campos ...string) string {
	for i, c := range campos {
		campos[i] = `"` + strings.ReplaceAll(c, `"`, `""`) + `"`
	}
	return strings.Join(campos, ";") + "\n"
}

func cnpjDe(basico, ordem string) string { return basico + ordem + texto.DVCNPJ(basico+ordem) }

func estabelecimento(basico, ordem, situacao, cnae, sec, uf, municipio, ddd, tel, email string) string {
	matriz := "2"
	if ordem == "0001" {
		matriz = "1"
	}
	return linhaCSV(basico, ordem, texto.DVCNPJ(basico+ordem), matriz, "FANTASIA "+basico, situacao, "20200101", "00", "", "", "20150610", cnae, sec,
		"RUA", "DAS FLORES", "100", "", "CENTRO", "60000000", uf, municipio, ddd, tel, "", "", "", "", email, "", "")
}

// publicarMes sobe os 37 arquivos de uma referência do CNPJ.
func publicarMes(t *testing.T, f *fontes, ref string, estab0 string) {
	t.Helper()
	conteudo := map[string]string{
		"Cnaes":         linhaCSV("4721102", "Padaria e confeitaria com predominância de revenda") + linhaCSV("5611203", "Lanchonetes, casas de chá, de sucos e similares") + linhaCSV("6201501", "Desenvolvimento de programas de computador sob encomenda"),
		"Municipios":    linhaCSV("1389", "FORTALEZA") + linhaCSV("7107", "SAO PAULO"),
		"Naturezas":     linhaCSV("2062", "Sociedade Empresária Limitada") + linhaCSV("2135", "Empresário (Individual)"),
		"Qualificacoes": linhaCSV("49", "Sócio-Administrador") + linhaCSV("22", "Sócio"),
		"Paises":        linhaCSV("105", "BRASIL"),
		"Motivos":       linhaCSV("00", "SEM MOTIVO"),
		"Simples":       linhaCSV("00000001", "S", "20150610", "00000000", "N", "00000000", "00000000") + linhaCSV("00000004", "S", "20150610", "00000000", "S", "20150610", "00000000"),
		"Empresas0": linhaCSV("00000001", "PADARIA EXEMPLO LTDA", "2062", "49", "10000,00", "01", "") +
			linhaCSV("00000002", "LANCHONETE EXEMPLO LTDA", "2062", "49", "50000,00", "03", "") +
			linhaCSV("00000003", "EMPRESA BAIXADA LTDA", "2062", "49", "1000,00", "01", ""),
		"Empresas1": linhaCSV("00000004", "MARIA EXEMPLO 11144477735", "2135", "50", "0,00", "01", "") +
			linhaCSV("00000005", "PADARIA DOIS LTDA", "2062", "49", "2000000,00", "05", "") +
			linhaCSV("00000006", "SOFTWARE EXEMPLO LTDA", "2062", "49", "1000,00", "01", ""),
		"Estabelecimentos0": estab0,
		"Estabelecimentos1": estabelecimento("00000001", "0002", "02", "4721102", "", "CE", "1389", "85", "32456789", "") +
			estabelecimento("00000004", "0001", "02", "5611203", "", "CE", "1389", "85", "88776655", "maria@gmail.com") +
			estabelecimento("00000005", "0001", "02", "4721102", "", "SP", "7107", "", "", ""),
		"Socios0": linhaCSV("00000001", "2", "SOCIO EXEMPLO UM", "***000001**", "49", "20150610", "", "***000000**", "", "00", "5") +
			linhaCSV("00000001", "2", "SOCIO EXEMPLO DOIS", "***000002**", "22", "20150610", "", "***000000**", "", "00", "4") +
			linhaCSV("00000003", "2", "SOCIO DE EMPRESA BAIXADA", "***000003**", "49", "20150610", "", "***000000**", "", "00", "5"),
	}
	for _, nome := range ArquivosCNPJ() {
		f.publicar("/cnpj/"+ref+"/"+nome+".zip", zipCom(t, map[string]string{"F.K03200$Z." + strings.ToUpper(nome) + "CSV": conteudo[nome]}))
	}
}

func estab0Padrao() string {
	return estabelecimento("00000001", "0001", "02", "4721102", "5611203", "CE", "1389", "85", "99887766", "contato@padaria.com.br") +
		estabelecimento("00000002", "0001", "02", "5611203", "", "SP", "7107", "11", "32456789", "") +
		estabelecimento("00000003", "0001", "08", "4721102", "", "CE", "1389", "85", "99887700", "")
}

const cabecalhoSIDA = "CPF_CNPJ;TIPO_PESSOA;TIPO_DEVEDOR;NOME_DEVEDOR;UF_DEVEDOR;UNIDADE_RESPONSAVEL;NUMERO_INSCRICAO;TIPO_SITUACAO_INSCRICAO;SITUACAO_INSCRICAO;RECEITA_PRINCIPAL;DATA_INSCRICAO;INDICADOR_AJUIZADO;VALOR_CONSOLIDADO\n"

func publicarTrimestre(t *testing.T, f *fontes, pasta string, devedores map[string]string) {
	t.Helper()
	sida := cabecalhoSIDA
	for cnpj, valor := range devedores {
		sida += cnpj + ";Pessoa jurídica;PRINCIPAL;NOME;CE;CEARA;3062004015281;Em cobrança;ATIVA AJUIZADA;Receita da dívida ativa - COFINS;29/06/2020;SIM;" + valor + "\n"
		sida += cnpj + ";Pessoa jurídica;CORRESPONSAVEL;NOME;CE;CEARA;3062004015281;Em cobrança;ATIVA AJUIZADA;Receita da dívida ativa - COFINS;29/06/2020;SIM;" + valor + "\n"
	}
	prev := strings.Replace(cabecalhoSIDA, "RECEITA_PRINCIPAL", "TIPO_CREDITO", 1)
	fgts := "CPF_CNPJ;TIPO_PESSOA;TIPO_DEVEDOR;NOME_DEVEDOR;UF_DEVEDOR;UNIDADE_RESPONSAVEL;ENTIDADE_RESPONSAVEL;UNIDADE_INSCRICAO;NUMERO_INSCRICAO;TIPO_SITUACAO_INSCRICAO;SITUACAO_INSCRICAO;RECEITA_PRINCIPAL;DATA_INSCRICAO;INDICADOR_AJUIZADO;VALOR_CONSOLIDADO\n"
	f.publicar("/pgfn/"+pasta+"/Dados_abertos_Sistema_SIDA.zip", zipCom(t, map[string]string{"arquivo_lai_SIDA_1.csv": sida, "arquivo_lai_SIDA_2.csv": cabecalhoSIDA}))
	f.publicar("/pgfn/"+pasta+"/Dados_abertos_Previdenciario.zip", zipCom(t, map[string]string{"arquivo_lai_PREV_1.csv": prev}))
	f.publicar("/pgfn/"+pasta+"/Dados_abertos_FGTS.zip", zipCom(t, map[string]string{"arquivo_lai_FGTS_CE.csv": fgts}))
}

func consultar[T any](t *testing.T, pool *pgxpool.Pool, sql string, args ...any) T {
	t.Helper()
	var v T
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return v
}

// Ciclo completo contra fontes simuladas: publicação em andamento, primeira
// carga, nada a fazer, trimestre novo da PGFN (reconstrução sem baixar a
// Receita), mês novo com arquivo fora do leiaute (a base anterior continua
// em uso) e retomada depois do conserto.
func TestCicloDeCargaCompleto(t *testing.T) {
	if urlBancoTeste == "" {
		t.Skip("sem PostgreSQL de teste (CRMIA_TEST_PG)")
	}
	ctx := context.Background()
	pool, err := banco.Conectar(ctx, urlBancoTeste, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := banco.Migrar(ctx, pool); err != nil {
		t.Fatal(err)
	}

	f := &fontes{arquivos: map[string][]byte{}, versoes: map[string]int{}, gets: map[string]int{}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	publicarMes(t, f, "2026-10", estab0Padrao())
	publicarMes(t, f, "2026-11", estab0Padrao())
	publicarTrimestre(t, f, "2026_trimestre_03", map[string]string{"00.000.001/0001-" + texto.DVCNPJ("000000010001"): "1500.50"})

	dir := t.TempDir()
	cfg := config.Config{DatabaseURL: urlBancoTeste, CNPJURL: srv.URL + "/cnpj/", PGFNURL: srv.URL + "/pgfn/", DirTrabalho: dir,
		Conexoes: 2, EspacoEntreRequisicoes: time.Millisecond, DiscoBancoGB: 70, EsperaPublicacao: 0}
	s := Novo(cfg)
	s.Cliente = clienteDeTeste()
	s.MinimoTrabalho, s.MinimoTrabalhoPGFN = 0, 0

	// 1. primeira listagem da referência nova: ainda pode estar subindo. A
	// dívida ativa já entra; o CNPJ espera a segunda listagem igual.
	if err := s.Executar(ctx, FonteTudo); !errors.Is(err, ErrAguardando) {
		t.Fatalf("primeira execução: esperado ErrAguardando, veio %v", err)
	}
	if ref := consultar[string](t, pool, `SELECT referencia FROM pgfn.meta`); ref != "2026-T3" {
		t.Fatalf("referência da PGFN = %q", ref)
	}
	if existe := consultar[bool](t, pool, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'prospec')`); existe {
		t.Fatal("a base não pode existir antes de a publicação estar completa")
	}
	if sit, _ := Situacao(ctx, pool); sit.Estado != "ociosa" || sit.Etapa != "aguardando publicação completa" {
		t.Fatalf("situação = %+v", sit)
	}
	if f.gets["/cnpj/2026-11/Cnaes.zip"] != 0 {
		t.Fatal("nada do CNPJ pode ser baixado antes da segunda listagem")
	}

	// 2. segunda listagem igual: carga completa
	if err := s.Executar(ctx, FonteTudo); err != nil {
		t.Fatalf("segunda execução: %v", err)
	}
	if n := consultar[int](t, pool, `SELECT count(*) FROM prospec.base_ativa`); n != 5 {
		t.Fatalf("estabelecimentos ativos = %d, esperado 5", n)
	}
	refCNPJ := consultar[string](t, pool, `SELECT referencia_cnpj || '/' || referencia_pgfn || '/' || empresas || '/' || socios || '/' || devedores FROM prospec.meta`)
	if refCNPJ != "2026-11/2026-T3/5/2/1" {
		t.Fatalf("meta = %s", refCNPJ)
	}
	// CPF cortado da razão social; pessoa física marcada; dívida só da linha principal
	if r := consultar[string](t, pool, `SELECT razao_social || '|' || pf || '|' || mei || '|' || email_tipo FROM prospec.base_ativa WHERE cnpj = $1`, cnpjDe("00000004", "0001")); r != "MARIA EXEMPLO|true|true|gratuito" {
		t.Fatalf("empresário individual = %s", r)
	}
	if v := consultar[string](t, pool, `SELECT total::text || '|' || inscricoes FROM prospec.divida WHERE cnpj = $1`, cnpjDe("00000001", "0001")); v != "1500.50|1" {
		t.Fatalf("dívida = %s", v)
	}
	if n := consultar[int](t, pool, `SELECT count(*) FROM prospec.busca WHERE tem_divida`); n != 1 {
		t.Fatalf("com dívida = %d", n)
	}
	// ordem física (uf, municipio, cnae, cnpj) = ordem do id
	if n := consultar[int](t, pool, `SELECT count(*) FROM (SELECT id, row_number() OVER (ORDER BY uf, municipio, cnae, cnpj) AS pos FROM prospec.base_ativa) x WHERE id <> pos`); n != 0 {
		t.Fatalf("%d linhas fora da ordem (uf, municipio, cnae, cnpj)", n)
	}
	// limpeza: tabelas de carga, zips e controle somem depois da troca
	if existe := consultar[bool](t, pool, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname IN ('carga', 'prospec_novo', 'prospec_velho'))`); existe {
		t.Fatal("esquemas temporários deveriam ter sido apagados")
	}
	if n := consultar[int](t, pool, `SELECT count(*) FROM carga_arquivos`); n != 0 {
		t.Fatalf("controle de arquivos com %d linhas", n)
	}
	if sobras, _ := os.ReadDir(dir); len(sobras) != 0 {
		t.Fatalf("zips deveriam ter sido apagados: %v", sobras)
	}
	if f.gets["/cnpj/2026-11/Paises.zip"] != 0 || f.gets["/cnpj/2026-11/Empresas0.zip"] != 1 || f.gets["/cnpj/2026-10/Empresas0.zip"] != 0 {
		t.Fatalf("downloads inesperados: %v", f.gets)
	}
	versao1 := consultar[string](t, pool, `SELECT versao FROM prospec.meta`)

	// 3. mesma referência: nada a fazer
	if err := s.Executar(ctx, FonteTudo); err != nil {
		t.Fatal(err)
	}
	if v := consultar[string](t, pool, `SELECT versao FROM prospec.meta`); v != versao1 {
		t.Fatal("a base não deveria ter sido trocada")
	}
	if sit, _ := Situacao(ctx, pool); sit.Estado != "ociosa" || !strings.Contains(sit.Etapa, "nada a fazer") {
		t.Fatalf("situação = %+v", sit)
	}

	// 4. só a PGFN mudou: reconstrói a partir de prospec.base_ativa, sem baixar a Receita
	publicarTrimestre(t, f, "2026_trimestre_04", map[string]string{
		"00.000.002/0001-" + texto.DVCNPJ("000000020001"): "100.00", "00.000.005/0001-" + texto.DVCNPJ("000000050001"): "999.99",
		"99.999.999/0001-" + texto.DVCNPJ("999999990001"): "5.00"}) // devedor fora da base de ativas
	getsAntes := f.gets["/cnpj/2026-11/Empresas0.zip"]
	if err := s.Executar(ctx, FontePGFN); err != nil {
		t.Fatalf("carga da PGFN: %v", err)
	}
	if m := consultar[string](t, pool, `SELECT referencia_cnpj || '/' || referencia_pgfn || '/' || empresas || '/' || socios || '/' || devedores FROM prospec.meta`); m != "2026-11/2026-T4/5/2/2" {
		t.Fatalf("meta depois da PGFN = %s", m)
	}
	if v := consultar[string](t, pool, `SELECT versao FROM prospec.meta`); v == versao1 {
		t.Fatal("a versão tem de mudar a cada troca")
	}
	if f.gets["/cnpj/2026-11/Empresas0.zip"] != getsAntes {
		t.Fatal("a Receita não pode ser baixada de novo quando só a PGFN mudou")
	}
	if n := consultar[int](t, pool, `SELECT count(*) FROM prospec.busca WHERE tem_divida`); n != 2 {
		t.Fatalf("com dívida depois da PGFN = %d", n)
	}

	// 5. mês novo com um arquivo fora do leiaute (29 campos): aborta a
	// referência e a base anterior continua em uso
	quebrado := estab0Padrao()
	quebrado = quebrado[:strings.LastIndex(strings.TrimSuffix(quebrado, "\n"), ";")] + "\n"
	publicarMes(t, f, "2026-12", quebrado)
	if err := s.Executar(ctx, FonteCNPJ); !errors.Is(err, ErrAguardando) {
		t.Fatalf("mês novo, primeira listagem: %v", err)
	}
	err = s.Executar(ctx, FonteCNPJ)
	if err == nil || !strings.Contains(err.Error(), "fora do leiaute") {
		t.Fatalf("esperado erro de leiaute, veio %v", err)
	}
	if ref := consultar[string](t, pool, `SELECT referencia_cnpj FROM prospec.meta`); ref != "2026-11" {
		t.Fatalf("a base anterior deveria continuar em uso, está em %s", ref)
	}
	sit, _ := Situacao(ctx, pool)
	if sit.Estado != "erro" || !strings.Contains(sit.Erro, "Estabelecimentos0") || len(sit.Arquivos) != 35 {
		t.Fatalf("situação depois do erro = %+v", sit)
	}

	// 6. a Receita conserta o arquivo (com uma empresa a mais). No meio
	// tempo um desligamento sujo esvaziou uma tabela UNLOGGED: os arquivos
	// dela voltam a "baixado" e são relidos do zip, sem novo download.
	publicarMes(t, f, "2026-12", estab0Padrao()+estabelecimento("00000006", "0001", "02", "6201501", "4721102", "SP", "7107", "11", "45671234", "contato@contabilidade.exemplo"))
	if _, err := pool.Exec(ctx, `TRUNCATE carga.empresas`); err != nil {
		t.Fatal(err)
	}
	getsEmpresas := f.gets["/cnpj/2026-12/Empresas0.zip"]
	if err := s.Executar(ctx, FonteCNPJ); err != nil {
		t.Fatalf("retomada: %v", err)
	}
	if m := consultar[string](t, pool, `SELECT referencia_cnpj || '/' || referencia_pgfn || '/' || empresas || '/' || devedores FROM prospec.meta`); m != "2026-12/2026-T4/6/2" {
		t.Fatalf("meta depois da retomada = %s", m)
	}
	if n := consultar[int](t, pool, `SELECT count(*) FROM prospec.base_ativa WHERE razao_social <> ''`); n != 6 {
		t.Fatalf("empresas com razão social = %d (a tabela esvaziada não foi recarregada?)", n)
	}
	// publicarMes troca todos os ETags: tudo é baixado de novo exatamente uma vez
	if g := f.gets["/cnpj/2026-12/Empresas0.zip"]; g != getsEmpresas+1 {
		t.Fatalf("Empresas0 baixado %d vezes depois do conserto", g-getsEmpresas)
	}
	if r := consultar[string](t, pool, `SELECT tipo1 || '|' || email_tipo || '|' || array_to_string(cnaes, ',') FROM prospec.base_ativa WHERE cnpj = $1`, cnpjDe("00000006", "0001")); r != "indefinido|contabilidade|6201501,4721102" {
		t.Fatalf("empresa nova = %s", r)
	}
	if sit, _ := Situacao(ctx, pool); sit.Estado != "ociosa" || len(sit.Arquivos) != 0 {
		t.Fatalf("situação final = %+v", sit)
	}
}

func TestCargaRecusadaSemEspacoEEmModoAmostra(t *testing.T) {
	if urlBancoTeste == "" {
		t.Skip("sem PostgreSQL de teste (CRMIA_TEST_PG)")
	}
	ctx := context.Background()
	if err := Novo(config.Config{Amostra: true}).Executar(ctx, FonteTudo); !errors.Is(err, ErrAmostra) {
		t.Fatalf("modo amostra: %v", err)
	}
	s := Novo(config.Config{DirTrabalho: t.TempDir(), DiscoBancoGB: 10})
	e := &execucao{s: s}
	if err := e.conferirDisco(true); err == nil || !strings.Contains(err.Error(), "DADOS_DISCO_MINIMO_GB") {
		s.MinimoTrabalho = 0
		if err := e.conferirDisco(true); err == nil || !strings.Contains(err.Error(), "DADOS_DISCO_MINIMO_GB") {
			t.Fatalf("esperado erro de espaço no banco, veio %v", err)
		}
	}
	s.MinimoTrabalho = 1 << 60
	if err := e.conferirDisco(true); err == nil || !strings.Contains(err.Error(), "nada foi baixado") {
		t.Fatalf("esperado erro de espaço no diretório de trabalho, veio %v", err)
	}
}
