package enriquecimento

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// -----------------------------------------------------------------------------
// Do Perfil (ficha da empresa) e do resultado do lote para o PerfilContato do
// item da lista de prospecção (docs/PROSPECCAO.md, seção 7). Telefones saem só
// com dígitos e DDD; CPF sempre mascarado.
// -----------------------------------------------------------------------------

// telefoneContato converte um número qualquer (+55…, (85) 9…) no telefone do
// contrato; ok = false quando não é um número brasileiro com DDD.
func telefoneContato(numero string, whatsapp, naoPerturbe *bool, pessoa string) (models.ProspTelefone, bool) {
	n := models.NormalizarTelefone(numero)
	if n == "" {
		return models.ProspTelefone{}, false
	}
	tipo := models.TelefoneFixo
	if len(n) == 11 {
		tipo = models.TelefoneCelular
	}
	return models.ProspTelefone{
		Numero: n, Tipo: tipo, WhatsApp: whatsapp, NaoPerturbe: naoPerturbe,
		Origem: models.OrigemContatoWhoLead, Pessoa: pessoa,
	}, true
}

// tipoEmailContato usa os rótulos do contrato (corporativo | gratuito).
func tipoEmailContato(email string) string {
	if DominioDoEmail(email) != "" {
		return "corporativo"
	}
	return "gratuito"
}

// PerfilContatoDePerfil resume o Perfil no formato do item da lista.
func PerfilContatoDePerfil(p *Perfil) *models.PerfilContato {
	if p == nil {
		return nil
	}
	pc := &models.PerfilContato{
		Provedor:          p.Provedor,
		Demo:              p.Demo,
		ConsultadoEm:      p.ConsultadoEm.UTC().Format(time.RFC3339),
		Telefones:         []models.ProspTelefone{},
		Emails:            []models.EmailContato{},
		Pessoas:           []models.PessoaContato{},
		FuncionariosFaixa: p.Texto("employees_range"),
		FaturamentoFaixa:  p.Texto("revenue_range"),
		Site:              p.Texto("website"),
		Observacao:        p.Mensagem,
	}
	if p.ConsultadoEm.IsZero() {
		pc.ConsultadoEm = time.Now().UTC().Format(time.RFC3339)
	}
	if p.Contatos != nil {
		vistos := map[string]bool{}
		for _, t := range p.Contatos.Telefones {
			tc, ok := telefoneContato(t.Numero, t.WhatsApp, t.NaoPerturbe, "")
			if !ok || vistos[tc.Numero] {
				continue
			}
			vistos[tc.Numero] = true
			tc.Principal = t.Principal
			pc.Telefones = append(pc.Telefones, tc)
		}
		emails := map[string]bool{}
		for _, e := range p.Contatos.Emails {
			email := strings.ToLower(strings.TrimSpace(e.Email))
			if email == "" || emails[email] {
				continue
			}
			emails[email] = true
			ec := models.EmailContato{Email: email, Tipo: tipoEmailContato(email)}
			if e.Validado {
				v := true
				ec.Validado = &v
			}
			pc.Emails = append(pc.Emails, ec)
		}
	}
	for _, pessoa := range p.Pessoas {
		pessoa.Documento = MascararCPF(pessoa.Documento)
		pc.Pessoas = juntarPessoa(pc.Pessoas, pessoa)
	}
	return pc
}

// papelDaQualificacao decide o papel de persons[] pela qualificação.
func papelDaQualificacao(q string) string {
	u := strings.ToLower(q)
	if strings.Contains(u, "represent") || strings.Contains(u, "responsável") || strings.Contains(u, "responsavel") {
		return PapelRepresentante
	}
	return PapelSocio
}

// PerfilContatoDeResultado monta o PerfilContato de um item do lote: o
// cnpj_data pelo mapeador tolerante e as pessoas de persons[] (com os
// telefones de cada uma). Formato desconhecido não é erro: fica só com as
// pessoas e a observação. Devolve também o Perfil (nil em formato
// desconhecido), para quem quiser gravar a inteligência da empresa.
func PerfilContatoDeResultado(r ResultadoWhoLead, agora time.Time) (*models.PerfilContato, *Perfil, string) {
	p, formato, err := ParseCnpjData(r.CnpjData, models.ProvedorWhoLead, r.CNPJ)
	if err != nil {
		p, formato = nil, FormatoDesconhecido
	}
	var pc *models.PerfilContato
	if p != nil {
		p.ConsultadoEm = agora.UTC().Truncate(time.Second)
		pc = PerfilContatoDePerfil(p)
	} else {
		pc = &models.PerfilContato{
			Provedor: models.ProvedorWhoLead, ConsultadoEm: agora.UTC().Format(time.RFC3339),
			Telefones: []models.ProspTelefone{}, Emails: []models.EmailContato{}, Pessoas: []models.PessoaContato{},
		}
		if len(r.CnpjData) > 0 && string(r.CnpjData) != "null" {
			pc.Observacao = "dados da empresa num formato ainda não reconhecido; só as pessoas foram lidas"
		}
	}
	for _, pw := range r.Pessoas {
		pessoa := models.PessoaContato{
			Nome: pw.Nome, Documento: MascararCPF(pw.CPF), Papel: papelDaQualificacao(pw.Qualificacao),
			Cargo: pw.Qualificacao, Telefones: []models.ProspTelefone{}, Emails: []string{},
		}
		vistos := map[string]bool{}
		for _, t := range pw.Telefones {
			if tc, ok := telefoneContato(t, nil, nil, strings.TrimSpace(pw.Nome)); ok && !vistos[tc.Numero] {
				vistos[tc.Numero] = true
				pessoa.Telefones = append(pessoa.Telefones, tc)
			}
		}
		pc.Pessoas = juntarPessoa(pc.Pessoas, pessoa)
	}
	return pc, p, formato
}

// PerfilContatoTemContato diz se o enriquecimento trouxe algum telefone ou
// e-mail (da empresa ou de alguma pessoa).
func PerfilContatoTemContato(pc *models.PerfilContato) bool {
	if pc == nil {
		return false
	}
	if len(pc.Telefones) > 0 || len(pc.Emails) > 0 {
		return true
	}
	for _, p := range pc.Pessoas {
		if len(p.Telefones) > 0 || len(p.Emails) > 0 {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Demonstração
// -----------------------------------------------------------------------------

// preencherDemoWhoLead completa o perfil fictício do Who Lead: telefones com
// WhatsApp e "não perturbe", e-mails, sócio com celular e responsável legal.
func preencherDemoWhoLead(p *Perfil, s *sorteio, faixa int) {
	ddd := 11 + s.ate(80)
	sim, nao := true, false
	cel := fmt.Sprintf("+55%d9%08d", ddd, s.ate(100000000))
	p.Contatos = &Contatos{
		Telefones: []Telefone{
			{Numero: cel, Tipo: "celular", Principal: true, Validado: true, WhatsApp: &sim, NaoPerturbe: &nao},
			{Numero: fmt.Sprintf("+55%d9%08d", ddd, s.ate(100000000)), Tipo: "celular", WhatsApp: &nao, NaoPerturbe: &sim},
			{Numero: fmt.Sprintf("+55%d3%07d", ddd, s.ate(10000000)), Tipo: "fixo", Validado: s.ate(2) == 0},
		},
		Emails: []EmailContato{
			{Email: "contato@empresa-demo.com.br", Tipo: "corporativo", Validado: true},
			{Email: "comercial@empresa-demo.com.br", Tipo: "corporativo"},
		},
	}
	p.Set("phone", FormatarTelefone(p.Contatos.Telefones[2].Numero))
	p.Set("email", "contato@empresa-demo.com.br")
	p.Set("employees_range", faixasFuncionarios[faixa])
	p.Set("revenue_range", faixasFaturamento[faixa])
	celSocio := fmt.Sprintf("%d9%08d", ddd, s.ate(100000000))
	tSocio, _ := telefoneContato(celSocio, &sim, &nao, "SÓCIO DEMONSTRAÇÃO")
	p.Socios = []Socio{{Nome: "SÓCIO DEMONSTRAÇÃO", Documento: "***000000**", Qualificacao: "Sócio-Administrador", Desde: "2015-03-10", Tipo: "PF"}}
	p.Pessoas = []models.PessoaContato{
		{Nome: "SÓCIO DEMONSTRAÇÃO", Documento: "***000000**", Papel: PapelSocio, Cargo: "Sócio-Administrador",
			Telefones: []models.ProspTelefone{tSocio}, Emails: []string{"socio@empresa-demo.com.br"}},
		{Nome: "RESPONSÁVEL DEMONSTRAÇÃO", Documento: "***000001**", Papel: PapelRepresentante, Cargo: "Responsável legal",
			Telefones: []models.ProspTelefone{}, Emails: []string{}},
	}
}

// PerfilContatoDemo devolve o PerfilContato fictício e determinístico do Who
// Lead para o CNPJ (modo demonstração do lote: custo zero, sempre rotulado).
func PerfilContatoDemo(cnpj string) (*models.PerfilContato, error) {
	d := &Demo{Fonte: models.ProvedorWhoLead}
	p, _, err := d.Consultar(context.Background(), cnpj, Opcoes{})
	if err != nil {
		return nil, err
	}
	pc := PerfilContatoDePerfil(p)
	pc.Observacao = "dados de demonstração"
	return pc, nil
}
