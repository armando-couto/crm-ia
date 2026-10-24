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
