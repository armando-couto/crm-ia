package texto

import "testing"

func TestLimpa(t *testing.T) {
	casos := map[string]string{
		"  PADARIA   DO  ZE ":       "PADARIA DO ZE",
		"LOJA\tDA\nESQUINA":         "LOJA DA ESQUINA",
		"SALA\x1a 2\x7f\u008f":      "SALA 2",
		"":                          "",
		"   ":                       "",
		"AÇOUGUE SÃO JOSÉ":          "AÇOUGUE SÃO JOSÉ",
		"QUEBRA\r\nDENTRO DE ASPAS": "QUEBRA DENTRO DE ASPAS",
	}
	for entrada, esperado := range casos {
		if g := Limpa(entrada); g != esperado {
			t.Errorf("Limpa(%q) = %q, esperado %q", entrada, g, esperado)
		}
	}
}

func TestSemAcento(t *testing.T) {
	if g := SemAcento(" Açaí  da praça São João "); g != "ACAI DA PRACA SAO JOAO" {
		t.Fatalf("SemAcento = %q", g)
	}
}

func TestDatas(t *testing.T) {
	casos := map[string]string{"20240229": "2024-02-29", "20230229": "", "0": "", "00000000": "", "20241301": "", "20240431": "", "": "", "2024010": "", "19991231": "1999-12-31"}
	for e, s := range casos {
		if g := Data(e); g != s {
			t.Errorf("Data(%q) = %q, esperado %q", e, g, s)
		}
	}
	if DataBR("15/12/1992") != "1992-12-15" || DataBR("01/01/1000") != "" || DataBR("31/02/2020") != "" || DataBR("2020-01-01") != "" {
		t.Error("DataBR")
	}
}

func TestCapital(t *testing.T) {
	casos := map[string]string{"1000,00": "1000.00", "0,00": "0.00", "150000": "150000", "1.000,00": "", "abc": "", "": ""}
	for e, s := range casos {
		if g := Capital(e); g != s {
			t.Errorf("Capital(%q) = %q, esperado %q", e, g, s)
		}
	}
}

// CPFs de teste gerados pela regra dos dígitos verificadores (não são de
// ninguém em particular): 111.444.777-35 e 529.982.247-25.
func TestRemoveCPF(t *testing.T) {
	casos := []struct {
		entrada, saida string
		cortes         Cortes
	}{
		{"MARIA DA SILVA 11144477735", "MARIA DA SILVA", Cortes{Fim: 1}},                          // CPF no fim (natureza 2135)
		{"JOAO 52998224725 PINTURAS", "JOAO PINTURAS", Cortes{Meio: 1}},                           // CPF no meio
		{"JOSE SOUZA 111.444.777-35", "JOSE SOUZA", Cortes{Formatado: 1}},                         // formatado
		{"ANA 529.982.247-25 COSTURAS 11144477735", "ANA COSTURAS", Cortes{Fim: 1, Formatado: 1}}, // dois padrões
		{"61.114.725 MARIA DA SILVA", "61.114.725 MARIA DA SILVA", Cortes{}},                      // prefixo novo do MEI fica
		{"LOJA 12345678901", "LOJA 12345678901", Cortes{}},                                        // 11 dígitos que não são CPF
		{"TELE 111444777350", "TELE 111444777350", Cortes{}},                                      // 12 dígitos: tem dígito vizinho
		{"X 00000000000", "X 00000000000", Cortes{}},                                              // dígito repetido não é CPF
		{"POSTO 2000 LTDA", "POSTO 2000 LTDA", Cortes{}},
		{"11144477735", "", Cortes{Fim: 1}},
	}
	for _, c := range casos {
		g, n := RemoveCPF(c.entrada)
		if g != c.saida || n != c.cortes {
			t.Errorf("RemoveCPF(%q) = %q %+v, esperado %q %+v", c.entrada, g, n, c.saida, c.cortes)
		}
	}
}

func TestMascaraCPF(t *testing.T) {
	if MascaraCPF("11144477735") != "***444777**" || MascaraCPF("***444777**") != "***444777**" || MascaraCPF("12345678000195") != "12345678000195" {
		t.Fatal("MascaraCPF")
	}
}

func TestTelefone(t *testing.T) {
	casos := []struct{ ddd, tel, numero, tipo string }{
		{"85", "99887766", "85999887766", Celular}, // 6 a 9: celular, repõe o 9
		{"85", "88776655", "85988776655", Celular},
		{"85", "61234567", "85961234567", Celular},
		{"85", "32456789", "8532456789", Fixo}, // 2 a 5: fixo
		{"0085", "32456789", "8532456789", Fixo},
		{"21", "45671234", "2145671234", Fixo},
		{"11", "45671234", "1145671234", Indefinido}, // DDD 11 com 4 ou 5
		{"11", "56781234", "1156781234", Indefinido},
		{"11", "32456789", "1132456789", Fixo},
		{"11", "12345670", "11912345670", Celular}, // começa por 1: só no DDD 11
		{"85", "12345670", "", ""},
		{"85", "99999999", "", ""}, // dígito repetido
		{"85", "12345678", "", ""}, // sequência crescente
		{"85", "98765432", "", ""}, // sequência decrescente
		{"85", "30000000", "", ""}, // X0000000
		{"85", "01234567", "", ""}, // começa por zero
		{"85", "3245678", "", ""},  // 7 dígitos
		{"20", "32456789", "", ""}, // DDD inválido
		{"", "32456789", "", ""},   // sem DDD
		{"85", "", "", ""},         // vazio
		{"85", "3245-6789", "8532456789", Fixo},
		{"85", "999887766", "85999887766", Celular}, // 9 dígitos já com o nono
	}
	for _, c := range casos {
		n, tp := Telefone(c.ddd, c.tel)
		if n != c.numero || tp != c.tipo {
			t.Errorf("Telefone(%q, %q) = %q %q, esperado %q %q", c.ddd, c.tel, n, tp, c.numero, c.tipo)
		}
	}
}

func TestEmail(t *testing.T) {
	if Email(" Contato@Padaria.COM.BR ") != "contato@padaria.com.br" || Email("sem arroba") != "" || Email("a@b") != "" || Email("a b@c.com") != "" {
		t.Fatal("Email")
	}
	casos := map[string]string{
		"contato@padaria.com.br":       EmailCorporativo,
		"jose@gmail.com":               EmailGratuito,
		"fiscal@contabilidadeabc.com":  EmailContabilidade,
		"escritorio.silva@hotmail.com": EmailContabilidade,
		"":                             "",
	}
	for e, s := range casos {
		if g := TipoEmail(e); g != s {
			t.Errorf("TipoEmail(%q) = %q, esperado %q", e, g, s)
		}
	}
}

func TestCNPJ(t *testing.T) {
	if NormalizarCNPJ("12.abc.345/01de-35") != "12ABC34501DE35" {
		t.Fatal("NormalizarCNPJ")
	}
	// Exemplo oficial do CNPJ alfanumérico (Receita/Serpro): 12.ABC.345/01DE-35.
	if !CNPJValido("12ABC34501DE35") {
		t.Error("CNPJ alfanumérico de exemplo deveria ser válido")
	}
	if !CNPJValido("11222333000181") || CNPJValido("11222333000182") || CNPJValido("1122233300018") || CNPJValido("11222333000A81") {
		t.Error("CNPJValido")
	}
	if DVCNPJ("000000010001") != "36" {
		t.Errorf("DVCNPJ = %s", DVCNPJ("000000010001"))
	}
}
