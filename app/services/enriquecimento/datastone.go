package enriquecimento

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
)

// -----------------------------------------------------------------------------
// Leitura tolerante do payload do Who Lead (cnpj_data do lote e data da
// consulta síncrona). Esses objetos não têm esquema publicado
// (docs/PROSPECCAO.md, seção 7).
//
// HIPÓTESE (não confirmada): o Who Lead repassa o item da Data Stone. O leitor
// entende os dois formatos documentados pela Data Stone e, fora deles, procura
// um objeto com chaves conhecidas em qualquer nível. Formato desconhecido não
// é erro: devolve perfil nil, o bruto continua guardado (com CPF mascarado) e
// só este arquivo muda quando chegar um exemplo real.
// -----------------------------------------------------------------------------

const (
	FormatoDataStoneConsulta = "datastone_consulta" // GET /companies/ (chaves em inglês)
	FormatoDataStoneB2B      = "datastone_b2b"      // detalhes_contato (chaves em português)
	FormatoDesconhecido      = "desconhecido"
)

// Papéis de pessoa no PerfilContato.
const (
	PapelSocio         = "socio"
	PapelRepresentante = "representante"
	PapelFuncionario   = "funcionario"
)

// chavesConsulta identificam o formato "consulta" da Data Stone.
var chavesConsulta = []string{"company_name", "trading_name", "mobile_phones", "land_lines", "partners",
	"registry_situation", "legal_representative", "related_company_members"}

// DetectarFormatoCnpjData decide o formato pelas chaves presentes.
func DetectarFormatoCnpjData(m mapa) string {
	if m == nil {
		return FormatoDesconhecido
	}
	for _, k := range chavesConsulta {
		if _, ok := m[k]; ok {
			return FormatoDataStoneConsulta
		}
	}
	if m.sub("cadastro") != nil || m.sub("contatos") != nil {
		return FormatoDataStoneB2B
	}
	return FormatoDesconhecido
}

// acharObjetoConhecido procura, em largura e até 4 níveis, o primeiro objeto
// de formato reconhecido (o payload pode vir dentro de um envelope:
// {"result": {...}}, {"empresa": {...}}, [{...}]).
func acharObjetoConhecido(v any) (mapa, string) {
	nivel := []any{v}
	for profundidade := 0; profundidade < 5 && len(nivel) > 0; profundidade++ {
		proximo := []any{}
		for _, it := range nivel {
			switch t := it.(type) {
			case map[string]any:
				m := mapa(t)
				if f := DetectarFormatoCnpjData(m); f != FormatoDesconhecido {
					return m, f
				}
				chaves := make([]string, 0, len(t))
				for k := range t {
					chaves = append(chaves, k)
				}
				sort.Strings(chaves) // ordem estável
				for _, k := range chaves {
					switch t[k].(type) {
					case map[string]any, []any:
						proximo = append(proximo, t[k])
					}
				}
			case []any:
				proximo = append(proximo, t...)
			}
		}
		nivel = proximo
	}
	return nil, FormatoDesconhecido
}

// ParseCnpjData converte o payload bruto em Perfil. Aceita objeto ou lista (a
// Data Stone devolve lista; vale o primeiro objeto reconhecido). Payload
// vazio, nulo ou de formato desconhecido devolve perfil nil e erro nil; só
// JSON inválido é erro.
func ParseCnpjData(corpo []byte, provedor, cnpj string) (*Perfil, string, error) {
	if len(strings.TrimSpace(string(corpo))) == 0 {
		return nil, FormatoDesconhecido, nil
	}
	var bruto any
	if err := json.Unmarshal(corpo, &bruto); err != nil {
		return nil, FormatoDesconhecido, falha(FalhaExterna, "a fonte devolveu um JSON inválido")
	}
	m, formato := acharObjetoConhecido(bruto)
	var p *Perfil
	switch formato {
	case FormatoDataStoneConsulta:
		p = lerDataStoneConsulta(m, provedor)
	case FormatoDataStoneB2B:
		p = lerDataStoneB2B(m, provedor)
	default:
		return nil, formato, nil
	}
	if c := NormalizarCNPJ(cnpj); ValidarCNPJ(c) {
		p.Set("cnpj", FormatarCNPJ(c))
	}
	completarDerivados(p)
	derivarSite(p)
	p.Sinais = CalcularSinais(p)
	return p, formato, nil
}

// zerosEsquerda repõe zeros perdidos quando o documento veio como número.
func zerosEsquerda(s string, tamanho int) string {
	d := SomenteDigitos(s)
	if d == "" || len(d) >= tamanho {
		return d
	}
	return strings.Repeat("0", tamanho-len(d)) + d
}

// MascararCPF deixa o CPF como a Receita publica no QSA (***456789**),
// repondo antes os zeros à esquerda perdidos quando o CPF veio como número.
// Qualquer outra entrada vira só asteriscos (nunca devolve o documento cru).
func MascararCPF(cpf string) string {
	if strings.Contains(cpf, "*") {
		return strings.TrimSpace(cpf) // já mascarado
	}
	d := SomenteDigitos(cpf)
	if d == "" {
		return ""
	}
	if len(d) < 11 {
		d = zerosEsquerda(d, 11)
	}
	return models.MascararCPF(d)
}

func prioridade(m mapa) int {
	if v, ok := m.num("priority", "prioridade"); ok && v > 0 {
		return int(v)
	}
	return 1 << 20
}

func ordenarPorPrioridade(itens []any) []mapa {
	out := []mapa{}
	for _, it := range itens {
		if im := comoMapa(it); im != nil {
			out = append(out, im)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return prioridade(out[i]) < prioridade(out[j]) })
	return out
}

func ponteiroBool(m mapa, chave string) *bool {
	if v, ok := m.boolSim(chave); ok {
		return &v
	}
	return nil
}

// lerTelefonesDS lê itens {ddd, number|numero, priority|prioridade, whatsapp,
// active_nao_me_ligue, hot_datetime, cpc_datetime}. ddd vem número no formato
// consulta e texto no B2B.
func lerTelefonesDS(itens []any, tipo string) []Telefone {
	out := []Telefone{}
	for _, im := range ordenarPorPrioridade(itens) {
		num := SomenteDigitos(im.str("ddd") + im.str("number", "numero"))
		if len(num) < 10 {
			continue
		}
		t := Telefone{
			Numero:      "+55" + num,
			Tipo:        tipo,
			Principal:   prioridade(im) == 1,
			WhatsApp:    ponteiroBool(im, "whatsapp"),
			NaoPerturbe: ponteiroBool(im, "active_nao_me_ligue"),
		}
		t.Validado = im.str("hot_datetime") != "" || im.str("cpc_datetime") != "" || (t.WhatsApp != nil && *t.WhatsApp)
		out = append(out, t)
	}
	return out
}

func tipoEmail(email string) string {
	if DominioDoEmail(email) != "" {
		return "corporativo"
	}
	return "pessoal"
}

// preencherContatos grava a seção e escolhe phone/email da ficha: o fixo
// principal (senão o celular principal), pulando número bloqueado quando há
// outro, e o primeiro e-mail.
func preencherContatos(p *Perfil, fixos, celulares []Telefone, emails []EmailContato) {
	tels := append(append([]Telefone{}, celulares...), fixos...)
	if len(tels) == 0 && len(emails) == 0 {
		return
	}
	p.Contatos = &Contatos{Telefones: tels, Emails: emails}
	bloqueado := func(t Telefone) bool { return t.NaoPerturbe != nil && *t.NaoPerturbe }
	for _, permitir := range []bool{false, true} {
		for _, t := range append(append([]Telefone{}, fixos...), celulares...) {
			if p.Texto("phone") == "" && (permitir || !bloqueado(t)) {
				p.Set("phone", FormatarTelefone(t.Numero))
			}
		}
	}
	if len(emails) > 0 {
		p.Set("email", emails[0].Email)
	}
}

// chavePessoa compara nomes sem caixa, sem acento e sem espaços repetidos.
func chavePessoa(nome string) string {
	r := strings.NewReplacer("Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A", "É", "E", "Ê", "E", "È", "E",
		"Í", "I", "Ì", "I", "Ó", "O", "Ô", "O", "Õ", "O", "Ò", "O", "Ú", "U", "Ü", "U", "Ù", "U", "Ç", "C")
	return strings.Join(strings.Fields(r.Replace(strings.ToUpper(nome))), " ")
}

// juntarPessoa acrescenta a pessoa ou completa a que já existe com o mesmo
// nome (papel mais forte vence: sócio > representante > funcionário).
func juntarPessoa(lista []models.PessoaContato, nova models.PessoaContato) []models.PessoaContato {
	nova.Nome = strings.TrimSpace(nova.Nome)
	if nova.Nome == "" {
		return lista
	}
	if nova.Telefones == nil {
		nova.Telefones = []models.ProspTelefone{}
	}
	if nova.Emails == nil {
		nova.Emails = []string{}
	}
	forca := map[string]int{PapelSocio: 3, PapelRepresentante: 2, PapelFuncionario: 1}
	chave := chavePessoa(nova.Nome)
	for i := range lista {
		a := &lista[i]
		if chavePessoa(a.Nome) != chave {
			continue
		}
		if forca[nova.Papel] > forca[a.Papel] {
			a.Papel = nova.Papel
		}
		if a.Cargo == "" {
			a.Cargo = nova.Cargo
		}
		if a.Documento == "" {
			a.Documento = nova.Documento
		}
		vistos := map[string]bool{}
		for _, t := range a.Telefones {
			vistos[t.Numero] = true
		}
		for _, t := range nova.Telefones {
			if !vistos[t.Numero] {
				vistos[t.Numero] = true
				a.Telefones = append(a.Telefones, t)
			}
		}
		emails := map[string]bool{}
		for _, e := range a.Emails {
			emails[e] = true
		}
		for _, e := range nova.Emails {
			if !emails[e] {
				emails[e] = true
				a.Emails = append(a.Emails, e)
			}
		}
		return lista
	}
	return append(lista, nova)
}

// cargoSemCBO tira o código CBO do começo ("354125 - ASSISTENTE DE VENDAS").
func cargoSemCBO(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, " - "); i > 0 && SomenteDigitos(s[:i]) == strings.TrimSpace(s[:i]) {
		s = strings.TrimSpace(s[i+3:])
	}
	return tituloEndereco(s)
}

// lerPessoasDS lê sócios, responsável legal e funcionários do formato
// consulta. O CPF sai sempre mascarado.
func lerPessoasDS(m mapa) []models.PessoaContato {
	pessoas := []models.PessoaContato{}
	for _, s := range m.lista("partners") {
		sm := comoMapa(s)
		if sm == nil || sm.str("cnpj") != "" {
			continue // sócio PJ fica só em Socios
		}
		pessoas = juntarPessoa(pessoas, models.PessoaContato{
			Nome: sm.str("name"), Documento: MascararCPF(sm.str("cpf")), Papel: PapelSocio, Cargo: sm.str("qualification"),
		})
	}
	for _, r := range m.lista("legal_representative") {
		rm := comoMapa(r)
		if rm == nil {
			continue
		}
		pessoas = juntarPessoa(pessoas, models.PessoaContato{
			Nome: rm.str("name", "nome"), Documento: MascararCPF(rm.str("cpf")), Papel: PapelRepresentante, Cargo: "Responsável legal",
		})
	}
	for _, f := range m.lista("related_company_members") {
		fm := comoMapa(f)
		if fm == nil || strings.Contains(strings.ToLower(fm.str("position_status")), "hist") {
			continue // vínculo antigo não é contato atual
		}
		pessoas = juntarPessoa(pessoas, models.PessoaContato{
			Nome: fm.str("name", "nome"), Documento: MascararCPF(fm.str("cpf")), Papel: PapelFuncionario, Cargo: cargoSemCBO(fm.str("position")),
		})
	}
	return pessoas
}

// lerDataStoneConsulta mapeia o item de GET /companies/.
func lerDataStoneConsulta(m mapa, provedor string) *Perfil {
	p := NovoPerfil(provedor)
	p.Set("legal_name", m.str("company_name"))
	p.Set("trade_name", m.str("trading_name"))
	p.Set("registration_status", strings.ToUpper(m.str("registry_situation")))
	p.Set("founded_at", NormalizarData(m.str("creation_date")))
	p.Set("cnae_code", FormatarCNAE(zerosEsquerda(m.str("cnae_code"), 7)))
	p.Set("cnae_description", m.str("cnae_description"))
	p.Set("legal_nature", m.str("juridical_type"))
	p.Set("company_size", NormalizarPorte(m.str("business_size")))
	p.Set("industry", m.str("segment"))
	if capital, ok := m.num("share_capital"); ok {
		p.Set("capital", capital)
	}
	// Faixas vão só para os campos: Indicadores preenchido sem os dados de
	// domínio faria CalcularSinais dizer "Sem presença digital identificada".
	p.Set("employees_range", m.str("employee_count"))
	p.Set("revenue_range", m.str("estimated_revenue"))

	if ends := ordenarPorPrioridade(m.lista("addresses")); len(ends) > 0 {
		a := ends[0]
		p.Set("street", tituloEndereco(strings.TrimSpace(a.str("type")+" "+a.str("street"))))
		p.Set("street_number", a.str("number"))
		p.Set("complement", a.str("complement"))
		p.Set("neighborhood", tituloEndereco(a.str("neighborhood")))
		p.Set("city", tituloEndereco(a.str("city")))
		p.Set("state", strings.ToUpper(a.str("district")))
		p.Set("zip", SomenteDigitos(a.str("postal_code")))
	}
	if partes := strings.Split(m.str("city_uf"), ","); len(partes) == 2 {
		if p.Texto("city") == "" {
			p.Set("city", tituloEndereco(strings.TrimSpace(partes[0])))
		}
		if p.Texto("state") == "" {
			p.Set("state", strings.ToUpper(strings.TrimSpace(partes[1])))
		}
	}

	emails := []EmailContato{}
	for _, em := range ordenarPorPrioridade(m.lista("emails")) {
		if e := strings.ToLower(em.str("email")); e != "" {
			emails = append(emails, EmailContato{Email: e, Tipo: tipoEmail(e)})
		}
	}
	preencherContatos(p, lerTelefonesDS(m.lista("land_lines"), "fixo"), lerTelefonesDS(m.lista("mobile_phones"), "celular"), emails)

	for _, s := range m.lista("partners") {
		sm := comoMapa(s)
		if sm == nil {
			continue
		}
		socio := Socio{Nome: sm.str("name"), Qualificacao: sm.str("qualification"), Tipo: "PF"}
		if cnpj := sm.str("cnpj"); cnpj != "" {
			socio.Tipo, socio.Documento = "PJ", FormatarCNPJ(zerosEsquerda(cnpj, 14))
		} else if cpf := sm.str("cpf"); cpf != "" {
			socio.Documento = MascararCPF(cpf)
		}
		p.Socios = append(p.Socios, socio)
	}
	if pessoas := lerPessoasDS(m); len(pessoas) > 0 {
		p.Pessoas = pessoas
	}
	return p
}

// lerDataStoneB2B mapeia `detalhes_contato` do enriquecimento B2B.
func lerDataStoneB2B(m mapa, provedor string) *Perfil {
	p := NovoPerfil(provedor)
	if c := m.sub("cadastro"); c != nil {
		p.Set("legal_name", c.str("razao_social"))
		p.Set("trade_name", c.str("nome_fantasia"))
		p.Set("registration_status", strings.ToUpper(c.str("situacao_cadastral")))
		p.Set("founded_at", NormalizarData(c.str("data_fundacao")))
		p.Set("cnae_code", FormatarCNAE(zerosEsquerda(c.str("codigo_cnae_principal"), 7)))
		p.Set("cnae_description", c.str("descricao_cnae_principal"))
		p.Set("legal_nature", c.str("descricao_natureza_juridica"))
		p.Set("company_size", NormalizarPorte(c.str("porte_empresa")))
		p.Set("revenue_range", c.str("faturamento_estimado"))
		if capital, ok := c.num("capital_social"); ok {
			p.Set("capital", capital)
		}
		for _, s := range c.lista("cnaes_secundarios") {
			if sm := comoMapa(s); sm != nil {
				p.CNAEsSecundarios = append(p.CNAEsSecundarios, CNAE{
					Codigo: FormatarCNAE(zerosEsquerda(sm.str("id_cnae_sec"), 7)), Descricao: sm.str("ds_cnae_sec"),
				})
			}
		}
	}
	if s := m.sub("sobre"); s != nil {
		p.Set("industry", s.str("setor"))
		p.Set("employees_range", s.str("faixa_funcionarios"))
		p.Set("website", s.str("url_website"))
		if lk := s.str("url_perfil_linkedin"); lk != "" {
			p.PresencaDigital = &PresencaDigital{Dominios: []Dominio{}, Redes: map[string]string{"linkedin": lk}}
		}
	}
	if l := m.sub("localizacao"); l != nil {
		// `rua` pode vir com o número no fim ("Presidente Kennedy 4121").
		rua := strings.TrimSpace(strings.TrimSuffix(l.str("rua"), " "+l.str("numero")))
		p.Set("street", tituloEndereco(strings.TrimSpace(l.str("tipo")+" "+rua)))
		p.Set("street_number", l.str("numero"))
		p.Set("complement", l.str("complemento"))
		p.Set("neighborhood", tituloEndereco(l.str("bairro")))
		p.Set("city", tituloEndereco(l.str("cidade")))
		p.Set("state", strings.ToUpper(l.str("distrito")))
		p.Set("zip", SomenteDigitos(l.str("cep")))
		lat, okLat := l.num("latitude")
		lng, okLng := l.num("longitude")
		if okLat && okLng && (lat != 0 || lng != 0) {
			p.Localizacao = &Localizacao{Lat: lat, Lng: lng}
		}
	}
	if c := m.sub("contatos"); c != nil {
		emails := []EmailContato{}
		for _, em := range ordenarPorPrioridade(c.lista("emails")) {
			if e := strings.ToLower(em.str("email")); e != "" {
				emails = append(emails, EmailContato{Email: e, Tipo: tipoEmail(e), Validado: em.str("status_email") == "Válido"})
			}
		}
		var fixos, celulares []Telefone
		if t := c.sub("telefones"); t != nil {
			fixos, celulares = lerTelefonesDS(t.lista("fixo"), "fixo"), lerTelefonesDS(t.lista("celular"), "celular")
		}
		preencherContatos(p, fixos, celulares, emails)
	}
	if s := m.sub("socios"); s != nil {
		pessoas := []models.PessoaContato{}
		for _, it := range s.lista("itens") {
			sm := comoMapa(it)
			if sm == nil {
				continue
			}
			socio := Socio{Nome: sm.str("nome"), Qualificacao: sm.str("cargo"), Tipo: "PF"}
			if cnpj := sm.str("cnpj"); cnpj != "" {
				socio.Tipo, socio.Documento = "PJ", FormatarCNPJ(zerosEsquerda(cnpj, 14))
			} else {
				if cpf := sm.str("cpf"); cpf != "" {
					socio.Documento = MascararCPF(cpf)
				}
				pessoas = juntarPessoa(pessoas, models.PessoaContato{
					Nome: socio.Nome, Documento: socio.Documento, Papel: PapelSocio, Cargo: socio.Qualificacao,
				})
			}
			p.Socios = append(p.Socios, socio)
		}
		if len(pessoas) > 0 {
			p.Pessoas = pessoas
		}
	}
	return p
}
