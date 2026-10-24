package enriquecimento

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// BaseBigDataCorp é o endereço da plataforma (os testes apontam para httptest).
var BaseBigDataCorp = "https://plataforma.bigdatacorp.com.br"

// BigDataCorp consulta POST /empresas com os datasets pedidos.
type BigDataCorp struct {
	TokenID        string
	AccessToken    string
	BaseURL        string
	DatasetsPadrao []string
	Precos         map[string]int
}

func (b *BigDataCorp) Codigo() string { return models.ProvedorBigDataCorp }

func (b *BigDataCorp) base() string {
	if b.BaseURL != "" {
		return strings.TrimRight(b.BaseURL, "/")
	}
	return BaseBigDataCorp
}

// Custo estima o preço dos datasets pela tabela configurada.
func (b *BigDataCorp) Custo(datasets []string) int {
	total := 0
	for _, d := range datasets {
		total += b.Precos[d]
	}
	return total
}

// datasetsOuPadrao normaliza a lista pedida.
func (b *BigDataCorp) datasetsOuPadrao(pedidos []string) []string {
	out := []string{}
	visto := map[string]bool{}
	for _, d := range pedidos {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" && models.BigDataCorpDatasetValido(d) && !visto[d] {
			visto[d] = true
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		out = append(out, b.DatasetsPadrao...)
	}
	if len(out) == 0 {
		out = append(out, models.BigDataCorpDatasetsPadrao...)
	}
	return out
}

func (b *BigDataCorp) Consultar(ctx context.Context, cnpj string, opts Opcoes) (*Perfil, *Bruto, error) {
	digits := SomenteDigitos(cnpj)
	if !ValidarCNPJ(digits) {
		return nil, nil, falha(FalhaEntrada, "CNPJ inválido")
	}
	datasets := b.datasetsOuPadrao(opts.Datasets)
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancelar := context.WithTimeout(ctx, timeout)
	defer cancelar()

	corpoReq, _ := json.Marshal(map[string]any{
		"q": "doc{" + digits + "}", "Datasets": strings.Join(datasets, ","), "Limit": 1,
	})
	status, corpo, err := requisitar(ctx, http.MethodPost, b.base()+"/empresas", bytes.NewReader(corpoReq), map[string]string{
		"Content-Type": "application/json",
		"AccessToken":  b.AccessToken,
		"TokenId":      b.TokenID,
	})
	if err != nil {
		return nil, nil, falha(FalhaExterna, "não foi possível falar com o BigDataCorp (%s)", MensagemDaFalha(err))
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return nil, nil, falha(FalhaCredencial, "o BigDataCorp recusou as credenciais (TokenId/AccessToken). Confira em Configurações → Integrações.")
	case status == http.StatusTooManyRequests:
		return nil, nil, falha(FalhaLimite, "limite de consultas do BigDataCorp atingido; tente novamente em alguns minutos")
	case status >= 500:
		return nil, nil, falha(FalhaExterna, "BigDataCorp indisponível (HTTP %d)", status)
	case status != http.StatusOK:
		return nil, nil, falha(FalhaExterna, "BigDataCorp respondeu HTTP %d", status)
	}
	p, err := ParseBigDataCorp(corpo, digits, datasets)
	if err != nil {
		return nil, nil, err
	}
	p.CustoCentavos = b.Custo(datasets)
	p.Sinais = CalcularSinais(p)
	return p, &Bruto{Fonte: models.ProvedorBigDataCorp, Corpo: json.RawMessage(corpo)}, nil
}

// statusBDC traduz os códigos de status da plataforma.
func statusBDC(code int) (tipo, msg string) {
	switch code {
	case 0, 1:
		return "ok", ""
	case -204:
		return FalhaSemResultado, "nenhum resultado para o CNPJ"
	case -111, -112, -1003:
		return FalhaCredencial, "credenciais recusadas (TokenId/AccessToken)"
	case -1214, 429:
		return FalhaLimite, "limite de consultas atingido"
	case -109, -110, -125:
		return "dataset", "conjunto de dados indisponível no contrato ou nome inválido"
	}
	return "erro", fmt.Sprintf("erro %d na plataforma", code)
}

// ParseBigDataCorp converte a resposta de /empresas em Perfil. Avalia os
// Status por dataset antes de ler o Result.
func ParseBigDataCorp(corpo []byte, cnpj string, datasets []string) (*Perfil, error) {
	m, err := decodificar(corpo)
	if err != nil {
		return nil, falha(FalhaExterna, "BigDataCorp devolveu uma resposta inesperada")
	}
	// Status: {"basic_data":[{"Code":0,"Message":"OK"}], ...} ou lista plana.
	avisos := []string{}
	semResultado := 0
	totalStatus := 0
	avaliar := func(dataset string, item mapa) error {
		code, _ := item.num("Code", "code")
		totalStatus++
		tipo, msg := statusBDC(int(code))
		switch tipo {
		case "ok":
		case FalhaSemResultado:
			semResultado++
		case FalhaCredencial:
			return falha(FalhaCredencial, "o BigDataCorp recusou as credenciais (TokenId/AccessToken). Confira em Configurações → Integrações.")
		case FalhaLimite:
			return falha(FalhaLimite, "limite de consultas do BigDataCorp atingido; tente novamente em alguns minutos")
		default:
			if dataset != "" {
				msg = dataset + ": " + msg
			}
			if texto := item.str("Message", "message"); texto != "" && texto != "OK" {
				msg += " (" + texto + ")"
			}
			avisos = append(avisos, msg)
		}
		return nil
	}
	switch st := m["Status"].(type) {
	case map[string]any:
		chaves := make([]string, 0, len(st))
		for k := range st {
			chaves = append(chaves, k)
		}
		sort.Strings(chaves)
		for _, k := range chaves {
			switch v := st[k].(type) {
			case []any:
				for _, it := range v {
					if im := comoMapa(it); im != nil {
						if err := avaliar(k, im); err != nil {
							return nil, err
						}
					}
				}
			case map[string]any:
				if err := avaliar(k, mapa(v)); err != nil {
					return nil, err
				}
			}
		}
	case []any:
		for _, it := range st {
			if im := comoMapa(it); im != nil {
				if err := avaliar(im.str("Dataset", "dataset"), im); err != nil {
					return nil, err
				}
			}
		}
	}

	resultados := m.lista("Result")
	if len(resultados) == 0 {
		if totalStatus > 0 && semResultado == totalStatus {
			return nil, falha(FalhaSemResultado, "o BigDataCorp não encontrou dados para este CNPJ")
		}
		if len(avisos) > 0 {
			return nil, falha(FalhaExterna, "BigDataCorp: %s", strings.Join(avisos, "; "))
		}
		return nil, falha(FalhaSemResultado, "o BigDataCorp não encontrou dados para este CNPJ")
	}
	r := comoMapa(resultados[0])
	if r == nil {
		return nil, falha(FalhaExterna, "BigDataCorp devolveu uma resposta inesperada")
	}

	p := NovoPerfil(models.ProvedorBigDataCorp)
	if len(avisos) > 0 {
		p.Mensagem = strings.Join(avisos, "; ")
	}
	p.Set("cnpj", FormatarCNPJ(cnpj))
	if bd := r.sub("BasicData"); bd != nil {
		lerBasicData(p, bd)
	}
	if rd := r.sub("RegistrationData"); rd != nil {
		if bd := rd.sub("BasicData"); bd != nil {
			lerBasicData(p, bd)
		}
		lerRegistrationData(p, rd)
	}
	if ai := r.sub("ActivityIndicators"); ai != nil {
		lerActivityIndicators(p, ai)
	}
	if rel := r.sub("Relationships"); rel != nil {
		lerRelationships(p, rel)
	}
	lerContatosEstendidos(p, r)
	if dom := primeiroSub(r, "ExtendedDomains", "Domains"); dom != nil {
		lerDominios(p, dom)
	}
	if pr := primeiroSub(r, "Processes", "Lawsuits"); pr != nil {
		lerProcessos(p, pr, cnpj)
	}
	if kyc := primeiroSub(r, "KycData", "KYC", "Kyc"); kyc != nil {
		lerKYC(p, kyc)
	}
	if md := r.sub("MediaProfileAndExposure"); md != nil {
		lerMidia(p, md)
	}
	completarDerivados(p)
	return p, nil
}

func primeiroSub(m mapa, chaves ...string) mapa {
	for _, k := range chaves {
		if s := m.sub(k); s != nil {
			return s
		}
	}
	return nil
}

func lerBasicData(p *Perfil, bd mapa) {
	set := func(campo, valor string) {
		if p.Texto(campo) == "" {
			p.Set(campo, valor)
		}
	}
	set("legal_name", bd.str("OfficialName"))
	set("trade_name", bd.str("TradeName"))
	set("registration_status", strings.ToUpper(bd.str("TaxIdStatus")))
	set("registration_status_at", NormalizarData(bd.str("TaxIdStatusDate")))
	set("founded_at", NormalizarData(bd.str("FoundedDate")))
	if ln := bd.sub("LegalNature"); ln != nil {
		set("legal_nature", ln.str("Activity", "Description"))
	} else {
		set("legal_nature", bd.str("LegalNature"))
	}
	set("company_size", NormalizarPorte(bd.str("CompanySize", "Size")))
	if cap, ok := bd.num("CapitalStock", "Capital"); ok && p.Texto("capital") == "" {
		p.Set("capital", cap)
	}
	for _, a := range bd.lista("Activities") {
		am := comoMapa(a)
		if am == nil {
			continue
		}
		principal, _ := am.boolSim("IsMain")
		if principal && p.Texto("cnae_code") == "" {
			p.Set("cnae_code", FormatarCNAE(am.str("Code")))
			p.Set("cnae_description", am.str("Activity", "Description"))
		} else if !principal {
			p.CNAEsSecundarios = append(p.CNAEsSecundarios, CNAE{Codigo: FormatarCNAE(am.str("Code")), Descricao: am.str("Activity", "Description")})
		}
	}
	if tr := bd.sub("TaxRegime"); tr != nil {
		if v, ok := tr.boolSim("IsSimples", "Simples"); ok {
			p.Campos["simples"] = v
		}
		if v, ok := tr.boolSim("IsMEI", "MEI"); ok {
			p.Campos["mei"] = v
		}
	}
}

func lerEndereco(p *Perfil, a mapa) {
	set := func(campo, valor string) {
		if p.Texto(campo) == "" {
			p.Set(campo, valor)
		}
	}
	rua := strings.TrimSpace(a.str("Typology") + " " + a.str("AddressMain", "Street"))
	set("street", tituloEndereco(rua))
	set("street_number", a.str("Number"))
	set("complement", a.str("Complement"))
	set("neighborhood", tituloEndereco(a.str("Neighborhood")))
	set("zip", SomenteDigitos(a.str("ZipCode")))
	set("city", tituloEndereco(a.str("City")))
	set("state", strings.ToUpper(a.str("State")))
	if p.Localizacao == nil {
		lat, okLat := a.num("Latitude")
		lng, okLng := a.num("Longitude")
		if okLat && okLng && (lat != 0 || lng != 0) {
			p.Localizacao = &Localizacao{Lat: lat, Lng: lng}
		}
	}
}

func lerRegistrationData(p *Perfil, rd mapa) {
	if em := rd.sub("Emails"); em != nil {
		if pr := em.sub("Primary"); pr != nil && p.Texto("email") == "" {
			p.Set("email", strings.ToLower(pr.str("EmailAddress", "Email")))
		}
	}
	if ph := rd.sub("Phones"); ph != nil {
		if pr := ph.sub("Primary"); pr != nil && p.Texto("phone") == "" {
			p.Set("phone", FormatarTelefone(pr.str("AreaCode")+pr.str("Number")))
		}
	}
	if ad := rd.sub("Addresses"); ad != nil {
		if pr := ad.sub("Primary"); pr != nil {
			lerEndereco(p, pr)
		}
	}
}

func nivelAtividade(v any) float64 {
	switch t := v.(type) {
	case float64:
		if t > 1 {
			return t / 100
		}
		return t
	case string:
		switch strings.ToUpper(strings.TrimSpace(t)) {
		case "MUITO ALTA", "VERY HIGH":
			return 1
		case "ALTA", "HIGH":
			return 0.85
		case "MEDIA", "MÉDIA", "MEDIUM":
			return 0.6
		case "BAIXA", "LOW":
			return 0.3
		case "MUITO BAIXA", "VERY LOW", "INATIVA", "NENHUMA":
			return 0.1
		}
	}
	return 0
}

func lerActivityIndicators(p *Perfil, ai mapa) {
	ind := &Indicadores{
		FuncionariosFaixa: ai.str("EmployeesRange", "EmployeeRange"),
		FaturamentoFaixa:  ai.str("IncomeRange", "RevenueRange", "PresumedIncomeRange"),
	}
	if v, ok := ai["ActivityLevel"]; ok {
		ind.NivelAtividade = nivelAtividade(v)
	}
	ind.TemDominioAtivo, _ = ai.boolSim("HasActiveDomain", "HasDomain")
	ind.TemSSL, _ = ai.boolSim("HasActiveSSL", "HasSSL")
	ind.TemEmailCorporativo, _ = ai.boolSim("HasCorporateEmail")
	p.Indicadores = ind
	if ind.FuncionariosFaixa != "" {
		p.Set("employees_range", ind.FuncionariosFaixa)
	}
	if ind.FaturamentoFaixa != "" {
		p.Set("revenue_range", ind.FaturamentoFaixa)
	}
}

func lerRelationships(p *Perfil, rel mapa) {
	for _, chave := range []string{"CurrentRelationships", "Relationships"} {
		for _, it := range rel.lista(chave) {
			im := comoMapa(it)
			if im == nil {
				continue
			}
			tipoDoc := strings.ToUpper(im.str("RelatedEntityTaxIdType"))
			tipo := "PF"
			if tipoDoc == "CNPJ" {
				tipo = "PJ"
			}
			p.Socios = append(p.Socios, Socio{
				Nome:         im.str("RelatedEntityName"),
				Documento:    im.str("RelatedEntityTaxIdNumber"),
				Qualificacao: tituloEndereco(strings.ReplaceAll(im.str("RelationshipLevel", "RelationshipType"), "-", " ")),
				Desde:        NormalizarData(im.str("RelationshipStartDate")),
				Tipo:         tipo,
			})
		}
		if len(p.Socios) > 0 {
			break
		}
	}
}

func lerContatosEstendidos(p *Perfil, r mapa) {
	var c *Contatos
	garantir := func() *Contatos {
		if c == nil {
			c = &Contatos{Telefones: []Telefone{}, Emails: []EmailContato{}}
		}
		return c
	}
	if ph := r.sub("ExtendedPhones"); ph != nil {
		for _, it := range ph.lista("Phones") {
			im := comoMapa(it)
			if im == nil {
				continue
			}
			num := SomenteDigitos(im.str("CountryCode") + im.str("AreaCode") + im.str("Number"))
			if num == "" {
				continue
			}
			if !strings.HasPrefix(num, "55") {
				num = "55" + num
			}
			principal, _ := im.boolSim("IsMain")
			ativo, temAtivo := im.boolSim("IsActive")
			validado := temAtivo && ativo
			if !temAtivo {
				validado = strings.EqualFold(im.str("ValidationStatus"), "VALID")
			}
			garantir().Telefones = append(c.Telefones, Telefone{
				Numero: "+" + num, Tipo: strings.ToLower(im.str("Type")), Principal: principal, Validado: validado,
			})
		}
	}
	if em := r.sub("ExtendedEmails"); em != nil {
		for _, it := range em.lista("Emails") {
			im := comoMapa(it)
			if im == nil {
				continue
			}
			email := strings.ToLower(im.str("EmailAddress", "Email"))
			if email == "" {
				continue
			}
			tipo := strings.ToLower(im.str("Type"))
			if tipo == "" {
				if DominioDoEmail(email) != "" {
					tipo = "corporativo"
				} else {
					tipo = "pessoal"
				}
			}
			if tipo == "corporate" {
				tipo = "corporativo"
			}
			ativo, temAtivo := im.boolSim("IsActive")
			validado := (temAtivo && ativo) || strings.EqualFold(im.str("ValidationStatus"), "VALID")
			garantir().Emails = append(c.Emails, EmailContato{Email: email, Tipo: tipo, Validado: validado})
		}
	}
	if ad := r.sub("ExtendedAddresses"); ad != nil {
		for _, it := range ad.lista("Addresses") {
			if im := comoMapa(it); im != nil {
				lerEndereco(p, im)
				break
			}
		}
	}
	if c != nil {
		p.Contatos = c
		if p.Texto("phone") == "" {
			for _, t := range c.Telefones {
				if t.Principal || t.Validado {
					p.Set("phone", FormatarTelefone(t.Numero))
					break
				}
			}
		}
		if p.Texto("email") == "" {
			for _, e := range c.Emails {
				if e.Validado {
					p.Set("email", e.Email)
					break
				}
			}
		}
	}
}

func lerDominios(p *Perfil, dom mapa) {
	pd := &PresencaDigital{Dominios: []Dominio{}, Redes: map[string]string{}}
	for _, it := range dom.lista("Domains") {
		im := comoMapa(it)
		if im == nil {
			continue
		}
		nome := strings.ToLower(im.str("DomainName", "Domain", "Name"))
		if nome == "" {
			continue
		}
		ativo, _ := im.boolSim("IsActive", "Active")
		ssl, temSSL := im.boolSim("HasSSL", "SSL")
		if !temSSL {
			ssl = strings.EqualFold(im.str("SSLStatus"), "VALID") || strings.EqualFold(im.str("SSLStatus"), "ACTIVE")
		}
		pd.Dominios = append(pd.Dominios, Dominio{Dominio: nome, Ativo: ativo, SSL: ssl})
	}
	for _, chave := range []string{"SocialNetworks", "SocialMedia", "Profiles"} {
		for _, it := range dom.lista(chave) {
			im := comoMapa(it)
			if im == nil {
				continue
			}
			rede := strings.ToLower(im.str("Network", "Type", "Name"))
			url := im.str("Url", "URL", "Profile", "Handle")
			if rede != "" && url != "" {
				pd.Redes[rede] = url
			}
		}
	}
	p.PresencaDigital = pd
	if p.Texto("domain") == "" {
		for _, d := range pd.Dominios {
			if d.Ativo {
				p.Set("domain", d.Dominio)
				p.Set("website", "https://"+d.Dominio)
				break
			}
		}
	}
}

func lerProcessos(p *Perfil, pr mapa, cnpj string) {
	pp := &Processos{Ultimos: []Processo{}}
	if n, ok := pr.num("TotalLawsuits", "Total"); ok {
		pp.Total = int(n)
	}
	if n, ok := pr.num("TotalLawsuitsAsDefendant", "AsDefendant"); ok {
		pp.ComoReu = int(n)
	}
	ativos := 0
	for i, it := range pr.lista("Lawsuits") {
		im := comoMapa(it)
		if im == nil {
			continue
		}
		valor, _ := im.num("Value")
		pp.ValorTotal += valor
		status := im.str("Status")
		if strings.EqualFold(status, "ATIVO") || strings.EqualFold(status, "ACTIVE") || strings.EqualFold(status, "EM ANDAMENTO") {
			ativos++
			status = "Ativo"
		}
		polo := ""
		for _, pt := range im.lista("Parties") {
			pm := comoMapa(pt)
			if pm == nil {
				continue
			}
			if SomenteDigitos(pm.str("Doc")) == cnpj {
				switch strings.ToUpper(pm.str("Polarity")) {
				case "PASSIVE", "PASSIVO":
					polo = "Réu"
				case "ACTIVE", "ATIVO":
					polo = "Autor"
				}
				break
			}
		}
		if i < 10 {
			pp.Ultimos = append(pp.Ultimos, Processo{
				Numero:   im.str("Number"),
				Tipo:     im.str("Type", "MainSubject"),
				Tribunal: im.str("CourtName", "CourtType"),
				Status:   status,
				Polo:     polo,
				Valor:    valor,
				Data:     NormalizarData(im.str("PublicationDate", "NoticeDate", "LastMovementDate")),
			})
		}
	}
	if pp.Total == 0 {
		pp.Total = len(pr.lista("Lawsuits"))
	}
	pp.Ativos = ativos
	p.Processos = pp
}

func lerKYC(p *Perfil, kyc mapa) {
	c := &Conformidade{Listas: []string{}}
	c.PEP, _ = kyc.boolSim("IsCurrentlyPEP", "IsPEP", "PEP")
	c.Sancionado, _ = kyc.boolSim("IsCurrentlySanctioned", "IsSanctioned", "Sanctioned")
	if !c.PEP && len(kyc.lista("PEPHistory")) > 0 {
		c.PEP = true
	}
	for _, it := range kyc.lista("SanctionsHistory") {
		im := comoMapa(it)
		if im == nil {
			continue
		}
		if nome := im.str("Source", "StandardizedSanctionType", "Type"); nome != "" {
			c.Listas = append(c.Listas, nome)
		}
	}
	if len(c.Listas) > 0 {
		c.Sancionado = true
	}
	p.Conformidade = c
}

func lerMidia(p *Perfil, md mapa) {
	m := &Midia{Noticias: []Noticia{}}
	switch strings.ToUpper(md.str("MediaExposureLevel", "ExposureLevel")) {
	case "", "NONE", "NENHUMA", "SEM EXPOSICAO":
		m.Exposicao = "nenhuma"
	case "LOW", "BAIXA":
		m.Exposicao = "baixa"
	case "MEDIUM", "MEDIA", "MÉDIA":
		m.Exposicao = "media"
	case "HIGH", "ALTA":
		m.Exposicao = "alta"
	case "VERY HIGH", "MUITO ALTA":
		m.Exposicao = "muito alta"
	default:
		m.Exposicao = strings.ToLower(md.str("MediaExposureLevel", "ExposureLevel"))
	}
	for _, chave := range []string{"NewsItems", "RecentNews", "News"} {
		for _, it := range md.lista(chave) {
			im := comoMapa(it)
			if im == nil {
				continue
			}
			sent := strings.ToLower(im.str("Sentiment"))
			switch sent {
			case "positive":
				sent = "positivo"
			case "negative":
				sent = "negativo"
			case "", "neutral":
				sent = "neutro"
			}
			m.Noticias = append(m.Noticias, Noticia{
				Titulo: im.str("Title"), URL: im.str("Url", "URL"), Sentimento: sent,
				Data: NormalizarData(im.str("PublicationDate", "Date")),
			})
			if len(m.Noticias) >= 10 {
				break
			}
		}
		if len(m.Noticias) > 0 {
			break
		}
	}
	p.Midia = m
}
