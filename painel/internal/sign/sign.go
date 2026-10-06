// Package sign é o cliente do painel para a cobrança recorrente das licenças.
//
// A CRM IA é um estabelecimento no serviço de recorrência da Fix Pay (Sign) e
// usa o PRÓPRIO token: é com ele que cria o pagador (o cliente, por CNPJ), a
// assinatura mensal e recebe a autorização do cartão. O cliente final nunca
// vê esse nome — ele cadastra o cartão numa página com a marca do CRM IA, e o
// painel repassa os dados ao gateway pela rota de integração (sem redirecionar
// para a página do processador).
//
// Em FIXPAY_SIMULADO=true (ou sem token) tudo é respondido localmente: ids
// começam com "sim-" e o cartão terminado em 0002 é recusado.
package sign

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TokenAPI string // token do estabelecimento (CRM IA)
	ApisURL  string // https://apis.fixpay.com.br       — contrato withtokenapi
	Gateway  string // https://signpayments.fixpay.com.br — rota de integração do cartão
	GrupoID  int64  // grupo do estabelecimento no serviço (exigido por enabledisable e pelo checkout)
	Simulado bool
}

type Cliente struct {
	cfg  Config
	http *http.Client
}

func Novo(cfg Config) *Cliente {
	cfg.ApisURL = strings.TrimRight(cfg.ApisURL, "/")
	cfg.Gateway = strings.TrimRight(cfg.Gateway, "/")
	if cfg.TokenAPI == "" {
		cfg.Simulado = true
	}
	return &Cliente{cfg: cfg, http: &http.Client{Timeout: 40 * time.Second}}
}

func (c *Cliente) Simulado() bool { return c.cfg.Simulado }

// Configurado diz se o painel tem como cobrar de verdade.
func (c *Cliente) Configurado() bool {
	return c.cfg.Simulado || (c.cfg.TokenAPI != "" && c.cfg.GrupoID > 0)
}

// MotivoNaoConfigurado explica, para a tela, o que falta.
func (c *Cliente) MotivoNaoConfigurado() string {
	switch {
	case c.cfg.Simulado:
		return ""
	case c.cfg.TokenAPI == "":
		return "FIXPAY_TOKEN_API não configurado"
	case c.cfg.GrupoID <= 0:
		return "FIXPAY_SIGN_GRUPO_ID não configurado"
	}
	return ""
}

// ErrNaoAutorizado: o token do estabelecimento foi recusado.
var ErrNaoAutorizado = errors.New("token do estabelecimento recusado pelo serviço de recorrência")

// -----------------------------------------------------------------------------
// Pagador
// -----------------------------------------------------------------------------

// Pagador é quem assina: o grupo (cobrança unificada) ou o CNPJ. O serviço
// exige o endereço completo.
type Pagador struct {
	Nome       string
	Documento  string
	Email      string
	Telefone   string
	CEP        string
	Logradouro string
	Numero     string
	Bairro     string
	Municipio  string
	UF         string
}

func (p Pagador) validar() error {
	if strings.TrimSpace(p.Nome) == "" || soDigitos(p.Documento) == "" {
		return errors.New("informe nome e CPF/CNPJ do pagador")
	}
	if p.CEP == "" || p.Logradouro == "" || p.Bairro == "" || p.Municipio == "" {
		return errors.New("a cobrança recorrente exige o endereço completo do pagador (CEP, logradouro, bairro e município) — complete o cadastro antes de contratar")
	}
	return nil
}

// GarantirPagador cadastra o pagador e devolve o id dele. Documento já
// cadastrado não é erro: a assinatura vincula pelo CPF/CNPJ.
func (c *Cliente) GarantirPagador(ctx context.Context, p Pagador) (string, error) {
	if err := p.validar(); err != nil {
		return "", err
	}
	if c.cfg.Simulado {
		return "sim-pag-" + aleatorio(6), nil
	}
	corpo := map[string]any{
		"nome": p.Nome, "cpf_cnpj": soDigitos(p.Documento), "email": p.Email, "telefone1": soDigitos(p.Telefone),
		"cep": soDigitos(p.CEP), "logradouro": p.Logradouro, "numero": numeroDeEndereco(p.Numero),
		"bairro": p.Bairro, "municipio": p.Municipio, "estado": strings.ToUpper(p.UF), "descricao": "CRM IA",
	}
	var saida struct {
		Data idFlex `json:"data"`
	}
	err := c.chamar(ctx, http.MethodPost, c.cfg.ApisURL+"/v2/signature/withtokenapi/client", corpo, &saida)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "mesmo cpf") || strings.Contains(strings.ToLower(err.Error()), "já existe") || strings.Contains(strings.ToLower(err.Error()), "ja existe") {
			return "existente", nil
		}
		return "", err
	}
	if saida.Data == "" {
		return "existente", nil
	}
	return string(saida.Data), nil
}

// -----------------------------------------------------------------------------
// Assinatura
// -----------------------------------------------------------------------------

type NovaAssinatura struct {
	Documento        string
	ValorCentavos    int64
	Produto          string
	PrimeiraCobranca time.Time
}

// Criada traz id, token (é o que a rota de integração do cartão usa) e o link
// do processador — que a CRM IA NÃO manda ao cliente: usa a própria página.
type Criada struct {
	ID    string
	Token string
	Link  string
}

// CriarAssinatura cria a recorrência MENSAL no cartão.
func (c *Cliente) CriarAssinatura(ctx context.Context, a NovaAssinatura) (Criada, error) {
	if a.ValorCentavos < 501 {
		return Criada{}, errors.New("o valor mínimo da assinatura no cartão é R$ 5,01")
	}
	if c.cfg.Simulado {
		tok := aleatorio(10)
		return Criada{ID: "sim-ass-" + aleatorio(6), Token: tok, Link: "simulado://" + tok}, nil
	}
	quando := a.PrimeiraCobranca
	if quando.IsZero() {
		quando = time.Now()
	}
	corpo := map[string]any{
		"forma_pagamento":        "cartao",
		"data_primeira_cobranca": quando.UTC().Format(time.RFC3339),
		"data_proxima_cobranca":  quando.UTC().Format(time.RFC3339),
		"data_expiracao":         quando.UTC().AddDate(10, 0, 0).Format(time.RFC3339),
		"desconto":               0.0,
		"juros":                  0.0,
		"valor":                  float64(a.ValorCentavos) / 100,
		"tipo_recorrencia":       "MENSAL",
		"cliente_cpf_cnpj":       soDigitos(a.Documento),
		"product_name":           a.Produto,
	}
	var saida struct {
		Data  idFlex `json:"data"`
		Link  string `json:"link"`
		Token string `json:"token"`
	}
	if err := c.chamar(ctx, http.MethodPost, c.cfg.ApisURL+"/v2/signature/withtokenapi", corpo, &saida); err != nil {
		return Criada{}, err
	}
	if saida.Data == "" || saida.Data == "0" {
		return Criada{}, errors.New("a assinatura foi criada mas o serviço não devolveu o id — confira no portal antes de tentar de novo")
	}
	if saida.Token == "" {
		// O token também aparece no fim do link de autorização.
		if i := strings.LastIndex(saida.Link, "/"); i >= 0 {
			saida.Token = saida.Link[i+1:]
		}
	}
	return Criada{ID: string(saida.Data), Token: saida.Token, Link: saida.Link}, nil
}

// AlternarAssinatura liga/desliga a assinatura (é o cancelamento e a reativação).
func (c *Cliente) AlternarAssinatura(ctx context.Context, assinaturaID string) error {
	if c.cfg.Simulado || assinaturaID == "" {
		return nil
	}
	id, err := strconv.ParseInt(assinaturaID, 10, 64)
	if err != nil {
		return fmt.Errorf("assinatura %q não tem um id numérico — foi criada em modo simulado? Cancele no portal", assinaturaID)
	}
	if c.cfg.GrupoID <= 0 {
		return errors.New("configure FIXPAY_SIGN_GRUPO_ID para cancelar assinaturas")
	}
	return c.chamar(ctx, http.MethodPut, c.cfg.ApisURL+"/v2/signature/withtokenapi/enabledisable",
		map[string]any{"id": id, "grupo_id": c.cfg.GrupoID}, nil)
}

// -----------------------------------------------------------------------------
// Checkout com a marca do CRM IA: o painel recebe o cartão da página própria
// e o entrega ao gateway pela rota de integração. Os dados do cartão passam
// e não ficam: nada é gravado nem logado além de bandeira e final.
// -----------------------------------------------------------------------------

type Cartao struct {
	Titular     string
	Documento   string // CPF/CNPJ do titular
	Numero      string
	Validade    string // MM/AA ou MM/AAAA
	CVV         string
	Bandeira    string // detectada pelo número quando vazia
	IP          string
	CEP         string
	Logradouro  string
	Numero_     string
	Complemento string
	Bairro      string
	Cidade      string
	UF          string
}

type ResultadoCartao struct {
	Aprovado bool
	// URL3DS: quando o emissor exige autenticação, o cliente é levado para lá.
	URL3DS     string
	Final      string
	Bandeira   string
	Mensagem   string
	Referencia string
}

// CadastrarCartao envia o cartão para a assinatura identificada por token.
func (c *Cliente) CadastrarCartao(ctx context.Context, token string, cartao Cartao) (ResultadoCartao, error) {
	numero := soDigitos(cartao.Numero)
	if len(numero) < 13 || len(numero) > 19 || !luhnOK(numero) {
		return ResultadoCartao{}, errors.New("número do cartão inválido")
	}
	mes, ano, err := validade(cartao.Validade)
	if err != nil {
		return ResultadoCartao{}, err
	}
	cvv := soDigitos(cartao.CVV)
	if len(cvv) < 3 || len(cvv) > 4 {
		return ResultadoCartao{}, errors.New("código de segurança inválido")
	}
	if strings.TrimSpace(cartao.Titular) == "" || soDigitos(cartao.Documento) == "" {
		return ResultadoCartao{}, errors.New("informe o nome e o CPF/CNPJ do titular do cartão")
	}
	bandeira := cartao.Bandeira
	if bandeira == "" {
		bandeira = Bandeira(numero)
	}
	if bandeira == "" {
		return ResultadoCartao{}, errors.New("não reconhecemos a bandeira deste cartão (Visa, Mastercard, Elo, Amex ou Hipercard)")
	}
	res := ResultadoCartao{Final: numero[len(numero)-4:], Bandeira: bandeira}

	if c.cfg.Simulado {
		if strings.HasSuffix(numero, "0002") {
			res.Mensagem = "cartão recusado pelo emissor (simulação)"
			return res, nil
		}
		res.Aprovado = true
		res.Referencia = "sim-" + aleatorio(8)
		return res, nil
	}
	if !c.Configurado() {
		return res, errors.New(c.MotivoNaoConfigurado())
	}
	corpo := map[string]any{
		"grupo_id": c.cfg.GrupoID, "token": token,
		"cpf_cnpj": soDigitos(cartao.Documento), "bandeira_cartao": bandeira, "titular": strings.TrimSpace(cartao.Titular),
		"numero_cartao": numero, "verificacao": cvv, "validade": fmt.Sprintf("%02d/%04d", mes, ano), "ip": cartao.IP,
		"city": cartao.Cidade, "state": strings.ToUpper(cartao.UF), "country": "BRA", "street": cartao.Logradouro,
		"house_number": numeroDeEndereco(cartao.Numero_), "complement": cartao.Complemento,
		"neighborhood": cartao.Bairro, "postal_code": soDigitos(cartao.CEP),
	}
	var saida struct {
		Data struct {
			SafeLoad     bool   `json:"SafeLoad"`
			SafeURL      string `json:"SafeUrl"`
			NumeroCartao string `json:"NumeroCartao"`
		} `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	err = c.chamar(ctx, http.MethodPost, fmt.Sprintf("%s/signature/integration/%d/%s", c.cfg.Gateway, c.cfg.GrupoID, token), corpo, &saida)
	if err != nil {
		// Recusa do emissor volta como erro HTTP 400 com a mensagem; é um
		// resultado de negócio, não falha técnica.
		var re *ErroResposta
		if errors.As(err, &re) && re.Status == http.StatusBadRequest {
			res.Mensagem = re.Mensagem
			return res, nil
		}
		return res, err
	}
	if saida.Data.SafeLoad && saida.Data.SafeURL != "" {
		res.URL3DS = saida.Data.SafeURL
		return res, nil
	}
	res.Aprovado = true
	return res, nil
}

// -----------------------------------------------------------------------------
// Conferência de pagamentos: relatório de cobranças de uma assinatura.
// -----------------------------------------------------------------------------

// Cobranca é um pagamento registrado na recorrência.
type Cobranca struct {
	Referencia    string
	Data          time.Time
	ValorCentavos int64
	Pago          bool
}

// Cobrancas lista o histórico de cobranças de uma assinatura. O serviço não
// tem um formato único de resposta; o leitor aceita variações (lista ou
// objeto com data/items) e ignora o que não reconhece.
func (c *Cliente) Cobrancas(ctx context.Context, assinaturaID string) ([]Cobranca, error) {
	if c.cfg.Simulado {
		return nil, nil
	}
	id, err := strconv.ParseInt(assinaturaID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("assinatura %q sem id numérico", assinaturaID)
	}
	var bruto json.RawMessage
	if err := c.chamar(ctx, http.MethodPost, c.cfg.ApisURL+"/v2/signature/withtokenapi/billingreportsignatures",
		map[string]any{"signature_id": id, "payment_status": "ADIMPLENTE"}, &bruto); err != nil {
		return nil, err
	}
	return lerCobrancas(bruto), nil
}

type registroCobranca struct {
	ID            json.Number `json:"id"`
	Tid           string      `json:"tid"`
	Referencia    string      `json:"referencia"`
	Status        string      `json:"status"`
	StatusDesc    string      `json:"status_descricao"`
	Valor         json.Number `json:"valor"`
	DataPagamento string      `json:"data_pagamento"`
	DataCobranca  string      `json:"data_cobranca"`
	PaymentDate   string      `json:"payment_date"`
	Amount        json.Number `json:"amount"`
}

func lerCobrancas(bruto json.RawMessage) []Cobranca {
	var registros []registroCobranca
	if json.Unmarshal(bruto, &registros) != nil {
		var env struct {
			Data  []registroCobranca `json:"data"`
			Items []registroCobranca `json:"items"`
		}
		if json.Unmarshal(bruto, &env) == nil {
			registros = append(env.Data, env.Items...)
		}
	}
	var saida []Cobranca
	for _, r := range registros {
		data := primeiraData(r.DataPagamento, r.PaymentDate, r.DataCobranca)
		valor := r.Valor
		if valor == "" {
			valor = r.Amount
		}
		f, _ := valor.Float64()
		ref := r.Tid
		if ref == "" {
			ref = r.Referencia
		}
		if ref == "" {
			ref = r.ID.String()
		}
		st := strings.ToUpper(r.Status + " " + r.StatusDesc)
		pago := strings.Contains(st, "PAG") || strings.Contains(st, "APROV") || strings.Contains(st, "PAID") || strings.Contains(st, "CAPTUR")
		if r.DataPagamento != "" || r.PaymentDate != "" {
			pago = pago || st == " "
		}
		saida = append(saida, Cobranca{Referencia: ref, Data: data, ValorCentavos: int64(f*100 + 0.5), Pago: pago})
	}
	return saida
}

func primeiraData(valores ...string) time.Time {
	for _, v := range valores {
		if v == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02", "02/01/2006 15:04:05", "02/01/2006"} {
			if t, err := time.Parse(layout, v); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// -----------------------------------------------------------------------------

// ErroResposta é a recusa HTTP do serviço, com a mensagem dele.
type ErroResposta struct {
	Status   int
	Mensagem string
}

func (e *ErroResposta) Error() string {
	return fmt.Sprintf("serviço de recorrência respondeu %d: %s", e.Status, e.Mensagem)
}

func (c *Cliente) chamar(ctx context.Context, metodo, url string, corpo, saida any) error {
	if c.cfg.TokenAPI == "" {
		return errors.New("FIXPAY_TOKEN_API não configurado")
	}
	var leitor io.Reader
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			return err
		}
		leitor = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, url, leitor)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.TokenAPI)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("serviço de recorrência inacessível: %w", err)
	}
	defer resp.Body.Close()
	dados, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: %s", ErrNaoAutorizado, resumo(dados))
	}
	if resp.StatusCode >= 300 {
		return &ErroResposta{Status: resp.StatusCode, Mensagem: resumo(dados)}
	}
	if saida != nil && len(dados) > 0 {
		if raw, ok := saida.(*json.RawMessage); ok {
			*raw = append((*raw)[:0], dados...)
			return nil
		}
		dec := json.NewDecoder(bytes.NewReader(dados))
		dec.UseNumber()
		if err := dec.Decode(saida); err != nil {
			return fmt.Errorf("resposta inesperada do serviço de recorrência: %w", err)
		}
	}
	return nil
}

// resumo extrai a mensagem de erro do corpo, seja texto ou objeto.
func resumo(b []byte) string {
	var e struct {
		Error  json.RawMessage `json:"error"`
		Errors json.RawMessage `json:"Errors"`
		Msg    string          `json:"message"`
	}
	if json.Unmarshal(b, &e) == nil {
		for _, raw := range []json.RawMessage{e.Error, e.Errors} {
			if len(raw) == 0 {
				continue
			}
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				return s
			}
			var m map[string]string
			if json.Unmarshal(raw, &m) == nil {
				var partes []string
				for k, v := range m {
					if k == "Token" {
						continue
					}
					partes = append(partes, v)
				}
				if len(partes) > 0 {
					return strings.Join(partes, "; ")
				}
			}
		}
		if e.Msg != "" {
			return e.Msg
		}
	}
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// idFlex aceita o id vindo como número ou como texto.
type idFlex string

func (i *idFlex) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*i = ""
		return nil
	}
	if len(s) >= 2 && s[0] == '"' {
		var t string
		if err := json.Unmarshal(b, &t); err != nil {
			return err
		}
		*i = idFlex(strings.TrimSpace(t))
		return nil
	}
	*i = idFlex(s)
	return nil
}

func soDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func numeroDeEndereco(s string) int {
	n, _ := strconv.Atoi(soDigitos(s))
	return n
}

func aleatorio(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

func luhnOK(numero string) bool {
	soma, alterna := 0, false
	for i := len(numero) - 1; i >= 0; i-- {
		d := int(numero[i] - '0')
		if alterna {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		soma += d
		alterna = !alterna
	}
	return soma%10 == 0
}

func validade(v string) (int, int, error) {
	partes := strings.Split(strings.TrimSpace(v), "/")
	if len(partes) != 2 {
		return 0, 0, errors.New("validade no formato MM/AA")
	}
	mes, err1 := strconv.Atoi(strings.TrimSpace(partes[0]))
	ano, err2 := strconv.Atoi(strings.TrimSpace(partes[1]))
	if err1 != nil || err2 != nil || mes < 1 || mes > 12 {
		return 0, 0, errors.New("validade no formato MM/AA")
	}
	if ano < 100 {
		ano += 2000
	}
	agora := time.Now()
	if ano < agora.Year() || (ano == agora.Year() && mes < int(agora.Month())) {
		return 0, 0, errors.New("cartão vencido")
	}
	return mes, ano, nil
}

// Bandeira identifica a bandeira pelo início do número.
func Bandeira(numero string) string {
	switch {
	case strings.HasPrefix(numero, "4"):
		return "VISA"
	case strings.HasPrefix(numero, "34"), strings.HasPrefix(numero, "37"):
		return "AMEX"
	case strings.HasPrefix(numero, "606282"), strings.HasPrefix(numero, "3841"):
		return "HIPERCARD"
	case eloPrefixo(numero):
		return "ELO"
	case numero[0] == '5' && numero[1] >= '1' && numero[1] <= '5', strings.HasPrefix(numero, "2"):
		return "MASTERCARD"
	}
	return ""
}

func eloPrefixo(n string) bool {
	for _, p := range []string{"4011", "4312", "4389", "4514", "4576", "5041", "5066", "5067", "509", "6277", "6362", "6363", "650", "6516", "6550"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}
