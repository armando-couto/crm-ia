package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
)

// FormField descreve um campo do formulário de criação de contato.
type FormField struct {
	Key      string `json:"key"`
	Visible  bool   `json:"visible"`
	Required bool   `json:"required"`
}

// ContactFormFields são os campos disponíveis para o formulário de contato.
var ContactFormFields = []string{
	"email", "first_name", "last_name", "phone", "job_title",
	"lifecycle_stage", "source", "company_id", "owner_id",
}

// DefaultContactForm é a configuração inicial, no espírito do HubSpot:
// e-mail primeiro, nome obrigatório.
func DefaultContactForm() []FormField {
	return []FormField{
		{Key: "email", Visible: true, Required: false},
		{Key: "first_name", Visible: true, Required: true},
		{Key: "last_name", Visible: true, Required: false},
		{Key: "owner_id", Visible: true, Required: false},
		{Key: "job_title", Visible: true, Required: false},
		{Key: "phone", Visible: true, Required: false},
		{Key: "lifecycle_stage", Visible: true, Required: false},
		{Key: "source", Visible: false, Required: false},
		{Key: "company_id", Visible: false, Required: false},
	}
}

// ValidateContactForm garante que a configuração recebida é usável.
func ValidateContactForm(fields []FormField) error {
	if len(fields) == 0 {
		return fmt.Errorf("configuração vazia")
	}
	seen := map[string]bool{}
	hasName := false
	for _, f := range fields {
		if !slices.Contains(ContactFormFields, f.Key) {
			return fmt.Errorf("campo desconhecido: %s", f.Key)
		}
		if seen[f.Key] {
			return fmt.Errorf("campo duplicado: %s", f.Key)
		}
		seen[f.Key] = true
		if f.Key == "first_name" && f.Visible && f.Required {
			hasName = true
		}
		if f.Required && !f.Visible {
			return fmt.Errorf("campo %s não pode ser obrigatório e oculto", f.Key)
		}
	}
	if !hasName {
		return fmt.Errorf("o campo nome deve permanecer visível e obrigatório")
	}
	return nil
}

// DealFormFields são os campos disponíveis para o formulário de negócio.
var DealFormFields = []string{
	"name", "pipeline_id", "stage_id", "amount", "owner_id",
	"temperature", "contact_id", "company_id", "close_date",
}

// dealFormLocked são os campos que devem permanecer visíveis e obrigatórios
// (sem eles não é possível criar o negócio).
var dealFormLocked = map[string]bool{"name": true, "pipeline_id": true, "stage_id": true}

// DefaultDealForm espelha o painel "Criar Negócio" do HubSpot.
func DefaultDealForm() []FormField {
	return []FormField{
		{Key: "name", Visible: true, Required: true},
		{Key: "pipeline_id", Visible: true, Required: true},
		{Key: "stage_id", Visible: true, Required: true},
		{Key: "amount", Visible: true, Required: false},
		{Key: "owner_id", Visible: true, Required: false},
		{Key: "temperature", Visible: true, Required: false},
		{Key: "contact_id", Visible: true, Required: false},
		{Key: "company_id", Visible: true, Required: false},
		{Key: "close_date", Visible: false, Required: false},
	}
}

// ValidateDealForm garante que a configuração do formulário de negócio é usável.
func ValidateDealForm(fields []FormField) error {
	if len(fields) == 0 {
		return fmt.Errorf("configuração vazia")
	}
	seen := map[string]bool{}
	for _, f := range fields {
		if !slices.Contains(DealFormFields, f.Key) {
			return fmt.Errorf("campo desconhecido: %s", f.Key)
		}
		if seen[f.Key] {
			return fmt.Errorf("campo duplicado: %s", f.Key)
		}
		seen[f.Key] = true
		if dealFormLocked[f.Key] && (!f.Visible || !f.Required) {
			return fmt.Errorf("o campo %s deve permanecer visível e obrigatório", f.Key)
		}
		if f.Required && !f.Visible {
			return fmt.Errorf("campo %s não pode ser obrigatório e oculto", f.Key)
		}
	}
	for key := range dealFormLocked {
		if !seen[key] {
			return fmt.Errorf("o campo %s é obrigatório na configuração", key)
		}
	}
	return nil
}

// GetSetting lê uma configuração do app; ok=false quando não existe.
func GetSetting(db *sql.DB, key string, out any) (bool, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM app_settings WHERE key = $1`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), out)
}

// SetSetting grava (upsert) uma configuração do app.
func SetSetting(db *sql.DB, key string, value any, userID *int64) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO app_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value,
		    updated_by = EXCLUDED.updated_by, updated_at = NOW()`,
		key, string(raw), userID)
	return err
}
