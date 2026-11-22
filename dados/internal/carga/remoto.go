package carga

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ArquivoRemoto é um arquivo publicado por uma das fontes.
type ArquivoRemoto struct {
	Nome       string // com .zip
	URL        string
	Tamanho    int64
	ETag       string
	Modificado string
}

// ErrFirewall: a Receita devolveu HTTP 200 com a página HTML "Request
// Rejected" (cerca de 30 requisições em 30 s disparam o bloqueio).
var ErrFirewall = errors.New("fonte recusou a requisição (página do firewall)")

// Cliente fala com as fontes: espaça as requisições novas, reconhece a
// página do firewall e baixa com retomada.
type Cliente struct {
	HTTP *http.Client
	// Espaco é o intervalo mínimo entre requisições novas (2,5 s na Receita).
	Espaco time.Duration
	// EsperaFirewall é quanto esperar depois da página "Request Rejected".
	EsperaFirewall time.Duration
	// Inatividade corta uma transferência muda (uma conexão da PGFN ficou
	// minutos sem mandar nada).
	Inatividade time.Duration
	// Tentativas seguidas sem nenhum byte novo antes de desistir do arquivo.
	Tentativas int

	mu     sync.Mutex
	ultima time.Time
}

// NovoCliente devolve o cliente com os tempos medidos na pesquisa.
func NovoCliente(espaco time.Duration) *Cliente {
	return &Cliente{
		HTTP: &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 90 * time.Second,
			MaxIdleConnsPerHost:   4,
		}},
		Espaco:         espaco,
		EsperaFirewall: 30 * time.Second,
		Inatividade:    90 * time.Second,
		Tentativas:     8,
	}
}

const agente = "crm-ia-dados/1.0"

// vez espera a vez de uma requisição nova.
func (c *Cliente) vez(ctx context.Context) error {
	c.mu.Lock()
	agora := time.Now()
	quando := c.ultima.Add(c.Espaco)
	if quando.Before(agora) {
		quando = agora
	}
	c.ultima = quando
	c.mu.Unlock()
	return dormir(ctx, time.Until(quando))
}

func dormir(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func pareceHTML(tipo string, inicio []byte) bool {
	if strings.Contains(strings.ToLower(tipo), "text/html") {
		return true
	}
	i := bytes.ToLower(bytes.TrimSpace(inicio))
	return bytes.HasPrefix(i, []byte("<html")) || bytes.HasPrefix(i, []byte("<!doctype html")) || bytes.Contains(i, []byte("request rejected"))
}

// texto busca um documento pequeno (listagem), repetindo quando o firewall
// responde.
func (c *Cliente) documento(ctx context.Context, metodo, endereco string, cabecalhos map[string]string, corpo string, aceitaHTML bool) ([]byte, error) {
	var ultimo error
	for tentativa := 0; tentativa < 4; tentativa++ {
		if err := c.vez(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, metodo, endereco, strings.NewReader(corpo))
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", agente)
		for k, v := range cabecalhos {
			req.Header.Set(k, v)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			ultimo = err
		} else {
			dados, errLeitura := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			resp.Body.Close()
			switch {
			case errLeitura != nil:
				ultimo = errLeitura
			case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
				ultimo = fmt.Errorf("%s %s: HTTP %d", metodo, endereco, resp.StatusCode)
			case resp.StatusCode >= 300:
				return nil, fmt.Errorf("%s %s: HTTP %d", metodo, endereco, resp.StatusCode)
			case !aceitaHTML && pareceHTML(resp.Header.Get("Content-Type"), dados[:min(len(dados), 512)]),
				bytes.Contains(dados[:min(len(dados), 2048)], []byte("Request Rejected")):
				ultimo = ErrFirewall
			default:
				return dados, nil
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		espera := c.EsperaFirewall
		if !errors.Is(ultimo, ErrFirewall) {
			espera = time.Duration(tentativa+1) * 5 * c.Espaco
		}
		slog.Warn("listagem da fonte falhou, nova tentativa", "endereco", endereco, "erro", ultimo, "espera", espera.String())
		if err := dormir(ctx, espera); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%s %s: %w", metodo, endereco, ultimo)
}

// ---------------------------------------------------------------- Receita (WebDAV)

type multistatus struct {
	Respostas []struct {
		Href  string `xml:"href"`
		Props []struct {
			Prop struct {
				Tamanho    string `xml:"getcontentlength"`
				ETag       string `xml:"getetag"`
				Modificado string `xml:"getlastmodified"`
				Tipo       struct {
					Colecao *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

type entradaDAV struct {
	nome       string
	pasta      bool
	tamanho    int64
	etag, mod  string
	enderecoRe string
}

const corpoPropfind = `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:getlastmodified/><d:getcontentlength/><d:getetag/><d:resourcetype/></d:prop></d:propfind>`

// propfind lista uma pasta do Nextcloud da Receita (Depth: 1), sem a própria
// pasta.
func (c *Cliente) propfind(ctx context.Context, endereco string) ([]entradaDAV, error) {
	dados, err := c.documento(ctx, "PROPFIND", endereco, map[string]string{"Depth": "1", "Content-Type": "application/xml"}, corpoPropfind, false)
	if err != nil {
		return nil, err
	}
	var ms multistatus
	if err := xml.Unmarshal(dados, &ms); err != nil {
		return nil, fmt.Errorf("PROPFIND %s: resposta ilegível: %w", endereco, err)
	}
	base, err := url.Parse(endereco)
	if err != nil {
		return nil, err
	}
	raiz := strings.TrimSuffix(base.Path, "/")
	var saida []entradaDAV
	for _, r := range ms.Respostas {
		href, err := url.PathUnescape(r.Href)
		if err != nil {
			href = r.Href
		}
		if u, err := url.Parse(r.Href); err == nil && u.Path != "" {
			href = u.Path
		}
		limpo := strings.TrimSuffix(href, "/")
		if limpo == raiz || limpo == "" {
			continue
		}
		e := entradaDAV{nome: path.Base(limpo)}
		for _, p := range r.Props {
			if p.Prop.Tipo.Colecao != nil {
				e.pasta = true
			}
			if p.Prop.Tamanho != "" {
				e.tamanho, _ = strconv.ParseInt(strings.TrimSpace(p.Prop.Tamanho), 10, 64)
			}
			if p.Prop.ETag != "" {
				e.etag = strings.TrimSpace(p.Prop.ETag)
			}
			if p.Prop.Modificado != "" {
				e.mod = strings.TrimSpace(p.Prop.Modificado)
			}
		}
		saida = append(saida, e)
	}
	return saida, nil
}

var reReferenciaCNPJ = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// ReferenciasCNPJ devolve as pastas AAAA-MM publicadas, da mais nova para a
// mais antiga.
func (c *Cliente) ReferenciasCNPJ(ctx context.Context, base string) ([]string, error) {
	entradas, err := c.propfind(ctx, base)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, e := range entradas {
		if e.pasta && reReferenciaCNPJ.MatchString(e.nome) {
			refs = append(refs, e.nome)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(refs)))
	return refs, nil
}

// ArquivosCNPJ lista os zips de uma referência, com tamanho, ETag e data.
func (c *Cliente) ArquivosCNPJ(ctx context.Context, base, referencia string) ([]ArquivoRemoto, error) {
	pasta := base + referencia + "/"
	entradas, err := c.propfind(ctx, pasta)
	if err != nil {
		return nil, err
	}
	var arquivos []ArquivoRemoto
	for _, e := range entradas {
		if e.pasta || !strings.HasSuffix(strings.ToLower(e.nome), ".zip") {
			continue
		}
		arquivos = append(arquivos, ArquivoRemoto{Nome: e.nome, URL: pasta + url.PathEscape(e.nome), Tamanho: e.tamanho, ETag: e.etag, Modificado: e.mod})
	}
	sort.Slice(arquivos, func(i, j int) bool { return arquivos[i].Nome < arquivos[j].Nome })
	return arquivos, nil
}

// Assinatura resume uma listagem (nome, tamanho e ETag): duas listagens
// iguais com 30 minutos de intervalo dizem que a publicação terminou.
func Assinatura(arquivos []ArquivoRemoto) string {
	var b strings.Builder
	for _, a := range arquivos {
		fmt.Fprintf(&b, "%s|%d|%s\n", a.Nome, a.Tamanho, a.ETag)
	}
	return b.String()
}

// FaltamCNPJ devolve os nomes esperados que a listagem não tem.
func FaltamCNPJ(arquivos []ArquivoRemoto) []string {
	tem := map[string]bool{}
	for _, a := range arquivos {
		tem[strings.TrimSuffix(a.Nome, ".zip")] = true
	}
	var faltam []string
	for _, n := range ArquivosCNPJ() {
		if !tem[n] {
			faltam = append(faltam, n)
		}
	}
	return faltam
}

// ---------------------------------------------------------------- PGFN (índice HTML)

var (
	reLink           = regexp.MustCompile(`(?i)href="([^"?#]+)"`)
	rePastaTrimestre = regexp.MustCompile(`^([0-9]{4})_trimestre_([0-9]{2})$`)
)

func (c *Cliente) links(ctx context.Context, endereco string) ([]string, error) {
	dados, err := c.documento(ctx, http.MethodGet, endereco, nil, "", true)
	if err != nil {
		return nil, err
	}
	var saida []string
	for _, m := range reLink.FindAllSubmatch(dados, -1) {
		l, err := url.PathUnescape(string(m[1]))
		if err != nil {
			l = string(m[1])
		}
		saida = append(saida, l)
	}
	return saida, nil
}

// ReferenciaPGFN converte a pasta AAAA_trimestre_NN na referência AAAA-TN.
func ReferenciaPGFN(pasta string) (string, bool) {
	m := rePastaTrimestre.FindStringSubmatch(strings.Trim(pasta, "/"))
	if m == nil {
		return "", false
	}
	n, _ := strconv.Atoi(m[2])
	if n < 1 || n > 4 {
		return "", false
	}
	return fmt.Sprintf("%s-T%d", m[1], n), true
}

// PastaPGFN é o inverso de ReferenciaPGFN.
func PastaPGFN(referencia string) string {
	if len(referencia) == 7 && referencia[4:6] == "-T" {
		return referencia[:4] + "_trimestre_0" + referencia[6:]
	}
	return referencia
}

// ReferenciasPGFN devolve os trimestres publicados (AAAA-TN), do mais novo
// para o mais antigo.
func (c *Cliente) ReferenciasPGFN(ctx context.Context, base string) ([]string, error) {
	links, err := c.links(ctx, base)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, l := range links {
		if r, ok := ReferenciaPGFN(l); ok {
			refs = append(refs, r)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(refs)))
	return refs, nil
}

// ArquivosPGFN lista os zips do trimestre. A referência só é elegível com os
// três: FGTS, Previdenciário e o não previdenciário, que é o zip que não é
// nenhum dos outros dois (o nome dele mudou em 2026T3). Os tamanhos vêm por
// HEAD.
func (c *Cliente) ArquivosPGFN(ctx context.Context, base, referencia string) ([]ArquivoRemoto, error) {
	pasta := base + PastaPGFN(referencia) + "/"
	links, err := c.links(ctx, pasta)
	if err != nil {
		return nil, err
	}
	var fgts, prev, outros []string
	for _, l := range links {
		nome := path.Base(l)
		baixo := strings.ToLower(nome)
		switch {
		case !strings.HasSuffix(baixo, ".zip") || strings.Contains(strings.Trim(l, "/"), "/"):
		case strings.Contains(baixo, "fgts"):
			fgts = append(fgts, nome)
		case strings.Contains(baixo, "previdenciario") && !strings.Contains(baixo, "nao_previdenciario"):
			prev = append(prev, nome)
		default:
			outros = append(outros, nome)
		}
	}
	if len(fgts) != 1 || len(prev) != 1 || len(outros) != 1 {
		return nil, fmt.Errorf("PGFN %s: esperados 3 zips (FGTS, Previdenciário e não previdenciário); encontrados fgts=%v previdenciário=%v outros=%v", referencia, fgts, prev, outros)
	}
	var arquivos []ArquivoRemoto
	for _, nome := range []string{fgts[0], prev[0], outros[0]} {
		a := ArquivoRemoto{Nome: nome, URL: pasta + url.PathEscape(nome)}
		if err := c.vez(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, a.URL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", agente)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("HEAD %s: %w", a.URL, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HEAD %s: HTTP %d", a.URL, resp.StatusCode)
		}
		a.Tamanho, a.ETag, a.Modificado = resp.ContentLength, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified")
		if a.Tamanho < 0 {
			a.Tamanho = 0
		}
		arquivos = append(arquivos, a)
	}
	return arquivos, nil
}

// TipoZipPGFN diz qual dos três zips é pelo nome.
func TipoZipPGFN(nome string) string {
	baixo := strings.ToLower(nome)
	switch {
	case strings.Contains(baixo, "fgts"):
		return DividaFGTS
	case strings.Contains(baixo, "previdenciario") && !strings.Contains(baixo, "nao_previdenciario"):
		return DividaPrevidenciaria
	}
	return DividaGeral
}

// ---------------------------------------------------------------- download

// Baixar grava a.URL em destino, com retomada. A transferência vai para
// destino+".part"; o servidor da Receita corta transferências longas e não
// anuncia Accept-Ranges, mas aceita Range: a retomada manda Range + If-Range
// com o ETag guardado, e uma resposta 200 no lugar de 206 recomeça do zero.
// A conferência é o tamanho da listagem, os bytes "PK" e a leitura do
// diretório do zip (o CRC de cada membro é conferido na leitura em fluxo).
func (c *Cliente) Baixar(ctx context.Context, a ArquivoRemoto, destino string, progresso func(bytes int64)) error {
	if info, err := os.Stat(destino); err == nil && (a.Tamanho == 0 || info.Size() == a.Tamanho) && zipIntegro(destino) == nil {
		if progresso != nil {
			progresso(info.Size())
		}
		return nil
	}
	parcial := destino + ".part"
	semProgresso := 0
	var ultimo error
	for semProgresso < c.Tentativas {
		if err := ctx.Err(); err != nil {
			return err
		}
		antes := tamanhoDe(parcial)
		if a.Tamanho > 0 && antes > a.Tamanho {
			_ = os.Remove(parcial)
			antes = 0
		}
		if a.Tamanho > 0 && antes == a.Tamanho {
			if err := zipIntegro(parcial); err != nil {
				slog.Warn("arquivo baixado não é um zip íntegro; recomeçando", "arquivo", a.Nome, "erro", err)
				_ = os.Remove(parcial)
				ultimo = err
				semProgresso++
				continue
			}
			return os.Rename(parcial, destino)
		}
		completo, err := c.trecho(ctx, a, parcial, antes, progresso)
		if err == nil && completo && a.Tamanho == 0 {
			if err = zipIntegro(parcial); err == nil {
				return os.Rename(parcial, destino)
			}
			_ = os.Remove(parcial)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if tamanhoDe(parcial) > antes {
			semProgresso = 0
		} else {
			semProgresso++
		}
		if err != nil {
			ultimo = err
			espera := 2 * c.Espaco
			if errors.Is(err, ErrFirewall) {
				espera = c.EsperaFirewall
			}
			slog.Warn("download interrompido, retomando", "arquivo", a.Nome, "bytes", tamanhoDe(parcial), "total", a.Tamanho, "erro", err.Error())
			if err := dormir(ctx, espera); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("download de %s falhou depois de %d tentativas sem avanço: %w", a.Nome, c.Tentativas, ultimo)
}

func tamanhoDe(caminho string) int64 {
	info, err := os.Stat(caminho)
	if err != nil {
		return 0
	}
	return info.Size()
}

func zipIntegro(caminho string) error {
	z, err := zip.OpenReader(caminho)
	if err != nil {
		return err
	}
	return z.Close()
}

// trecho faz uma requisição e acrescenta o que vier ao arquivo parcial.
// completo = o corpo chegou ao fim sem erro.
func (c *Cliente) trecho(ctx context.Context, a ArquivoRemoto, parcial string, inicio int64, progresso func(bytes int64)) (completo bool, err error) {
	if err := c.vez(ctx); err != nil {
		return false, err
	}
	ctx, cancelar := context.WithCancel(ctx)
	defer cancelar()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", agente)
	req.Header.Set("Accept-Encoding", "identity")
	if inicio > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", inicio))
		if a.ETag != "" {
			req.Header.Set("If-Range", a.ETag)
		}
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		inicio = 0 // sem Range, ou o servidor não honrou o If-Range: do zero
	case http.StatusPartialContent:
		if de, ok := inicioDoContentRange(resp.Header.Get("Content-Range")); !ok || de != inicio {
			return false, fmt.Errorf("Content-Range inesperado: %q (pedido a partir de %d)", resp.Header.Get("Content-Range"), inicio)
		}
	case http.StatusRequestedRangeNotSatisfiable:
		_ = os.Remove(parcial)
		return false, fmt.Errorf("HTTP 416: o arquivo mudou no servidor; recomeçando")
	default:
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return false, ErrFirewall
	}
	corpo := io.Reader(resp.Body)
	if inicio == 0 {
		// o começo de um zip é sempre "PK"; a página do firewall vem com 200
		cabeca := make([]byte, 4)
		n, _ := io.ReadFull(resp.Body, cabeca)
		if n < 2 || cabeca[0] != 'P' || cabeca[1] != 'K' {
			resto, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			if pareceHTML("", append(cabeca[:n], resto...)) {
				return false, ErrFirewall
			}
			return false, fmt.Errorf("resposta não é um zip (começa por %q)", cabeca[:n])
		}
		corpo = io.MultiReader(bytes.NewReader(cabeca[:n]), resp.Body)
	}
	modo := os.O_CREATE | os.O_WRONLY
	if inicio == 0 {
		modo |= os.O_TRUNC
	} else {
		modo |= os.O_APPEND
	}
	arq, err := os.OpenFile(parcial, modo, 0o644)
	if err != nil {
		return false, err
	}
	defer func() {
		if e := arq.Close(); err == nil && e != nil {
			err = e
		}
	}()

	// corta a transferência que fica muda
	vigia := time.AfterFunc(c.Inatividade, cancelar)
	defer vigia.Stop()
	total := inicio
	buf := make([]byte, 256<<10)
	for {
		n, errLeitura := corpo.Read(buf)
		if n > 0 {
			vigia.Reset(c.Inatividade)
			if _, err := arq.Write(buf[:n]); err != nil {
				return false, err
			}
			total += int64(n)
			if progresso != nil {
				progresso(total)
			}
		}
		if errLeitura == io.EOF {
			if a.Tamanho > 0 && total < a.Tamanho {
				return false, fmt.Errorf("transferência cortada em %d de %d bytes", total, a.Tamanho)
			}
			return true, nil
		}
		if errLeitura != nil {
			return false, errLeitura
		}
	}
}

func inicioDoContentRange(v string) (int64, bool) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "bytes"))
	traco := strings.IndexByte(v, '-')
	if traco <= 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v[:traco]), 10, 64)
	return n, err == nil
}
