package enriquecimento

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

// Endereços dos ambientes Serasa Experian.
var (
	BaseSerasaUAT  = "https://uat-api.serasaexperian.com.br"
	BaseSerasaProd = "https://api.serasaexperian.com.br"
)

// Serasa faz login (Basic client_id:secret → accessToken) e pede o relatório
// de PJ. O schema de resposta não é público: o parser é tolerante.
type Serasa struct {
	ClientID         string
	ClientSecret     string
	Ambiente         string // uat|prod
	BaseURL          string
	ReportName       string
	OptionalFeatures string
	ReportPath       string
	PrecoCentavos    int
}

func (s *Serasa) Codigo() string { return models.ProvedorSerasa }

func (s *Serasa) base() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	if s.Ambiente == "prod" {
		return BaseSerasaProd
	}
	return BaseSerasaUAT
}

func (s *Serasa) chaveToken() string {
	soma := sha256.Sum256([]byte(s.base() + "|" + s.ClientID))
	return "enriq:serasa:token:" + hex.EncodeToString(soma[:8])
}

// Login obtém (ou reaproveita do cache) o accessToken.
func (s *Serasa) Login(ctx context.Context) (string, error) {
	if tok, ok := utils.Cache.Get(s.chaveToken()); ok && tok != "" {
		return tok, nil
	}
	cred := base64.StdEncoding.EncodeToString([]byte(s.ClientID + ":" + s.ClientSecret))
	status, corpo, err := requisitar(ctx, http.MethodPost, s.base()+"/security/iam/v1/client-identities/login",
		strings.NewReader(""), map[string]string{
			"Authorization": "Basic " + cred,
			"Content-Type":  "application/json",
		})
	if err != nil {
		return "", falha(FalhaExterna, "não foi possível falar com o Serasa (%s)", MensagemDaFalha(err))
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "", falha(FalhaCredencial, "o Serasa recusou o client_id/client_secret. Confira em Configurações → Integrações.")
	case status == http.StatusTooManyRequests:
		return "", falha(FalhaLimite, "limite de chamadas do Serasa atingido; tente novamente em instantes")
	case status >= 500:
		return "", falha(FalhaExterna, "Serasa indisponível (HTTP %d)", status)
	case status != http.StatusOK && status != http.StatusCreated:
		return "", falha(FalhaExterna, "login do Serasa respondeu HTTP %d", status)
	}
	m, err := decodificar(corpo)
	if err != nil {
		return "", falha(FalhaExterna, "login do Serasa devolveu uma resposta inesperada")
	}
	tok := m.str("accessToken", "access_token", "token")
	if tok == "" {
		return "", falha(FalhaExterna, "login do Serasa não devolveu o token de acesso")
	}
	exp, _ := m.num("expiresIn", "expires_in")
	ttl := time.Duration(exp)*time.Second - 60*time.Second
	if exp <= 0 {
		ttl = 10 * time.Minute
	}
	if ttl > 0 {
		utils.Cache.Set(s.chaveToken(), tok, ttl)
	}
	return tok, nil
}

// caminhoRelatorio monta a URL do relatório a partir do template configurável.
func (s *Serasa) caminhoRelatorio(cnpj string) string {
	tpl := s.ReportPath
	if tpl == "" {
		tpl = models.SerasaReportPathPadrao
	}
	report := s.ReportName
	if report == "" {
		report = "RELATORIO_AVANCADO_PJ_PME"
	}
	r := strings.NewReplacer(
		"{report}", url.QueryEscape(report),
		"{features}", url.QueryEscape(s.OptionalFeatures),
		"{cnpj}", cnpj,
	)
	caminho := r.Replace(tpl)
	if !strings.HasPrefix(caminho, "/") {
		caminho = "/" + caminho
	}
	return s.base() + caminho
}

func (s *Serasa) Consultar(ctx context.Context, cnpj string, opts Opcoes) (*Perfil, *Bruto, error) {
	digits := SomenteDigitos(cnpj)
	if !ValidarCNPJ(digits) {
		return nil, nil, falha(FalhaEntrada, "CNPJ inválido")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancelar := context.WithTimeout(ctx, timeout)
	defer cancelar()

	tok, err := s.Login(ctx)
	if err != nil {
		return nil, nil, err
	}
	status, corpo, err := requisitar(ctx, http.MethodGet, s.caminhoRelatorio(digits), nil, map[string]string{
		"Authorization": "Bearer " + tok,
	})
	if err != nil {
		return nil, nil, falha(FalhaExterna, "não foi possível falar com o Serasa (%s)", MensagemDaFalha(err))
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		utils.Cache.Del(s.chaveToken())
		return nil, nil, falha(FalhaCredencial, "o Serasa recusou o acesso ao relatório (token expirado ou produto não contratado)")
	case status == http.StatusNotFound:
		return nil, nil, falha(FalhaSemResultado, "o Serasa não encontrou dados para este CNPJ")
	case status == http.StatusTooManyRequests:
		return nil, nil, falha(FalhaLimite, "limite de chamadas do Serasa atingido; tente novamente em instantes")
	case status >= 500:
		return nil, nil, falha(FalhaExterna, "Serasa indisponível (HTTP %d)", status)
	case status != http.StatusOK:
		return nil, nil, falha(FalhaExterna, "Serasa respondeu HTTP %d", status)
	}
	p, err := ParseSerasa(corpo, digits)
	if err != nil {
		return nil, nil, err
	}
	p.CustoCentavos = s.PrecoCentavos
	p.Sinais = CalcularSinais(p)
	return p, &Bruto{Fonte: models.ProvedorSerasa, Corpo: json.RawMessage(corpo)}, nil
}

// -----------------------------------------------------------------------------
// Parser tolerante
// -----------------------------------------------------------------------------

// ParseSerasa percorre o JSON em qualquer profundidade procurando chaves
// conhecidas (score, pendências, protestos, cheques, ações, consultas, limite,
// sócios, razão social, situação) e preenche o que achar.
func ParseSerasa(corpo []byte, cnpj string) (*Perfil, error) {
	var raiz any
	if err := json.Unmarshal(corpo, &raiz); err != nil {
		return nil, falha(FalhaExterna, "Serasa devolveu uma resposta inesperada")
	}
	p := NovoPerfil(models.ProvedorSerasa)
	p.Set("cnpj", FormatarCNPJ(cnpj))
	r := &Risco{}
	achouRisco := false
	scoreDefinido := false
	visto := map[string]bool{} // contadores já preenchidos (primeira ocorrência vale)

	percorrer(raiz, func(chave string, val any) bool {
		k := normalizarChave(chave)
		switch {
		case k == "scorepositivo" || k == "score" || k == "pontuacao" || k == "scorepj":
			if scoreDefinido && k != "scorepositivo" {
				return false
			}
			if n, ok := numeroDe(val); ok && n > 0 && n <= 1000 {
				r.Score = int(n)
				achouRisco, scoreDefinido = true, true
				return true
			}
			if m := comoMapa(val); m != nil {
				if n, ok := m.num("pontuacao", "score", "valor", "value", "scorePositivo"); ok && n > 0 {
					r.Score = int(n)
					achouRisco, scoreDefinido = true, true
				}
				if f := m.str("faixa", "classe", "classificacao", "rating"); f != "" && len(f) <= 3 && r.Faixa == "" {
					r.Faixa = strings.ToUpper(f)
				}
				if obs := m.str("mensagem", "descricao", "texto", "message"); obs != "" && r.Observacao == "" {
					r.Observacao = obs
				}
				return false
			}
		case (k == "faixa" || k == "classe" || k == "rating" || k == "faixascore") && r.Faixa == "":
			if s, ok := val.(string); ok && s != "" && len(s) <= 3 {
				r.Faixa = strings.ToUpper(s)
				achouRisco = true
			}
		case k == "pendencias" || k == "pendenciasfinanceiras" || k == "pefin" || k == "refin" || k == "anotacoesnegativas":
			if visto["pendencias"] {
				return false
			}
			if qtd, valor, ok := contagem(val); ok {
				visto["pendencias"] = true
				r.PendenciasQtd, r.PendenciasValor = qtd, valor
				achouRisco = true
				return true
			}
		case k == "protestos" || k == "protesto":
			if visto["protestos"] {
				return false
			}
			if qtd, valor, ok := contagem(val); ok {
				visto["protestos"] = true
				r.ProtestosQtd = qtd
				if r.PendenciasValor == 0 {
					r.PendenciasValor = valor
				}
				achouRisco = true
				return true
			}
		case k == "chequessemfundo" || k == "chequessemfundos" || k == "ccf" || k == "cheques":
			if visto["cheques"] {
				return false
			}
			if qtd, _, ok := contagem(val); ok {
				visto["cheques"] = true
				r.ChequesSemFundo = qtd
				achouRisco = true
				return true
			}
		case k == "acoesjudiciais" || k == "acoes" || k == "acaojudicial":
			if visto["acoes"] {
				return false
			}
			if qtd, _, ok := contagem(val); ok {
				visto["acoes"] = true
				r.AcoesQtd = qtd
				achouRisco = true
				return true
			}
		case k == "consultas" || k == "consultasrecentes" || k == "consultas90dias" || k == "quantidadeconsultas":
			if visto["consultas"] {
				return false
			}
			if qtd, _, ok := contagem(val); ok {
				visto["consultas"] = true
				r.Consultas90d = qtd
				achouRisco = true
				return true
			}
		case k == "limitecredito" || k == "limitesugerido" || k == "limitedecredito" || k == "creditlimit":
			if r.LimiteSugerido == 0 {
				if n, ok := numeroDe(val); ok {
					r.LimiteSugerido = n
					achouRisco = true
					return true
				}
				if m := comoMapa(val); m != nil {
					if n, ok := m.num("valor", "value", "limite", "valorSugerido"); ok {
						r.LimiteSugerido = n
						achouRisco = true
					}
					return false
				}
			}
		case k == "participacoes" || k == "socios" || k == "quadrosocietario" || k == "administradores" || k == "qsa":
			if len(p.Socios) == 0 {
				p.Socios = lerSociosSerasa(val)
				return len(p.Socios) > 0
			}
		case k == "razaosocial":
			if s, ok := val.(string); ok && p.Texto("legal_name") == "" {
				p.Set("legal_name", s)
			}
		case k == "nomefantasia":
			if s, ok := val.(string); ok && p.Texto("trade_name") == "" {
				p.Set("trade_name", s)
			}
		case k == "situacao" || k == "situacaocadastral" || k == "situacaorfb":
			if p.Texto("registration_status") != "" {
				return false
			}
			if s, ok := val.(string); ok {
				p.Set("registration_status", strings.ToUpper(s))
			} else if m := comoMapa(val); m != nil {
				p.Set("registration_status", strings.ToUpper(m.str("descricao", "status", "situacao")))
				p.Set("registration_status_at", NormalizarData(m.str("data", "dataSituacao")))
				return true
			}
		case k == "datafundacao" || k == "dataabertura" || k == "datainicioatividade":
			if s, ok := val.(string); ok && p.Texto("founded_at") == "" {
				p.Set("founded_at", NormalizarData(s))
			}
		case k == "cnae" || k == "atividadeprincipal" || k == "cnaeprincipal":
			if m := comoMapa(val); m != nil && p.Texto("cnae_code") == "" {
				p.Set("cnae_code", FormatarCNAE(m.str("codigo", "code", "id")))
				p.Set("cnae_description", m.str("descricao", "description"))
				return true
			}
		case k == "capitalsocial":
			if n, ok := numeroDe(val); ok && p.Texto("capital") == "" {
				p.Set("capital", n)
			}
		}
		return false
	})

	if achouRisco {
		if r.Score > 0 {
			r.Nivel = NivelDoScore(r.Score)
			if r.Faixa == "" {
				r.Faixa = FaixaDoScore(r.Score)
			}
		}
		p.Risco = r
	}
	if !achouRisco && len(p.Socios) == 0 && p.Texto("legal_name") == "" {
		p.Mensagem = "relatório recebido, mas sem campos reconhecidos; o conteúdo bruto foi guardado"
	}
	completarDerivados(p)
	return p, nil
}

// percorrer visita todas as chaves do JSON (profundidade primeiro). Quando fn
// devolve true, os filhos daquele valor não são visitados.
func percorrer(v any, fn func(chave string, val any) bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if fn(k, val) {
				continue
			}
			percorrer(val, fn)
		}
	case []any:
		for _, it := range t {
			percorrer(it, fn)
		}
	}
}

// normalizarChave reduz "scorePositivo", "score_positivo", "Score-Positivo" a "scorepositivo".
func normalizarChave(k string) string {
	k = strings.ToLower(k)
	k = strings.NewReplacer("_", "", "-", "", " ", "", "ç", "c", "ã", "a", "õ", "o", "é", "e", "ê", "e", "í", "i", "ó", "o", "ú", "u", "á", "a", "â", "a").Replace(k)
	return k
}

func numeroDe(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		s := strings.TrimSpace(t)
		s = strings.ReplaceAll(strings.ReplaceAll(s, "R$", ""), " ", "")
		if strings.Count(s, ",") == 1 && strings.Count(s, ".") >= 1 {
			s = strings.ReplaceAll(s, ".", "")
		}
		s = strings.ReplaceAll(s, ",", ".")
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// contagem extrai (quantidade, valor) de um número, lista ou objeto resumo.
func contagem(v any) (int, float64, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), 0, true
	case string:
		if n, ok := numeroDe(t); ok {
			return int(n), 0, true
		}
	case []any:
		total := 0.0
		for _, it := range t {
			if im := comoMapa(it); im != nil {
				if n, ok := im.num("valor", "value", "valorTotal", "amount"); ok {
					total += n
				}
			}
		}
		return len(t), total, true
	case map[string]any:
		m := mapa(t)
		qtd, okQ := m.num("quantidade", "qtd", "total", "quantidadeTotal", "count", "quantity", "totalOcorrencias")
		valor, _ := m.num("valorTotal", "valor", "value", "totalValue", "amount", "somaValores")
		if !okQ {
			for _, chave := range []string{"ocorrencias", "lista", "itens", "items", "registros", "detalhes"} {
				if l := m.lista(chave); l != nil {
					qtd = float64(len(l))
					okQ = true
					if valor == 0 {
						_, valor, _ = contagem(l)
					}
					break
				}
			}
		}
		if okQ {
			return int(qtd), valor, true
		}
	}
	return 0, 0, false
}

func lerSociosSerasa(v any) []Socio {
	out := []Socio{}
	itens, ok := v.([]any)
	if !ok {
		if m := comoMapa(v); m != nil {
			for _, chave := range []string{"lista", "itens", "items", "socios", "participacoes"} {
				if l := m.lista(chave); l != nil {
					itens = l
					break
				}
			}
		}
	}
	for _, it := range itens {
		im := comoMapa(it)
		if im == nil {
			continue
		}
		nome := im.str("nome", "razaoSocial", "name", "nomeSocio")
		if nome == "" {
			continue
		}
		doc := im.str("documento", "cpf", "cnpj", "cpfCnpj", "document", "cpf_cnpj")
		tipo := "PF"
		if len(SomenteDigitos(doc)) == 14 || im.str("tipo", "tipoPessoa") == "PJ" {
			tipo = "PJ"
		}
		qual := im.str("qualificacao", "cargo", "tipoParticipacao", "funcao", "vinculo")
		if pct, ok := im.num("percentual", "participacao", "percentualParticipacao"); ok && pct > 0 {
			if qual != "" {
				qual += " · "
			}
			qual += strconv.FormatFloat(pct, 'f', -1, 64) + "%"
		}
		out = append(out, Socio{
			Nome: nome, Documento: doc, Qualificacao: qual,
			Desde: NormalizarData(im.str("dataEntrada", "desde", "dataInicio", "data")), Tipo: tipo,
		})
	}
	return out
}
