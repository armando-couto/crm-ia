package models

import "strings"

// CNPJ é texto de 14 caracteres: doze alfanuméricos (raiz e ordem) e dois
// dígitos verificadores. O formato alfanumérico entrou em produção em julho de
// 2026 e convive com o numérico; todo código compara pela forma normalizada.

// CNPJNormSQL é a expressão SQL do CNPJ normalizado de companies (alias "c").
// É exatamente a expressão do índice idx_companies_cnpj_norm (migração 028):
// mudar um sem o outro faz a busca por CNPJ voltar a varrer a tabela.
const CNPJNormSQL = `upper(regexp_replace(COALESCE(c.cnpj,''), '[^0-9A-Za-z]', '', 'g'))`

// CNPJNormSQLColuna devolve a mesma expressão para outra referência de coluna
// (ex.: "cnpj" numa consulta sem alias).
func CNPJNormSQLColuna(coluna string) string {
	return `upper(regexp_replace(COALESCE(` + coluna + `,''), '[^0-9A-Za-z]', '', 'g'))`
}

// NormalizarCNPJ deixa só letras e números, em maiúsculas (sem pontuação).
// Não valida: o resultado pode ter qualquer tamanho.
func NormalizarCNPJ(s string) string {
	var b strings.Builder
	b.Grow(14)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'A' && c <= 'Z':
			b.WriteByte(c)
		case c >= 'a' && c <= 'z':
			b.WriteByte(c - 'a' + 'A')
		}
	}
	return b.String()
}

// CNPJFormatoValido confere só o desenho: 12 alfanuméricos e 2 dígitos.
func CNPJFormatoValido(s string) bool {
	c := NormalizarCNPJ(s)
	if len(c) != 14 {
		return false
	}
	return c[12] >= '0' && c[12] <= '9' && c[13] >= '0' && c[13] <= '9'
}

// CNPJValido confere tamanho, desenho, sequência repetida e os dois dígitos
// verificadores pela regra alfanumérica (valor do caractere = código ASCII − 48,
// o que mantém o cálculo antigo para CNPJ só com números).
func CNPJValido(s string) bool {
	c := NormalizarCNPJ(s)
	if !CNPJFormatoValido(c) {
		return false
	}
	repetido := true
	for i := 1; i < 14; i++ {
		if c[i] != c[0] {
			repetido = false
			break
		}
	}
	if repetido {
		return false
	}
	return cnpjDigito(c, 12) == int(c[12]-'0') && cnpjDigito(c, 13) == int(c[13]-'0')
}

// cnpjDigito calcula o dígito verificador sobre os n primeiros caracteres
// (pesos de 2 a 9, da direita para a esquerda, módulo 11).
func cnpjDigito(c string, n int) int {
	soma, peso := 0, 2
	for i := n - 1; i >= 0; i-- {
		soma += int(c[i]-'0') * peso
		peso++
		if peso > 9 {
			peso = 2
		}
	}
	resto := soma % 11
	if resto < 2 {
		return 0
	}
	return 11 - resto
}

// CNPJDigitosVerificadores devolve os dois dígitos para uma base de 12
// caracteres (útil em dados de demonstração e testes); vazio se a base for inválida.
func CNPJDigitosVerificadores(base string) string {
	b := NormalizarCNPJ(base)
	if len(b) != 12 {
		return ""
	}
	d1 := cnpjDigito(b, 12)
	b += string(rune('0' + d1))
	d2 := cnpjDigito(b, 13)
	return string(rune('0'+d1)) + string(rune('0'+d2))
}

// FormatarCNPJ devolve 00.000.000/0000-00 (ou a entrada, se não tiver 14 caracteres).
func FormatarCNPJ(s string) string {
	c := NormalizarCNPJ(s)
	if len(c) != 14 {
		return s
	}
	return c[0:2] + "." + c[2:5] + "." + c[5:8] + "/" + c[8:12] + "-" + c[12:14]
}

// CNPJRaiz devolve os 8 primeiros caracteres (a empresa, sem ordem nem DV).
func CNPJRaiz(s string) string {
	c := NormalizarCNPJ(s)
	if len(c) < 8 {
		return c
	}
	return c[:8]
}
