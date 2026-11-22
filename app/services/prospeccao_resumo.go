package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/armando-couto/crm-ia/app/models"
	"github.com/armando-couto/crm-ia/app/utils"
)

// -----------------------------------------------------------------------------
// Resumo do lead e ficha completa (LeadCompleto).
//
// MontarResumoLead é a "ficha pronta" gravada em leads.resumo no envio ao CRM
// (docs/PROSPECCAO.md 8.1 passo 4): frases curtas, decisores com telefone, os
// sinais (por que ligar e o que observar) e o cadastro. É função pura.
//
// MontarLeadCompleto é o que GET /leads/{id} e a fila devolvem: o lead, o
// resumo, os contatos da empresa, as ligações já feitas, o roteiro com as
// variáveis trocadas, os alertas e a janela de contato. É aqui que o bloqueio
// de telefone aparece (adendo, 8.3): o número bloqueado vem "invalido: true".
// -----------------------------------------------------------------------------

// MontarResumoLead monta o resumo a partir do retrato da base pública (dados,
// pode ser nil) e do enriquecimento (perfil, pode ser nil). telefones é a
// lista já ordenada e sem os bloqueados (a mesma que vai para o lead).
func MontarResumoLead(dados *models.ProspEmpresa, perfil *models.PerfilContato, telefones []models.ProspTelefone, agora time.Time) models.ResumoLead {
	r := models.ResumoLead{
		Frases:    []string{},
		Decisores: []models.ResumoDecisor{},
		Sinais:    []models.Sinal{},
		Cadastro:  models.ResumoCadastro{Socios: []models.ProspSocio{}},
	}
	if dados != nil {
		r.Cadastro = cadastroDoResumo(dados)
		r.Frases = append(r.Frases, frasesDaEmpresa(dados, agora)...)
	}
	if perfil != nil {
		if perfil.Site != "" {
			r.Cadastro.Site = perfil.Site
		}
		if perfil.FaturamentoFaixa != "" {
			r.Frases = append(r.Frases, "Faturamento estimado: "+perfil.FaturamentoFaixa)
		}
		if perfil.FuncionariosFaixa != "" {
			r.Frases = append(r.Frases, "Funcionários: "+perfil.FuncionariosFaixa)
		}
		if r.Cadastro.Email == "" && len(perfil.Emails) > 0 {
			r.Cadastro.Email = strings.ToLower(strings.TrimSpace(perfil.Emails[0].Email))
		}
	}
	r.Decisores = decisoresDoResumo(dados, perfil, telefones)
	r.Sinais = sinaisDoResumo(dados, perfil, telefones, r.Decisores, agora)
	return r
}

func cadastroDoResumo(d *models.ProspEmpresa) models.ResumoCadastro {
	c := models.ResumoCadastro{
		RazaoSocial:  d.RazaoSocial,
		NomeFantasia: d.NomeFantasia,
		CNAE:         d.CNAE.Descricao,
		Porte:        d.Porte,
		Abertura:     d.Abertura,
		Capital:      d.Capital,
		Natureza:     d.Natureza.Descricao,
		Simples:      d.Simples,
		MEI:          d.MEI,
		Endereco:     EnderecoEmLinha(d.Endereco),
		Municipio:    d.Endereco.Municipio,
		UF:           d.Endereco.UF,
		Email:        strings.ToLower(strings.TrimSpace(d.Email)),
		Socios:       d.Socios,
		Divida:       d.Divida,
	}
	if c.Socios == nil {
		c.Socios = []models.ProspSocio{}
	}
	return c
}

// EnderecoEmLinha junta logradouro, número, complemento, bairro e CEP.
func EnderecoEmLinha(e models.ProspEndereco) string {
	rua := strings.TrimSpace(strings.TrimSpace(e.TipoLogradouro) + " " + strings.TrimSpace(e.Logradouro))
	partes := []string{}
	if rua != "" {
		if n := strings.TrimSpace(e.Numero); n != "" {
			rua += ", " + n
		}
		partes = append(partes, rua)
	}
	for _, p := range []string{e.Complemento, e.Bairro} {
		if p = strings.TrimSpace(p); p != "" {
			partes = append(partes, p)
		}
	}
	if cep := models.SomenteDigitos(e.CEP); len(cep) == 8 {
		partes = append(partes, "CEP "+cep[:5]+"-"+cep[5:])
	}
	return strings.Join(partes, " - ")
}

var portesPorExtenso = map[string]string{
	models.PorteME:     "microempresa",
	models.PorteEPP:    "pequeno porte",
	models.PorteDemais: "médio ou grande porte",
}

// frasesDaEmpresa devolve as frases de abertura da ficha: quem é, onde fica,
// há quanto tempo existe e de que tamanho é.
func frasesDaEmpresa(d *models.ProspEmpresa, agora time.Time) []string {
	frases := []string{}
	partes := []string{}
	segmento := primeiraMaiuscula(strings.ToLower(strings.TrimSpace(d.CNAE.Descricao)))
	lugar := strings.TrimSpace(NomeProprio(d.Endereco.Municipio))
	if lugar != "" && d.Endereco.UF != "" {
		lugar += "/" + d.Endereco.UF
	}
	switch {
	case segmento != "" && lugar != "":
		partes = append(partes, segmento+" em "+lugar)
	case segmento != "":
		partes = append(partes, segmento)
	case lugar != "":
		partes = append(partes, "Empresa em "+lugar)
	}
	if anos, ok := anosDesde(d.Abertura, agora); ok {
		switch {
		case anos < 1:
			partes = append(partes, "aberta há menos de 1 ano")
		case anos == 1:
			partes = append(partes, "aberta há 1 ano")
		default:
			partes = append(partes, fmt.Sprintf("aberta há %d anos", anos))
		}
	}
	if p := portesPorExtenso[d.Porte]; p != "" {
		partes = append(partes, p)
	}
	if len(partes) > 0 {
		frases = append(frases, strings.Join(partes, ", "))
	}
	if d.Capital != nil && *d.Capital > 0 {
		frases = append(frases, "Capital social de "+ReaisPorExtenso(*d.Capital))
	}
	switch {
	case d.MEI:
		frases = append(frases, "Microempreendedor individual (MEI)")
	case d.Simples:
		frases = append(frases, "Optante do Simples Nacional")
	}
	if n := len(d.Socios); n > 0 {
		nomes := []string{}
		for _, s := range d.Socios {
			if len(nomes) == 2 {
				break
			}
			nomes = append(nomes, NomeProprio(s.Nome))
		}
		rotulo := "1 sócio"
		if n > 1 {
			rotulo = fmt.Sprintf("%d sócios", n)
		}
		frase := rotulo + ": " + strings.Join(nomes, " e ")
		if n > len(nomes) {
			frase += " e outros"
		}
		frases = append(frases, frase)
	}
	return frases
}

// anosDesde devolve os anos completos entre a data "AAAA-MM-DD" e agora.
func anosDesde(data *string, agora time.Time) (int, bool) {
	if data == nil || len(*data) < 10 {
		return 0, false
	}
	t, err := time.ParseInLocation("2006-01-02", (*data)[:10], time.Local)
	if err != nil || t.After(agora) {
		return 0, false
	}
	agora = agora.In(time.Local)
	anos := agora.Year() - t.Year()
	if agora.Month() < t.Month() || (agora.Month() == t.Month() && agora.Day() < t.Day()) {
		anos--
	}
	return anos, true
}

// ReaisPorExtenso formata o valor como "R$ 1.234.567,89" (sem centavos acima de mil).
func ReaisPorExtenso(v float64) string {
	centavos := int64(v*100 + 0.5)
	inteiro := fmt.Sprint(centavos / 100)
	var b strings.Builder
	for i, r := range inteiro {
		if i > 0 && (len(inteiro)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if v >= 1000 {
		return "R$ " + b.String()
	}
	return fmt.Sprintf("R$ %s,%02d", b.String(), centavos%100)
}

// NomeProprio troca "JOAO DA SILVA" por "Joao da Silva". Texto que já tem
// minúsculas é devolvido como veio.
func NomeProprio(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || s != strings.ToUpper(s) {
		return s
	}
	minusculas := map[string]bool{"de": true, "da": true, "do": true, "das": true, "dos": true, "e": true}
	palavras := strings.Fields(strings.ToLower(s))
	for i, w := range palavras {
		if i > 0 && minusculas[w] {
			continue
		}
		palavras[i] = primeiraMaiuscula(w)
	}
	return strings.Join(palavras, " ")
}

func primeiraMaiuscula(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// ChaveNome é a chave de comparação de nomes de pessoa: sem acento, sem caixa
// e com espaços simples. Serve para não repetir contato na mesma empresa.
func ChaveNome(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.Join(strings.Fields(s), " ")) {
		switch r {
		case 'á', 'à', 'â', 'ã', 'ä':
			r = 'a'
		case 'é', 'è', 'ê', 'ë':
			r = 'e'
		case 'í', 'ì', 'î', 'ï':
			r = 'i'
		case 'ó', 'ò', 'ô', 'õ', 'ö':
			r = 'o'
		case 'ú', 'ù', 'û', 'ü':
			r = 'u'
		case 'ç':
			r = 'c'
		case 'ñ':
			r = 'n'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// SocioAdministrador diz se a qualificação é de quem administra a empresa.
func SocioAdministrador(qualificacao string) bool {
	q := ChaveNome(qualificacao)
	for _, p := range []string{"administrador", "titular", "presidente", "diretor", "empresario"} {
		if strings.Contains(q, p) {
			return true
		}
	}
	return false
}

// SocioMenor diz se o sócio é menor de idade ou incapaz (tem representante
// legal no quadro, ou a qualificação diz). Nunca vira contato.
func SocioMenor(s models.ProspSocio) bool {
	q := ChaveNome(s.Qualificacao)
	return strings.TrimSpace(s.Representante) != "" || strings.Contains(q, "menor") || strings.Contains(q, "incapaz")
}

// decisoresDoResumo lista quem decide: primeiro as pessoas do enriquecimento
// (com os telefones que sobraram depois dos bloqueios), depois os
// sócios-administradores do quadro societário que ainda não apareceram.
func decisoresDoResumo(dados *models.ProspEmpresa, perfil *models.PerfilContato, telefones []models.ProspTelefone) []models.ResumoDecisor {
	out := []models.ResumoDecisor{}
	vistos := map[string]bool{}
	porPessoa := map[string][]models.ProspTelefone{}
	for _, t := range telefones {
		if t.Pessoa != "" {
			k := ChaveNome(t.Pessoa)
			porPessoa[k] = append(porPessoa[k], t)
		}
	}
	juntar := func(nome, cargo string) {
		k := ChaveNome(nome)
		if k == "" || vistos[k] {
			return
		}
		vistos[k] = true
		tels := porPessoa[k]
		if tels == nil {
			tels = []models.ProspTelefone{}
		}
		out = append(out, models.ResumoDecisor{Nome: NomeProprio(nome), Cargo: cargo, Telefones: tels})
	}
	if perfil != nil {
		// Quem tem telefone vem primeiro.
		for _, comTelefone := range []bool{true, false} {
			for _, p := range perfil.Pessoas {
				if (len(porPessoa[ChaveNome(p.Nome)]) > 0) != comTelefone || pessoaMenor(p) {
					continue
				}
				cargo := p.Cargo
				if cargo == "" {
					cargo = rotuloPapel(p.Papel)
				}
				juntar(p.Nome, cargo)
			}
		}
	}
	if dados != nil {
		for _, s := range dados.Socios {
			if s.Tipo != "" && s.Tipo != "pf" {
				continue
			}
			if SocioMenor(s) || !SocioAdministrador(s.Qualificacao) {
				continue
			}
			juntar(s.Nome, s.Qualificacao)
		}
	}
	return out
}

func pessoaMenor(p models.PessoaContato) bool {
	c := ChaveNome(p.Cargo)
	return strings.Contains(c, "menor") || strings.Contains(c, "incapaz")
}

func rotuloPapel(papel string) string {
	switch papel {
	case "socio":
		return "Sócio"
	case "representante":
		return "Representante legal"
	case "funcionario":
		return "Funcionário"
	}
	return ""
}

func verdadeiro(b *bool) bool { return b != nil && *b }

// sinaisDoResumo monta os motivos para ligar (ok) e os pontos de atenção.
func sinaisDoResumo(dados *models.ProspEmpresa, perfil *models.PerfilContato, telefones []models.ProspTelefone,
	decisores []models.ResumoDecisor, agora time.Time) []models.Sinal {
	sinais := []models.Sinal{}
	add := func(tipo, nivel, texto string) {
		sinais = append(sinais, models.Sinal{Tipo: tipo, Nivel: nivel, Texto: texto})
	}
	var whatsapp, celular, naoPerturbe, proprio bool
	for _, t := range telefones {
		whatsapp = whatsapp || verdadeiro(t.WhatsApp)
		celular = celular || t.Tipo == models.TelefoneCelular
		naoPerturbe = naoPerturbe || verdadeiro(t.NaoPerturbe)
		proprio = proprio || !t.Compartilhado
	}
	decisorComTelefone := false
	for _, d := range decisores {
		decisorComTelefone = decisorComTelefone || len(d.Telefones) > 0
	}
	switch {
	case len(telefones) == 0:
		add("sem_telefone", "risco", "Nenhum telefone válido para ligar")
	case whatsapp:
		add("whatsapp", "ok", "Celular com WhatsApp confirmado")
	case celular:
		add("celular", "ok", "Tem celular para contato direto")
	}
	switch {
	case decisorComTelefone:
		add("decisor", "ok", "Decisor identificado com telefone direto")
	case len(decisores) > 0:
		add("decisor", "ok", "Sócio-administrador identificado: "+decisores[0].Nome)
	}
	if len(telefones) > 0 && !proprio {
		add("contador", "atencao", "O telefone é compartilhado com outras empresas (provável contabilidade)")
	} else if dados != nil && dados.EmailTipo == "contabilidade" {
		add("contador", "atencao", "O e-mail do cadastro parece ser da contabilidade")
	}
	if naoPerturbe {
		add("nao_perturbe", "risco", "Há número em lista de bloqueio de telemarketing (não perturbe)")
	}
	if dados != nil {
		if anos, ok := anosDesde(dados.Abertura, agora); ok && anos < 1 {
			add("empresa_nova", "ok", "Empresa aberta há menos de 1 ano: está montando a operação")
		}
		if dados.PessoaFisica {
			add("pessoa_fisica", "atencao", "MEI ou empresário individual: os dados são de pessoa física (LGPD)")
		}
		if d := dados.Divida; d != nil && d.Total > 0 {
			nivel, texto := "atencao", "Dívida ativa na União de "+ReaisPorExtenso(d.Total)
			if d.Ajuizada {
				nivel = "risco"
				texto += ", com cobrança ajuizada"
			}
			if d.Referencia != "" {
				texto += " (referência " + d.Referencia + ")"
			}
			add("divida", nivel, texto)
		}
	}
	if perfil != nil && perfil.Demo {
		add("demonstracao", "atencao", "Contatos de demonstração: não são dados reais")
	}
	return sinais
}

// -----------------------------------------------------------------------------
// LeadCompleto
// -----------------------------------------------------------------------------

// Alerta é um aviso da ficha do lead (interface Alerta).
type Alerta struct {
	Tipo  string `json:"tipo"`  // pessoa_fisica | nao_perturbe | bloqueado | fora_da_janela | contador | divida | whatsapp_sem_optin | limite_tentativas
	Nivel string `json:"nivel"` // info | atencao | risco
	Texto string `json:"texto"`
}

// Roteiro é o manual de atividades da lista (ou o padrão), com as variáveis trocadas.
type Roteiro struct {
	Titulo string `json:"titulo"`
	Corpo  string `json:"corpo"`
}

// LeadCompleto é a ficha do lead (GET /leads/{id} e POST /leads/fila/proximo).
type LeadCompleto struct {
	Lead       *models.Lead       `json:"lead"`
	Resumo     models.ResumoLead  `json:"resumo"`
	Contatos   []models.Contact   `json:"contatos"`
	Tentativas []models.Tentativa `json:"tentativas"`
	Roteiro    *Roteiro           `json:"roteiro"`
	Alertas    []Alerta           `json:"alertas"`
	Janela     models.Janela      `json:"janela"`
}

// MontarLeadCompleto monta a ficha do lead já lido (com a visão conferida por
// quem chama). vendedor é o nome de quem está olhando, para o roteiro.
func MontarLeadCompleto(db *sql.DB, l *models.Lead, vendedor string, agora time.Time) (*LeadCompleto, error) {
	cfg := models.LoadConfigProspeccao(db)
	lc := &LeadCompleto{
		Lead:       l,
		Resumo:     l.Resumo,
		Contatos:   []models.Contact{},
		Tentativas: []models.Tentativa{},
		Alertas:    []Alerta{},
		Janela:     cfg.JanelaEm(agora),
	}
	normalizarResumo(&lc.Resumo)

	// Bloqueios: o de CNPJ barra a empresa (alerta); o de telefone só tira o
	// número, que vem marcado como inválido.
	numeros := make([]string, 0, len(l.Telefones))
	for _, t := range l.Telefones {
		numeros = append(numeros, t.Numero)
	}
	bloq, err := models.ConferirBloqueios(db, l.CNPJ, numeros, nil)
	if err != nil {
		return nil, err
	}
	for i := range l.Telefones {
		if bloq.Telefones[models.NormalizarTelefone(l.Telefones[i].Numero)] {
			l.Telefones[i].Invalido = true
		}
	}

	var naoPerturbeEmpresa bool
	if l.CompanyID != nil {
		contatos, _, err := models.ListContacts(db, models.ContactFilter{
			CompanyID: *l.CompanyID, Pagination: models.Pagination{Page: 1, PerPage: 100}})
		if err != nil {
			return nil, err
		}
		lc.Contatos = contatos
		// Empresa apagada depois do lead não é erro: o lead segue sem ela.
		_ = db.QueryRow(`SELECT do_not_disturb FROM companies WHERE id = $1`, *l.CompanyID).Scan(&naoPerturbeEmpresa)
	}
	if lc.Tentativas, err = models.TentativasDoLead(db, l.ID); err != nil {
		return nil, err
	}
	if lc.Tentativas == nil {
		lc.Tentativas = []models.Tentativa{}
	}
	lc.Roteiro = roteiroDoLead(db, l, cfg, lc.Resumo, vendedor)
	lc.Alertas = alertasDoLead(l, cfg, lc.Resumo, bloq.CNPJ, naoPerturbeEmpresa, lc.Janela)
	return lc, nil
}

// normalizarResumo garante listas vazias (nunca null) no JSON do resumo.
func normalizarResumo(r *models.ResumoLead) {
	if r.Frases == nil {
		r.Frases = []string{}
	}
	if r.Decisores == nil {
		r.Decisores = []models.ResumoDecisor{}
	}
	for i := range r.Decisores {
		if r.Decisores[i].Telefones == nil {
			r.Decisores[i].Telefones = []models.ProspTelefone{}
		}
	}
	if r.Sinais == nil {
		r.Sinais = []models.Sinal{}
	}
	if r.Cadastro.Socios == nil {
		r.Cadastro.Socios = []models.ProspSocio{}
	}
}

// roteiroDoLead devolve o manual da lista do lead, ou o padrão das
// configurações, com as variáveis trocadas. nil quando não há manual ativo.
func roteiroDoLead(db *sql.DB, l *models.Lead, cfg models.ConfigProspeccao, resumo models.ResumoLead, vendedor string) *Roteiro {
	var playbookID *int64
	if l.ListaID != nil {
		_ = db.QueryRow(`SELECT playbook_id FROM prospeccao_listas WHERE id = $1`, *l.ListaID).Scan(&playbookID)
	}
	if playbookID == nil {
		playbookID = cfg.PlaybookID
	}
	if playbookID == nil {
		return nil
	}
	p, err := models.PlaybookByID(db, *playbookID)
	if err != nil || !p.Active {
		return nil
	}
	empresa := l.CompanyName
	if empresa == "" {
		empresa = l.Titulo
	}
	razao := resumo.Cadastro.RazaoSocial
	if razao == "" {
		razao = empresa
	}
	decisor := l.ContactName
	if decisor == "" && len(resumo.Decisores) > 0 {
		decisor = resumo.Decisores[0].Nome
	}
	c := utils.Cfg
	minha := models.LoadWorkspaceSettings(db, c.TenantNome, c.CorPrimaria, c.LogoURL).Name
	troca := strings.NewReplacer(
		"{{empresa}}", empresa,
		"{{razao_social}}", razao,
		"{{cidade}}", NomeProprio(l.Cidade),
		"{{uf}}", l.UF,
		"{{segmento}}", strings.ToLower(resumo.Cadastro.CNAE),
		"{{decisor}}", primeiroNome(decisor),
		"{{vendedor}}", primeiroNome(vendedor),
		"{{minha_empresa}}", minha,
	)
	return &Roteiro{Titulo: p.Name, Corpo: troca.Replace(p.Body)}
}

func primeiroNome(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return NomeProprio(f[0])
	}
	return ""
}

func alertasDoLead(l *models.Lead, cfg models.ConfigProspeccao, resumo models.ResumoLead, cnpjBloqueado, naoPerturbeEmpresa bool,
	janela models.Janela) []Alerta {
	alertas := []Alerta{}
	add := func(tipo, nivel, texto string) {
		alertas = append(alertas, Alerta{Tipo: tipo, Nivel: nivel, Texto: texto})
	}

	if cnpjBloqueado {
		add("bloqueado", "risco", "Esta empresa está na lista de bloqueio (não contatar)")
	}
	var validos, compartilhados, naoPerturbe, celulares int
	for _, t := range l.Telefones {
		if t.Invalido {
			continue
		}
		validos++
		if t.Compartilhado {
			compartilhados++
		}
		if verdadeiro(t.NaoPerturbe) {
			naoPerturbe++
		}
		if t.Tipo == models.TelefoneCelular {
			celulares++
		}
	}
	switch {
	case naoPerturbeEmpresa:
		add("nao_perturbe", "risco", "A empresa pediu para não ser incomodada")
	case naoPerturbe > 0 && naoPerturbe == validos:
		add("nao_perturbe", "risco", "Todos os números estão em lista de bloqueio de telemarketing (não perturbe)")
	case naoPerturbe > 0:
		add("nao_perturbe", "atencao", "Há número em lista de bloqueio de telemarketing: prefira os outros")
	}
	if l.PessoaFisica {
		add("pessoa_fisica", "atencao", "MEI ou empresário individual: você está falando com uma pessoa física (LGPD)")
	}
	if validos > 0 && compartilhados == validos {
		add("contador", "atencao", "O telefone é compartilhado com outras empresas: pode ser da contabilidade")
	}
	if d := resumo.Cadastro.Divida; d != nil && d.Total > 0 {
		texto := "Dívida ativa na União de " + ReaisPorExtenso(d.Total)
		if d.Referencia != "" {
			texto += " (referência " + d.Referencia + ")"
		}
		add("divida", "info", texto)
	}
	if cfg.ExigirOptinWhatsApp && !l.WhatsAppOptin && celulares > 0 {
		add("whatsapp_sem_optin", "info", "WhatsApp só depois que o cliente autorizar")
	}
	if l.Aberto() && l.Tentativas > 0 && l.Tentativas >= cfg.MaxTentativas-1 {
		add("limite_tentativas", "atencao", fmt.Sprintf("Última tentativa: %d de %d sem contato", l.Tentativas, cfg.MaxTentativas))
	}
	if !janela.Aberta {
		add("fora_da_janela", "atencao", "Fora da janela de contato ("+janela.Texto+")")
	}
	return alertas
}

// ResumoDaEmpresa monta o resumo de um lead criado à mão a partir do
// cadastro da empresa do CRM (sem enriquecimento: só o cadastro e as frases
// que dá para tirar dele).
func ResumoDaEmpresa(c *models.Company, agora time.Time) models.ResumoLead {
	r := models.ResumoLead{Frases: []string{}, Decisores: []models.ResumoDecisor{}, Sinais: []models.Sinal{},
		Cadastro: models.ResumoCadastro{Socios: []models.ProspSocio{}}}
	if c == nil {
		return r
	}
	porte := map[string]string{"ME": models.PorteME, "EPP": models.PorteEPP, "DEMAIS": models.PorteDemais}[strings.ToUpper(c.CompanySize)]
	if porte == "" {
		porte = models.PorteNaoInformado
	}
	d := &models.ProspEmpresa{
		RazaoSocial:  c.LegalName,
		NomeFantasia: c.TradeName,
		Natureza:     models.ProspCodigoDescricao{Descricao: c.LegalNature},
		Porte:        porte,
		Capital:      c.Capital,
		CNAE:         models.ProspCodigoDescricao{Descricao: c.CNAEDescription},
		Endereco: models.ProspEndereco{Logradouro: c.Street, Numero: c.StreetNumber, Complemento: c.Complement,
			Bairro: c.Neighborhood, CEP: c.Zip, Municipio: c.City, UF: c.State},
		Email: c.Email,
	}
	if d.RazaoSocial == "" {
		d.RazaoSocial = c.Name
	}
	if c.FoundedAt != nil {
		s := c.FoundedAt.Format("2006-01-02")
		d.Abertura = &s
	}
	if c.Simples != nil {
		d.Simples = *c.Simples
	}
	if c.MEI != nil {
		d.MEI = *c.MEI
	}
	r.Cadastro = cadastroDoResumo(d)
	r.Cadastro.Site = c.Website
	r.Frases = frasesDaEmpresa(d, agora)
	return r
}
