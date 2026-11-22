package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

// -----------------------------------------------------------------------------
// Cliente da plataforma: o app só fala com o painel (PAINEL_URL + TOKEN_INTERNO,
// rede interna do Docker). O painel autentica o ambiente, aplica a cota do
// plano e repassa ao serviço de dados, que fica invisível daqui.
//
// Usam este cliente os controllers (com o contexto da requisição), o worker de
// jobs (com context.Background() e prazo próprio: fechar a aba não cancela o
// job) e o provedor "receita" do enriquecimento (enriquecimento.FichaDaPlataforma).
// -----------------------------------------------------------------------------

const (
	// plataformaLimiteResposta: maior resposta aceita do painel (um lote de
	// 200 fichas completas fica bem abaixo disso).
	plataformaLimiteResposta = 8 << 20
	plataformaTimeout        = 30 * time.Second
)

var clientePlataforma = &http.Client{Timeout: plataformaTimeout}

// PlataformaBaseURL, quando preenchida, substitui o endereço montado a partir
// da configuração (PAINEL_URL + "/api/empresas/<slug>"). Existe para os testes
// apontarem o cliente para um servidor simulado; veja UsarPlataformaDeTeste.
var PlataformaBaseURL string

// UsarPlataformaDeTeste aponta o cliente para baseURL (ex.: httptest.Server.URL)
// e devolve a função que desfaz. Os caminhos chegam ao servidor sem o prefixo
// /api/empresas/<slug>: "/prospeccao/contagem", "/plano" etc.
func UsarPlataformaDeTeste(baseURL string) (desfazer func()) {
	anterior := PlataformaBaseURL
	PlataformaBaseURL = strings.TrimRight(baseURL, "/")
	return func() { PlataformaBaseURL = anterior }
}

// PlataformaConfigurada diz se o ambiente tem painel para falar.
func PlataformaConfigurada() bool {
	if PlataformaBaseURL != "" {
		return true
	}
	return utils.Cfg.PainelURL != "" && utils.Cfg.TokenInterno != ""
}

func plataformaBase() string {
	if PlataformaBaseURL != "" {
		return PlataformaBaseURL
	}
	return utils.Cfg.PainelURL + "/api/empresas/" + url.PathEscape(utils.Cfg.TenantSlug)
}

// Erros de transporte do cliente.
var (
	// ErrPlataformaNaoConfigurada: ambiente sem PAINEL_URL ou TOKEN_INTERNO.
	ErrPlataformaNaoConfigurada = errors.New("este ambiente não está ligado à plataforma")
	// ErrPlataformaInacessivel: falha de rede, prazo estourado ou resposta grande demais.
	ErrPlataformaInacessivel = errors.New("não foi possível falar com a plataforma, tente de novo em instantes")
)

// RespostaPainel é a resposta crua de uma chamada ao painel.
type RespostaPainel struct {
	Status    int
	Corpo     []byte
	Cabecalho http.Header
}

// ChamarPainelBruto faz a chamada e devolve status, corpo e cabeçalhos sem
// interpretar. caminho começa com "/" e vem depois de /api/empresas/<slug>;
// consulta (opcional) é a query string. Erros: ErrPlataformaNaoConfigurada, ou
// ErrPlataformaInacessivel embrulhando a causa.
func ChamarPainelBruto(ctx context.Context, metodo, caminho string, consulta url.Values, corpo any) (*RespostaPainel, error) {
	if !PlataformaConfigurada() {
		return nil, ErrPlataformaNaoConfigurada
	}
	var leitor io.Reader
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			return nil, err
		}
		leitor = bytes.NewReader(b)
	}
	alvo := plataformaBase() + caminho
	if len(consulta) > 0 {
		alvo += "?" + consulta.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, metodo, alvo, leitor)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+utils.Cfg.TokenInterno)
	req.Header.Set("Accept", "application/json")
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := clientePlataforma.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPlataformaInacessivel, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, plataformaLimiteResposta+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPlataformaInacessivel, err)
	}
	if len(b) > plataformaLimiteResposta {
		return nil, fmt.Errorf("%w: resposta maior que %d MiB", ErrPlataformaInacessivel, plataformaLimiteResposta>>20)
	}
	return &RespostaPainel{Status: resp.StatusCode, Corpo: b, Cabecalho: resp.Header}, nil
}

// ChamarPainel é a forma curta: status, corpo e erro de transporte. Não
// interpreta o status (quem chama decide o que fazer com 4xx e 5xx).
func ChamarPainel(ctx context.Context, metodo, caminho string, corpo any) (status int, resposta []byte, err error) {
	r, err := ChamarPainelBruto(ctx, metodo, caminho, nil, corpo)
	if err != nil {
		return 0, nil, err
	}
	return r.Status, r.Corpo, nil
}

// Códigos de erro da prospecção que o app devolve à tela (e.data.codigo).
const (
	CodigoCotaEsgotada      = "cota_esgotada"      // 429, com cota
	CodigoMuitasRequisicoes = "muitas_requisicoes" // 429, com tentar_em_segundos
	CodigoBaseEmCarga       = "base_em_carga"      // 503
	CodigoBaseAtualizada    = "base_atualizada"    // 409 (cursor de versão antiga)
	CodigoFiltroInvalido    = "filtro_invalido"    // 422
	CodigoJobAtivo          = "job_ativo"          // 409, com job
	CodigoCustoMudou        = "custo_mudou"        // 409, com estimativa
	CodigoNaoEncontrado     = "nao_encontrado"     // 404
	CodigoPlataforma        = "plataforma"         // 502: sem painel, rede, ou ambiente não reconhecido
)

// ErroPlataforma é o erro normalizado de uma chamada de prospecção: status e
// mensagem já são os que o app devolve à tela.
type ErroPlataforma struct {
	Status           int    // status HTTP para a resposta do app
	Codigo           string // um dos Codigo* acima
	Mensagem         string
	Cota             *models.ProspCota // em cota_esgotada
	TentarEmSegundos int               // em muitas_requisicoes (Retry-After)
	StatusPainel     int               // status que o painel devolveu (0 = não houve resposta; -1 = ambiente sem painel)
}

func (e *ErroPlataforma) Error() string { return e.Mensagem }

// Transitorio diz se vale tentar de novo mais tarde (rede, base indisponível,
// limite de requisições). Cota esgotada, filtro inválido e cursor antigo não são.
func (e *ErroPlataforma) Transitorio() bool {
	switch e.Codigo {
	case CodigoBaseEmCarga, CodigoMuitasRequisicoes:
		return true
	case CodigoPlataforma:
		return e.StatusPainel == 0 || e.StatusPainel >= 500
	}
	return false
}

// Corpo monta o JSON de erro do app: {"error", "codigo"} mais "cota" ou
// "tentar_em_segundos" quando houver.
func (e *ErroPlataforma) Corpo() map[string]any {
	m := map[string]any{"error": e.Mensagem, "codigo": e.Codigo}
	if e.Cota != nil {
		m["cota"] = e.Cota
	}
	if e.TentarEmSegundos > 0 {
		m["tentar_em_segundos"] = e.TentarEmSegundos
	}
	return m
}

// ComoErroPlataforma extrai o *ErroPlataforma de err, se houver.
func ComoErroPlataforma(err error) (*ErroPlataforma, bool) {
	var e *ErroPlataforma
	ok := errors.As(err, &e)
	return e, ok
}

// erroDeTransporte normaliza as falhas de ChamarPainelBruto.
func erroDeTransporte(err error) *ErroPlataforma {
	if errors.Is(err, ErrPlataformaNaoConfigurada) {
		return &ErroPlataforma{Status: http.StatusServiceUnavailable, Codigo: CodigoPlataforma,
			Mensagem: "a prospecção ainda não está disponível neste ambiente", StatusPainel: -1}
	}
	return &ErroPlataforma{Status: http.StatusBadGateway, Codigo: CodigoPlataforma, Mensagem: ErrPlataformaInacessivel.Error()}
}

// erroDaResposta traduz uma resposta de erro do painel para a mensagem e o
// código do app.
func erroDaResposta(r *RespostaPainel) *ErroPlataforma {
	var corpo struct {
		Error            string            `json:"error"`
		Erro             string            `json:"erro"`
		Codigo           string            `json:"codigo"`
		Cota             *models.ProspCota `json:"cota"`
		TentarEmSegundos int               `json:"tentar_em_segundos"`
	}
	_ = json.Unmarshal(r.Corpo, &corpo)
	msg := corpo.Error
	if msg == "" {
		msg = corpo.Erro
	}
	e := &ErroPlataforma{Status: r.Status, StatusPainel: r.Status, Mensagem: msg}
	switch r.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		e.Status, e.Codigo, e.Mensagem = http.StatusBadGateway, CodigoPlataforma, "a plataforma não reconheceu este ambiente"
	case http.StatusTooManyRequests:
		if corpo.Codigo == "limite_requisicoes" || (corpo.Cota == nil && corpo.Codigo != "cota") {
			e.Codigo = CodigoMuitasRequisicoes
			e.Mensagem = "muitas consultas em pouco tempo, aguarde alguns segundos"
			if msg != "" && strings.Contains(msg, "diário") {
				e.Mensagem = msg // limite diário de fichas avulsas: a mensagem do painel é mais precisa
			}
			e.TentarEmSegundos = corpo.TentarEmSegundos
			if s, err := strconv.Atoi(r.Cabecalho.Get("Retry-After")); err == nil && s > 0 {
				e.TentarEmSegundos = s
			}
			if e.TentarEmSegundos <= 0 {
				e.TentarEmSegundos = 5
			}
		} else {
			e.Codigo, e.Mensagem, e.Cota = CodigoCotaEsgotada, "cota mensal de prospecção esgotada", corpo.Cota
		}
	case http.StatusServiceUnavailable:
		e.Codigo, e.Mensagem = CodigoBaseEmCarga, "a base de empresas ainda não está disponível, tente em alguns minutos"
	case http.StatusConflict:
		e.Codigo, e.Mensagem = CodigoBaseAtualizada, "a base foi atualizada; refaça a busca"
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		e.Status, e.Codigo = http.StatusUnprocessableEntity, CodigoFiltroInvalido
		if e.Mensagem == "" {
			e.Mensagem = "filtro inválido"
		}
	case http.StatusNotFound:
		e.Codigo = CodigoNaoEncontrado
		if e.Mensagem == "" {
			e.Mensagem = "registro não encontrado"
		}
	default:
		e.Status, e.Codigo = http.StatusBadGateway, CodigoPlataforma
		e.Mensagem = "a plataforma respondeu com erro, tente de novo em instantes"
	}
	return e
}

// chamarProspeccao chama /prospeccao<caminho> e decodifica a resposta em out
// (nil = descarta). Todo erro sai como *ErroPlataforma.
func chamarProspeccao(ctx context.Context, metodo, caminho string, consulta url.Values, corpo, out any) error {
	r, err := ChamarPainelBruto(ctx, metodo, "/prospeccao"+caminho, consulta, corpo)
	if err != nil {
		return erroDeTransporte(err)
	}
	if r.Status < 200 || r.Status > 299 {
		return erroDaResposta(r)
	}
	if out == nil {
		return nil
	}
	if raw, ok := out.(*json.RawMessage); ok {
		*raw = append((*raw)[:0], r.Corpo...)
		return nil
	}
	if err := json.Unmarshal(r.Corpo, out); err != nil {
		return &ErroPlataforma{Status: http.StatusBadGateway, Codigo: CodigoPlataforma, StatusPainel: r.Status,
			Mensagem: "a plataforma devolveu uma resposta que o app não entendeu"}
	}
	return nil
}

// PlataformaResumo devolve cota e situação da base (GET /prospeccao). Sem
// painel configurado não é erro: volta habilitada = false com o motivo.
func PlataformaResumo(ctx context.Context) (*models.ProspResumoPlataforma, error) {
	if !PlataformaConfigurada() {
		return &models.ProspResumoPlataforma{Motivo: "a prospecção ainda não está disponível neste ambiente"}, nil
	}
	var out models.ProspResumoPlataforma
	if err := chamarProspeccao(ctx, http.MethodGet, "", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CatalogosPlataforma são os nomes aceitos em GET /prospeccao/catalogos/{nome}.
var CatalogosPlataforma = []string{"cnaes", "municipios", "naturezas", "ufs"}

// PlataformaCatalogo repassa a consulta de um catálogo; a query string segue
// como veio e a resposta volta crua ({ "itens": [...] }).
func PlataformaCatalogo(ctx context.Context, nome string, consulta url.Values) (json.RawMessage, error) {
	valido := false
	for _, n := range CatalogosPlataforma {
		valido = valido || n == nome
	}
	if !valido {
		return nil, &ErroPlataforma{Status: http.StatusNotFound, Codigo: CodigoNaoEncontrado, Mensagem: "catálogo desconhecido"}
	}
	var out json.RawMessage
	if err := chamarProspeccao(ctx, http.MethodGet, "/catalogos/"+nome, consulta, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PlataformaContagem conta as empresas do filtro (grátis).
func PlataformaContagem(ctx context.Context, filtros models.ProspFiltros) (*models.ProspContagem, error) {
	var out models.ProspContagem
	if err := chamarProspeccao(ctx, http.MethodPost, "/contagem", nil, map[string]any{"filtros": filtros}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PreviaPlataforma é a resposta da prévia gratuita (até 25 empresas, sem contatos).
type PreviaPlataforma struct {
	Itens         []models.ProspEmpresaPrevia `json:"itens"`
	OrdemAplicada string                      `json:"ordem_aplicada"`
	Versao        string                      `json:"versao,omitempty"`
}

// PlataformaPrevia devolve a prévia do filtro. limite 0 = o padrão do serviço (25).
func PlataformaPrevia(ctx context.Context, filtros models.ProspFiltros, ordem string, limite int) (*PreviaPlataforma, error) {
	corpo := map[string]any{"filtros": filtros}
	if ordem != "" {
		corpo["ordem"] = ordem
	}
	if limite > 0 {
		corpo["limite"] = limite
	}
	out := PreviaPlataforma{Itens: []models.ProspEmpresaPrevia{}}
	if err := chamarProspeccao(ctx, http.MethodPost, "/previa", nil, corpo, &out); err != nil {
		return nil, err
	}
	if out.OrdemAplicada == "" {
		out.OrdemAplicada = ordem
	}
	return &out, nil
}

// CandidatosPedido é uma página de POST /prospeccao/candidatos. PedidoID é o
// UUID da lista (o painel exige e registra).
type CandidatosPedido struct {
	PedidoID string              `json:"pedido_id"`
	Filtros  models.ProspFiltros `json:"filtros"`
	Ordem    string              `json:"ordem,omitempty"`
	Limite   int                 `json:"limite,omitempty"` // até 5.000
	Cursor   string              `json:"cursor,omitempty"`
}

// CandidatosPlataforma é a página devolvida: só CNPJs (barato).
type CandidatosPlataforma struct {
	CNPJs         []string `json:"cnpjs"`
	Cursor        string   `json:"cursor"`
	Fim           bool     `json:"fim"`
	Versao        string   `json:"versao"`
	OrdemAplicada string   `json:"ordem_aplicada"`
}

// PlataformaCandidatos pede uma página de candidatos. Cursor de versão antiga
// volta como ErroPlataforma com Codigo = CodigoBaseAtualizada: o job descarta
// os candidatos ainda não extraídos e refaz a busca.
func PlataformaCandidatos(ctx context.Context, p CandidatosPedido) (*CandidatosPlataforma, error) {
	var out CandidatosPlataforma
	if err := chamarProspeccao(ctx, http.MethodPost, "/candidatos", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExtracaoPlataforma é a resposta de uma extração: fichas completas e a cota
// depois do desconto.
type ExtracaoPlataforma struct {
	Itens          []models.ProspEmpresa `json:"itens"`
	NaoEncontrados []string              `json:"nao_encontrados"`
	Cota           models.ProspCota      `json:"cota"`
}

// MaxCNPJsExtracao é o teto do painel por chamada de extração (o job usa lotes de 200).
const MaxCNPJsExtracao = 500

// PlataformaExtracao entrega as fichas do lote e desconta da cota. É
// idempotente: repetir o mesmo pedidoID com o mesmo lote não desconta de novo,
// por isso o job grava os lotes no estado ANTES de chamar e reenvia os mesmos
// na retomada. Sem saldo para o lote inteiro devolve ErroPlataforma com
// Codigo = CodigoCotaEsgotada (e a Cota) e nada é entregue.
func PlataformaExtracao(ctx context.Context, pedidoID string, cnpjs []string, incluirPF bool) (*ExtracaoPlataforma, error) {
	if len(cnpjs) > MaxCNPJsExtracao {
		return nil, fmt.Errorf("extração aceita até %d CNPJs por chamada", MaxCNPJsExtracao)
	}
	var out ExtracaoPlataforma
	corpo := map[string]any{"pedido_id": pedidoID, "cnpjs": cnpjs, "incluir_pf": incluirPF}
	if err := chamarProspeccao(ctx, http.MethodPost, "/extracoes", nil, corpo, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PlataformaCNPJ devolve a ficha de um CNPJ na base própria (grátis, com
// limite por minuto e por dia; pessoa física vem sem contatos). CNPJ que não
// está na base volta como ErroPlataforma com Codigo = CodigoNaoEncontrado.
func PlataformaCNPJ(ctx context.Context, cnpj string) (*models.ProspEmpresa, error) {
	cnpj = models.NormalizarCNPJ(cnpj)
	if !models.CNPJFormatoValido(cnpj) {
		return nil, &ErroPlataforma{Status: http.StatusUnprocessableEntity, Codigo: CodigoFiltroInvalido, Mensagem: "CNPJ inválido"}
	}
	var out models.ProspEmpresa
	if err := chamarProspeccao(ctx, http.MethodGet, "/cnpj/"+cnpj, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
