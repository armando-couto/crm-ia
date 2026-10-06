package models

import "testing"

func TestRenderTemplate(t *testing.T) {
	contact := &Contact{
		FirstName:   "Carlos",
		LastName:    "Lima",
		Email:       "carlos@bompreco.com.br",
		CompanyName: "Mercado Bom Preço",
		JobTitle:    "Diretor",
	}

	in := "Olá {{nome}} {{sobrenome}}, tudo bem? Vi que você é {{cargo}} na {{empresa}} ({{email}})."
	want := "Olá Carlos Lima, tudo bem? Vi que você é Diretor na Mercado Bom Preço (carlos@bompreco.com.br)."
	if got := RenderTemplate(in, contact); got != want {
		t.Fatalf("RenderTemplate:\n got:  %s\n want: %s", got, want)
	}
}

func TestRenderTemplateNilContact(t *testing.T) {
	in := "Olá {{nome}}"
	if got := RenderTemplate(in, nil); got != in {
		t.Fatalf("com contato nulo o texto deveria ficar intacto, obtido: %s", got)
	}
}

func TestRenderTemplateMissingFields(t *testing.T) {
	contact := &Contact{FirstName: "Ana"}
	got := RenderTemplate("{{nome}} da {{empresa}}", contact)
	if got != "Ana da " {
		t.Fatalf("campos vazios devem virar string vazia, obtido: %q", got)
	}
}

func TestNormalizeSubject(t *testing.T) {
	cases := map[string]string{
		"Re: Proposta":           "Proposta",
		"RES: RE: Proposta":      "Proposta",
		"FWD: Enc: Fatura":       "Fatura",
		"  Assunto normal  ":     "Assunto normal",
		"":                       "(sem assunto)",
		"re:":                    "(sem assunto)",
		"Reunião de alinhamento": "Reunião de alinhamento",
	}
	for in, want := range cases {
		if got := NormalizeSubject(in); got != want {
			t.Errorf("NormalizeSubject(%q) = %q, esperado %q", in, got, want)
		}
	}
}
