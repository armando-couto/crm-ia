package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Tipos de campo disponíveis nas propriedades personalizadas.
const (
	FieldTexto      = "texto"
	FieldTextoLongo = "texto_longo"
	FieldNumero     = "numero"
	FieldData       = "data"
	FieldSelecao    = "selecao"
	FieldMultipla   = "multipla"
	FieldBooleano   = "booleano"
)

var FieldTypes = []string{FieldTexto, FieldTextoLongo, FieldNumero, FieldData, FieldSelecao, FieldMultipla, FieldBooleano}

// PropertyEntities são os objetos que aceitam propriedades personalizadas.
var PropertyEntities = []string{"contacts", "companies", "deals", "tickets"}

func ValidPropertyEntity(entity string) bool {
	return slices.Contains(PropertyEntities, entity)
}

type PropertyOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type CustomProperty struct {
	ID          int64            `json:"id"`
	Entity      string           `json:"entity"`
	Key         string           `json:"key"`
	Label       string           `json:"label"`
	Description string           `json:"description"`
	FieldType   string           `json:"field_type"`
	Options     []PropertyOption `json:"options"`
	GroupName   string           `json:"group_name"`
	Position    int              `json:"position"`
	CreatedBy   *int64           `json:"created_by"`
	CreatorName string           `json:"creator_name,omitempty"`
	UsedCount   int              `json:"used_count"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,59}$`)

// SlugifyKey gera o nome interno a partir do rótulo (ex.: "Nº do EC" -> "n_do_ec").
func SlugifyKey(label string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
		"é", "e", "ê", "e", "è", "e", "í", "i", "î", "i",
		"ó", "o", "õ", "o", "ô", "o", "ú", "u", "ü", "u", "ç", "c",
	)
	s := replacer.Replace(strings.ToLower(strings.TrimSpace(label)))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore && b.Len() > 0 {
				b.WriteRune('_')
				lastUnderscore = true
			}
		}
	}
	key := strings.Trim(b.String(), "_")
	if key == "" || (key[0] >= '0' && key[0] <= '9') {
		key = "campo_" + key
	}
	if len(key) > 60 {
		key = key[:60]
	}
	return strings.Trim(key, "_")
}

// ValidateProperty checa a definição da propriedade antes de gravar.
func ValidateProperty(p *CustomProperty) error {
	if !ValidPropertyEntity(p.Entity) {
		return fmt.Errorf("objeto inválido (contacts, companies, deals ou tickets)")
	}
	p.Label = strings.TrimSpace(p.Label)
	if p.Label == "" {
		return fmt.Errorf("informe o rótulo da propriedade")
	}
	if !slices.Contains(FieldTypes, p.FieldType) {
		return fmt.Errorf("tipo de campo inválido")
	}
	if p.Key == "" {
		p.Key = SlugifyKey(p.Label)
	}
	if !keyPattern.MatchString(p.Key) {
		return fmt.Errorf("nome interno inválido: use letras minúsculas, números e _ (começando por letra)")
	}
	if p.GroupName == "" {
		p.GroupName = "Informações personalizadas"
	}

	if p.FieldType == FieldSelecao || p.FieldType == FieldMultipla {
		if len(p.Options) == 0 {
			return fmt.Errorf("adicione ao menos uma opção para este tipo de campo")
		}
		if len(p.Options) > 100 {
			return fmt.Errorf("no máximo 100 opções")
		}
		seen := map[string]bool{}
		for i := range p.Options {
			p.Options[i].Label = strings.TrimSpace(p.Options[i].Label)
			if p.Options[i].Label == "" {
				return fmt.Errorf("toda opção precisa de um rótulo")
			}
			if p.Options[i].Value == "" {
				p.Options[i].Value = SlugifyKey(p.Options[i].Label)
			}
			if seen[p.Options[i].Value] {
				return fmt.Errorf("opções com valor interno duplicado: %s", p.Options[i].Value)
			}
			seen[p.Options[i].Value] = true
		}
	} else {
		p.Options = []PropertyOption{}
	}
	return nil
}

const propertySelect = `
	SELECT p.id, p.entity, p.key, p.label, COALESCE(p.description,''), p.field_type, p.options,
	       p.group_name, p.position, p.created_by, COALESCE(u.name,''),
	       (SELECT COUNT(*) FROM custom_property_values v WHERE v.property_id = p.id),
	       p.created_at, p.updated_at
	FROM custom_properties p
	LEFT JOIN users u ON u.id = p.created_by`

func scanProperty(row interface{ Scan(...any) error }) (*CustomProperty, error) {
	var p CustomProperty
	var options string
	err := row.Scan(&p.ID, &p.Entity, &p.Key, &p.Label, &p.Description, &p.FieldType, &options,
		&p.GroupName, &p.Position, &p.CreatedBy, &p.CreatorName, &p.UsedCount, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.Options = []PropertyOption{}
	if options != "" {
		_ = json.Unmarshal([]byte(options), &p.Options)
	}
	return &p, nil
}

// ListProperties lista as propriedades de um objeto (ou todas com entity vazia).
func ListProperties(db *sql.DB, entity string) ([]CustomProperty, error) {
	query := propertySelect
	args := []any{}
	if entity != "" {
		query += ` WHERE p.entity = $1`
		args = append(args, entity)
	}
	query += ` ORDER BY p.entity, p.position, p.id`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []CustomProperty{}
	for rows.Next() {
		p, err := scanProperty(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	return list, rows.Err()
}

func PropertyByID(db *sql.DB, id int64) (*CustomProperty, error) {
	row := db.QueryRow(propertySelect+` WHERE p.id = $1`, id)
	return scanProperty(row)
}

func CreateProperty(db *sql.DB, p *CustomProperty) error {
	options, err := json.Marshal(p.Options)
	if err != nil {
		return err
	}
	return db.QueryRow(`
		INSERT INTO custom_properties (entity, key, label, description, field_type, options, group_name, position, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7,
		        COALESCE((SELECT MAX(position) + 1 FROM custom_properties WHERE entity = $8), 0), $9)
		RETURNING id, position, created_at, updated_at`,
		p.Entity, p.Key, p.Label, p.Description, p.FieldType, string(options), p.GroupName, p.Entity, p.CreatedBy,
	).Scan(&p.ID, &p.Position, &p.CreatedAt, &p.UpdatedAt)
}

// UpdateProperty altera rótulo, descrição, grupo e opções (entity/key/tipo são imutáveis).
func UpdateProperty(db *sql.DB, p *CustomProperty) error {
	options, err := json.Marshal(p.Options)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		UPDATE custom_properties SET label = $1, description = $2, options = $3, group_name = $4, updated_at = NOW()
		WHERE id = $5`,
		p.Label, p.Description, string(options), p.GroupName, p.ID)
	return err
}

func DeleteProperty(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM custom_properties WHERE id = $1`, id)
	return err
}

// PropertyValues devolve os valores das propriedades de um registro, por chave.
func PropertyValues(db *sql.DB, entity string, recordID int64) (map[string]json.RawMessage, error) {
	rows, err := db.Query(`
		SELECT p.key, v.value
		FROM custom_property_values v
		JOIN custom_properties p ON p.id = v.property_id
		WHERE p.entity = $1 AND v.record_id = $2`, entity, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := map[string]json.RawMessage{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		values[key] = json.RawMessage(raw)
	}
	return values, rows.Err()
}

// SavePropertyValues grava os valores informados (por chave) para o registro.
// Valor vazio remove a propriedade do registro.
func SavePropertyValues(db *sql.DB, entity string, recordID int64, values map[string]any) error {
	props, err := ListProperties(db, entity)
	if err != nil {
		return err
	}
	byKey := map[string]CustomProperty{}
	for _, p := range props {
		byKey[p.Key] = p
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for key, raw := range values {
		prop, ok := byKey[key]
		if !ok {
			continue // ignora chaves desconhecidas
		}
		normalized, empty, err := NormalizePropertyValue(prop, raw)
		if err != nil {
			tx.Rollback()
			return err
		}
		if empty {
			if _, err := tx.Exec(`DELETE FROM custom_property_values WHERE property_id = $1 AND record_id = $2`,
				prop.ID, recordID); err != nil {
				tx.Rollback()
				return err
			}
			continue
		}
		encoded, err := json.Marshal(normalized)
		if err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO custom_property_values (property_id, record_id, value, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (property_id, record_id) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
			prop.ID, recordID, string(encoded)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// NormalizePropertyValue converte e valida o valor conforme o tipo do campo.
// Retorna empty=true quando o valor deve limpar a propriedade.
func NormalizePropertyValue(p CustomProperty, raw any) (any, bool, error) {
	if raw == nil {
		return nil, true, nil
	}

	switch p.FieldType {
	case FieldTexto, FieldTextoLongo:
		s := strings.TrimSpace(fmt.Sprint(raw))
		if s == "" {
			return nil, true, nil
		}
		if len(s) > 5000 {
			return nil, false, fmt.Errorf("%s: texto muito longo", p.Label)
		}
		return s, false, nil

	case FieldNumero:
		s := strings.TrimSpace(fmt.Sprint(raw))
		if s == "" {
			return nil, true, nil
		}
		n, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		if err != nil {
			return nil, false, fmt.Errorf("%s: informe um número válido", p.Label)
		}
		return n, false, nil

	case FieldData:
		s := strings.TrimSpace(fmt.Sprint(raw))
		if s == "" {
			return nil, true, nil
		}
		if len(s) > 10 {
			s = s[:10]
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, false, fmt.Errorf("%s: data inválida (use AAAA-MM-DD)", p.Label)
		}
		return s, false, nil

	case FieldBooleano:
		switch v := raw.(type) {
		case bool:
			return v, false, nil
		case string:
			if v == "" {
				return nil, true, nil
			}
			return v == "true" || v == "sim" || v == "1", false, nil
		default:
			return nil, false, fmt.Errorf("%s: valor inválido", p.Label)
		}

	case FieldSelecao:
		s := strings.TrimSpace(fmt.Sprint(raw))
		if s == "" {
			return nil, true, nil
		}
		for _, o := range p.Options {
			if o.Value == s {
				return s, false, nil
			}
		}
		return nil, false, fmt.Errorf("%s: opção inválida", p.Label)

	case FieldMultipla:
		items, ok := raw.([]any)
		if !ok {
			return nil, false, fmt.Errorf("%s: informe uma lista de opções", p.Label)
		}
		if len(items) == 0 {
			return nil, true, nil
		}
		valid := map[string]bool{}
		for _, o := range p.Options {
			valid[o.Value] = true
		}
		out := []string{}
		for _, item := range items {
			s := strings.TrimSpace(fmt.Sprint(item))
			if !valid[s] {
				return nil, false, fmt.Errorf("%s: opção inválida (%s)", p.Label, s)
			}
			out = append(out, s)
		}
		return out, false, nil
	}
	return nil, false, fmt.Errorf("%s: tipo de campo desconhecido", p.Label)
}
