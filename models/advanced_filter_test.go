package models

import (
	"strings"
	"testing"
)

func TestParseAdvancedFiltersValid(t *testing.T) {
	raw := `{"groups":[
		{"conditions":[
			{"field":"lifecycle_stage","op":"any_of","values":["cliente"]},
			{"field":"source","op":"eq","value":"site"}
		]},
		{"conditions":[{"field":"owner_id","op":"empty"}]}
	]}`
	af, err := ParseAdvancedFilters(raw, ContactFilterFieldsSpec)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(af.Groups) != 2 || len(af.Groups[0].Conditions) != 2 {
		t.Fatalf("estrutura inesperada: %+v", af)
	}
}

func TestParseAdvancedFiltersEmpty(t *testing.T) {
	af, err := ParseAdvancedFilters("", ContactFilterFieldsSpec)
	if err != nil || af != nil {
		t.Fatalf("filtro vazio deveria retornar nil sem erro, obtido %v/%v", af, err)
	}
	af, err = ParseAdvancedFilters(`{"groups":[]}`, ContactFilterFieldsSpec)
	if err != nil || af != nil {
		t.Fatalf("groups vazio deveria retornar nil sem erro")
	}
}

func TestParseAdvancedFiltersInvalid(t *testing.T) {
	cases := map[string]string{
		"json quebrado":        `{grupos`,
		"campo desconhecido":   `{"groups":[{"conditions":[{"field":"senha","op":"eq","value":"x"}]}]}`,
		"operador errado":      `{"groups":[{"conditions":[{"field":"email","op":"any_of","values":["a"]}]}]}`,
		"grupo vazio":          `{"groups":[{"conditions":[]}]}`,
		"valor faltando":       `{"groups":[{"conditions":[{"field":"email","op":"eq"}]}]}`,
		"ref não numérica":     `{"groups":[{"conditions":[{"field":"owner_id","op":"any_of","values":["abc"]}]}]}`,
		"dias inválidos":       `{"groups":[{"conditions":[{"field":"created_at","op":"last_days","value":"-5"}]}]}`,
		"data sem valor":       `{"groups":[{"conditions":[{"field":"created_at","op":"last_days","value":"x"}]}]}`,
	}
	for name, raw := range cases {
		if _, err := ParseAdvancedFilters(raw, ContactFilterFieldsSpec); err == nil {
			t.Errorf("%s: deveria falhar", name)
		}
	}
}

func TestBuildAdvancedWhere(t *testing.T) {
	raw := `{"groups":[
		{"conditions":[
			{"field":"lifecycle_stage","op":"any_of","values":["cliente","oportunidade"]},
			{"field":"email","op":"not_empty"}
		]},
		{"conditions":[{"field":"last_activity","op":"older_than","value":"30"}]}
	]}`
	af, err := ParseAdvancedFilters(raw, ContactFilterFieldsSpec)
	if err != nil {
		t.Fatal(err)
	}

	args := []any{"pre-existente"}
	where := BuildAdvancedWhere(af, ContactFilterFieldsSpec, &args)

	// Grupos em OR, condições em AND, parâmetros continuando a numeração.
	if !strings.Contains(where, ") OR (") {
		t.Fatalf("grupos deveriam ser unidos por OR: %s", where)
	}
	if !strings.Contains(where, "c.lifecycle_stage = ANY($2)") {
		t.Fatalf("any_of deveria usar parâmetro $2: %s", where)
	}
	if !strings.Contains(where, "COALESCE(c.email,'') <> ''") {
		t.Fatalf("not_empty de texto incorreto: %s", where)
	}
	if !strings.Contains(where, "make_interval(days => $3)") {
		t.Fatalf("older_than deveria parametrizar os dias: %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("esperados 3 args, obtidos %d", len(args))
	}
}

// Valores maliciosos nunca entram no SQL: viram parâmetros posicionais.
func TestBuildAdvancedWhereInjectionSafe(t *testing.T) {
	raw := `{"groups":[{"conditions":[{"field":"name","op":"contains","value":"'; DROP TABLE contacts; --"}]}]}`
	af, err := ParseAdvancedFilters(raw, ContactFilterFieldsSpec)
	if err != nil {
		t.Fatal(err)
	}
	args := []any{}
	where := BuildAdvancedWhere(af, ContactFilterFieldsSpec, &args)
	if strings.Contains(where, "DROP TABLE") {
		t.Fatalf("valor do usuário vazou para o SQL: %s", where)
	}
	if len(args) != 1 || !strings.Contains(args[0].(string), "drop table") {
		t.Fatalf("valor deveria estar nos args (minúsculo): %+v", args)
	}
}

func TestBuildAdvancedWhereRefNoneOfIncludesNull(t *testing.T) {
	raw := `{"groups":[{"conditions":[{"field":"owner_id","op":"none_of","values":["7"]}]}]}`
	af, _ := ParseAdvancedFilters(raw, ContactFilterFieldsSpec)
	args := []any{}
	where := BuildAdvancedWhere(af, ContactFilterFieldsSpec, &args)
	if !strings.Contains(where, "c.owner_id IS NULL OR c.owner_id <> ALL($1)") {
		t.Fatalf("none_of de referência deve incluir registros sem valor: %s", where)
	}
}
