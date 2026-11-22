package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/services/enriquecimento"
)

// -----------------------------------------------------------------------------
// Envio ao CRM: a empresa com todo o cadastro da base e do enriquecimento, a
// inteligência (intel), as fontes por campo e os contatos decisores. Funções
// chamadas pelo job enviar_crm; nenhuma dispara automação.
// -----------------------------------------------------------------------------

// contatosDoItem são os telefones e e-mails do item já sem os bloqueados.
type contatosDoItem struct {
	telefones []models.ProspTelefone // ordenados do melhor para o pior
	emails    []string
	bloq      models.BloqueiosDoContato
}

func (c contatosDoItem) telefoneLivre(numero string) bool {
	n := models.NormalizarTelefone(numero)
	return n != "" && !c.bloq.Telefones[n]
}

func (c contatosDoItem) emailLivre(email string) bool {
	e := strings.ToLower(strings.TrimSpace(email))
	return e != "" && !c.bloq.Emails[e]
}

// todosEmailsDoItem junta o e-mail do cadastro e os do enriquecimento (da
// empresa e das pessoas), minúsculos e sem repetir.
func todosEmailsDoItem(dados *models.ProspEmpresa, perfil *models.PerfilContato) []string {
	out := []string{}
	add := func(e string) {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" || !strings.Contains(e, "@") {
			return
		}
		for _, x := range out {
			if x == e {
				return
			}
		}
		out = append(out, e)
	}
	if perfil != nil {
		for _, e := range perfil.Emails {
			add(e.Email)
		}
		for _, p := range perfil.Pessoas {
			for _, e := range p.Emails {
				add(e)
			}
		}
	}
	if dados != nil {
		add(dados.Email)
	}
	return out
}

// conferirContatosDoItem confere os bloqueios do item (adendo, 8.3): devolve
// os contatos livres e se o item inteiro fica bloqueado (CNPJ bloqueado, ou
// todos os telefones bloqueados e nenhum e-mail livre).
func conferirContatosDoItem(db models.Executor, it *models.ProspItem) (contatosDoItem, bool, string, error) {
	todos := TelefonesDoItem(it.Dados, it.Enriquecimento)
	emails := todosEmailsDoItem(it.Dados, it.Enriquecimento)
	numeros := make([]string, 0, len(todos))
	for _, t := range todos {
		numeros = append(numeros, t.Numero)
	}
	bloq, err := models.ConferirBloqueios(db, it.CNPJ, numeros, emails)
	if err != nil {
		return contatosDoItem{}, false, "", err
	}
	c := contatosDoItem{telefones: []models.ProspTelefone{}, emails: []string{}, bloq: bloq}
	for _, t := range todos {
		if !bloq.Telefones[t.Numero] {
			t.Principal = false
			c.telefones = append(c.telefones, t)
		}
	}
	if len(c.telefones) > 0 {
		c.telefones[0].Principal = true
	}
	for _, e := range emails {
		if !bloq.Emails[e] {
			c.emails = append(c.emails, e)
		}
	}
	switch {
	case bloq.CNPJ:
		return c, true, "CNPJ na lista de bloqueio", nil
	case len(todos) > 0 && len(c.telefones) == 0 && len(c.emails) == 0:
		return c, true, "todos os telefones estão na lista de bloqueio", nil
	}
	return c, false, "", nil
}

// portesDaReceita traduz o porte do contrato para o valor de companies.company_size.
var portesDaReceita = map[string]string{models.PorteME: "ME", models.PorteEPP: "EPP", models.PorteDemais: "DEMAIS"}

// perfilDaBase monta o Perfil (fonte "receita") a partir do retrato da base
// pública, só com os contatos livres.
func perfilDaBase(d *models.ProspEmpresa, c contatosDoItem) *enriquecimento.Perfil {
	p := enriquecimento.NovoPerfil(models.ProvedorReceita)
	nome := strings.TrimSpace(d.NomeFantasia)
	if nome == "" {
		nome = strings.TrimSpace(d.RazaoSocial)
	}
	p.Set("name", nome)
	p.Set("legal_name", d.RazaoSocial)
	p.Set("trade_name", d.NomeFantasia)
	p.Set("registration_status", strings.ToUpper(d.Situacao))
	if d.SituacaoDesde != nil {
		p.Set("registration_status_at", enriquecimento.NormalizarData(*d.SituacaoDesde))
	}
	if d.Abertura != nil {
		p.Set("founded_at", enriquecimento.NormalizarData(*d.Abertura))
	}
	if d.CNAE.Codigo > 0 {
		p.Set("cnae_code", enriquecimento.FormatarCNAE(fmt.Sprintf("%07d", d.CNAE.Codigo)))
	}
	p.Set("cnae_description", d.CNAE.Descricao)
	p.Set("industry", d.CNAE.Descricao)
	p.Set("legal_nature", d.Natureza.Descricao)
	p.Set("company_size", portesDaReceita[d.Porte])
	if d.Capital != nil && *d.Capital > 0 {
		p.Set("capital", *d.Capital)
	}
	p.Campos["simples"] = d.Simples
	p.Campos["mei"] = d.MEI
	e := d.Endereco
	p.Set("street", NomeProprio(strings.TrimSpace(e.TipoLogradouro+" "+e.Logradouro)))
	p.Set("street_number", e.Numero)
	p.Set("complement", e.Complemento)
	p.Set("neighborhood", NomeProprio(e.Bairro))
	p.Set("zip", models.SomenteDigitos(e.CEP))
	p.Set("city", NomeProprio(e.Municipio))
	p.Set("state", strings.ToUpper(e.UF))
	contatos := &enriquecimento.Contatos{Telefones: []enriquecimento.Telefone{}, Emails: []enriquecimento.EmailContato{}}
	for _, t := range d.Telefones {
		if !c.telefoneLivre(t.Numero) {
			continue
		}
		if p.Texto("phone") == "" {
			p.Set("phone", enriquecimento.FormatarTelefone(t.Numero))
		}
		contatos.Telefones = append(contatos.Telefones, enriquecimento.Telefone{
			Numero: enriquecimento.FormatarTelefone(t.Numero), Tipo: t.Tipo, Principal: len(contatos.Telefones) == 0,
			NaoPerturbe: t.NaoPerturbe, WhatsApp: t.WhatsApp,
		})
	}
	if c.emailLivre(d.Email) {
		email := strings.ToLower(strings.TrimSpace(d.Email))
		p.Set("email", email)
		contatos.Emails = append(contatos.Emails, enriquecimento.EmailContato{Email: email, Tipo: d.EmailTipo})
	}
	if len(contatos.Telefones) > 0 || len(contatos.Emails) > 0 {
		p.Contatos = contatos
	}
	for _, s := range d.CNAEsSecundarios {
		p.CNAEsSecundarios = append(p.CNAEsSecundarios, enriquecimento.CNAE{
			Codigo: enriquecimento.FormatarCNAE(fmt.Sprintf("%07d", s.Codigo)), Descricao: s.Descricao})
	}
	for _, s := range d.Socios {
		desde := ""
		if s.Desde != nil {
			desde = *s.Desde
		}
		tipo := map[string]string{"pf": "PF", "pj": "PJ", "estrangeiro": "Estrangeiro"}[s.Tipo]
		p.Socios = append(p.Socios, enriquecimento.Socio{
			Nome: s.Nome, Documento: s.Documento, Qualificacao: s.Qualificacao, Desde: desde, Tipo: tipo})
	}
	p.Sinais = enriquecimento.CalcularSinais(p)
	return p
}

// perfilDoEnriquecimento monta o Perfil do provedor do enriquecimento (Who
// Lead ou demonstração), só com os contatos livres.
func perfilDoEnriquecimento(pc *models.PerfilContato, c contatosDoItem) *enriquecimento.Perfil {
	provedor := strings.TrimSpace(pc.Provedor)
	if provedor == "" {
		provedor = models.ProvedorWhoLead
	}
	p := enriquecimento.NovoPerfil(cortar(provedor, 20))
	p.Demo = pc.Demo
	if pc.Demo {
		p.Mensagem = "dados de demonstração"
	}
	p.Set("website", pc.Site)
	if dom := dominioDoSite(pc.Site); dom != "" {
		p.Set("domain", dom)
	}
	p.Set("employees_range", pc.FuncionariosFaixa)
	p.Set("revenue_range", pc.FaturamentoFaixa)
	if pc.FuncionariosFaixa != "" || pc.FaturamentoFaixa != "" {
		p.Indicadores = &enriquecimento.Indicadores{FuncionariosFaixa: pc.FuncionariosFaixa, FaturamentoFaixa: pc.FaturamentoFaixa}
	}
	contatos := &enriquecimento.Contatos{Telefones: []enriquecimento.Telefone{}, Emails: []enriquecimento.EmailContato{}}
	for _, t := range pc.Telefones {
		if !c.telefoneLivre(t.Numero) {
			continue
		}
		contatos.Telefones = append(contatos.Telefones, enriquecimento.Telefone{
			Numero: enriquecimento.FormatarTelefone(t.Numero), Tipo: t.Tipo, Principal: len(contatos.Telefones) == 0,
			WhatsApp: t.WhatsApp, NaoPerturbe: t.NaoPerturbe,
		})
	}
	for _, e := range pc.Emails {
		if !c.emailLivre(e.Email) {
			continue
		}
		validado := e.Validado != nil && *e.Validado
		contatos.Emails = append(contatos.Emails, enriquecimento.EmailContato{
			Email: strings.ToLower(strings.TrimSpace(e.Email)), Tipo: e.Tipo, Validado: validado})
	}
	if len(contatos.Telefones) > 0 || len(contatos.Emails) > 0 {
		p.Contatos = contatos
	}
	for _, pessoa := range pc.Pessoas {
		pp := pessoa
		pp.Telefones = []models.ProspTelefone{}
		for _, t := range pessoa.Telefones {
			if c.telefoneLivre(t.Numero) {
				pp.Telefones = append(pp.Telefones, t)
			}
		}
		pp.Emails = []string{}
		for _, e := range pessoa.Emails {
			if c.emailLivre(e) {
				pp.Emails = append(pp.Emails, strings.ToLower(strings.TrimSpace(e)))
			}
		}
		p.Pessoas = append(p.Pessoas, pp)
	}
	p.Sinais = enriquecimento.CalcularSinais(p)
	return p
}

// dominioDoSite tira esquema, "www." e caminho do endereço do site.
func dominioDoSite(site string) string {
	s := strings.ToLower(strings.TrimSpace(site))
	for _, pre := range []string{"https://", "http://"} {
		s = strings.TrimPrefix(s, pre)
	}
	s = strings.TrimPrefix(s, "www.")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if !strings.Contains(s, ".") || strings.ContainsAny(s, " @") {
		return ""
	}
	return s
}

func cortar(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}

// perfisDoItem devolve os perfis do item (base e, se houver, enriquecimento).
func perfisDoItem(it *models.ProspItem, c contatosDoItem) []*enriquecimento.Perfil {
	perfis := []*enriquecimento.Perfil{}
	if it.Dados != nil {
		perfis = append(perfis, perfilDaBase(it.Dados, c))
	}
	if it.Enriquecimento != nil {
		perfis = append(perfis, perfilDoEnriquecimento(it.Enriquecimento, c))
	}
	return perfis
}

// empresaDoItem monta a empresa nova com o cadastro mesclado (base pública e
// enriquecimento), na lista branca de colunas do enriquecimento.
func empresaDoItem(it *models.ProspItem, mesclado *enriquecimento.Perfil, ownerID *int64) *models.Company {
	c := &models.Company{OwnerID: ownerID, CNPJ: models.FormatarCNPJ(it.CNPJ), Products: []string{}}
	if mesclado != nil {
		for _, campo := range models.EnrichableColumnsOrdenadas() {
			if v, ok := mesclado.Campos[campo]; ok {
				enriquecimento.AtribuirCampo(c, campo, v)
			}
		}
	}
	if strings.TrimSpace(c.Name) == "" {
		c.Name = strings.TrimSpace(it.NomeFantasia)
	}
	if strings.TrimSpace(c.Name) == "" {
		c.Name = strings.TrimSpace(it.RazaoSocial)
	}
	if strings.TrimSpace(c.Name) == "" {
		c.Name = models.FormatarCNPJ(it.CNPJ)
	}
	if c.City == "" {
		c.City = NomeProprio(it.Municipio)
	}
	if c.State == "" {
		c.State = strings.ToUpper(it.UF)
	}
	// Tamanho das colunas de companies (texto da base nunca derruba o envio).
	for _, x := range []struct {
		v *string
		n int
	}{
		{&c.Name, 200}, {&c.Domain, 150}, {&c.Phone, 30}, {&c.Industry, 255}, {&c.City, 100}, {&c.State, 50},
		{&c.LegalName, 255}, {&c.TradeName, 255}, {&c.CNAECode, 10}, {&c.CNAEDescription, 255},
		{&c.LegalNature, 120}, {&c.CompanySize, 30}, {&c.RegistrationStatus, 30}, {&c.Street, 255},
		{&c.StreetNumber, 20}, {&c.Complement, 120}, {&c.Neighborhood, 120}, {&c.Zip, 10},
		{&c.Email, 255}, {&c.Website, 255}, {&c.EmployeesRange, 40}, {&c.RevenueRange, 60},
	} {
		*x.v = cortar(*x.v, x.n)
	}
	return c
}

// gravarIntelDoEnvio grava uma consulta por fonte em company_enrichments (com
// custo 0: o custo do lote já está no job de enriquecimento), as fontes de
// cada campo preenchido e o perfil consolidado em companies.intel.
func gravarIntelDoEnvio(db *sql.DB, companyID int64, perfis []*enriquecimento.Perfil, mesclado *enriquecimento.Perfil, userID *int64) error {
	if mesclado == nil {
		return nil
	}
	idPorProvedor := map[string]int64{}
	ids := []int64{}
	for _, p := range perfis {
		b, err := json.Marshal(p)
		if err != nil {
			return err
		}
		row := models.Enrichment{CompanyID: companyID, Provedor: p.Provedor, Status: models.EnrichmentOK,
			Demo: p.Demo, Mensagem: p.Mensagem, CustoCentavos: 0, Perfil: b, UserID: userID}
		if err := models.InsertEnrichment(db, &row); err != nil {
			return err
		}
		idPorProvedor[p.Provedor] = row.ID
		ids = append(ids, row.ID)
	}
	campos := []string{}
	fontes := []models.FieldSource{}
	for _, campo := range models.EnrichableColumnsOrdenadas() {
		if v, ok := mesclado.Campos[campo]; !ok || enriquecimento.Vazio(v) {
			continue
		}
		prov := mesclado.ProvedorDoCampo(campo)
		fs := models.FieldSource{Campo: campo, Provedor: prov}
		if id, ok := idPorProvedor[prov]; ok {
			fs.EnrichmentID = &id
		}
		campos = append(campos, campo)
		fontes = append(fontes, fs)
	}
	if err := models.UpsertFieldSources(db, companyID, fontes); err != nil {
		return err
	}
	b, err := json.Marshal(mesclado)
	if err != nil {
		return err
	}
	if err := models.SetCompanyIntel(db, companyID, b); err != nil {
		return err
	}
	return models.MarkEnrichmentsApplied(db, ids, campos)
}

// candidatoContato é uma pessoa que pode virar contato decisor.
type candidatoContato struct {
	nome, cargo, telefone, email string
}

// candidatosAContato lista, na ordem do contrato, quem pode virar contato:
// pessoas do enriquecimento com telefone livre primeiro, depois os
// sócios-administradores do quadro (nunca menor de idade nem incapaz).
func candidatosAContato(it *models.ProspItem, c contatosDoItem) []candidatoContato {
	out := []candidatoContato{}
	vistos := map[string]bool{}
	emailDaPessoa := map[string]string{}
	if pc := it.Enriquecimento; pc != nil {
		for _, p := range pc.Pessoas {
			for _, e := range p.Emails {
				if k := ChaveNome(p.Nome); k != "" && emailDaPessoa[k] == "" && c.emailLivre(e) {
					emailDaPessoa[k] = strings.ToLower(strings.TrimSpace(e))
				}
			}
		}
		for _, p := range pc.Pessoas {
			k := ChaveNome(p.Nome)
			if k == "" || vistos[k] || pessoaMenor(p) {
				continue
			}
			tel := ""
			for _, t := range p.Telefones {
				if c.telefoneLivre(t.Numero) {
					tel = models.NormalizarTelefone(t.Numero)
					break
				}
			}
			if tel == "" {
				continue
			}
			vistos[k] = true
			cargo := p.Cargo
			if cargo == "" {
				cargo = rotuloPapel(p.Papel)
			}
			out = append(out, candidatoContato{nome: p.Nome, cargo: cargo, telefone: tel, email: emailDaPessoa[k]})
		}
	}
	if d := it.Dados; d != nil {
		for _, s := range d.Socios {
			k := ChaveNome(s.Nome)
			if k == "" || vistos[k] || (s.Tipo != "" && s.Tipo != "pf") || SocioMenor(s) || !SocioAdministrador(s.Qualificacao) {
				continue
			}
			vistos[k] = true
			out = append(out, candidatoContato{nome: s.Nome, cargo: s.Qualificacao, email: emailDaPessoa[k]})
		}
	}
	return out
}

// MaxContatosPorEmpresa é o teto de contatos criados pelo envio numa empresa.
const MaxContatosPorEmpresa = 3

// OrigemContatoProspeccao é o contacts.source dos contatos criados pelo envio.
const OrigemContatoProspeccao = "prospeccao"

// criarContatosDoItem cria os contatos decisores da empresa sem repetir nome
// (sem acento e sem caixa) e sem passar de três criados pela prospecção, o que
// torna o passo repetível. Devolve o contato principal (o primeiro candidato,
// novo ou que já existia) e quantos foram criados.
func criarContatosDoItem(db *sql.DB, companyID int64, ownerID *int64, cands []candidatoContato) (*int64, int, error) {
	if len(cands) == 0 {
		return nil, 0, nil
	}
	existentes := map[string]int64{}
	daProspeccao := 0
	rows, err := db.Query(`SELECT id, first_name, COALESCE(last_name,''), COALESCE(source,'') FROM contacts WHERE company_id = $1`, companyID)
	if err != nil {
		return nil, 0, err
	}
	for rows.Next() {
		var id int64
		var nome, sobrenome, origem string
		if err := rows.Scan(&id, &nome, &sobrenome, &origem); err != nil {
			rows.Close()
			return nil, 0, err
		}
		if k := ChaveNome(nome + " " + sobrenome); existentes[k] == 0 {
			existentes[k] = id
		}
		if origem == OrigemContatoProspeccao {
			daProspeccao++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var principal *int64
	criados := 0
	for _, cand := range cands {
		k := ChaveNome(cand.nome)
		if id, ok := existentes[k]; ok {
			if principal == nil {
				id := id
				principal = &id
			}
			continue
		}
		if daProspeccao+criados >= MaxContatosPorEmpresa {
			break
		}
		nome := strings.Fields(NomeProprio(cand.nome))
		ct := &models.Contact{
			FirstName:      cortar(nome[0], 100),
			LastName:       cortar(strings.Join(nome[1:], " "), 100),
			Email:          cortar(cand.email, 255),
			Phone:          enriquecimento.FormatarTelefone(cand.telefone),
			JobTitle:       cortar(NomeProprio(cand.cargo), 100),
			LifecycleStage: "lead",
			Source:         OrigemContatoProspeccao,
			CompanyID:      &companyID,
			OwnerID:        ownerID,
			BuyingRole:     "decisor",
		}
		if cand.telefone == "" {
			ct.Phone = ""
		}
		if err := models.CreateContact(db, ct); err != nil {
			return principal, criados, err
		}
		existentes[k] = ct.ID
		criados++
		if principal == nil {
			id := ct.ID
			principal = &id
		}
	}
	return principal, criados, nil
}
