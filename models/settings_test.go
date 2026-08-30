package models

import "testing"

func TestDefaultContactFormIsValid(t *testing.T) {
	if err := ValidateContactForm(DefaultContactForm()); err != nil {
		t.Fatalf("configuração padrão deveria ser válida: %v", err)
	}
}

func TestValidateContactForm(t *testing.T) {
	cases := []struct {
		name   string
		fields []FormField
		ok     bool
	}{
		{"vazia", []FormField{}, false},
		{"campo desconhecido", []FormField{
			{Key: "first_name", Visible: true, Required: true},
			{Key: "cpf", Visible: true},
		}, false},
		{"campo duplicado", []FormField{
			{Key: "first_name", Visible: true, Required: true},
			{Key: "first_name", Visible: true},
		}, false},
		{"nome oculto", []FormField{
			{Key: "first_name", Visible: false, Required: true},
			{Key: "email", Visible: true},
		}, false},
		{"nome opcional", []FormField{
			{Key: "first_name", Visible: true, Required: false},
			{Key: "email", Visible: true},
		}, false},
		{"obrigatório oculto", []FormField{
			{Key: "first_name", Visible: true, Required: true},
			{Key: "phone", Visible: false, Required: true},
		}, false},
		{"mínima válida", []FormField{
			{Key: "first_name", Visible: true, Required: true},
		}, true},
		{"completa válida", []FormField{
			{Key: "email", Visible: true, Required: true},
			{Key: "first_name", Visible: true, Required: true},
			{Key: "company_id", Visible: false, Required: false},
		}, true},
	}
	for _, c := range cases {
		err := ValidateContactForm(c.fields)
		if c.ok && err != nil {
			t.Errorf("%s: deveria ser válida, erro: %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: deveria ser inválida", c.name)
		}
	}
}

func TestValidViewEntity(t *testing.T) {
	if !ValidViewEntity("contacts") || !ValidViewEntity("companies") || !ValidViewEntity("deals") {
		t.Error("contacts, companies e deals deveriam ser válidas")
	}
	if ValidViewEntity("tickets") || ValidViewEntity("") {
		t.Error("entidades desconhecidas não podem ser válidas")
	}
}

// A ordenação de empresas também vem da URL: whitelist obrigatória.
func TestCompanyOrderBy(t *testing.T) {
	cases := []struct{ sortBy, sortDir, want string }{
		{"", "", "LOWER(c.name) ASC NULLS LAST"},
		{"created_at", "desc", "c.created_at DESC NULLS LAST"},
		{"contacts", "desc", "contacts_count DESC NULLS LAST"},
		{"name; DROP TABLE companies", "asc", "LOWER(c.name) ASC NULLS LAST"},
	}
	for _, c := range cases {
		if got := companyOrderBy(c.sortBy, c.sortDir); got != c.want {
			t.Errorf("companyOrderBy(%q, %q) = %q, esperado %q", c.sortBy, c.sortDir, got, c.want)
		}
	}
}
