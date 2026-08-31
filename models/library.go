package models

import (
	"database/sql"
	"strings"
	"time"
)

// Playbook é um manual de atividades (roteiro para a equipe seguir).
type Playbook struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Body        string    `json:"body"`
	Active      bool      `json:"active"`
	CreatedBy   *int64    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func ListPlaybooks(db *sql.DB) ([]Playbook, error) {
	rows, err := db.Query(`
		SELECT id, name, COALESCE(description,''), body, active, created_by, created_at, updated_at
		FROM playbooks ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Playbook{}
	for rows.Next() {
		var p Playbook
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Body, &p.Active, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func PlaybookByID(db *sql.DB, id int64) (*Playbook, error) {
	var p Playbook
	err := db.QueryRow(`
		SELECT id, name, COALESCE(description,''), body, active, created_by, created_at, updated_at
		FROM playbooks WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Body, &p.Active, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func CreatePlaybook(db *sql.DB, p *Playbook) error {
	return db.QueryRow(`
		INSERT INTO playbooks (name, description, body, active, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`,
		p.Name, p.Description, p.Body, p.Active, p.CreatedBy,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func UpdatePlaybook(db *sql.DB, p *Playbook) error {
	_, err := db.Exec(`
		UPDATE playbooks SET name = $1, description = $2, body = $3, active = $4, updated_at = NOW()
		WHERE id = $5`,
		p.Name, p.Description, p.Body, p.Active, p.ID)
	return err
}

func DeletePlaybook(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM playbooks WHERE id = $1`, id)
	return err
}

// MessageTemplate é um modelo de e-mail com variáveis ({{nome}}, {{empresa}}...).
type MessageTemplate struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	CreatedBy *int64    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ListMessageTemplates(db *sql.DB) ([]MessageTemplate, error) {
	rows, err := db.Query(`
		SELECT id, name, subject, body, created_by, created_at, updated_at
		FROM message_templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []MessageTemplate{}
	for rows.Next() {
		var t MessageTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Subject, &t.Body, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// MessageTemplateByID busca um modelo específico (usado no envio manual e nas
// automações, que não precisam carregar a biblioteca inteira).
func MessageTemplateByID(db *sql.DB, id int64) (*MessageTemplate, error) {
	var t MessageTemplate
	err := db.QueryRow(`
		SELECT id, name, subject, body, created_by, created_at, updated_at
		FROM message_templates WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.Subject, &t.Body, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func CreateMessageTemplate(db *sql.DB, t *MessageTemplate) error {
	return db.QueryRow(`
		INSERT INTO message_templates (name, subject, body, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		t.Name, t.Subject, t.Body, t.CreatedBy,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func UpdateMessageTemplate(db *sql.DB, t *MessageTemplate) error {
	_, err := db.Exec(`
		UPDATE message_templates SET name = $1, subject = $2, body = $3, updated_at = NOW()
		WHERE id = $4`,
		t.Name, t.Subject, t.Body, t.ID)
	return err
}

func DeleteMessageTemplate(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM message_templates WHERE id = $1`, id)
	return err
}

// RenderTemplate substitui as variáveis do modelo pelos dados do contato.
func RenderTemplate(text string, c *Contact) string {
	if c == nil {
		return text
	}
	replacer := strings.NewReplacer(
		"{{nome}}", c.FirstName,
		"{{sobrenome}}", c.LastName,
		"{{email}}", c.Email,
		"{{empresa}}", c.CompanyName,
		"{{cargo}}", c.JobTitle,
	)
	return replacer.Replace(text)
}

// Snippet é um trecho de texto reutilizável.
type Snippet struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Shortcut  string    `json:"shortcut"`
	Body      string    `json:"body"`
	CreatedBy *int64    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ListSnippets(db *sql.DB) ([]Snippet, error) {
	rows, err := db.Query(`
		SELECT id, name, shortcut, body, created_by, created_at, updated_at
		FROM snippets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Snippet{}
	for rows.Next() {
		var s Snippet
		if err := rows.Scan(&s.ID, &s.Name, &s.Shortcut, &s.Body, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func CreateSnippet(db *sql.DB, s *Snippet) error {
	if s.Shortcut != "" && !strings.HasPrefix(s.Shortcut, "#") {
		s.Shortcut = "#" + s.Shortcut
	}
	return db.QueryRow(`
		INSERT INTO snippets (name, shortcut, body, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		s.Name, s.Shortcut, s.Body, s.CreatedBy,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

func UpdateSnippet(db *sql.DB, s *Snippet) error {
	if s.Shortcut != "" && !strings.HasPrefix(s.Shortcut, "#") {
		s.Shortcut = "#" + s.Shortcut
	}
	_, err := db.Exec(`
		UPDATE snippets SET name = $1, shortcut = $2, body = $3, updated_at = NOW()
		WHERE id = $4`,
		s.Name, s.Shortcut, s.Body, s.ID)
	return err
}

func DeleteSnippet(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM snippets WHERE id = $1`, id)
	return err
}
