package services

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
)

// -----------------------------------------------------------------------------
// Exportação da lista de prospecção em CSV (GET /prospeccao/listas/{id}/exportar):
// uma linha por empresa, com o retrato da base pública e os contatos do
// enriquecimento. O bruto do provedor nunca entra (nem é lido) e nenhum
// documento de pessoa sai, nem mascarado.
// -----------------------------------------------------------------------------

// ColunasExportacaoLista é o cabeçalho do CSV, na ordem das colunas.
var ColunasExportacaoLista = []string{
	"cnpj", "razao_social", "nome_fantasia", "estado_item", "nota", "motivos_nota",
	"uf", "municipio", "bairro", "cep", "endereco",
	"cnae", "cnae_descricao", "porte", "natureza", "abertura", "capital", "simples", "mei", "pessoa_fisica",
	"telefone_principal", "telefones", "emails", "decisores", "site",
	"tem_celular", "tem_whatsapp", "tem_divida", "divida_total",
	"enriquecido_em", "provedor", "enviado_em",
}

// LinhaExportacaoItem monta a linha do CSV de um item lido com dados e
// enriquecimento (models.ProspItensCompletos).
func LinhaExportacaoItem(it *models.ProspItem) []string {
	sim := func(b bool) string {
		if b {
			return "sim"
		}
		return "nao"
	}
	dados := it.Dados
	if dados == nil {
		dados = &models.ProspEmpresa{}
	}
	end := dados.Endereco
	endereco := strings.Join(naoVazios(
		strings.TrimSpace(end.TipoLogradouro+" "+end.Logradouro), end.Numero, end.Complemento), ", ")

	motivos := make([]string, 0, len(it.NotaMotivos))
	for _, m := range it.NotaMotivos {
		motivos = append(motivos, fmt.Sprintf("%s (%+d)", m.Texto, m.Pontos))
	}

	telefones := []string{}
	for _, t := range TelefonesDoItem(it.Dados, it.Enriquecimento) {
		desc := t.Numero + " (" + t.Tipo
		if t.Pessoa != "" {
			desc += ", " + t.Pessoa
		}
		if t.WhatsApp != nil && *t.WhatsApp {
			desc += ", WhatsApp"
		}
		if t.NaoPerturbe != nil && *t.NaoPerturbe {
			desc += ", não perturbe"
		}
		if t.Compartilhado {
			desc += ", compartilhado"
		}
		telefones = append(telefones, desc+")")
	}

	emails := []string{}
	vistoEmail := map[string]bool{}
	addEmail := func(e string) {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" && !vistoEmail[e] {
			vistoEmail[e] = true
			emails = append(emails, e)
		}
	}
	addEmail(dados.Email)

	decisores := []string{}
	site := ""
	if p := it.Enriquecimento; p != nil {
		for _, e := range p.Emails {
			addEmail(e.Email)
		}
		for _, pe := range p.Pessoas {
			for _, e := range pe.Emails {
				addEmail(e)
			}
			if pe.Papel == "funcionario" || strings.TrimSpace(pe.Nome) == "" {
				continue
			}
			d := pe.Nome
			if pe.Cargo != "" {
				d += " (" + pe.Cargo + ")"
			}
			decisores = append(decisores, d)
		}
		site = p.Site
	}
	if len(decisores) == 0 {
		for _, s := range dados.Socios {
			if s.Tipo != "pj" && strings.TrimSpace(s.Nome) != "" && strings.Contains(strings.ToLower(s.Qualificacao), "administrador") {
				decisores = append(decisores, s.Nome+" ("+s.Qualificacao+")")
			}
		}
	}

	capital := ""
	if dados.Capital != nil {
		capital = strconv.FormatFloat(*dados.Capital, 'f', 2, 64)
	}
	divida := ""
	if dados.Divida != nil && dados.Divida.Total > 0 {
		divida = strconv.FormatFloat(dados.Divida.Total, 'f', 2, 64)
	}
	data := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	enriquecidoEm, enviadoEm := "", ""
	if it.EnriquecidoEm != nil {
		enriquecidoEm = it.EnriquecidoEm.Format("2006-01-02 15:04")
	}
	if it.EnviadoEm != nil {
		enviadoEm = it.EnviadoEm.Format("2006-01-02 15:04")
	}
	cnae := ""
	if it.CnaeCodigo > 0 {
		cnae = strconv.Itoa(it.CnaeCodigo)
	}

	linha := []string{
		models.FormatarCNPJ(it.CNPJ), it.RazaoSocial, it.NomeFantasia, it.Estado, strconv.Itoa(it.Nota), strings.Join(motivos, "; "),
		it.UF, it.Municipio, end.Bairro, end.CEP, endereco,
		cnae, it.CnaeDescricao, it.Porte, dados.Natureza.Descricao, data(dados.Abertura), capital,
		sim(dados.Simples), sim(dados.MEI), sim(it.PessoaFisica),
		it.TelefonePrincipal, strings.Join(telefones, " | "), strings.Join(emails, " | "), strings.Join(decisores, " | "), site,
		sim(it.TemCelular), sim(it.TemWhatsApp), sim(it.TemDivida), divida,
		enriquecidoEm, it.EnriquecimentoProvedor, enviadoEm,
	}
	for i := range linha {
		linha[i] = celulaCSVSegura(linha[i])
	}
	return linha
}

func naoVazios(vs ...string) []string {
	out := vs[:0]
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// celulaCSVSegura neutraliza fórmula de planilha: texto da base pública que
// comece por =, +, -, @ ou tabulação ganha um apóstrofo na frente.
func celulaCSVSegura(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
