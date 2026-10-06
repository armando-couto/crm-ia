package models

import "testing"

func TestSlugifyKey(t *testing.T) {
	cases := map[string]string{
		"Número do EC":               "numero_do_ec",
		"Cliente ou Não é Cliente ?": "cliente_ou_nao_e_cliente",
		"  Modalidade  ":             "modalidade",
		"TPV médio (R$)":             "tpv_medio_r",
		"123":                        "campo_123",
		"@@@":                        "campo",
	}
	for in, want := range cases {
		if got := SlugifyKey(in); got != want {
			t.Errorf("SlugifyKey(%q) = %q, esperado %q", in, got, want)
		}
	}
}

func TestValidateProperty(t *testing.T) {
	// Rótulo gera o nome interno automaticamente.
	p := &CustomProperty{Entity: "deals", Label: "Origem do lead", FieldType: FieldTexto}
	if err := ValidateProperty(p); err != nil {
		t.Fatalf("propriedade simples deveria ser válida: %v", err)
	}
	if p.Key != "origem_do_lead" {
		t.Fatalf("chave gerada inesperada: %s", p.Key)
	}
	if p.GroupName == "" {
		t.Fatal("grupo padrão deveria ser preenchido")
	}

	// Objeto inválido.
	if err := ValidateProperty(&CustomProperty{Entity: "faturas", Label: "X", FieldType: FieldTexto}); err == nil {
		t.Error("objeto inválido deveria falhar")
	}

	// Tipo inválido.
	if err := ValidateProperty(&CustomProperty{Entity: "deals", Label: "X", FieldType: "moeda"}); err == nil {
		t.Error("tipo inválido deveria falhar")
	}

	// Seleção sem opções.
	if err := ValidateProperty(&CustomProperty{Entity: "deals", Label: "Canal", FieldType: FieldSelecao}); err == nil {
		t.Error("seleção sem opções deveria falhar")
	}

	// Opções ganham valor interno a partir do rótulo.
	sel := &CustomProperty{
		Entity:    "companies",
		Label:     "Produtos contratados",
		FieldType: FieldMultipla,
		Options:   []PropertyOption{{Label: "Link de pagamento"}, {Label: "Pix"}},
	}
	if err := ValidateProperty(sel); err != nil {
		t.Fatalf("multipla válida falhou: %v", err)
	}
	if sel.Options[0].Value != "link_de_pagamento" || sel.Options[1].Value != "pix" {
		t.Fatalf("valores internos inesperados: %+v", sel.Options)
	}

	// Valores internos duplicados.
	dup := &CustomProperty{
		Entity:    "deals",
		Label:     "Canal",
		FieldType: FieldSelecao,
		Options:   []PropertyOption{{Value: "site", Label: "Site"}, {Value: "site", Label: "Website"}},
	}
	if err := ValidateProperty(dup); err == nil {
		t.Error("opções duplicadas deveriam falhar")
	}
}

func TestNormalizePropertyValue(t *testing.T) {
	texto := CustomProperty{Label: "Obs", FieldType: FieldTexto}
	if v, empty, err := NormalizePropertyValue(texto, "  olá  "); err != nil || empty || v != "olá" {
		t.Errorf("texto: %v %v %v", v, empty, err)
	}
	if _, empty, _ := NormalizePropertyValue(texto, "   "); !empty {
		t.Error("texto vazio deveria limpar a propriedade")
	}

	numero := CustomProperty{Label: "TPV", FieldType: FieldNumero}
	if v, _, err := NormalizePropertyValue(numero, "1234,50"); err != nil || v.(float64) != 1234.5 {
		t.Errorf("número com vírgula: %v %v", v, err)
	}
	if _, _, err := NormalizePropertyValue(numero, "muito"); err == nil {
		t.Error("número inválido deveria falhar")
	}

	data := CustomProperty{Label: "Credenciamento", FieldType: FieldData}
	if v, _, err := NormalizePropertyValue(data, "2024-05-10T00:00:00Z"); err != nil || v != "2024-05-10" {
		t.Errorf("data: %v %v", v, err)
	}
	if _, _, err := NormalizePropertyValue(data, "10/05/2024"); err == nil {
		t.Error("data em formato errado deveria falhar")
	}

	sel := CustomProperty{Label: "Canal", FieldType: FieldSelecao, Options: []PropertyOption{{Value: "site", Label: "Site"}}}
	if v, _, err := NormalizePropertyValue(sel, "site"); err != nil || v != "site" {
		t.Errorf("seleção válida: %v %v", v, err)
	}
	if _, _, err := NormalizePropertyValue(sel, "indicacao"); err == nil {
		t.Error("opção fora da lista deveria falhar")
	}

	multi := CustomProperty{
		Label:     "Produtos",
		FieldType: FieldMultipla,
		Options:   []PropertyOption{{Value: "pix", Label: "Pix"}, {Value: "link", Label: "Link"}},
	}
	if v, _, err := NormalizePropertyValue(multi, []any{"pix", "link"}); err != nil || len(v.([]string)) != 2 {
		t.Errorf("múltipla válida: %v %v", v, err)
	}
	if _, _, err := NormalizePropertyValue(multi, []any{"boleto"}); err == nil {
		t.Error("opção inválida na múltipla deveria falhar")
	}
	if _, empty, _ := NormalizePropertyValue(multi, []any{}); !empty {
		t.Error("lista vazia deveria limpar a propriedade")
	}

	boolean := CustomProperty{Label: "Validador", FieldType: FieldBooleano}
	if v, _, err := NormalizePropertyValue(boolean, true); err != nil || v != true {
		t.Errorf("booleano: %v %v", v, err)
	}
}
