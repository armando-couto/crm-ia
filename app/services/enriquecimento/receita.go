package enriquecimento

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// Endereços das fontes públicas (variáveis para os testes apontarem a httptest).
var (
	BaseBrasilAPI = "https://brasilapi.com.br"
	BaseOpenCNPJ  = "https://api.opencnpj.org"
	BaseCNPJWS    = "https://publica.cnpj.ws"
	BaseViaCEP    = "https://viacep.com.br"
)

// Receita consulta os dados públicos do CNPJ: BrasilAPI, com fallback para
// OpenCNPJ e CNPJ.ws pública, e normaliza o endereço pelo CEP.
type Receita struct {
	PrazoCache time.Duration
	// SemCEP desliga a normalização por CEP (testes).
	SemCEP bool
}

func (r *Receita) Codigo() string { return models.ProvedorReceita }

func (r *Receita) Consultar(ctx context.Context, cnpj string, opts Opcoes) (*Perfil, *Bruto, error) {
	digits := SomenteDigitos(cnpj)
	if !ValidarCNPJ(digits) {
		return nil, nil, falha(FalhaEntrada, "CNPJ inválido")
	}
	if !opts.Forcar {
		if p, b, ok := lerCache(models.ProvedorReceita, digits); ok {
			return p, b, nil
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancelar := context.WithTimeout(ctx, timeout)
	defer cancelar()

	var ultimo error
	fontes := []struct {
		nome string
		fn   func(context.Context, string) (*Perfil, []byte, error)
	}{
		{"brasilapi", consultarBrasilAPI},
		{"opencnpj", consultarOpenCNPJ},
		{"cnpjws", consultarCNPJWS},
	}
	for _, f := range fontes {
		if ctx.Err() != nil {
			break
		}
		p, corpo, err := f.fn(ctx, digits)
		if err != nil {
			ultimo = err
			// Sem resultado numa fonte confiável encerra: as demais têm a mesma base.
			if TipoDaFalha(err) == FalhaSemResultado {
				return nil, nil, err
			}
			continue
		}
		p.Set("cnpj", FormatarCNPJ(digits))
		if !r.SemCEP {
			completarPorCEP(ctx, p)
		}
		derivarSite(p)
		p.Sinais = CalcularSinais(p)
		b := &Bruto{Fonte: f.nome, Corpo: json.RawMessage(corpo)}
		prazo := r.PrazoCache
		if prazo <= 0 {
			prazo = 30 * 24 * time.Hour
		}
		gravarCache(models.ProvedorReceita, digits, p, b, prazo)
		return p, b, nil
	}
	if ultimo == nil {
		ultimo = falha(FalhaExterna, "Receita Federal indisponível no momento")
	}
	if _, ok := ultimo.(*Falha); !ok {
		ultimo = falha(FalhaExterna, "não foi possível consultar a Receita Federal agora (%s)", MensagemDaFalha(ultimo))
	}
	return nil, nil, ultimo
}

// derivarSite preenche domain/website a partir do e-mail corporativo.
func derivarSite(p *Perfil) {
	if p.Texto("domain") != "" {
		return
	}
	if dom := DominioDoEmail(p.Texto("email")); dom != "" {
		p.Set("domain", dom)
		if p.Texto("website") == "" {
			p.Set("website", "https://"+dom)
		}
	}
}

// -----------------------------------------------------------------------------
// Leitura tolerante de JSON genérico
// -----------------------------------------------------------------------------

type mapa map[string]any

func (m mapa) str(chaves ...string) string {
	for _, k := range chaves {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				return s
			}
		case float64:
			if t == float64(int64(t)) {
				return strconv.FormatInt(int64(t), 10)
			}
			return strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			if t {
				return "true"
			}
			return "false"
		}
	}
	return ""
}

func (m mapa) num(chaves ...string) (float64, bool) {
	for _, k := range chaves {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			return t, true
		case string:
			s := strings.ReplaceAll(strings.TrimSpace(t), ",", ".")
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

func (m mapa) boolSim(chaves ...string) (bool, bool) {
	for _, k := range chaves {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case bool:
			return t, true
		case string:
			switch strings.ToUpper(strings.TrimSpace(t)) {
			case "SIM", "S", "TRUE", "OPTANTE":
				return true, true
			case "NÃO", "NAO", "N", "FALSE":
				return false, true
			}
		}
	}
	return false, false
}

func (m mapa) sub(chave string) mapa {
	if v, ok := m[chave].(map[string]any); ok {
		return mapa(v)
	}
	return nil
}

func (m mapa) lista(chave string) []any {
	if v, ok := m[chave].([]any); ok {
		return v
	}
	return nil
}

func comoMapa(v any) mapa {
	if m, ok := v.(map[string]any); ok {
		return mapa(m)
	}
	return nil
}

func decodificar(corpo []byte) (mapa, error) {
	var m map[string]any
	if err := json.Unmarshal(corpo, &m); err != nil {
		return nil, err
	}
	return mapa(m), nil
}

// tipoSocio reduz "Pessoa Física"/2/"CPF" a PF|PJ|Estrangeiro.
func tipoSocio(v string) string {
	u := strings.ToUpper(strings.TrimSpace(v))
	switch {
	case u == "1", strings.Contains(u, "JUR"), u == "CNPJ", u == "PJ":
		return "PJ"
	case u == "2", strings.Contains(u, "FÍS"), strings.Contains(u, "FIS"), u == "CPF", u == "PF":
		return "PF"
	case u == "3", strings.Contains(u, "ESTRANG"):
		return "Estrangeiro"
	}
	return v
}

func tratarStatusHTTP(fonte string, status int) error {
	switch {
	case status == http.StatusNotFound:
		return falha(FalhaSemResultado, "CNPJ não encontrado na Receita Federal")
	case status == http.StatusTooManyRequests:
		return falha(FalhaLimite, "%s: limite de consultas atingido", fonte)
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return falha(FalhaEntrada, "%s recusou o CNPJ informado", fonte)
	case status >= 500:
		return falha(FalhaExterna, "%s indisponível (HTTP %d)", fonte, status)
	case status != http.StatusOK:
		return falha(FalhaExterna, "%s respondeu HTTP %d", fonte, status)
	}
	return nil
}

// -----------------------------------------------------------------------------
// BrasilAPI
// -----------------------------------------------------------------------------

func consultarBrasilAPI(ctx context.Context, cnpj string) (*Perfil, []byte, error) {
	status, corpo, err := requisitar(ctx, http.MethodGet, BaseBrasilAPI+"/api/cnpj/v1/"+cnpj, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := tratarStatusHTTP("BrasilAPI", status); err != nil {
		return nil, nil, err
	}
	p, err := ParseBrasilAPI(corpo)
	if err != nil {
		return nil, nil, falha(FalhaExterna, "BrasilAPI devolveu uma resposta inesperada")
	}
	return p, corpo, nil
}

// ParseBrasilAPI converte a resposta de /api/cnpj/v1 em Perfil.
func ParseBrasilAPI(corpo []byte) (*Perfil, error) {
	m, err := decodificar(corpo)
	if err != nil {
		return nil, err
	}
	if m.str("razao_social") == "" && m.str("cnpj") == "" {
		return nil, fmt.Errorf("resposta sem dados")
	}
	p := NovoPerfil(models.ProvedorReceita)
	p.Set("legal_name", m.str("razao_social"))
	p.Set("trade_name", m.str("nome_fantasia"))
	p.Set("registration_status", strings.ToUpper(m.str("descricao_situacao_cadastral")))
	p.Set("registration_status_at", NormalizarData(m.str("data_situacao_cadastral")))
	p.Set("founded_at", NormalizarData(m.str("data_inicio_atividade")))
	p.Set("cnae_code", FormatarCNAE(m.str("cnae_fiscal")))
	p.Set("cnae_description", m.str("cnae_fiscal_descricao"))
	p.Set("legal_nature", m.str("natureza_juridica"))
	p.Set("company_size", NormalizarPorte(m.str("descricao_porte", "porte")))
	if cap, ok := m.num("capital_social"); ok {
		p.Set("capital", cap)
	}
	if v, ok := m.boolSim("opcao_pelo_simples"); ok {
		p.Campos["simples"] = v
	}
	if v, ok := m.boolSim("opcao_pelo_mei"); ok {
		p.Campos["mei"] = v
	}
	rua := strings.TrimSpace(m.str("descricao_tipo_de_logradouro") + " " + m.str("logradouro"))
	p.Set("street", tituloEndereco(rua))
	p.Set("street_number", m.str("numero"))
	p.Set("complement", m.str("complemento"))
	p.Set("neighborhood", tituloEndereco(m.str("bairro")))
	p.Set("zip", SomenteDigitos(m.str("cep")))
	p.Set("city", tituloEndereco(m.str("municipio")))
	p.Set("state", strings.ToUpper(m.str("uf")))
	p.Set("phone", FormatarTelefone(m.str("ddd_telefone_1")))
	p.Set("email", strings.ToLower(m.str("email")))
	for _, c := range m.lista("cnaes_secundarios") {
		cm := comoMapa(c)
		if cm == nil {
			continue
		}
		p.CNAEsSecundarios = append(p.CNAEsSecundarios, CNAE{Codigo: FormatarCNAE(cm.str("codigo")), Descricao: cm.str("descricao")})
	}
	for _, s := range m.lista("qsa") {
		sm := comoMapa(s)
		if sm == nil {
			continue
		}
		p.Socios = append(p.Socios, Socio{
			Nome:         sm.str("nome_socio"),
			Documento:    sm.str("cnpj_cpf_do_socio"),
			Qualificacao: sm.str("qualificacao_socio"),
			Desde:        NormalizarData(sm.str("data_entrada_sociedade")),
			Tipo:         tipoSocio(sm.str("identificador_de_socio")),
		})
	}
	completarDerivados(p)
	return p, nil
}

// completarDerivados preenche name/industry a partir do cadastro.
func completarDerivados(p *Perfil) {
	if p.Texto("name") == "" {
		if n := p.Texto("trade_name"); n != "" {
			p.Set("name", n)
		} else if n := p.Texto("legal_name"); n != "" {
			p.Set("name", n)
		}
	}
	if p.Texto("industry") == "" {
		p.Set("industry", p.Texto("cnae_description"))
	}
}

// tituloEndereco passa "RUA DAS FLORES" para "Rua das Flores".
func tituloEndereco(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s != strings.ToUpper(s) {
		return s
	}
	minusculas := map[string]bool{"DE": true, "DA": true, "DO": true, "DAS": true, "DOS": true, "E": true}
	palavras := strings.Fields(strings.ToLower(s))
	for i, w := range palavras {
		if i > 0 && minusculas[strings.ToUpper(w)] {
			continue
		}
		r := []rune(w)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		palavras[i] = string(r)
	}
	return strings.Join(palavras, " ")
}

// -----------------------------------------------------------------------------
// OpenCNPJ
// -----------------------------------------------------------------------------

func consultarOpenCNPJ(ctx context.Context, cnpj string) (*Perfil, []byte, error) {
	status, corpo, err := requisitar(ctx, http.MethodGet, BaseOpenCNPJ+"/"+cnpj, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := tratarStatusHTTP("OpenCNPJ", status); err != nil {
		return nil, nil, err
	}
	p, err := ParseOpenCNPJ(corpo)
	if err != nil {
		return nil, nil, falha(FalhaExterna, "OpenCNPJ devolveu uma resposta inesperada")
	}
	return p, corpo, nil
}

// ParseOpenCNPJ converte a resposta de api.opencnpj.org em Perfil.
func ParseOpenCNPJ(corpo []byte) (*Perfil, error) {
	m, err := decodificar(corpo)
	if err != nil {
		return nil, err
	}
	if m.str("razao_social") == "" && m.str("cnpj") == "" {
		return nil, fmt.Errorf("resposta sem dados")
	}
	p := NovoPerfil(models.ProvedorReceita)
	p.Set("legal_name", m.str("razao_social"))
	p.Set("trade_name", m.str("nome_fantasia"))
	p.Set("registration_status", strings.ToUpper(m.str("situacao_cadastral")))
	p.Set("registration_status_at", NormalizarData(m.str("data_situacao_cadastral")))
	p.Set("founded_at", NormalizarData(m.str("data_inicio_atividade")))
	p.Set("legal_nature", m.str("natureza_juridica"))
	p.Set("company_size", NormalizarPorte(m.str("porte")))
	if cap, ok := m.num("capital_social"); ok {
		p.Set("capital", cap)
	}
	if v, ok := m.boolSim("opcao_simples"); ok {
		p.Campos["simples"] = v
	}
	if v, ok := m.boolSim("opcao_mei"); ok {
		p.Campos["mei"] = v
	}
	// CNAE principal: código simples ou objeto {codigo, descricao}.
	if cp := m.sub("cnae_principal"); cp != nil {
		p.Set("cnae_code", FormatarCNAE(cp.str("codigo", "id")))
		p.Set("cnae_description", cp.str("descricao"))
	} else {
		p.Set("cnae_code", FormatarCNAE(m.str("cnae_principal")))
		p.Set("cnae_description", m.str("cnae_principal_descricao", "descricao_cnae_principal"))
	}
	for _, c := range m.lista("cnaes_secundarios") {
		switch t := c.(type) {
		case string:
			p.CNAEsSecundarios = append(p.CNAEsSecundarios, CNAE{Codigo: FormatarCNAE(t)})
		case map[string]any:
			cm := mapa(t)
			p.CNAEsSecundarios = append(p.CNAEsSecundarios, CNAE{Codigo: FormatarCNAE(cm.str("codigo", "id")), Descricao: cm.str("descricao")})
		}
	}
	p.Set("street", tituloEndereco(strings.TrimSpace(m.str("tipo_logradouro")+" "+m.str("logradouro"))))
	p.Set("street_number", m.str("numero"))
	p.Set("complement", m.str("complemento"))
	p.Set("neighborhood", tituloEndereco(m.str("bairro")))
	p.Set("zip", SomenteDigitos(m.str("cep")))
	p.Set("city", tituloEndereco(m.str("municipio")))
	p.Set("state", strings.ToUpper(m.str("uf")))
	p.Set("email", strings.ToLower(m.str("email")))
	for _, t := range m.lista("telefones") {
		tm := comoMapa(t)
		if tm == nil {
			continue
		}
		if fax, _ := tm.boolSim("is_fax"); fax {
			continue
		}
		if tel := FormatarTelefone(tm.str("ddd") + tm.str("numero")); tel != "" {
			p.Set("phone", tel)
			break
		}
	}
	for _, chave := range []string{"QSA", "qsa", "socios"} {
		for _, s := range m.lista(chave) {
			sm := comoMapa(s)
			if sm == nil {
				continue
			}
			p.Socios = append(p.Socios, Socio{
				Nome:         sm.str("nome_socio", "nome"),
				Documento:    sm.str("cnpj_cpf_socio", "cnpj_cpf_do_socio", "documento"),
				Qualificacao: sm.str("qualificacao_socio", "qualificacao"),
				Desde:        NormalizarData(sm.str("data_entrada_sociedade", "data_entrada")),
				Tipo:         tipoSocio(sm.str("identificador_socio", "tipo")),
			})
		}
		if len(p.Socios) > 0 {
			break
		}
	}
	completarDerivados(p)
	return p, nil
}

// -----------------------------------------------------------------------------
// CNPJ.ws pública
// -----------------------------------------------------------------------------

func consultarCNPJWS(ctx context.Context, cnpj string) (*Perfil, []byte, error) {
	status, corpo, err := requisitar(ctx, http.MethodGet, BaseCNPJWS+"/cnpj/"+cnpj, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := tratarStatusHTTP("CNPJ.ws", status); err != nil {
		return nil, nil, err
	}
	p, err := ParseCNPJWS(corpo)
	if err != nil {
		return nil, nil, falha(FalhaExterna, "CNPJ.ws devolveu uma resposta inesperada")
	}
	return p, corpo, nil
}

// ParseCNPJWS converte a resposta de publica.cnpj.ws em Perfil.
func ParseCNPJWS(corpo []byte) (*Perfil, error) {
	m, err := decodificar(corpo)
	if err != nil {
		return nil, err
	}
	est := m.sub("estabelecimento")
	if m.str("razao_social") == "" || est == nil {
		return nil, fmt.Errorf("resposta sem dados")
	}
	p := NovoPerfil(models.ProvedorReceita)
	p.Set("legal_name", m.str("razao_social"))
	p.Set("trade_name", est.str("nome_fantasia"))
	p.Set("registration_status", strings.ToUpper(est.str("situacao_cadastral")))
	p.Set("registration_status_at", NormalizarData(est.str("data_situacao_cadastral")))
	p.Set("founded_at", NormalizarData(est.str("data_inicio_atividade")))
	if nj := m.sub("natureza_juridica"); nj != nil {
		p.Set("legal_nature", nj.str("descricao"))
	}
	if porte := m.sub("porte"); porte != nil {
		p.Set("company_size", NormalizarPorte(porte.str("descricao")))
	}
	if cap, ok := m.num("capital_social"); ok {
		p.Set("capital", cap)
	}
	if simples := m.sub("simples"); simples != nil {
		if v, ok := simples.boolSim("simples"); ok {
			p.Campos["simples"] = v
		}
		if v, ok := simples.boolSim("mei"); ok {
			p.Campos["mei"] = v
		}
	}
	if ap := est.sub("atividade_principal"); ap != nil {
		p.Set("cnae_code", FormatarCNAE(ap.str("subclasse", "id")))
		p.Set("cnae_description", ap.str("descricao"))
	}
	for _, c := range est.lista("atividades_secundarias") {
		cm := comoMapa(c)
		if cm == nil {
			continue
		}
		p.CNAEsSecundarios = append(p.CNAEsSecundarios, CNAE{Codigo: FormatarCNAE(cm.str("subclasse", "id")), Descricao: cm.str("descricao")})
	}
	p.Set("street", tituloEndereco(strings.TrimSpace(est.str("tipo_logradouro")+" "+est.str("logradouro"))))
	p.Set("street_number", est.str("numero"))
	p.Set("complement", est.str("complemento"))
	p.Set("neighborhood", tituloEndereco(est.str("bairro")))
	p.Set("zip", SomenteDigitos(est.str("cep")))
	if cid := est.sub("cidade"); cid != nil {
		p.Set("city", tituloEndereco(cid.str("nome")))
	}
	if uf := est.sub("estado"); uf != nil {
		p.Set("state", strings.ToUpper(uf.str("sigla")))
	}
	p.Set("phone", FormatarTelefone(est.str("ddd1")+est.str("telefone1")))
	p.Set("email", strings.ToLower(est.str("email")))
	for _, s := range m.lista("socios") {
		sm := comoMapa(s)
		if sm == nil {
			continue
		}
		qual := ""
		if q := sm.sub("qualificacao_socio"); q != nil {
			qual = q.str("descricao")
		} else {
			qual = sm.str("qualificacao_socio")
		}
		p.Socios = append(p.Socios, Socio{
			Nome:         sm.str("nome"),
			Documento:    sm.str("cpf_cnpj_socio"),
			Qualificacao: qual,
			Desde:        NormalizarData(sm.str("data_entrada")),
			Tipo:         tipoSocio(sm.str("tipo")),
		})
	}
	completarDerivados(p)
	return p, nil
}

// -----------------------------------------------------------------------------
// CEP: BrasilAPI v2 (com lat/long) → ViaCEP
// -----------------------------------------------------------------------------

// completarPorCEP preenche logradouro, bairro, cidade e UF que faltam e a
// localização. Falha é silenciosa: o CEP é só complemento.
func completarPorCEP(ctx context.Context, p *Perfil) {
	cep := SomenteDigitos(p.Texto("zip"))
	if len(cep) != 8 {
		return
	}
	ctx, cancelar := context.WithTimeout(ctx, 4*time.Second)
	defer cancelar()
	end, ok := ConsultarCEP(ctx, cep)
	if !ok {
		return
	}
	preencher := func(campo, valor string) {
		if p.Texto(campo) == "" && valor != "" {
			p.Set(campo, valor)
		}
	}
	preencher("street", end.Logradouro)
	preencher("neighborhood", end.Bairro)
	preencher("city", end.Cidade)
	preencher("state", end.UF)
	if end.Lat != 0 || end.Lng != 0 {
		p.Localizacao = &Localizacao{Lat: end.Lat, Lng: end.Lng}
	}
}

// Endereco é o resultado normalizado de uma consulta de CEP.
type Endereco struct {
	CEP, Logradouro, Bairro, Cidade, UF string
	Lat, Lng                            float64
}

// ConsultarCEP tenta BrasilAPI v2 e cai para ViaCEP.
func ConsultarCEP(ctx context.Context, cep string) (Endereco, bool) {
	cep = SomenteDigitos(cep)
	if len(cep) != 8 {
		return Endereco{}, false
	}
	if status, corpo, err := requisitar(ctx, http.MethodGet, BaseBrasilAPI+"/api/cep/v2/"+cep, nil, nil); err == nil && status == http.StatusOK {
		if m, err := decodificar(corpo); err == nil {
			e := Endereco{CEP: cep, Logradouro: m.str("street"), Bairro: m.str("neighborhood"), Cidade: m.str("city"), UF: strings.ToUpper(m.str("state"))}
			if loc := m.sub("location"); loc != nil {
				if coord := loc.sub("coordinates"); coord != nil {
					e.Lat, _ = coord.num("latitude")
					e.Lng, _ = coord.num("longitude")
				}
			}
			if e.Cidade != "" {
				return e, true
			}
		}
	}
	if ctx.Err() != nil {
		return Endereco{}, false
	}
	status, corpo, err := requisitar(ctx, http.MethodGet, BaseViaCEP+"/ws/"+cep+"/json/", nil, nil)
	if err != nil || status != http.StatusOK {
		return Endereco{}, false
	}
	m, err := decodificar(corpo)
	if err != nil {
		return Endereco{}, false
	}
	if erro, _ := m.boolSim("erro"); erro {
		return Endereco{}, false
	}
	e := Endereco{CEP: cep, Logradouro: m.str("logradouro"), Bairro: m.str("bairro"), Cidade: m.str("localidade"), UF: strings.ToUpper(m.str("uf"))}
	return e, e.Cidade != ""
}
