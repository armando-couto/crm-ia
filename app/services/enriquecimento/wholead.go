package enriquecimento

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

// -----------------------------------------------------------------------------
// Who Lead (Plus Soluções): cliente da API do integrador
// (https://api.wholead.com.br/api/v1, especificação pública em /openapi.json).
// Contrato: docs/PROSPECCAO.md, seção 7.
//
//   - token OAuth2 client credentials, guardado em cache até expires_in − 60 s;
//     401 numa chamada emite outro token uma vez;
//   - consulta síncrona de um CNPJ (Provedor.Consultar, quarta fonte da ficha);
//   - lote assíncrono (criar, acompanhar, ler resultados em páginas), usado
//     pelo job "enriquecer" das listas de prospecção;
//   - dinheiro vem como texto decimal: CentavosDeDecimal, nunca float.
// -----------------------------------------------------------------------------

// WhoLeadResultadosLimite é o tamanho máximo de página de resultados do lote.
const WhoLeadResultadosLimite = 500

// WhoLead fala com a API do integrador.
type WhoLead struct {
	ClientID                string
	ClientSecret            string
	BaseURL                 string
	IncluirSocios           bool
	IncluirResponsavelLegal bool
	IncluirPortabilidade    bool
	PrecoCentavos           int
}

func (w *WhoLead) Codigo() string { return models.ProvedorWhoLead }

func (w *WhoLead) base() string {
	if b := strings.TrimRight(strings.TrimSpace(w.BaseURL), "/"); b != "" {
		return b
	}
	return models.WhoLeadBaseURLPadrao
}

// PrefixoCacheTokenWhoLead é apagado quando as credenciais mudam.
const PrefixoCacheTokenWhoLead = "enriq:wholead:token:"

func (w *WhoLead) chaveToken() string {
	soma := sha256.Sum256([]byte(w.base() + "|" + w.ClientID + "|" + w.ClientSecret))
	return PrefixoCacheTokenWhoLead + hex.EncodeToString(soma[:8])
}

// erroWhoLead traduz o status HTTP do Who Lead numa Falha com mensagem para o
// usuário. 402 é estado de negócio (saldo ou franquia esgotados).
func erroWhoLead(status int, corpo []byte) error {
	codigo := codigoErroWhoLead(corpo)
	switch {
	case status == http.StatusUnauthorized:
		return falha(FalhaCredencial, "o Who Lead recusou o Client ID/Client Secret. Confira em Configurações → Integrações.")
	case status == http.StatusForbidden:
		if codigo != "" {
			return falha(FalhaCredencial, "a conta do Who Lead não tem acesso a este recurso (%s)", codigo)
		}
		return falha(FalhaCredencial, "a conta do Who Lead não tem acesso a este recurso")
	case status == http.StatusPaymentRequired:
		return falha(FalhaSaldo, "saldo ou franquia do Who Lead esgotados. Recarregue a conta no Who Lead para continuar.")
	case status == http.StatusNotFound:
		return falha(FalhaSemResultado, "CNPJ não encontrado no Who Lead")
	case status == http.StatusConflict:
		return falha(FalhaOcupado, "o Who Lead está processando um pedido igual; tente novamente em instantes")
	case status == http.StatusTooManyRequests:
		return falha(FalhaLimite, "limite de chamadas do Who Lead atingido; tente novamente em instantes")
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return falha(FalhaEntrada, "o Who Lead recusou o pedido (HTTP %d)", status)
	case status >= 500:
		return falha(FalhaExterna, "Who Lead indisponível (HTTP %d)", status)
	}
	return falha(FalhaExterna, "o Who Lead respondeu HTTP %d", status)
}

// codigoErroWhoLead lê {"detail": {"code": "..."}} (ou "detail" em texto).
func codigoErroWhoLead(corpo []byte) string {
	m, err := decodificar(corpo)
	if err != nil || m == nil {
		return ""
	}
	if d := m.sub("detail"); d != nil {
		return d.str("code")
	}
	if s := m.str("code", "error"); s != "" && len(s) < 60 {
		return s
	}
	return ""
}

// Token obtém (ou reaproveita do cache) o access_token.
func (w *WhoLead) Token(ctx context.Context) (string, error) {
	if tok, ok := utils.Cache.Get(w.chaveToken()); ok && tok != "" {
		return tok, nil
	}
	if w.ClientID == "" || w.ClientSecret == "" {
		return "", falha(FalhaConfig, "informe o Client ID e o Client Secret do Who Lead em Configurações → Integrações")
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", w.ClientID)
	form.Set("client_secret", w.ClientSecret)
	status, corpo, err := requisitar(ctx, http.MethodPost, w.base()+"/oauth/token", strings.NewReader(form.Encode()),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if err != nil {
		return "", falha(FalhaExterna, "não foi possível falar com o Who Lead (%s)", MensagemDaFalha(err))
	}
	if status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "", falha(FalhaCredencial, "o Who Lead recusou o Client ID/Client Secret. Confira em Configurações → Integrações.")
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", erroWhoLead(status, corpo)
	}
	m, err := decodificar(corpo)
	if err != nil {
		return "", falha(FalhaExterna, "o Who Lead devolveu uma resposta inesperada ao emitir o token")
	}
	tok := m.str("access_token")
	if tok == "" {
		return "", falha(FalhaExterna, "o Who Lead não devolveu o token de acesso")
	}
	exp, _ := m.num("expires_in")
	ttl := time.Duration(exp)*time.Second - 60*time.Second
	if exp <= 0 {
		ttl = 10 * time.Minute
	}
	if ttl > 0 {
		utils.Cache.Set(w.chaveToken(), tok, ttl)
	}
	return tok, nil
}

// chamar faz a requisição autenticada; em 401 descarta o token e tenta uma
// vez com um novo. Devolve o corpo quando o status é 2xx; senão a Falha.
func (w *WhoLead) chamar(ctx context.Context, metodo, caminho string, corpo any) (int, []byte, error) {
	var payload []byte
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			return 0, nil, err
		}
		payload = b
	}
	for tentativa := 0; tentativa < 2; tentativa++ {
		tok, err := w.Token(ctx)
		if err != nil {
			return 0, nil, err
		}
		cab := map[string]string{"Authorization": "Bearer " + tok}
		var leitor *bytes.Reader
		if payload != nil {
			cab["Content-Type"] = "application/json"
			leitor = bytes.NewReader(payload)
		}
		var status int
		var resp []byte
		if leitor != nil {
			status, resp, err = requisitar(ctx, metodo, w.base()+caminho, leitor, cab)
		} else {
			status, resp, err = requisitar(ctx, metodo, w.base()+caminho, nil, cab)
		}
		if err != nil {
			return 0, nil, falha(FalhaExterna, "não foi possível falar com o Who Lead (%s)", MensagemDaFalha(err))
		}
		if status == http.StatusUnauthorized && tentativa == 0 {
			utils.Cache.Del(w.chaveToken())
			continue
		}
		if status < 200 || status > 299 {
			return status, resp, erroWhoLead(status, resp)
		}
		return status, resp, nil
	}
	return 0, nil, falha(FalhaCredencial, "o Who Lead recusou o Client ID/Client Secret. Confira em Configurações → Integrações.")
}

// CentavosDeDecimal converte o texto decimal do Who Lead ("0.80", "12,5",
// "3") em centavos, sem passar por ponto flutuante. Terceira casa em diante é
// arredondada (meio para cima).
func CentavosDeDecimal(s string) (int, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return 0, false
	}
	negativo := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	inteiro, frac, _ := strings.Cut(s, ".")
	if inteiro == "" {
		inteiro = "0"
	}
	if SomenteDigitos(inteiro) != inteiro || SomenteDigitos(frac) != frac {
		return 0, false
	}
	reais, err := strconv.Atoi(inteiro)
	if err != nil {
		return 0, false
	}
	frac += "000"
	cent, _ := strconv.Atoi(frac[:2])
	if frac[2] >= '5' {
		cent++
	}
	v := reais*100 + cent
	if negativo {
		v = -v
	}
	return v, true
}

// centavosDe lê um valor monetário do mapa (texto decimal ou número).
func centavosDe(m mapa, chaves ...string) (int, bool) {
	return CentavosDeDecimal(m.str(chaves...))
}

// -----------------------------------------------------------------------------
// Conta
// -----------------------------------------------------------------------------

// ContaWhoLead é o plano e o uso do ciclo (GET /account). No pré-pago os
// campos de franquia e ciclo vêm nulos.
type ContaWhoLead struct {
	Plano            string `json:"plano"`
	FranquiaTotal    *int   `json:"franquia_total"`
	FranquiaUsada    *int   `json:"franquia_usada"`
	FranquiaRestante *int   `json:"franquia_restante"`
	CicloInicio      string `json:"ciclo_inicio"`
	CicloFim         string `json:"ciclo_fim"`
}

func inteiroOpcional(m mapa, chave string) *int {
	if v, ok := m.num(chave); ok {
		n := int(v)
		return &n
	}
	return nil
}

// Conta consulta o plano e a franquia (sem custo).
func (w *WhoLead) Conta(ctx context.Context) (*ContaWhoLead, error) {
	_, corpo, err := w.chamar(ctx, http.MethodGet, "/account", nil)
	if err != nil {
		if TipoDaFalha(err) == FalhaSemResultado {
			return nil, falha(FalhaExterna, "o Who Lead não encontrou a conta")
		}
		return nil, err
	}
	m, err := decodificar(corpo)
	if err != nil {
		return nil, falha(FalhaExterna, "o Who Lead devolveu uma resposta inesperada para a conta")
	}
	return &ContaWhoLead{
		Plano:            m.str("plan_name"),
		FranquiaTotal:    inteiroOpcional(m, "quota_total"),
		FranquiaUsada:    inteiroOpcional(m, "quota_used"),
		FranquiaRestante: inteiroOpcional(m, "quota_remaining"),
		CicloInicio:      m.str("cycle_start"),
		CicloFim:         m.str("cycle_end"),
	}, nil
}

// -----------------------------------------------------------------------------
// Consulta síncrona de um CNPJ (Provedor.Consultar)
// -----------------------------------------------------------------------------

func (w *WhoLead) Consultar(ctx context.Context, cnpj string, opts Opcoes) (*Perfil, *Bruto, error) {
	c := NormalizarCNPJ(cnpj)
	if !ValidarCNPJ(c) {
		return nil, nil, falha(FalhaEntrada, "CNPJ inválido")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancelar := context.WithTimeout(ctx, timeout)
	defer cancelar()

	_, corpo, err := w.chamar(ctx, http.MethodGet, "/enrichment/cnpj/"+url.PathEscape(c), nil)
	if err != nil {
		return nil, nil, err
	}
	m, err := decodificar(corpo)
	if err != nil {
		return nil, nil, falha(FalhaExterna, "o Who Lead devolveu uma resposta inesperada")
	}
	if achou, ok := m.boolSim("found"); ok && !achou {
		return nil, nil, falha(FalhaSemResultado, "CNPJ não encontrado no Who Lead")
	}
	var dados []byte
	if v, ok := m["data"]; ok && v != nil {
		dados, _ = json.Marshal(v)
	}
	p, _, err := ParseCnpjData(dados, models.ProvedorWhoLead, c)
	if err != nil {
		return nil, nil, err
	}
	if p == nil {
		p = NovoPerfil(models.ProvedorWhoLead)
		p.Set("cnpj", FormatarCNPJ(c))
		p.Mensagem = "o Who Lead devolveu os dados num formato ainda não reconhecido; nada foi lido"
	}
	if custo, ok := centavosDe(m, "cost"); ok {
		p.CustoCentavos = custo
	} else {
		p.CustoCentavos = w.PrecoCentavos
	}
	return p, &Bruto{Fonte: models.ProvedorWhoLead, Corpo: MascararCPFsJSON(corpo)}, nil
}

// -----------------------------------------------------------------------------
// Lote assíncrono
// -----------------------------------------------------------------------------

// LoteWhoLead é a resposta 202 de POST /enrichment.
type LoteWhoLead struct {
	ConsultationID int64  `json:"consultation_id"`
	Status         string `json:"status"`
	CustoCentavos  int    `json:"custo_centavos"` // total_cost (texto decimal) em centavos
	CustoInformado bool   `json:"custo_informado"`
	Unidades       int    `json:"unidades"`
	Itens          int    `json:"itens"`
}

// CriarLote pede o enriquecimento dos CNPJs (já normalizados, aceitando letras).
func (w *WhoLead) CriarLote(ctx context.Context, cnpjs []string) (*LoteWhoLead, error) {
	_, corpo, err := w.chamar(ctx, http.MethodPost, "/enrichment", map[string]any{
		"cnpjs":                     cnpjs,
		"include_socios":            w.IncluirSocios,
		"include_responsavel_legal": w.IncluirResponsavelLegal,
		"include_portability":       w.IncluirPortabilidade,
	})
	if err != nil {
		return nil, err
	}
	m, err := decodificar(corpo)
	if err != nil {
		return nil, falha(FalhaExterna, "o Who Lead devolveu uma resposta inesperada ao criar o lote")
	}
	id, ok := m.num("consultation_id", "id")
	if !ok || id <= 0 {
		return nil, falha(FalhaExterna, "o Who Lead não devolveu o número da consulta do lote")
	}
	l := &LoteWhoLead{ConsultationID: int64(id), Status: m.str("status")}
	l.CustoCentavos, l.CustoInformado = centavosDe(m, "total_cost")
	if v, ok := m.num("units_debited"); ok {
		l.Unidades = int(v)
	}
	if v, ok := m.num("item_count"); ok {
		l.Itens = int(v)
	}
	return l, nil
}

// Estados do lote no Who Lead.
const (
	LotePendente    = "pending"
	LoteProcessando = "processing"
	LotePronto      = "ready"
	LoteFalhou      = "failed"
)

// AndamentoLote é a resposta de GET /enrichment/{id}.
type AndamentoLote struct {
	Status      string
	Pronto      bool
	Total       int
	Processados int
}

// Andamento consulta o estado do lote.
func (w *WhoLead) Andamento(ctx context.Context, id int64) (*AndamentoLote, error) {
	_, corpo, err := w.chamar(ctx, http.MethodGet, fmt.Sprintf("/enrichment/%d", id), nil)
	if err != nil {
		return nil, err
	}
	m, err := decodificar(corpo)
	if err != nil {
		return nil, falha(FalhaExterna, "o Who Lead devolveu uma resposta inesperada para o lote")
	}
	a := &AndamentoLote{Status: strings.ToLower(m.str("status"))}
	pronto, _ := m.boolSim("ready")
	a.Pronto = pronto || a.Status == LotePronto
	if v, ok := m.num("items_total"); ok {
		a.Total = int(v)
	}
	if v, ok := m.num("items_processed"); ok {
		a.Processados = int(v)
	}
	return a, nil
}

// Resultados do item do lote.
const (
	ResultadoEncontrado    = "found"
	ResultadoNaoEncontrado = "not_found"
	ResultadoLimitado      = "rate_limited"
	ResultadoErro          = "error"
)

// PessoaWhoLead é um item de persons[] (CPF completo: nunca gravar assim).
type PessoaWhoLead struct {
	CPF          string
	TipoPessoa   string
	Nome         string
	Qualificacao string
	Desde        string
	Telefones    []string
}

// ResultadoWhoLead é um item de GET /enrichment/{id}/results.
type ResultadoWhoLead struct {
	CNPJ         string
	Encontrado   bool
	Resultado    string // found | not_found | rate_limited | error
	Estornado    bool
	CnpjData     json.RawMessage
	NomeFantasia string
	Pessoas      []PessoaWhoLead
	// Bruto é o item inteiro com todo CPF já mascarado (é o que se grava).
	Bruto json.RawMessage
}

// PaginaResultados é uma página de resultados do lote.
type PaginaResultados struct {
	Pronto bool
	Total  int
	Itens  []ResultadoWhoLead
}

// Resultados lê uma página (limite até 500) de resultados do lote.
func (w *WhoLead) Resultados(ctx context.Context, id int64, offset, limite int) (*PaginaResultados, error) {
	if limite <= 0 || limite > WhoLeadResultadosLimite {
		limite = WhoLeadResultadosLimite
	}
	_, corpo, err := w.chamar(ctx, http.MethodGet, fmt.Sprintf("/enrichment/%d/results?offset=%d&limit=%d", id, offset, limite), nil)
	if err != nil {
		return nil, err
	}
	return LerPaginaResultados(corpo)
}

// LerPaginaResultados decodifica a página de resultados de forma tolerante.
func LerPaginaResultados(corpo []byte) (*PaginaResultados, error) {
	m, err := decodificar(corpo)
	if err != nil {
		return nil, falha(FalhaExterna, "o Who Lead devolveu uma resposta inesperada para os resultados do lote")
	}
	pag := &PaginaResultados{Itens: []ResultadoWhoLead{}}
	pag.Pronto, _ = m.boolSim("ready")
	if v, ok := m.num("total"); ok {
		pag.Total = int(v)
	}
	for _, it := range m.lista("items") {
		im := comoMapa(it)
		if im == nil {
			continue
		}
		r := ResultadoWhoLead{
			CNPJ:         NormalizarCNPJ(zerosEsquerdaCNPJ(im.str("cnpj"))),
			Resultado:    strings.ToLower(im.str("result")),
			NomeFantasia: im.str("nome_fantasia"),
		}
		r.Encontrado, _ = im.boolSim("found")
		r.Estornado, _ = im.boolSim("refunded")
		if r.Resultado == "" {
			r.Resultado = ResultadoNaoEncontrado
			if r.Encontrado {
				r.Resultado = ResultadoEncontrado
			}
		}
		if v, ok := im["cnpj_data"]; ok && v != nil {
			r.CnpjData, _ = json.Marshal(v)
		}
		for _, pv := range im.lista("persons") {
			pm := comoMapa(pv)
			if pm == nil {
				continue
			}
			pw := PessoaWhoLead{
				CPF: pm.str("cpf"), TipoPessoa: pm.str("tipo_pessoa"), Nome: pm.str("nome", "name"),
				Qualificacao: pm.str("qualificacao", "qualification"), Desde: NormalizarData(pm.str("data_entrada_sociedade")),
			}
			for _, t := range pm.lista("telefones") {
				switch tv := t.(type) {
				case string:
					pw.Telefones = append(pw.Telefones, tv)
				case float64:
					pw.Telefones = append(pw.Telefones, strconv.FormatInt(int64(tv), 10))
				case map[string]any:
					tm := mapa(tv)
					pw.Telefones = append(pw.Telefones, tm.str("ddd")+tm.str("number", "numero", "telefone"))
				}
			}
			r.Pessoas = append(r.Pessoas, pw)
		}
		bruto, _ := json.Marshal(it)
		r.Bruto = MascararCPFsJSON(bruto)
		pag.Itens = append(pag.Itens, r)
	}
	return pag, nil
}

// zerosEsquerdaCNPJ repõe zeros perdidos quando o CNPJ (só números) veio como número.
func zerosEsquerdaCNPJ(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && SomenteDigitos(s) == s && len(s) < 14 {
		return zerosEsquerda(s, 14)
	}
	return s
}

// -----------------------------------------------------------------------------
// CPF no bruto
// -----------------------------------------------------------------------------

// chavesCPF são os nomes de campo que carregam CPF no payload.
var chavesCPF = map[string]bool{"cpf": true, "cpfs": true, "cpf_socio": true, "cpf_cnpj": true, "documento": true, "nr_cpf": true}

// MascararCPFsJSON devolve o JSON com todo CPF trocado pela máscara
// (***123456**): valores de chaves de CPF e qualquer texto que seja um CPF
// formatado (NNN.NNN.NNN-NN). CPF completo não é persistido em lugar nenhum.
// JSON inválido volta como null.
func MascararCPFsJSON(corpo []byte) json.RawMessage {
	if len(bytes.TrimSpace(corpo)) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(corpo, &v); err != nil {
		return json.RawMessage("null")
	}
	v = mascararCPFsValor(v, false)
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

func mascararCPFsValor(v any, chaveCPF bool) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = mascararCPFsValor(x, chavesCPF[strings.ToLower(k)])
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = mascararCPFsValor(x, chaveCPF)
		}
		return t
	case string:
		d := SomenteDigitos(t)
		if chaveCPF && len(d) > 0 && len(d) <= 11 {
			return MascararCPF(t)
		}
		if len(t) == 14 && len(d) == 11 && t[3] == '.' && t[7] == '.' && t[11] == '-' {
			return MascararCPF(t)
		}
		return t
	case float64:
		if chaveCPF && t > 0 && t < 1e11 {
			return MascararCPF(strconv.FormatInt(int64(t), 10))
		}
		return t
	}
	return v
}
