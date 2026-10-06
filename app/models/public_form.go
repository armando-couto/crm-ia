package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Tipos aceitos nos campos do formulário público.
var formFieldTypes = []string{"texto", "email", "telefone", "textarea", "selecao"}

// PublicFormField é um campo do formulário público (o FormField de
// settings.go é o do formulário interno de cadastro).
type PublicFormField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

// PublicForm é o formulário embutido no site do cliente.
type PublicForm struct {
	ID             int64             `json:"id"`
	Slug           string            `json:"slug"`
	Name           string            `json:"name"`
	Headline       string            `json:"headline"`
	Description    string            `json:"description"`
	Fields         []PublicFormField `json:"fields"`
	SubmitLabel    string            `json:"submit_label"`
	SuccessMessage string            `json:"success_message"`
	RedirectURL    string            `json:"redirect_url"`
	OwnerID        *int64            `json:"owner_id"`
	OwnerName      string            `json:"owner_name,omitempty"`
	ListID         *int64            `json:"list_id"`
	ListName       string            `json:"list_name,omitempty"`
	LifecycleStage string            `json:"lifecycle_stage"`
	Source         string            `json:"source"`
	Active         bool              `json:"active"`
	Submissions    int               `json:"submissions"`
	CreatedBy      *int64            `json:"created_by"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// FormSubmission é um envio recebido pelo formulário.
type FormSubmission struct {
	ID        int64           `json:"id"`
	FormID    int64           `json:"form_id"`
	ContactID *int64          `json:"contact_id"`
	Payload   json.RawMessage `json:"payload"`
	IP        string          `json:"ip"`
	CreatedAt time.Time       `json:"created_at"`
}

// DefaultPublicFormFields é o formulário sugerido ao criar um novo.
func DefaultPublicFormFields() []PublicFormField {
	return []PublicFormField{
		{Key: "first_name", Label: "Nome", Type: "texto", Required: true},
		{Key: "last_name", Label: "Sobrenome", Type: "texto"},
		{Key: "email", Label: "E-mail", Type: "email", Required: true},
		{Key: "phone", Label: "Telefone", Type: "telefone"},
		{Key: "message", Label: "Mensagem", Type: "textarea"},
	}
}

// contactFieldKeys são os campos que alimentam direto o cadastro do contato;
// o resto vira observação na atividade de origem.
var contactFieldKeys = map[string]bool{
	"first_name": true, "last_name": true, "email": true,
	"phone": true, "job_title": true, "company": true,
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify transforma o nome do formulário em um slug de URL.
func Slugify(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
		"é", "e", "ê", "e", "è", "e", "í", "i", "î", "i",
		"ó", "o", "ô", "o", "õ", "o", "ú", "u", "ü", "u", "ç", "c")
	slug = replacer.Replace(slug)
	slug = slugPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 60 {
		slug = slug[:60]
	}
	return slug
}

// ValidatePublicFormFields checa chaves, tipos e duplicidade.
func ValidatePublicFormFields(fields []PublicFormField) error {
	if len(fields) == 0 {
		return fmt.Errorf("o formulário precisa de pelo menos um campo")
	}
	if len(fields) > 30 {
		return fmt.Errorf("no máximo 30 campos por formulário")
	}

	seen := map[string]bool{}
	hasEmail := false
	for i := range fields {
		f := &fields[i]
		f.Key = strings.TrimSpace(f.Key)
		f.Label = strings.TrimSpace(f.Label)
		if f.Key == "" || f.Label == "" {
			return fmt.Errorf("todo campo precisa de chave e rótulo")
		}
		if seen[f.Key] {
			return fmt.Errorf("campo duplicado: %s", f.Key)
		}
		seen[f.Key] = true

		if f.Type == "" {
			f.Type = "texto"
		}
		valid := false
		for _, t := range formFieldTypes {
			if f.Type == t {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("tipo de campo inválido: %s", f.Type)
		}
		if f.Type == "selecao" && len(f.Options) == 0 {
			return fmt.Errorf("o campo %s precisa de opções", f.Label)
		}
		if f.Key == "email" {
			hasEmail = true
		}
	}
	// Sem e-mail não dá para virar contato nem para deduplicar.
	if !hasEmail {
		return fmt.Errorf("o formulário precisa de um campo com a chave 'email'")
	}
	return nil
}

const formSelect = `
	SELECT f.id, f.slug, f.name, f.headline, f.description, f.fields, f.submit_label,
	       f.success_message, f.redirect_url, f.owner_id, COALESCE(u.name,''),
	       f.list_id, COALESCE(l.name,''), f.lifecycle_stage, f.source, f.active,
	       f.submissions, f.created_by, f.created_at, f.updated_at
	FROM public_forms f
	LEFT JOIN users u ON u.id = f.owner_id
	LEFT JOIN lists l ON l.id = f.list_id`

func scanForm(row interface{ Scan(...any) error }) (*PublicForm, error) {
	var f PublicForm
	var fields []byte
	err := row.Scan(&f.ID, &f.Slug, &f.Name, &f.Headline, &f.Description, &fields,
		&f.SubmitLabel, &f.SuccessMessage, &f.RedirectURL, &f.OwnerID, &f.OwnerName,
		&f.ListID, &f.ListName, &f.LifecycleStage, &f.Source, &f.Active,
		&f.Submissions, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(fields) > 0 {
		if err := json.Unmarshal(fields, &f.Fields); err != nil {
			return nil, err
		}
	}
	return &f, nil
}

func ListPublicForms(db *sql.DB) ([]PublicForm, error) {
	rows, err := db.Query(formSelect + ` ORDER BY f.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []PublicForm{}
	for rows.Next() {
		f, err := scanForm(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *f)
	}
	return list, rows.Err()
}

func PublicFormByID(db *sql.DB, id int64) (*PublicForm, error) {
	return scanForm(db.QueryRow(formSelect+` WHERE f.id = $1`, id))
}

func PublicFormBySlug(db *sql.DB, slug string) (*PublicForm, error) {
	return scanForm(db.QueryRow(formSelect+` WHERE f.slug = $1`, slug))
}

func CreatePublicForm(db *sql.DB, f *PublicForm) error {
	fields, err := json.Marshal(f.Fields)
	if err != nil {
		return err
	}
	return db.QueryRow(`
		INSERT INTO public_forms (slug, name, headline, description, fields, submit_label,
		    success_message, redirect_url, owner_id, list_id, lifecycle_stage, source, active, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, created_at, updated_at`,
		f.Slug, f.Name, f.Headline, f.Description, fields, f.SubmitLabel, f.SuccessMessage,
		f.RedirectURL, f.OwnerID, f.ListID, f.LifecycleStage, f.Source, f.Active, f.CreatedBy,
	).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
}

func UpdatePublicForm(db *sql.DB, f *PublicForm) error {
	fields, err := json.Marshal(f.Fields)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		UPDATE public_forms SET slug = $1, name = $2, headline = $3, description = $4,
		    fields = $5, submit_label = $6, success_message = $7, redirect_url = $8,
		    owner_id = $9, list_id = $10, lifecycle_stage = $11, source = $12,
		    active = $13, updated_at = NOW()
		WHERE id = $14`,
		f.Slug, f.Name, f.Headline, f.Description, fields, f.SubmitLabel, f.SuccessMessage,
		f.RedirectURL, f.OwnerID, f.ListID, f.LifecycleStage, f.Source, f.Active, f.ID)
	return err
}

func DeletePublicForm(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM public_forms WHERE id = $1`, id)
	return err
}

// SubmitPublicForm cria (ou reaproveita) o contato do envio e guarda o registro.
// Devolve o contato e se ele já existia.
func SubmitPublicForm(db *sql.DB, form *PublicForm, values map[string]string, ip string) (*Contact, bool, error) {
	email := NormalizeEmail(values["email"])
	if email == "" {
		return nil, false, fmt.Errorf("informe o e-mail")
	}

	// Mesmo e-mail não vira contato novo: alimenta o que já existe.
	contact, err := ContactByEmail(db, email)
	existed := err == nil
	if err != nil && err != sql.ErrNoRows {
		return nil, false, err
	}

	if !existed {
		contact = &Contact{
			FirstName:      strings.TrimSpace(values["first_name"]),
			LastName:       strings.TrimSpace(values["last_name"]),
			Email:          email,
			Phone:          strings.TrimSpace(values["phone"]),
			JobTitle:       strings.TrimSpace(values["job_title"]),
			LifecycleStage: form.LifecycleStage,
			Source:         form.Source,
			OwnerID:        form.OwnerID,
		}
		if contact.FirstName == "" {
			contact.FirstName = email
		}
		if err := CreateContact(db, contact); err != nil {
			return nil, false, err
		}
	} else {
		// Completa só o que estava vazio, sem sobrescrever o que a equipe já sabe.
		changed := false
		for field, target := range map[string]*string{
			"first_name": &contact.FirstName,
			"last_name":  &contact.LastName,
			"phone":      &contact.Phone,
			"job_title":  &contact.JobTitle,
		} {
			if strings.TrimSpace(*target) == "" && strings.TrimSpace(values[field]) != "" {
				*target = strings.TrimSpace(values[field])
				changed = true
			}
		}
		if changed {
			if err := UpdateContact(db, contact); err != nil {
				return nil, false, err
			}
		}
	}

	if form.ListID != nil {
		// Já estar na lista não é erro.
		if _, err := db.Exec(`
			INSERT INTO list_members (list_id, contact_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, *form.ListID, contact.ID); err != nil {
			return nil, false, err
		}
	}

	payload, err := json.Marshal(values)
	if err != nil {
		return nil, false, err
	}
	if _, err := db.Exec(`
		INSERT INTO form_submissions (form_id, contact_id, payload, ip) VALUES ($1, $2, $3, $4)`,
		form.ID, contact.ID, payload, ip); err != nil {
		return nil, false, err
	}
	if _, err := db.Exec(`UPDATE public_forms SET submissions = submissions + 1 WHERE id = $1`,
		form.ID); err != nil {
		return nil, false, err
	}

	return contact, existed, nil
}

// ExtraFormValues devolve os campos que não vão para o cadastro do contato,
// para virarem a observação da atividade de origem.
func ExtraFormValues(form *PublicForm, values map[string]string) string {
	var sb strings.Builder
	for _, field := range form.Fields {
		if contactFieldKeys[field.Key] {
			continue
		}
		if value := strings.TrimSpace(values[field.Key]); value != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			fmt.Fprintf(&sb, "%s: %s", field.Label, value)
		}
	}
	return sb.String()
}

func ListFormSubmissions(db *sql.DB, formID int64, limit int) ([]FormSubmission, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT id, form_id, contact_id, payload, ip, created_at
		FROM form_submissions WHERE form_id = $1 ORDER BY created_at DESC LIMIT $2`,
		formID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []FormSubmission{}
	for rows.Next() {
		var s FormSubmission
		var payload []byte
		if err := rows.Scan(&s.ID, &s.FormID, &s.ContactID, &payload, &s.IP, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Payload = json.RawMessage(payload)
		list = append(list, s)
	}
	return list, rows.Err()
}
