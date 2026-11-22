package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// -----------------------------------------------------------------------------
// Nota do item da lista de prospecção (0 a 100, sempre com os motivos).
// Contrato: docs/PROSPECCAO.md seção 6.3, com o adendo (telefone "indefinido"
// conta como fixo e não conta como celular).
//
// A nota é calculada quando o item nasce (só com o retrato da base pública) e
// recalculada depois do enriquecimento (retrato + PerfilContato). É uma função
// pura: quem grava é o job (gerar_lista ou enriquecer).
// -----------------------------------------------------------------------------

// NotaItem é o resultado do cálculo: a nota, os motivos e as marcas que a
// tabela de itens mostra e filtra.
type NotaItem struct {
	Nota                 int
	Motivos              []models.NotaMotivo
	TemCelular           bool
	TemEmail             bool
	TemDecisor           bool // decisor (sócio ou representante) com telefone, vindo do enriquecimento
	TemWhatsApp          bool
	TemDivida            bool
	ContatoCompartilhado bool // o único contato é compartilhado (5+ empresas) ou de contabilidade
	TelefonePrincipal    string
	// Telefones são todos os telefones conhecidos (empresa, enriquecimento e
	// pessoas), sem repetição, do melhor para o pior: "não perturbe" e
	// compartilhados vão para o fim.
	Telefones []models.ProspTelefone
}

// Pontos da nota (valores do contrato).
const (
	notaCelularProprio   = 35
	notaWhatsApp         = 10
	notaDecisorTelefone  = 15
	notaSocioAdm         = 8
	notaEmailCorporativo = 10
	notaEmailGratuito    = 4
	notaFixoProprio      = 10
	notaSite             = 5
	notaMaisDe2Anos      = 5
	notaPorte            = 5
	notaSoCompartilhado  = -25
	notaSemTelefone      = -40
	notaNaoPerturbe      = -10
)

// CalcularNotaItem calcula a nota de um item a partir do retrato da empresa
// (dados, pode ser nil) e do enriquecimento (perfil, nil antes de enriquecer).
func CalcularNotaItem(dados *models.ProspEmpresa, perfil *models.PerfilContato) NotaItem {
	return calcularNotaItemEm(dados, perfil, time.Now())
}

func calcularNotaItemEm(dados *models.ProspEmpresa, perfil *models.PerfilContato, agora time.Time) NotaItem {
	n := NotaItem{Motivos: []models.NotaMotivo{}}
	soma := 0
	add := func(pontos int, texto string) {
		soma += pontos
		n.Motivos = append(n.Motivos, models.NotaMotivo{Pontos: pontos, Texto: texto})
	}

	n.Telefones = TelefonesDoItem(dados, perfil)
	var celularProprio, fixoProprio, whatsapp bool
	naoPerturbe := 0
	for _, t := range n.Telefones {
		if t.NaoPerturbe != nil && *t.NaoPerturbe {
			naoPerturbe++
		}
		if t.Tipo == models.TelefoneCelular {
			n.TemCelular = true
		}
		if t.WhatsApp != nil && *t.WhatsApp {
			whatsapp = true
		}
		if t.Compartilhado {
			continue
		}
		if t.Tipo == models.TelefoneCelular {
			celularProprio = true
		} else {
			fixoProprio = true // fixo ou indefinido
		}
	}
	n.TemWhatsApp = whatsapp
	if len(n.Telefones) > 0 {
		n.TelefonePrincipal = n.Telefones[0].Numero
	}

	// E-mails: o da Receita (já classificado na carga) e os do enriquecimento.
	var emailCorporativo, emailGratuito bool
	marcarEmail := func(email, tipo string) {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" || !strings.Contains(email, "@") {
			return
		}
		n.TemEmail = true
		if tipo == "" {
			tipo = classificarEmail(email)
		}
		switch tipo {
		case "contabilidade":
			// não soma pontos nem conta como contato próprio
		case "gratuito":
			emailGratuito = true
		default:
			emailCorporativo = true
		}
	}
	if dados != nil {
		marcarEmail(dados.Email, dados.EmailTipo)
	}
	if perfil != nil {
		for _, e := range perfil.Emails {
			marcarEmail(e.Email, tipoEmailConhecido(e.Tipo))
		}
		for _, p := range perfil.Pessoas {
			for _, e := range p.Emails {
				marcarEmail(e, "")
			}
		}
	}

	// Decisor com telefone (só o enriquecimento sabe) ou sócio-administrador.
	if perfil != nil {
		for _, p := range perfil.Pessoas {
			if p.Papel == "funcionario" {
				continue
			}
			for _, t := range p.Telefones {
				if !t.Invalido && models.NormalizarTelefone(t.Numero) != "" {
					n.TemDecisor = true
				}
			}
		}
	}
	socioAdm := false
	if dados != nil {
		for _, s := range dados.Socios {
			q := strings.ToLower(s.Qualificacao)
			if s.Tipo != "pj" && strings.TrimSpace(s.Nome) != "" && strings.Contains(q, "administrador") {
				socioAdm = true
			}
		}
		n.TemDivida = dados.Divida != nil && dados.Divida.Total > 0
	}

	if celularProprio {
		add(notaCelularProprio, "Celular próprio")
	}
	if whatsapp {
		add(notaWhatsApp, "WhatsApp confirmado")
	}
	switch {
	case n.TemDecisor:
		add(notaDecisorTelefone, "Decisor com telefone")
	case socioAdm:
		add(notaSocioAdm, "Sócio-administrador identificado")
	}
	switch {
	case emailCorporativo:
		add(notaEmailCorporativo, "E-mail corporativo")
	case emailGratuito:
		add(notaEmailGratuito, "E-mail de provedor gratuito")
	}
	if fixoProprio {
		add(notaFixoProprio, "Telefone fixo próprio")
	}
	if perfil != nil && strings.TrimSpace(perfil.Site) != "" {
		add(notaSite, "Tem site")
	}
	if dados != nil {
		if dados.Abertura != nil {
			if ab, err := time.Parse("2006-01-02", *dados.Abertura); err == nil && ab.AddDate(2, 0, 0).Before(agora) {
				add(notaMaisDe2Anos, "Empresa com mais de 2 anos")
			}
		}
		if dados.Porte == models.PorteEPP || dados.Porte == models.PorteDemais {
			add(notaPorte, "Porte EPP ou maior")
		}
	}

	// Único contato compartilhado ou de contabilidade: nenhum telefone próprio
	// e nenhum e-mail que não seja de contador.
	temContato := len(n.Telefones) > 0 || n.TemEmail
	contatoProprio := celularProprio || fixoProprio || emailCorporativo || emailGratuito
	if temContato && !contatoProprio {
		n.ContatoCompartilhado = true
		add(notaSoCompartilhado, "O único contato é compartilhado ou de contabilidade")
	}
	if len(n.Telefones) == 0 {
		add(notaSemTelefone, "Sem telefone")
	}
	if naoPerturbe == 1 {
		add(notaNaoPerturbe, "1 telefone com bloqueio de telemarketing (não perturbe)")
	} else if naoPerturbe > 1 {
		add(notaNaoPerturbe*naoPerturbe, fmt.Sprintf("%d telefones com bloqueio de telemarketing (não perturbe)", naoPerturbe))
	}

	n.Nota = max(0, min(100, soma))
	return n
}

// TelefonesDoItem junta os telefones da empresa (Receita), do enriquecimento e
// das pessoas, sem repetir número e sem os marcados como inválidos, e ordena
// do melhor para o pior: primeiro os sem "não perturbe", depois os próprios
// (não compartilhados), celular antes de fixo, WhatsApp confirmado antes, e
// telefone de pessoa (decisor) antes do telefone geral da empresa.
func TelefonesDoItem(dados *models.ProspEmpresa, perfil *models.PerfilContato) []models.ProspTelefone {
	out := []models.ProspTelefone{}
	indice := map[string]int{}
	juntar := func(t models.ProspTelefone, origem, pessoa string) {
		t.Numero = models.NormalizarTelefone(t.Numero)
		if t.Numero == "" || t.Invalido {
			return
		}
		if t.Tipo == "" {
			t.Tipo = models.TelefoneFixo
			if len(t.Numero) == 11 {
				t.Tipo = models.TelefoneCelular
			}
		}
		if t.Origem == "" {
			t.Origem = origem
		}
		if t.Pessoa == "" {
			t.Pessoa = pessoa
		}
		t.Principal = false
		if i, ok := indice[t.Numero]; ok {
			// O mesmo número em duas fontes: fica o que sabe mais.
			a := &out[i]
			if a.WhatsApp == nil {
				a.WhatsApp = t.WhatsApp
			}
			if a.NaoPerturbe == nil {
				a.NaoPerturbe = t.NaoPerturbe
			}
			if a.Pessoa == "" {
				a.Pessoa = t.Pessoa
			}
			a.Compartilhado = a.Compartilhado || t.Compartilhado
			return
		}
		indice[t.Numero] = len(out)
		out = append(out, t)
	}
	if perfil != nil {
		for _, p := range perfil.Pessoas {
			for _, t := range p.Telefones {
				juntar(t, models.OrigemContatoWhoLead, p.Nome)
			}
		}
		for _, t := range perfil.Telefones {
			juntar(t, models.OrigemContatoWhoLead, "")
		}
	}
	if dados != nil {
		for _, t := range dados.Telefones {
			juntar(t, models.OrigemContatoReceita, "")
		}
	}
	peso := func(t models.ProspTelefone) int {
		p := 0
		if t.NaoPerturbe != nil && *t.NaoPerturbe {
			p += 1000
		}
		if t.Compartilhado {
			p += 100
		}
		if t.Tipo != models.TelefoneCelular {
			p += 10
		}
		if t.WhatsApp == nil || !*t.WhatsApp {
			p += 2
		}
		if t.Pessoa == "" {
			p++
		}
		return p
	}
	sort.SliceStable(out, func(i, j int) bool { return peso(out[i]) < peso(out[j]) })
	if len(out) > 0 {
		out[0].Principal = true
	}
	return out
}

// tipoEmailConhecido só aceita os três tipos do contrato; qualquer outro
// rótulo do provedor ("pessoal", "comercial"…) cai na classificação pelo endereço.
func tipoEmailConhecido(tipo string) string {
	switch tipo {
	case "corporativo", "gratuito", "contabilidade":
		return tipo
	}
	return ""
}

// provedoresGratuitos são os domínios de e-mail gratuito mais comuns no Brasil.
var provedoresGratuitos = map[string]bool{
	"gmail.com": true, "hotmail.com": true, "hotmail.com.br": true, "outlook.com": true, "outlook.com.br": true,
	"yahoo.com": true, "yahoo.com.br": true, "live.com": true, "msn.com": true, "icloud.com": true,
	"uol.com.br": true, "bol.com.br": true, "terra.com.br": true, "ig.com.br": true, "globo.com": true,
	"globomail.com": true, "r7.com": true, "zipmail.com.br": true, "oi.com.br": true, "me.com": true,
	"ymail.com": true, "protonmail.com": true, "proton.me": true, "aol.com": true,
}

// classificarEmail repete, para e-mails que não vieram classificados da base,
// a regra da carga: contabilidade pelas palavras do endereço, gratuito pelo
// domínio, senão corporativo.
func classificarEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, p := range []string{"contab", "contador", "escritorio", "assessoria"} {
		if strings.Contains(email, p) {
			return "contabilidade"
		}
	}
	if i := strings.LastIndexByte(email, '@'); i >= 0 && provedoresGratuitos[email[i+1:]] {
		return "gratuito"
	}
	return "corporativo"
}
