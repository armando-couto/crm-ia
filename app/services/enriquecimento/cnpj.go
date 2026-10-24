// Package enriquecimento consulta fontes externas (Receita Federal, BigDataCorp,
// Serasa Experian) e normaliza o que elas devolvem num Perfil único, que a
// ficha da empresa mostra e o vendedor escolhe aplicar.
package enriquecimento

import "strings"

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

// ValidarCNPJ confere tamanho, sequência repetida e os dois dígitos verificadores.
func ValidarCNPJ(s string) bool {
	d := SomenteDigitos(s)
	if len(d) != 14 {
		return false
	}
	repetido := true
	for i := 1; i < 14; i++ {
		if d[i] != d[0] {
			repetido = false
			break
		}
	}
	if repetido {
		return false
	}
	calc := func(n int, pesos []int) int {
		soma := 0
		for i := 0; i < n; i++ {
			soma += int(d[i]-'0') * pesos[i]
		}
		resto := soma % 11
		if resto < 2 {
			return 0
		}
		return 11 - resto
	}
	p1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	p2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	return calc(12, p1) == int(d[12]-'0') && calc(13, p2) == int(d[13]-'0')
}

// FormatarCNPJ devolve 00.000.000/0000-00 (ou a entrada, se não tiver 14 dígitos).
func FormatarCNPJ(s string) string {
	d := SomenteDigitos(s)
	if len(d) != 14 {
		return s
	}
	return d[0:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:14]
}

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
