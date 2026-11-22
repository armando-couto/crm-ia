package carga

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const paginaFirewall = `<html><head><title>Request Rejected</title></head><body>The requested URL was rejected. Please consult with your administrator.<br><br>Your support ID is: 1234567890</body></html>`

func clienteDeTeste() *Cliente {
	c := NovoCliente(time.Millisecond)
	c.EsperaFirewall = 5 * time.Millisecond
	c.Inatividade = 2 * time.Second
	c.Tentativas = 4
	return c
}

// zipDeTeste devolve um zip válido com conteúdo pouco compressível, para o
// arquivo ter tamanho suficiente para ser cortado no meio.
func zipDeTeste(t *testing.T, tamanho int, semente int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.Create("F.K03200$Z.D61114.CNAECSV")
	if err != nil {
		t.Fatal(err)
	}
	dados := make([]byte, tamanho)
	rand.New(rand.NewSource(semente)).Read(dados)
	_, _ = w.Write(dados)
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fonteInstavel imita o servidor da Receita: não anuncia Accept-Ranges mas
// aceita Range e If-Range, corta transferências no meio e, de vez em quando,
// devolve HTTP 200 com a página "Request Rejected" do firewall.
type fonteInstavel struct {
	mu       sync.Mutex
	dados    []byte
	etag     string
	cortes   []int // a n-ésima transferência de dados manda só cortes[n] bytes e derruba a conexão
	firewall map[int]bool
	pedidos  []string // "GET 0", "GET 1000 If-Range", "FIREWALL"...
}

func (f *fonteInstavel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	n := len(f.pedidos)
	if f.firewall[n] {
		f.pedidos = append(f.pedidos, "FIREWALL")
		f.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK) // o firewall responde 200
		_, _ = w.Write([]byte(paginaFirewall))
		return
	}
	inicio := 0
	registro := "GET 0"
	if v := r.Header.Get("Range"); v != "" {
		inicio, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(v, "bytes="), "-"))
		registro = fmt.Sprintf("GET %d", inicio)
		if ir := r.Header.Get("If-Range"); ir != "" {
			registro += " If-Range"
			if ir != f.etag {
				inicio = 0 // o arquivo mudou: o servidor manda tudo de novo, com 200
			}
		}
	}
	f.pedidos = append(f.pedidos, registro)
	dados := f.dados[inicio:]
	enviar := len(dados)
	transferencia := 0
	for _, p := range f.pedidos {
		if p != "FIREWALL" {
			transferencia++
		}
	}
	if transferencia <= len(f.cortes) {
		enviar = min(enviar, f.cortes[transferencia-1])
	}
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("ETag", f.etag)
	w.Header().Set("Content-Length", strconv.Itoa(len(dados)))
	if inicio > 0 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", inicio, len(f.dados)-1, len(f.dados)))
		w.WriteHeader(http.StatusPartialContent)
	}
	_, _ = w.Write(dados[:enviar])
	if enviar < len(dados) {
		// corta a transferência: o status e o começo ("PK") já foram certos
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		if h, ok := w.(http.Hijacker); ok {
			if c, _, err := h.Hijack(); err == nil {
				_ = c.Close()
			}
		}
	}
}

func TestDownloadRetomaDepoisDeCorteEDaPaginaDoFirewall(t *testing.T) {
	dados := zipDeTeste(t, 300_000, 1)
	fonte := &fonteInstavel{dados: dados, etag: `"abc123"`, cortes: []int{100_000, 50_000}, firewall: map[int]bool{1: true}}
	srv := httptest.NewServer(fonte)
	defer srv.Close()

	destino := filepath.Join(t.TempDir(), "Cnaes.zip")
	var ultimo int64
	err := clienteDeTeste().Baixar(context.Background(), ArquivoRemoto{Nome: "Cnaes.zip", URL: srv.URL + "/2026-11/Cnaes.zip", Tamanho: int64(len(dados)), ETag: `"abc123"`}, destino, func(b int64) { ultimo = b })
	if err != nil {
		t.Fatalf("download: %v (pedidos: %v)", err, fonte.pedidos)
	}
	baixado, err := os.ReadFile(destino)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(baixado, dados) {
		t.Fatalf("arquivo baixado difere do original (%d contra %d bytes)", len(baixado), len(dados))
	}
	if ultimo != int64(len(dados)) {
		t.Errorf("progresso final = %d", ultimo)
	}
	if _, err := os.Stat(destino + ".part"); !os.IsNotExist(err) {
		t.Error("o arquivo .part deveria ter virado o definitivo")
	}
	// 1º pedido cortado aos 100.000; 2º caiu no firewall (o .part não é
	// tocado); 3º retoma com Range + If-Range e é cortado de novo; 4º termina
	esperado := []string{"GET 0", "FIREWALL", "GET 100000 If-Range", "GET 150000 If-Range"}
	if strings.Join(fonte.pedidos, "|") != strings.Join(esperado, "|") {
		t.Errorf("pedidos = %v, esperado %v", fonte.pedidos, esperado)
	}
	// repetir não baixa de novo
	antes := len(fonte.pedidos)
	if err := clienteDeTeste().Baixar(context.Background(), ArquivoRemoto{Nome: "Cnaes.zip", URL: srv.URL + "/x", Tamanho: int64(len(dados))}, destino, nil); err != nil || len(fonte.pedidos) != antes {
		t.Errorf("arquivo já baixado não deveria gerar requisição: %v", err)
	}
}

func TestDownloadRecomecaQuandoOArquivoMudaNoServidor(t *testing.T) {
	antigo, novo := zipDeTeste(t, 200_000, 2), zipDeTeste(t, 200_000, 3)
	// uma execução anterior deixou o .part com 80.000 bytes do arquivo antigo;
	// o servidor trocou o arquivo: a retomada manda If-Range com o ETag
	// guardado, recebe 200 no lugar de 206 e recomeça do zero
	fonte := &fonteInstavel{dados: novo, etag: `"v2"`}
	srv := httptest.NewServer(fonte)
	defer srv.Close()
	destino := filepath.Join(t.TempDir(), "Simples.zip")
	if err := os.WriteFile(destino+".part", antigo[:80_000], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := clienteDeTeste().Baixar(context.Background(), ArquivoRemoto{Nome: "Simples.zip", URL: srv.URL + "/f", Tamanho: int64(len(novo)), ETag: `"v1"`}, destino, nil); err != nil {
		t.Fatalf("retomada: %v (pedidos %v)", err, fonte.pedidos)
	}
	baixado, _ := os.ReadFile(destino)
	if !bytes.Equal(baixado, novo) {
		t.Fatal("o arquivo final deveria ser o novo, inteiro")
	}
	if fonte.pedidos[0] != "GET 80000 If-Range" {
		t.Errorf("pedidos = %v", fonte.pedidos)
	}
}

func TestDownloadDesisteQuandoSoVemFirewall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// nem todo bloqueio vem com Content-Type de HTML
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(paginaFirewall))
	}))
	defer srv.Close()
	destino := filepath.Join(t.TempDir(), "Empresas0.zip")
	err := clienteDeTeste().Baixar(context.Background(), ArquivoRemoto{Nome: "Empresas0.zip", URL: srv.URL, Tamanho: 1000}, destino, nil)
	if !errors.Is(err, ErrFirewall) {
		t.Fatalf("esperado ErrFirewall, veio %v", err)
	}
	if tamanhoDe(destino) != 0 || tamanhoDe(destino+".part") != 0 {
		t.Fatal("a página do firewall não pode ser gravada como arquivo")
	}
}

// Trecho real da resposta do Nextcloud da Receita (22/11/2026), reduzido.
const respostaPropfindRaiz = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:s="http://sabredav.org/ns" xmlns:oc="http://owncloud.org/ns" xmlns:nc="http://nextcloud.org/ns"><d:response><d:href>/public.php/dav/files/TOKEN/</d:href><d:propstat><d:prop><d:getlastmodified>Mon, 16 Nov 2026 10:10:13 GMT</d:getlastmodified><d:resourcetype><d:collection/></d:resourcetype><d:quota-used-bytes>358090155262</d:quota-used-bytes><d:getetag>&quot;6afad6869fbe5&quot;</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:response><d:href>/public.php/dav/files/TOKEN/2026-10/</d:href><d:propstat><d:prop><d:getlastmodified>Tue, 20 Oct 2026 20:54:47 GMT</d:getlastmodified><d:resourcetype><d:collection/></d:resourcetype><d:getetag>&quot;682cec17d8c01&quot;</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:response><d:href>/public.php/dav/files/TOKEN/2026-11/</d:href><d:propstat><d:prop><d:getlastmodified>Mon, 16 Nov 2026 10:10:13 GMT</d:getlastmodified><d:resourcetype><d:collection/></d:resourcetype><d:getetag>&quot;6afad6869fbe6&quot;</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:response><d:href>/public.php/dav/files/TOKEN/LEIAME.pdf</d:href><d:propstat><d:prop><d:getcontentlength>1000</d:getcontentlength><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`

const respostaPropfindMes = `<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:"><d:response><d:href>/public.php/dav/files/TOKEN/2026-11/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:response><d:href>/public.php/dav/files/TOKEN/2026-11/Cnaes.zip</d:href><d:propstat><d:prop>
<d:getlastmodified>Mon, 16 Nov 2026 10:00:23 GMT</d:getlastmodified><d:getcontentlength>22078</d:getcontentlength>
<d:getetag>&quot;87d50b8d93ea716eeea7c8aaff13b795&quot;</d:getetag><d:getcontenttype>application/zip</d:getcontenttype>
</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:response><d:href>/public.php/dav/files/TOKEN/2026-11/Empresas0.zip</d:href><d:propstat><d:prop><d:getlastmodified>Mon, 16 Nov 2026 10:01:00 GMT</d:getlastmodified><d:getcontentlength>337438307</d:getcontentlength><d:getetag>&quot;e0&quot;</d:getetag><d:resourcetype/></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`

func TestListagemDoCNPJPorPROPFIND(t *testing.T) {
	var pedidos int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos++
		if r.Method != "PROPFIND" || r.Header.Get("Depth") != "1" {
			t.Errorf("pedido inesperado: %s Depth=%q", r.Method, r.Header.Get("Depth"))
		}
		if pedidos == 1 {
			// a listagem também cai no firewall: 200 com HTML
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(paginaFirewall))
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		if strings.HasSuffix(r.URL.Path, "/2026-11/") {
			_, _ = w.Write([]byte(respostaPropfindMes))
			return
		}
		_, _ = w.Write([]byte(respostaPropfindRaiz))
	}))
	defer srv.Close()
	c := clienteDeTeste()
	base := srv.URL + "/public.php/dav/files/TOKEN/"
	refs, err := c.ReferenciasCNPJ(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(refs, ",") != "2026-11,2026-10" {
		t.Fatalf("referências = %v", refs)
	}
	arquivos, err := c.ArquivosCNPJ(context.Background(), base, "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(arquivos) != 2 || arquivos[0].Nome != "Cnaes.zip" || arquivos[0].Tamanho != 22078 || arquivos[0].ETag != `"87d50b8d93ea716eeea7c8aaff13b795"` ||
		arquivos[0].URL != base+"2026-11/Cnaes.zip" || arquivos[1].Tamanho != 337438307 || arquivos[0].Modificado == "" {
		t.Fatalf("arquivos = %+v", arquivos)
	}
	if faltam := FaltamCNPJ(arquivos); len(faltam) != 35 {
		t.Errorf("faltam %d dos 37", len(faltam))
	}
	if Assinatura(arquivos) == Assinatura(arquivos[:1]) {
		t.Error("a assinatura tem de mudar quando a listagem muda")
	}
}

func TestListagemDaPGFN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`<html><body><h1>Index of /</h1><ul><li><a href="2026_trimestre_02/"> 2026_trimestre_02/</a></li>
<li><a href="2026_trimestre_03/"> 2026_trimestre_03/</a></li><li><a href="2025_trimestre_04/"> 2025_trimestre_04/</a></li>
<li><a href="Dados_abertos_FGTS.zip"> Dados_abertos_FGTS.zip</a></li><li><a href="Portal_da_Cidadania_Tributaria/"> Portal/</a></li></ul></body></html>`))
		case r.URL.Path == "/2026_trimestre_03/":
			_, _ = w.Write([]byte(`<html><body><ul><li><a href="/"> Parent Directory</a></li><li><a href="Dados_abertos_FGTS.zip"> x</a></li>
<li><a href="Dados_abertos_Previdenciario.zip"> x</a></li><li><a href="Dados_abertos_Sistema_SIDA.zip"> x</a></li></ul></body></html>`))
		case r.URL.Path == "/2026_trimestre_02/":
			_, _ = w.Write([]byte(`<html><body><ul><li><a href="Dados_abertos_FGTS.zip"> x</a></li></ul></body></html>`))
		case r.Method == http.MethodHead:
			w.Header().Set("Content-Length", "1234")
			w.Header().Set("ETag", `"pgfn"`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clienteDeTeste()
	refs, err := c.ReferenciasPGFN(context.Background(), srv.URL+"/")
	if err != nil || strings.Join(refs, ",") != "2026-T3,2026-T2,2025-T4" {
		t.Fatalf("referências = %v %v", refs, err)
	}
	arquivos, err := c.ArquivosPGFN(context.Background(), srv.URL+"/", "2026-T3")
	if err != nil {
		t.Fatal(err)
	}
	// o não previdenciário é o zip que não é FGTS nem Previdenciário (o nome mudou em 2026T3)
	if len(arquivos) != 3 || arquivos[2].Nome != "Dados_abertos_Sistema_SIDA.zip" || arquivos[0].Tamanho != 1234 || arquivos[0].ETag != `"pgfn"` ||
		arquivos[1].URL != srv.URL+"/2026_trimestre_03/Dados_abertos_Previdenciario.zip" {
		t.Fatalf("arquivos = %+v", arquivos)
	}
	if _, err := c.ArquivosPGFN(context.Background(), srv.URL+"/", "2026-T2"); err == nil {
		t.Fatal("trimestre sem os três zips não é elegível")
	}
}
