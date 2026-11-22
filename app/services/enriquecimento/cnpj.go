// Package enriquecimento consulta fontes externas (Receita Federal, BigDataCorp,
// Serasa Experian) e normaliza o que elas devolvem num Perfil único, que a
// ficha da empresa mostra e o vendedor escolhe aplicar.
package enriquecimento

import (
	"context"
	"strings"

	"github.com/armando-couto/crm-ia/app/models"
)

// SomenteDigitos remove tudo que não é número.
func SomenteDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NormalizarCNPJ deixa o CNPJ com 14 caracteres maiúsculos, sem pontuação,
// aceitando letras (CNPJ alfanumérico). É a forma usada para consultar
// provedores, montar a chave de cache e comparar com o cadastro.
// SomenteDigitos continua valendo para CEP e telefone.
func NormalizarCNPJ(s string) string { return models.NormalizarCNPJ(s) }

// ValidarCNPJ confere tamanho, sequência repetida e os dois dígitos
// verificadores, pela regra alfanumérica (models.CNPJValido).
func ValidarCNPJ(s string) bool { return models.CNPJValido(s) }

// CNPJSoNumeros informa se o CNPJ normalizado tem só dígitos. Provedor que
// não aceita CNPJ com letras deve responder FalhaSemResultado quando for falso.
func CNPJSoNumeros(s string) bool {
	c := models.NormalizarCNPJ(s)
	return c != "" && c == SomenteDigitos(c)
}

// FormatarCNPJ devolve 00.000.000/0000-00 (ou a entrada, se não tiver 14 caracteres).
func FormatarCNPJ(s string) string { return models.FormatarCNPJ(s) }

// FichaDaPlataforma consulta um CNPJ na base própria da plataforma (painel →
// serviço de dados). É preenchida na subida da aplicação (application.go) com
// services.PlataformaCNPJ; fica nil quando o ambiente não tem painel, e o
// provedor "receita" segue direto para a cadeia de APIs públicas. Existe como
// variável porque este pacote não pode importar services (ciclo).
var FichaDaPlataforma func(ctx context.Context, cnpj string) (*models.ProspEmpresa, error)

// FormatarTelefone transforma dígitos em (DD) 0000-0000 / (DD) 90000-0000.
func FormatarTelefone(s string) string {
	d := SomenteDigitos(s)
	if strings.HasPrefix(d, "55") && len(d) > 11 {
		d = d[2:]
	}
	switch len(d) {
	case 10:
		return "(" + d[0:2] + ") " + d[2:6] + "-" + d[6:]
	case 11:
		return "(" + d[0:2] + ") " + d[2:7] + "-" + d[7:]
	case 8:
		return d[0:4] + "-" + d[4:]
	case 9:
		return d[0:5] + "-" + d[5:]
	}
	return strings.TrimSpace(s)
}

// FormatarCNAE transforma 6201501 em 6201-5/01.
func FormatarCNAE(s string) string {
	d := SomenteDigitos(s)
	if len(d) != 7 {
		return strings.TrimSpace(s)
	}
	return d[0:4] + "-" + d[4:5] + "/" + d[5:]
}

// dominiosGenericos são provedores de e-mail pessoais: não indicam o site da empresa.
var dominiosGenericos = map[string]bool{
	"gmail.com": true, "hotmail.com": true, "hotmail.com.br": true, "outlook.com": true, "outlook.com.br": true,
	"live.com": true, "msn.com": true, "yahoo.com": true, "yahoo.com.br": true, "uol.com.br": true,
	"bol.com.br": true, "terra.com.br": true, "ig.com.br": true, "globo.com": true, "globomail.com": true,
	"icloud.com": true, "me.com": true, "aol.com": true, "zipmail.com.br": true, "oi.com.br": true,
	"protonmail.com": true, "proton.me": true, "r7.com": true, "yandex.com": true,
}

// DominioDoEmail devolve o domínio corporativo do e-mail; vazio quando o
// provedor é genérico (gmail, hotmail…) ou o e-mail é inválido.
func DominioDoEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	if at < 1 || at == len(email)-1 {
		return ""
	}
	dom := email[at+1:]
	if !strings.Contains(dom, ".") || dominiosGenericos[dom] {
		return ""
	}
	return dom
}
