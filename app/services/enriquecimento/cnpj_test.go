package enriquecimento

import "testing"

func TestValidarCNPJ(t *testing.T) {
	validos := []string{"00000000000191", "00.000.000/0001-91", "11222333000181", "11.222.333/0001-81"}
	for _, c := range validos {
		if !ValidarCNPJ(c) {
			t.Errorf("%s deveria ser válido", c)
		}
	}
	invalidos := []string{"", "123", "11111111111111", "11222333000182", "00000000000190", "1122233300018"}
	for _, c := range invalidos {
		if ValidarCNPJ(c) {
			t.Errorf("%s deveria ser inválido", c)
		}
	}
}

func TestFormatadores(t *testing.T) {
	if got := FormatarCNPJ("11222333000181"); got != "11.222.333/0001-81" {
		t.Errorf("FormatarCNPJ = %q", got)
	}
	if got := FormatarTelefone("8533334444"); got != "(85) 3333-4444" {
		t.Errorf("telefone fixo = %q", got)
	}
	if got := FormatarTelefone("+5585999990000"); got != "(85) 99999-0000" {
		t.Errorf("telefone celular = %q", got)
	}
	if got := FormatarCNAE("6201501"); got != "6201-5/01" {
		t.Errorf("CNAE = %q", got)
	}
	if got := FormatarCNAE("6201-5/01"); got != "6201-5/01" {
		t.Errorf("CNAE já formatado = %q", got)
	}
}

func TestDominioDoEmail(t *testing.T) {
	casos := map[string]string{
		"contato@acme.com.br":   "acme.com.br",
		"Fulano@GMAIL.com":      "",
		"x@hotmail.com":         "",
		"x@uol.com.br":          "",
		"financeiro@empresa.io": "empresa.io",
		"sem-arroba":            "",
		"":                      "",
	}
	for email, esperado := range casos {
		if got := DominioDoEmail(email); got != esperado {
			t.Errorf("DominioDoEmail(%q) = %q, esperado %q", email, got, esperado)
		}
	}
}

// O CNPJ alfanumérico (em produção desde julho de 2026) passa pelas mesmas
// funções: validar, normalizar e formatar delegam a models.
func TestCNPJAlfanumerico(t *testing.T) {
	if !ValidarCNPJ("12.ABC.345/01DE-35") || !ValidarCNPJ("12abc34501de35") {
		t.Error("o exemplo oficial do CNPJ alfanumérico deveria ser válido")
	}
	if ValidarCNPJ("12ABC34501DE36") || ValidarCNPJ("12ABC34501DEX5") {
		t.Error("dígito verificador errado ou com letra deveria ser inválido")
	}
	if got := NormalizarCNPJ(" 12.abc.345/01de-35 "); got != "12ABC34501DE35" {
		t.Errorf("NormalizarCNPJ = %q", got)
	}
	if got := NormalizarCNPJ("11.222.333/0001-81"); got != "11222333000181" {
		t.Errorf("NormalizarCNPJ numérico = %q", got)
	}
	// SomenteDigitos não serve para CNPJ: comeria as letras e viraria outro número.
	if SomenteDigitos("12ABC34501DE35") == NormalizarCNPJ("12ABC34501DE35") {
		t.Error("SomenteDigitos e NormalizarCNPJ deveriam divergir num CNPJ com letras")
	}
	if got := FormatarCNPJ("12abc34501de35"); got != "12.ABC.345/01DE-35" {
		t.Errorf("FormatarCNPJ alfanumérico = %q", got)
	}
	if !CNPJSoNumeros("11.222.333/0001-81") || CNPJSoNumeros("12ABC34501DE35") || CNPJSoNumeros("") {
		t.Error("CNPJSoNumeros distingue o CNPJ que provedores antigos aceitam")
	}
}
