package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// Filtros avançados no estilo HubSpot: grupos de condições onde as condições
// de um grupo são combinadas com E, e os grupos entre si com OU.

type FilterCondition struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Value  string   `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
}

type FilterGroup struct {
	Conditions []FilterCondition `json:"conditions"`
}

type AdvancedFilters struct {
	Groups []FilterGroup `json:"groups"`
}

// fieldSpec descreve um campo filtrável: a expressão SQL (fixa, nunca vinda
// do usuário) e o tipo, que define os operadores aceitos.
type fieldSpec struct {
	expr string
	kind string // text | enum | ref | date
}

// ContactFilterFieldsSpec são os campos filtráveis de contatos.
var ContactFilterFieldsSpec = map[string]fieldSpec{
	"name":            {expr: "(c.first_name || ' ' || COALESCE(c.last_name,''))", kind: "text"},
	"email":           {expr: "c.email", kind: "text"},
	"phone":           {expr: "c.phone", kind: "text"},
	"job_title":       {expr: "c.job_title", kind: "text"},
	"source":          {expr: "c.source", kind: "text"},
	"lifecycle_stage": {expr: "c.lifecycle_stage", kind: "enum"},
	"owner_id":        {expr: "c.owner_id", kind: "ref"},
	"company_id":      {expr: "c.company_id", kind: "ref"},
	"created_at":      {expr: "c.created_at", kind: "date"},
	"last_activity":   {expr: "(SELECT MAX(a.created_at) FROM activities a WHERE a.contact_id = c.id)", kind: "date"},
}

// CompanyFilterFieldsSpec são os campos filtráveis de empresas.
var CompanyFilterFieldsSpec = map[string]fieldSpec{
	"name":       {expr: "c.name", kind: "text"},
	"domain":     {expr: "c.domain", kind: "text"},
	"industry":   {expr: "c.industry", kind: "text"},
	"city":       {expr: "c.city", kind: "text"},
	"state":      {expr: "c.state", kind: "text"},
	"phone":      {expr: "c.phone", kind: "text"},
	"owner_id":   {expr: "c.owner_id", kind: "ref"},
	"created_at": {expr: "c.created_at", kind: "date"},
}

// ParseAdvancedFilters decodifica e valida o JSON do parâmetro af.
func ParseAdvancedFilters(raw string, specs map[string]fieldSpec) (*AdvancedFilters, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if len(raw) > 8192 {
		return nil, fmt.Errorf("filtros avançados grandes demais")
	}
	var af AdvancedFilters
	if err := json.Unmarshal([]byte(raw), &af); err != nil {
		return nil, fmt.Errorf("filtros avançados inválidos")
	}
	if len(af.Groups) == 0 {
		return nil, nil
	}
	if len(af.Groups) > 5 {
		return nil, fmt.Errorf("no máximo 5 grupos de filtros")
	}
	for _, g := range af.Groups {
		if len(g.Conditions) == 0 {
			return nil, fmt.Errorf("grupo de filtros vazio")
		}
		if len(g.Conditions) > 10 {
			return nil, fmt.Errorf("no máximo 10 condições por grupo")
		}
		for _, c := range g.Conditions {
			spec, ok := specs[c.Field]
			if !ok {
				return nil, fmt.Errorf("campo desconhecido: %s", c.Field)
			}
			if err := validateOp(spec.kind, c); err != nil {
				return nil, err
			}
		}
	}
	return &af, nil
}

func validateOp(kind string, c FilterCondition) error {
	valid := map[string][]string{
		"text": {"contains", "eq", "empty", "not_empty"},
		"enum": {"any_of", "none_of"},
		"ref":  {"any_of", "none_of", "empty", "not_empty"},
		"date": {"last_days", "older_than", "empty", "not_empty"},
	}[kind]
	found := false
	for _, op := range valid {
		if op == c.Op {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("operador %s inválido para o campo %s", c.Op, c.Field)
	}

	switch c.Op {
	case "contains", "eq":
		if strings.TrimSpace(c.Value) == "" {
			return fmt.Errorf("informe o valor do filtro %s", c.Field)
		}
	case "any_of", "none_of":
		if len(c.Values) == 0 || len(c.Values) > 50 {
			return fmt.Errorf("informe os valores do filtro %s", c.Field)
		}
		if kind == "ref" {
			for _, v := range c.Values {
				if _, err := strconv.ParseInt(v, 10, 64); err != nil {
					return fmt.Errorf("valor inválido no filtro %s", c.Field)
				}
			}
		}
	case "last_days", "older_than":
		n, err := strconv.Atoi(c.Value)
		if err != nil || n < 1 || n > 3650 {
			return fmt.Errorf("informe a quantidade de dias do filtro %s (1 a 3650)", c.Field)
		}
	}
	return nil
}

// BuildAdvancedWhere gera a cláusula SQL dos grupos: (c1 AND c2) OR (c3 ...).
// Os campos/operadores já foram validados por ParseAdvancedFilters; os valores
// entram sempre como parâmetros posicionais.
func BuildAdvancedWhere(af *AdvancedFilters, specs map[string]fieldSpec, args *[]any) string {
	if af == nil || len(af.Groups) == 0 {
		return ""
	}
	groups := make([]string, 0, len(af.Groups))
	for _, g := range af.Groups {
		conds := make([]string, 0, len(g.Conditions))
		for _, c := range g.Conditions {
			conds = append(conds, buildCondition(c, specs[c.Field], args))
		}
		groups = append(groups, "("+strings.Join(conds, " AND ")+")")
	}
	return "(" + strings.Join(groups, " OR ") + ")"
}

func buildCondition(c FilterCondition, spec fieldSpec, args *[]any) string {
	add := func(v any) int {
		*args = append(*args, v)
		return len(*args)
	}

	switch c.Op {
	case "contains":
		n := add("%" + strings.ToLower(strings.TrimSpace(c.Value)) + "%")
		return fmt.Sprintf("LOWER(COALESCE(%s,'')) LIKE $%d", spec.expr, n)
	case "eq":
		n := add(strings.ToLower(strings.TrimSpace(c.Value)))
		return fmt.Sprintf("LOWER(COALESCE(%s,'')) = $%d", spec.expr, n)
	case "empty":
		if spec.kind == "text" {
			return fmt.Sprintf("COALESCE(%s,'') = ''", spec.expr)
		}
		return fmt.Sprintf("%s IS NULL", spec.expr)
	case "not_empty":
		if spec.kind == "text" {
			return fmt.Sprintf("COALESCE(%s,'') <> ''", spec.expr)
		}
		return fmt.Sprintf("%s IS NOT NULL", spec.expr)
	case "any_of":
		if spec.kind == "ref" {
			n := add(pq.Array(toInt64s(c.Values)))
			return fmt.Sprintf("%s = ANY($%d)", spec.expr, n)
		}
		n := add(pq.Array(c.Values))
		return fmt.Sprintf("%s = ANY($%d)", spec.expr, n)
	case "none_of":
		if spec.kind == "ref" {
			n := add(pq.Array(toInt64s(c.Values)))
			return fmt.Sprintf("(%s IS NULL OR %s <> ALL($%d))", spec.expr, spec.expr, n)
		}
		n := add(pq.Array(c.Values))
		return fmt.Sprintf("(%s IS NULL OR %s <> ALL($%d))", spec.expr, spec.expr, n)
	case "last_days":
		days, _ := strconv.Atoi(c.Value)
		n := add(days)
		return fmt.Sprintf("%s >= NOW() - make_interval(days => $%d)", spec.expr, n)
	case "older_than":
		days, _ := strconv.Atoi(c.Value)
		n := add(days)
		return fmt.Sprintf("(%s IS NULL OR %s < NOW() - make_interval(days => $%d))", spec.expr, spec.expr, n)
	}
	return "1=1"
}

func toInt64s(values []string) []int64 {
	out := make([]int64, 0, len(values))
	for _, v := range values {
		n, _ := strconv.ParseInt(v, 10, 64)
		out = append(out, n)
	}
	return out
}
