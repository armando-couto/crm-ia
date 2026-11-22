// Package basepublica é o cliente HTTP do painel para o serviço `dados`, que
// guarda a base pública de empresas (CNPJ da Receita e dívida ativa da PGFN).
//
// O pacote não se chama `dados` porque o pacote web já tem um tipo com esse
// nome. O serviço é opcional: sem DADOS_URL ou DADOS_TOKEN o cliente existe,
// mas Configurado() devolve falso e toda chamada responde ErrNaoConfigurado.
package basepublica

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Limite de leitura de uma resposta do serviço. Um lote de 500 fichas com
// sócios e dívida fica na casa de poucos megabytes.
const limiteResposta = 32 << 20

var (
	// ErrNaoConfigurado: o painel está rodando sem o serviço `dados`.
	ErrNaoConfigurado = errors.New("a base de empresas não está configurada nesta instalação")
	// ErrIndisponivel: o serviço não respondeu (rede, tempo esgotado, 5xx).
	ErrIndisponivel = errors.New("a base de empresas não está respondendo")
)

// Erro é uma resposta do serviço com status de erro e a mensagem dele.
type Erro struct {
	Status   int
	Mensagem string
}

func (e *Erro) Error() string { return e.Mensagem }

type Cliente struct {
	base  string
	token string
	http  *http.Client
}

// Novo monta o cliente. base e token vazios são aceitos (serviço ausente).
func Novo(base, token string) *Cliente {
	return &Cliente{base: strings.TrimRight(strings.TrimSpace(base), "/"), token: strings.TrimSpace(token),
		http: &http.Client{Timeout: 25 * time.Second}}
}

// Configurado diz se há endereço e token para falar com o serviço.
func (c *Cliente) Configurado() bool { return c != nil && c.base != "" && c.token != "" }

// Arquivo é um arquivo da carga em curso.
type Arquivo struct {
	Nome       string `json:"nome"`
	Estado     string `json:"estado"`
	Bytes      int64  `json:"bytes"`
	TotalBytes int64  `json:"total_bytes"`
	Linhas     int64  `json:"linhas"`
}

// Percentual do arquivo já baixado (0 a 100).
func (a Arquivo) Percentual() int {
	if a.TotalBytes <= 0 {
		if a.Estado == "baixado" || a.Estado == "carregado" {
			return 100
		}
		return 0
	}
	p := int(a.Bytes * 100 / a.TotalBytes)
	if p > 100 {
		p = 100
	}
	return p
}

// Carga é o andamento da carga da base.
type Carga struct {
	Estado     string     `json:"estado"`
	Fonte      string     `json:"fonte"`
	Referencia string     `json:"referencia"`
	Etapa      string     `json:"etapa"`
	Percentual float64    `json:"percentual"`
	IniciadaEm *time.Time `json:"iniciada_em"`
	Erro       string     `json:"erro"`
	Arquivos   []Arquivo  `json:"arquivos"`
}

// PercentualInteiro: andamento de 0 a 100, para a barra da página.
func (c Carga) PercentualInteiro() int {
	switch {
	case c.Percentual <= 0:
		return 0
	case c.Percentual >= 100:
		return 100
	}
	return int(c.Percentual)
}

// EmCurso: há download, carga ou construção acontecendo agora.
func (c Carga) EmCurso() bool {
	return c.Estado == "baixando" || c.Estado == "carregando" || c.Estado == "construindo"
}

// Situacao é a resposta de GET /v1/situacao.
type Situacao struct {
	Pronta         bool       `json:"pronta"`
	Amostra        bool       `json:"amostra"`
	ReferenciaCNPJ string     `json:"referencia_cnpj"`
	ReferenciaPGFN string     `json:"referencia_pgfn"`
	Versao         string     `json:"versao"`
	Empresas       int64      `json:"empresas"`
	Socios         int64      `json:"socios"`
	Devedores      int64      `json:"devedores"`
	AtualizadaEm   *time.Time `json:"atualizada_em"`
	EspacoBytes    int64      `json:"espaco_bytes"`
	UFs            []string   `json:"ufs"`
	Carga          Carga      `json:"carga"`
}

// Repassar faz uma chamada e devolve o status e o corpo como vieram. Erro só
// quando não houve resposta aproveitável: serviço não configurado
// (ErrNaoConfigurado) ou fora do ar (ErrIndisponivel).
func (c *Cliente) Repassar(ctx context.Context, metodo, caminho string, consulta url.Values, corpo []byte) (int, []byte, error) {
	if !c.Configurado() {
		return 0, nil, ErrNaoConfigurado
	}
	endereco := c.base + caminho
	if len(consulta) > 0 {
		endereco += "?" + consulta.Encode()
	}
	var leitor io.Reader
	if corpo != nil {
		leitor = bytes.NewReader(corpo)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, endereco, leitor)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrIndisponivel, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limiteResposta+1))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrIndisponivel, err)
	}
	if len(b) > limiteResposta {
		return 0, nil, fmt.Errorf("%w: resposta grande demais", ErrIndisponivel)
	}
	return resp.StatusCode, b, nil
}

// chamar é o Repassar para quem quer a resposta decodificada: status de erro
// vira *Erro com a mensagem do serviço.
func (c *Cliente) chamar(ctx context.Context, metodo, caminho string, entrada, saida any) error {
	var corpo []byte
	if entrada != nil {
		var err error
		if corpo, err = json.Marshal(entrada); err != nil {
			return err
		}
	}
	status, b, err := c.Repassar(ctx, metodo, caminho, nil, corpo)
	if err != nil {
		return err
	}
	if status >= 300 {
		return &Erro{Status: status, Mensagem: MensagemDeErro(status, b)}
	}
	if saida != nil && len(b) > 0 {
		if err := json.Unmarshal(b, saida); err != nil {
			return fmt.Errorf("%w: resposta ilegível", ErrIndisponivel)
		}
	}
	return nil
}

// MensagemDeErro extrai o texto de `{"erro": "..."}` (ou cai no status).
func MensagemDeErro(status int, corpo []byte) string {
	var e struct {
		Erro  string `json:"erro"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(corpo, &e)
	switch {
	case e.Erro != "":
		return e.Erro
	case e.Error != "":
		return e.Error
	}
	return http.StatusText(status)
}

// Situacao consulta o estado da base, com prazo curto: é chamada por páginas
// e pelo resumo do app, que não podem ficar presos a um serviço lento.
func (c *Cliente) Situacao(ctx context.Context) (Situacao, error) {
	var s Situacao
	ctx, cancelar := context.WithTimeout(ctx, 5*time.Second)
	defer cancelar()
	err := c.chamar(ctx, http.MethodGet, "/v1/situacao", nil, &s)
	return s, err
}

// Detalhes é a resposta de POST /v1/empresas/detalhes. Os itens ficam crus: o
// painel só precisa contá-los e entregá-los.
type Detalhes struct {
	Itens          []json.RawMessage `json:"itens"`
	NaoEncontrados []string          `json:"nao_encontrados"`
}

// Detalhes pede as fichas completas de até 500 CNPJs. Sem incluirPF o serviço
// devolve pessoa física sem contatos e sem sócios.
func (c *Cliente) Detalhes(ctx context.Context, cnpjs []string, incluirPF bool) (Detalhes, error) {
	var d Detalhes
	err := c.chamar(ctx, http.MethodPost, "/v1/empresas/detalhes", map[string]any{"cnpjs": cnpjs, "incluir_pf": incluirPF}, &d)
	if d.Itens == nil {
		d.Itens = []json.RawMessage{}
	}
	if d.NaoEncontrados == nil {
		d.NaoEncontrados = []string{}
	}
	return d, err
}

// IniciarCarga pede ao serviço que confira as fontes e carregue o que houver
// de novo. fonte: "cnpj", "pgfn" ou "tudo". 409 = já há carga em curso (ou o
// serviço está em modo amostra).
func (c *Cliente) IniciarCarga(ctx context.Context, fonte string) error {
	ctx, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()
	return c.chamar(ctx, http.MethodPost, "/v1/cargas", map[string]string{"fonte": fonte}, nil)
}
