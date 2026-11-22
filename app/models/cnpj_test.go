package models

import "testing"

func TestNormalizarCNPJ(t *testing.T) {
	casos := map[string]string{
		"11.222.333/0001-81":  "11222333000181",
		" 12.abc.345/01de-35": "12ABC34501DE35",
		"12ABC34501DE35":      "12ABC34501DE35",
		"":                    "",
		"çã-/.":               "",
		"123":                 "123",
	}
	for entrada, quer := range casos {
		if got := NormalizarCNPJ(entrada); got != quer {
			t.Errorf("NormalizarCNPJ(%q) = %q, quer %q", entrada, got, quer)
		}
	}
}

func TestCNPJValido(t *testing.T) {
	validos := []string{
		"11.222.333/0001-81", // numérico
		"11222333000181",
		"12.ABC.345/01DE-35", // exemplo oficial do CNPJ alfanumérico (Serpro/RFB)
		"12abc34501de35",     // minúsculas são aceitas e normalizadas
		"00000000000191",     // Banco do Brasil
	}
	for _, c := range validos {
		if !CNPJValido(c) {
			t.Errorf("%q deveria ser válido", c)
		}
	}
	invalidos := []string{
		"", "123", "11222333000182", "12ABC34501DE36", "11111111111111", "00000000000000",
		"12ABC34501DEA5",  // letra no dígito verificador
		"112223330001810", // 15 caracteres
		"AAAAAAAAAAAAAA",
	}
	for _, c := range invalidos {
		if CNPJValido(c) {
			t.Errorf("%q deveria ser inválido", c)
		}
	}
}

func TestCNPJDigitosVerificadoresEFormato(t *testing.T) {
	if dv := CNPJDigitosVerificadores("12ABC34501DE"); dv != "35" {
		t.Errorf("DV de 12ABC34501DE = %q, quer 35", dv)
	}
	if dv := CNPJDigitosVerificadores("112223330001"); dv != "81" {
		t.Errorf("DV de 112223330001 = %q, quer 81", dv)
	}
	if dv := CNPJDigitosVerificadores("123"); dv != "" {
		t.Errorf("base curta deveria devolver vazio, veio %q", dv)
	}
	if f := FormatarCNPJ("12abc34501de35"); f != "12.ABC.345/01DE-35" {
		t.Errorf("FormatarCNPJ = %q", f)
	}
	if f := FormatarCNPJ("123"); f != "123" {
		t.Errorf("entrada curta deveria voltar intacta, veio %q", f)
	}
	if !CNPJFormatoValido("12ABC34501DE99") || CNPJFormatoValido("12ABC34501DEX9") {
		t.Error("CNPJFormatoValido confere só o desenho (12 alfanuméricos + 2 dígitos)")
	}
	if CNPJRaiz("12.ABC.345/01DE-35") != "12ABC345" {
		t.Error("CNPJRaiz deveria devolver os 8 primeiros caracteres")
	}
}
