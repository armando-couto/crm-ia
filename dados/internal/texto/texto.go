// Package texto reúne as regras de limpeza aplicadas na carga: texto,
// datas, capital, CPF em nomes, telefone, e-mail e CNPJ. Tudo é feito em Go
// (o serviço não depende de extensão do PostgreSQL).
package texto

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Limpa tira caracteres de controle (C0, DEL e C1, inclusive TAB e quebra de
// linha), junta espaços e apara. Vazio continua vazio (vira NULL na carga).
func Limpa(s string) string {
	limpo := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c >= 0x7f || (c == ' ' && (i == 0 || i == len(s)-1 || s[i+1] == ' ')) {
			limpo = false
			break
		}
	}
	if limpo {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	espaco := true // engole espaços do começo
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) || unicode.IsSpace(r) || r == unicode.ReplacementChar {
			r = ' '
		}
		if r == ' ' {
			if !espaco {
				b.WriteByte(' ')
			}
			espaco = true
			continue
		}
		espaco = false
		b.WriteRune(r)
	}
	return strings.TrimRight(b.String(), " ")
}

// SemAcento devolve o texto em maiúsculas, sem acentos e sem espaços
// repetidos: é a forma gravada nas colunas de busca e a forma em que o
// termo digitado é comparado.
func SemAcento(s string) string {
	s = Limpa(s)
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToUpper(s)
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

// SoDigitos devolve apenas os dígitos de s.
func SoDigitos(s string) string {
	limpo := true
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			limpo = false
			break
		}
	}
	if limpo {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b = append(b, s[i])
		}
	}
	return string(b)
}

func soDigitos(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Data converte AAAAMMDD em AAAA-MM-DD. Zero, vazio e data impossível
// (mês 13, 31 de fevereiro) devolvem "".
func Data(v string) string {
	if len(v) != 8 || !soDigitos(v) {
		return ""
	}
	ano := int(v[0]-'0')*1000 + int(v[1]-'0')*100 + int(v[2]-'0')*10 + int(v[3]-'0')
	mes := int(v[4]-'0')*10 + int(v[5]-'0')
	dia := int(v[6]-'0')*10 + int(v[7]-'0')
	if !dataValida(ano, mes, dia) {
		return ""
	}
	return v[0:4] + "-" + v[4:6] + "-" + v[6:8]
}

// DataBR converte dd/mm/aaaa (PGFN) em AAAA-MM-DD; a sentinela 01/01/1000 e
// datas impossíveis devolvem "".
func DataBR(v string) string {
	if len(v) != 10 || v[2] != '/' || v[5] != '/' {
		return ""
	}
	return Data(v[6:10] + v[3:5] + v[0:2])
}

func dataValida(ano, mes, dia int) bool {
	if ano < 1800 || ano > 2100 || mes < 1 || mes > 12 || dia < 1 {
		return false
	}
	dias := [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}[mes-1]
	if mes == 2 && (ano%4 == 0 && (ano%100 != 0 || ano%400 == 0)) {
		dias = 29
	}
	return dia <= dias
}

var reCapital = regexp.MustCompile(`^[0-9]{1,16}(,[0-9]{1,2})?$`)

// Capital converte "1000,00" (vírgula decimal) em "1000.00"; fora do formato
// devolve "".
func Capital(v string) string {
	v = strings.TrimSpace(v)
	if !reCapital.MatchString(v) {
		return ""
	}
	return strings.Replace(v, ",", ".", 1)
}

// Codigo devolve v se tiver exatamente n dígitos, senão "".
func Codigo(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) != n || !soDigitos(v) {
		return ""
	}
	return v
}

// ---------------------------------------------------------------- CPF

// CPFValido confere os dois dígitos verificadores de 11 dígitos. Sequências
// de um dígito só (000.000.000-00) não contam como CPF.
func CPFValido(d string) bool {
	if len(d) != 11 || !soDigitos(d) {
		return false
	}
	iguais := true
	for i := 1; i < 11; i++ {
		if d[i] != d[0] {
			iguais = false
			break
		}
	}
	if iguais {
		return false
	}
	for _, n := range []int{9, 10} {
		soma := 0
		for i := 0; i < n; i++ {
			soma += int(d[i]-'0') * (n + 1 - i)
		}
		dv := (soma * 10) % 11
		if dv == 10 {
			dv = 0
		}
		if dv != int(d[n]-'0') {
			return false
		}
	}
	return true
}

// Cortes conta os CPFs removidos de nomes, por padrão encontrado.
type Cortes struct {
	Fim       int64 `json:"fim"`       // 11 dígitos no fim do texto
	Meio      int64 `json:"meio"`      // 11 dígitos em outra posição
	Formatado int64 `json:"formatado"` // NNN.NNN.NNN-NN em qualquer posição
}

func (c *Cortes) Somar(o Cortes) {
	c.Fim += o.Fim
	c.Meio += o.Meio
	c.Formatado += o.Formatado
}

func (c Cortes) Total() int64 { return c.Fim + c.Meio + c.Formatado }

var reCPFFormatado = regexp.MustCompile(`[0-9]{3}\.[0-9]{3}\.[0-9]{3}-[0-9]{2}`)

func digito(s string, i int) bool { return i >= 0 && i < len(s) && s[i] >= '0' && s[i] <= '9' }

// RemoveCPF tira de um nome (razão social ou nome fantasia, de qualquer
// natureza jurídica) toda sequência de 11 dígitos sem dígito vizinho que
// passe nos dígitos verificadores do CPF e todo NNN.NNN.NNN-NN que passe, em
// qualquer posição. O prefixo novo do MEI (NN.NNN.NNN, raiz do CNPJ) fica.
func RemoveCPF(s string) (string, Cortes) {
	var c Cortes
	if len(s) < 11 {
		return s, c
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			n++
		}
	}
	if n < 11 {
		return s, c
	}
	// 1) formatado
	if strings.IndexByte(s, '-') >= 0 {
		var b strings.Builder
		ultimo := 0
		for _, m := range reCPFFormatado.FindAllStringIndex(s, -1) {
			if digito(s, m[0]-1) || digito(s, m[1]) || !CPFValido(SoDigitos(s[m[0]:m[1]])) {
				continue
			}
			b.WriteString(s[ultimo:m[0]])
			b.WriteByte(' ')
			ultimo = m[1]
			c.Formatado++
		}
		if c.Formatado > 0 {
			b.WriteString(s[ultimo:])
			s = b.String()
		}
	}
	// 2) 11 dígitos seguidos, sem dígito vizinho
	var b strings.Builder
	ultimo, cortou := 0, false
	for i := 0; i < len(s); {
		if !digito(s, i) {
			i++
			continue
		}
		j := i
		for digito(s, j) {
			j++
		}
		if j-i == 11 && CPFValido(s[i:j]) {
			b.WriteString(s[ultimo:i])
			b.WriteByte(' ')
			ultimo = j
			cortou = true
			if strings.TrimSpace(s[j:]) == "" {
				c.Fim++
			} else {
				c.Meio++
			}
		}
		i = j
	}
	if cortou {
		b.WriteString(s[ultimo:])
		s = b.String()
	}
	if c.Total() == 0 {
		return s, c
	}
	return strings.Trim(Limpa(s), " -/.,"), c
}

// MascaraCPF devolve o CPF no formato da Receita (***123456**). Documento
// que já vem mascarado ou que não é CPF volta como veio.
func MascaraCPF(doc string) string {
	d := SoDigitos(doc)
	if len(d) == 11 && len(doc) >= 11 && !strings.Contains(doc, "*") {
		return "***" + d[3:9] + "**"
	}
	return doc
}

// ---------------------------------------------------------------- telefone

// Tipos de telefone do contrato.
const (
	Celular    = "celular"
	Fixo       = "fixo"
	Indefinido = "indefinido"
)

// RegraTelefone é a versão da regra abaixo, gravada na ficha junto dos
// valores crus para reclassificar quando a numeração mudar (01/03/2027).
const RegraTelefone = 1

var dddValido = func() [100]bool {
	var t [100]bool
	for _, d := range []int{11, 12, 13, 14, 15, 16, 17, 18, 19, 21, 22, 24, 27, 28, 31, 32, 33, 34, 35, 37, 38,
		41, 42, 43, 44, 45, 46, 47, 48, 49, 51, 53, 54, 55, 61, 62, 63, 64, 65, 66, 67, 68, 69,
		71, 73, 74, 75, 77, 79, 81, 82, 83, 84, 85, 86, 87, 88, 89, 91, 92, 93, 94, 95, 96, 97, 98, 99} {
		t[d] = true
	}
	return t
}()

// DDDValido diz se o código nacional existe no plano de numeração.
func DDDValido(d string) bool {
	return len(d) == 2 && soDigitos(d) && dddValido[int(d[0]-'0')*10+int(d[1]-'0')]
}

// Telefone aplica a regra do contrato ao par (DDD, telefone) da Receita, que
// publica no máximo 8 dígitos. Devolve o número só com dígitos (celular: 11;
// fixo e indefinido: 10) e o tipo; número descartado devolve ("", "").
func Telefone(ddd, tel string) (numero, tipo string) {
	d := strings.TrimLeft(SoDigitos(ddd), "0")
	t := SoDigitos(tel)
	if !DDDValido(d) {
		return "", ""
	}
	if len(t) == 9 && t[0] == '9' && !lixo(t[1:]) {
		return d + t, Celular // já veio com o nono dígito
	}
	if len(t) != 8 || lixo(t) {
		return "", ""
	}
	switch t[0] {
	case '6', '7', '8', '9':
		return d + "9" + t, Celular
	case '2', '3':
		return d + t, Fixo
	case '4', '5':
		if d == "11" {
			return d + t, Indefinido // no DDD 11 metade destes é celular
		}
		return d + t, Fixo
	case '1':
		if d == "11" {
			return d + "9" + t, Celular
		}
	}
	return "", ""
}

// lixo: começa por zero, dígito repetido, sequência crescente ou decrescente
// e o padrão X0000000.
func lixo(t string) bool {
	if t[0] == '0' {
		return true
	}
	iguais, sobe, desce, zeros := true, true, true, true
	for i := 1; i < len(t); i++ {
		if t[i] != t[0] {
			iguais = false
		}
		if t[i] != t[i-1]+1 {
			sobe = false
		}
		if t[i] != t[i-1]-1 {
			desce = false
		}
		if t[i] != '0' {
			zeros = false
		}
	}
	return iguais || sobe || desce || zeros
}

// ---------------------------------------------------------------- e-mail

var reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Email devolve o endereço em minúsculas ou "" quando não tem forma de
// e-mail.
func Email(v string) string {
	v = strings.ToLower(Limpa(v))
	if len(v) > 115 || !reEmail.MatchString(v) {
		return ""
	}
	return v
}

// Tipos de e-mail do contrato.
const (
	EmailCorporativo   = "corporativo"
	EmailGratuito      = "gratuito"
	EmailContabilidade = "contabilidade"
)

var provedoresGratuitos = map[string]bool{
	"gmail.com": true, "hotmail.com": true, "hotmail.com.br": true, "outlook.com": true, "outlook.com.br": true,
	"yahoo.com": true, "yahoo.com.br": true, "ymail.com": true, "live.com": true, "live.com.br": true, "msn.com": true,
	"icloud.com": true, "me.com": true, "bol.com.br": true, "uol.com.br": true, "terra.com.br": true, "ig.com.br": true,
	"globo.com": true, "globomail.com": true, "r7.com": true, "oi.com.br": true, "zipmail.com.br": true,
	"aol.com": true, "protonmail.com": true, "proton.me": true, "rocketmail.com": true, "superig.com.br": true,
	"pop.com.br": true, "brturbo.com.br": true, "gmail.com.br": true, "mail.com": true, "gmx.com": true,
}

// TipoEmail classifica o endereço pelo texto: contabilidade (contab,
// contador, escritorio, assessoria), provedor gratuito ou corporativo. O
// e-mail repetido em 5 ou mais empresas vira contabilidade na construção.
func TipoEmail(email string) string {
	if email == "" {
		return ""
	}
	for _, p := range []string{"contab", "contador", "escritorio", "assessoria"} {
		if strings.Contains(email, p) {
			return EmailContabilidade
		}
	}
	if i := strings.LastIndexByte(email, '@'); i >= 0 && provedoresGratuitos[email[i+1:]] {
		return EmailGratuito
	}
	return EmailCorporativo
}

// ---------------------------------------------------------------- CNPJ

// NormalizarCNPJ devolve o CNPJ em maiúsculas só com [0-9A-Z]; o resultado
// só é um CNPJ quando tem 14 caracteres (ver CNPJFormaValida).
func NormalizarCNPJ(v string) string {
	b := make([]byte, 0, 14)
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c >= '0' && c <= '9', c >= 'A' && c <= 'Z':
			b = append(b, c)
		case c >= 'a' && c <= 'z':
			b = append(b, c-32)
		}
	}
	return string(b)
}

// CNPJFormaValida confere a forma [0-9A-Z]{12}[0-9]{2} (o CNPJ alfanumérico
// entrou em produção em julho de 2026).
func CNPJFormaValida(c string) bool {
	if len(c) != 14 {
		return false
	}
	for i := 0; i < 12; i++ {
		if !(c[i] >= '0' && c[i] <= '9') && !(c[i] >= 'A' && c[i] <= 'Z') {
			return false
		}
	}
	return c[12] >= '0' && c[12] <= '9' && c[13] >= '0' && c[13] <= '9'
}

// DVCNPJ calcula os dois dígitos verificadores dos 12 primeiros caracteres
// pela regra alfanumérica (valor do caractere = código ASCII − 48).
func DVCNPJ(base12 string) string {
	calc := func(s string) byte {
		soma, peso := 0, 2
		for i := len(s) - 1; i >= 0; i-- {
			soma += int(s[i]-'0') * peso
			peso++
			if peso > 9 {
				peso = 2
			}
		}
		if r := soma % 11; r >= 2 {
			return byte('0' + 11 - r)
		}
		return '0'
	}
	d1 := calc(base12)
	d2 := calc(base12 + string(d1))
	return string([]byte{d1, d2})
}

// CNPJValido confere forma e dígitos verificadores.
func CNPJValido(c string) bool {
	return CNPJFormaValida(c) && DVCNPJ(c[:12]) == c[12:]
}
