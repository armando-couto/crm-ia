package models

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Contato do Site":         "contato-do-site",
		"  Formulário  Nº 2  ":    "formulario-n-2",
		"Ação & Reação":           "acao-reacao",
		"---":                     "",
		"CNPJ/Inscrição Estadual": "cnpj-inscricao-estadual",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, esperado %q", in, got, want)
		}
	}
}

func TestValidatePublicFormFields(t *testing.T) {
	if err := ValidatePublicFormFields(DefaultPublicFormFields()); err != nil {
		t.Fatalf("o formulário padrão deveria ser válido: %v", err)
	}
}

func TestValidatePublicFormFieldsRequiresEmail(t *testing.T) {
	fields := []PublicFormField{{Key: "nome", Label: "Nome", Type: "texto"}}
	if err := ValidatePublicFormFields(fields); err == nil {
		t.Fatal("sem campo de e-mail o formulário não pode ser aceito")
	}
}

func TestValidatePublicFormFieldsRejectsDuplicates(t *testing.T) {
	fields := []PublicFormField{
		{Key: "email", Label: "E-mail", Type: "email"},
		{Key: "email", Label: "Confirmar", Type: "email"},
	}
	if err := ValidatePublicFormFields(fields); err == nil {
		t.Fatal("chave duplicada deveria ser recusada")
	}
}

func TestValidatePublicFormFieldsRejectsUnknownType(t *testing.T) {
	fields := []PublicFormField{
		{Key: "email", Label: "E-mail", Type: "email"},
		{Key: "arquivo", Label: "Anexo", Type: "upload"},
	}
	if err := ValidatePublicFormFields(fields); err == nil {
		t.Fatal("tipo desconhecido deveria ser recusado")
	}
}

func TestValidatePublicFormFieldsRequiresOptionsForSelect(t *testing.T) {
	fields := []PublicFormField{
		{Key: "email", Label: "E-mail", Type: "email"},
		{Key: "porte", Label: "Porte", Type: "selecao"},
	}
	if err := ValidatePublicFormFields(fields); err == nil {
		t.Fatal("seleção sem opções deveria ser recusada")
	}
}

// Campo vazio ou sem rótulo não passa.
func TestValidatePublicFormFieldsRejectsEmpty(t *testing.T) {
	if err := ValidatePublicFormFields(nil); err == nil {
		t.Fatal("formulário sem campos deveria ser recusado")
	}
	fields := []PublicFormField{
		{Key: "email", Label: "E-mail", Type: "email"},
		{Key: "  ", Label: "Sem chave", Type: "texto"},
	}
	if err := ValidatePublicFormFields(fields); err == nil {
		t.Fatal("campo sem chave deveria ser recusado")
	}
}

// Os campos fora do cadastro do contato viram observação na timeline.
func TestExtraFormValues(t *testing.T) {
	form := &PublicForm{Fields: []PublicFormField{
		{Key: "first_name", Label: "Nome"},
		{Key: "email", Label: "E-mail"},
		{Key: "message", Label: "Mensagem"},
		{Key: "porte", Label: "Porte da empresa"},
		{Key: "vazio", Label: "Não preenchido"},
	}}
	values := map[string]string{
		"first_name": "Ana",
		"email":      "ana@exemplo.com.br",
		"message":    "quero uma maquininha",
		"porte":      "pequeno",
		"vazio":      "",
	}

	got := ExtraFormValues(form, values)
	want := "Mensagem: quero uma maquininha\nPorte da empresa: pequeno"
	if got != want {
		t.Fatalf("observação inesperada:\n%q\nesperado:\n%q", got, want)
	}
}
