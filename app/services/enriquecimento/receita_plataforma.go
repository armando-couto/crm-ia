package enriquecimento

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/armando-couto/crm-ia/app/models"
)

// FontePlataforma é o nome da fonte no bruto quando a ficha vem da base
// própria da plataforma (painel → serviço de dados).
const FontePlataforma = "plataforma"

// prazoPlataforma limita a consulta à base própria: se ela demorar, a cadeia
// pública ainda tem tempo dentro do prazo da Receita.
var prazoPlataforma = 6 * time.Second

// consultarPlataforma consulta a base própria (FichaDaPlataforma). Devolve
// nil em qualquer falha (sem painel, fora do ar, limite, CNPJ ainda não
// carregado): o provedor "receita" segue para as APIs públicas.
func consultarPlataforma(ctx context.Context, cnpj string) (*Perfil, *Bruto) {
	ficha := FichaDaPlataforma
	if ficha == nil {
		return nil, nil
	}
	ctx, cancelar := context.WithTimeout(ctx, prazoPlataforma)
	defer cancelar()
	emp, err := ficha(ctx, cnpj)
	if err != nil || emp == nil || strings.TrimSpace(emp.RazaoSocial) == "" {
		return nil, nil
	}
	p := PerfilDaEmpresaPublica(emp)
	corpo, _ := json.Marshal(emp)
	return p, &Bruto{Fonte: FontePlataforma, Corpo: MascararCPFsJSON(corpo)}
}

// PerfilDaEmpresaPublica converte a ficha da base pública no Perfil da
// inteligência de empresas (provedor "receita").
func PerfilDaEmpresaPublica(emp *models.ProspEmpresa) *Perfil {
	p := NovoPerfil(models.ProvedorReceita)
	c := NormalizarCNPJ(emp.CNPJ)
	p.Set("cnpj", FormatarCNPJ(c))
	p.Set("legal_name", emp.RazaoSocial)
	p.Set("trade_name", emp.NomeFantasia)
	p.Set("registration_status", strings.ToUpper(emp.Situacao))
	if emp.SituacaoDesde != nil {
		p.Set("registration_status_at", NormalizarData(*emp.SituacaoDesde))
	}
	if emp.Abertura != nil {
		p.Set("founded_at", NormalizarData(*emp.Abertura))
	}
	if emp.CNAE.Codigo > 0 {
		p.Set("cnae_code", FormatarCNAE(zerosEsquerda(strconv.Itoa(emp.CNAE.Codigo), 7)))
	}
	p.Set("cnae_description", emp.CNAE.Descricao)
	p.Set("legal_nature", emp.Natureza.Descricao)
	switch emp.Porte {
	case models.PorteME:
		p.Set("company_size", "ME")
	case models.PorteEPP:
		p.Set("company_size", "EPP")
	case models.PorteDemais:
		p.Set("company_size", "DEMAIS")
	}
	if emp.Capital != nil {
		p.Set("capital", *emp.Capital)
	}
	p.Campos["simples"] = emp.Simples
	p.Campos["mei"] = emp.MEI
	e := emp.Endereco
	p.Set("street", tituloEndereco(strings.TrimSpace(e.TipoLogradouro+" "+e.Logradouro)))
	p.Set("street_number", e.Numero)
	p.Set("complement", e.Complemento)
	p.Set("neighborhood", tituloEndereco(e.Bairro))
	p.Set("zip", SomenteDigitos(e.CEP))
	p.Set("city", tituloEndereco(e.Municipio))
	p.Set("state", strings.ToUpper(e.UF))

	tels := []Telefone{}
	for i, t := range emp.Telefones {
		if t.Invalido {
			continue
		}
		n := models.NormalizarTelefone(t.Numero)
		if n == "" {
			continue
		}
		tipo := "fixo"
		if t.Tipo == models.TelefoneCelular {
			tipo = "celular"
		}
		tels = append(tels, Telefone{Numero: "+55" + n, Tipo: tipo, Principal: i == 0, WhatsApp: t.WhatsApp, NaoPerturbe: t.NaoPerturbe})
	}
	emails := []EmailContato{}
	if em := strings.ToLower(strings.TrimSpace(emp.Email)); em != "" {
		emails = append(emails, EmailContato{Email: em, Tipo: tipoEmail(em)})
		p.Set("email", em)
	}
	if len(tels) > 0 {
		p.Set("phone", FormatarTelefone(tels[0].Numero))
	}
	if len(tels) > 0 || len(emails) > 0 {
		p.Contatos = &Contatos{Telefones: tels, Emails: emails}
	}
	for _, cs := range emp.CNAEsSecundarios {
		if cs.Codigo > 0 {
			p.CNAEsSecundarios = append(p.CNAEsSecundarios, CNAE{Codigo: FormatarCNAE(zerosEsquerda(strconv.Itoa(cs.Codigo), 7)), Descricao: cs.Descricao})
		}
	}
	for _, s := range emp.Socios {
		tipo := "PF"
		switch s.Tipo {
		case "pj":
			tipo = "PJ"
		case "estrangeiro":
			tipo = "Estrangeiro"
		}
		desde := ""
		if s.Desde != nil {
			desde = NormalizarData(*s.Desde)
		}
		doc := s.Documento
		if tipo == "PF" {
			doc = MascararCPF(doc)
		}
		p.Socios = append(p.Socios, Socio{Nome: s.Nome, Documento: doc, Qualificacao: s.Qualificacao, Desde: desde, Tipo: tipo})
	}
	completarDerivados(p)
	return p
}
