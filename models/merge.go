package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DuplicateGroup é um conjunto de registros que aparentam ser a mesma pessoa
// ou empresa, junto do critério que os aproximou.
type DuplicateGroup struct {
	Reason  string           `json:"reason"`
	Value   string           `json:"value"`
	Records []DuplicateEntry `json:"records"`
}

type DuplicateEntry struct {
	ID        int64     `json:"id"`
	Label     string    `json:"label"`
	Email     string    `json:"email,omitempty"`
	Phone     string    `json:"phone,omitempty"`
	Extra     string    `json:"extra,omitempty"`
	OwnerName string    `json:"owner_name,omitempty"`
	Deals     int       `json:"deals"`
	CreatedAt time.Time `json:"created_at"`
}

// maxDuplicateGroups evita varreduras enormes na tela de duplicados.
const maxDuplicateGroups = 100

// FindDuplicateContacts agrupa contatos por e-mail, telefone e nome completo.
// A ordem importa: e-mail é o critério mais forte, nome o mais frágil.
func FindDuplicateContacts(db *sql.DB) ([]DuplicateGroup, error) {
	criteria := []struct {
		reason string
		key    string
		having string
	}{
		{"mesmo e-mail", "LOWER(TRIM(c.email))", "LOWER(TRIM(c.email)) <> ''"},
		{"mesmo telefone", "regexp_replace(COALESCE(c.phone,''), '[^0-9]', '', 'g')",
			"LENGTH(regexp_replace(COALESCE(c.phone,''), '[^0-9]', '', 'g')) >= 10"},
		{"mesmo nome", "LOWER(TRIM(c.first_name || ' ' || c.last_name))",
			"LENGTH(TRIM(c.first_name || ' ' || c.last_name)) > 3"},
	}

	seen := map[int64]bool{}
	groups := []DuplicateGroup{}

	for _, crit := range criteria {
		query := fmt.Sprintf(`
			SELECT %s AS chave, c.id, c.first_name, c.last_name, c.email, c.phone,
			       COALESCE(u.name,''), c.created_at,
			       (SELECT COUNT(*) FROM deals d WHERE d.contact_id = c.id)
			FROM contacts c
			LEFT JOIN users u ON u.id = c.owner_id
			WHERE %s AND %s IN (
			    SELECT %s FROM contacts c WHERE %s GROUP BY 1 HAVING COUNT(*) > 1
			)
			ORDER BY chave, c.created_at`,
			crit.key, crit.having, crit.key, crit.key, crit.having)

		rows, err := db.Query(query)
		if err != nil {
			return nil, err
		}

		byKey := map[string][]DuplicateEntry{}
		order := []string{}
		for rows.Next() {
			var key, first, last, email, phone, owner string
			var entry DuplicateEntry
			if err := rows.Scan(&key, &entry.ID, &first, &last, &email, &phone,
				&owner, &entry.CreatedAt, &entry.Deals); err != nil {
				rows.Close()
				return nil, err
			}
			entry.Label = strings.TrimSpace(first + " " + last)
			entry.Email = email
			entry.Phone = phone
			entry.OwnerName = owner
			if _, ok := byKey[key]; !ok {
				order = append(order, key)
			}
			byKey[key] = append(byKey[key], entry)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		for _, key := range order {
			entries := byKey[key]
			// Um grupo já mostrado por um critério mais forte não repete.
			if len(entries) < 2 || allSeen(entries, seen) {
				continue
			}
			for _, e := range entries {
				seen[e.ID] = true
			}
			groups = append(groups, DuplicateGroup{Reason: crit.reason, Value: key, Records: entries})
			if len(groups) >= maxDuplicateGroups {
				return groups, nil
			}
		}
	}
	return groups, nil
}

// FindDuplicateCompanies agrupa empresas por CNPJ, domínio e nome.
func FindDuplicateCompanies(db *sql.DB) ([]DuplicateGroup, error) {
	criteria := []struct {
		reason string
		key    string
		having string
	}{
		{"mesmo CNPJ", "regexp_replace(COALESCE(c.cnpj,''), '[^0-9]', '', 'g')",
			"LENGTH(regexp_replace(COALESCE(c.cnpj,''), '[^0-9]', '', 'g')) = 14"},
		{"mesmo domínio", "LOWER(TRIM(COALESCE(c.domain,'')))", "TRIM(COALESCE(c.domain,'')) <> ''"},
		{"mesmo nome", "LOWER(TRIM(c.name))", "LENGTH(TRIM(c.name)) > 2"},
	}

	seen := map[int64]bool{}
	groups := []DuplicateGroup{}

	for _, crit := range criteria {
		query := fmt.Sprintf(`
			SELECT %s AS chave, c.id, c.name, COALESCE(c.cnpj,''), COALESCE(c.domain,''),
			       COALESCE(u.name,''), c.created_at,
			       (SELECT COUNT(*) FROM deals d WHERE d.company_id = c.id)
			FROM companies c
			LEFT JOIN users u ON u.id = c.owner_id
			WHERE %s AND %s IN (
			    SELECT %s FROM companies c WHERE %s GROUP BY 1 HAVING COUNT(*) > 1
			)
			ORDER BY chave, c.created_at`,
			crit.key, crit.having, crit.key, crit.key, crit.having)

		rows, err := db.Query(query)
		if err != nil {
			return nil, err
		}

		byKey := map[string][]DuplicateEntry{}
		order := []string{}
		for rows.Next() {
			var key, cnpj, domain, owner string
			var entry DuplicateEntry
			if err := rows.Scan(&key, &entry.ID, &entry.Label, &cnpj, &domain,
				&owner, &entry.CreatedAt, &entry.Deals); err != nil {
				rows.Close()
				return nil, err
			}
			entry.Extra = strings.TrimSpace(strings.Trim(cnpj+" "+domain, " "))
			entry.OwnerName = owner
			if _, ok := byKey[key]; !ok {
				order = append(order, key)
			}
			byKey[key] = append(byKey[key], entry)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		for _, key := range order {
			entries := byKey[key]
			if len(entries) < 2 || allSeen(entries, seen) {
				continue
			}
			for _, e := range entries {
				seen[e.ID] = true
			}
			groups = append(groups, DuplicateGroup{Reason: crit.reason, Value: key, Records: entries})
			if len(groups) >= maxDuplicateGroups {
				return groups, nil
			}
		}
	}
	return groups, nil
}

func allSeen(entries []DuplicateEntry, seen map[int64]bool) bool {
	for _, e := range entries {
		if !seen[e.ID] {
			return false
		}
	}
	return true
}

// contactRefs são as tabelas que apontam para um contato e precisam migrar
// para o registro principal na mesclagem.
var contactRefs = []string{"activities", "deals", "tasks", "tickets", "calls", "meetings", "conversations"}

// companyRefs são as tabelas que apontam para uma empresa.
var companyRefs = []string{"activities", "contacts", "deals", "tasks", "tickets", "calls", "meetings"}

// MergeContacts move tudo do duplicado para o principal e apaga o duplicado.
// Campos vazios do principal são preenchidos com o que o duplicado tinha.
func MergeContacts(db *sql.DB, primaryID, duplicateID int64) (*Contact, error) {
	if primaryID == duplicateID {
		return nil, fmt.Errorf("escolha dois contatos diferentes")
	}

	primary, err := ContactByID(db, primaryID)
	if err != nil {
		return nil, err
	}
	duplicate, err := ContactByID(db, duplicateID)
	if err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for _, table := range contactRefs {
		if _, err := tx.Exec(fmt.Sprintf(
			`UPDATE %s SET contact_id = $1 WHERE contact_id = $2`, table), primaryID, duplicateID); err != nil {
			return nil, err
		}
	}
	// Listas usam chave composta: move só o que ainda não existe no principal.
	if _, err := tx.Exec(`
		UPDATE list_members SET contact_id = $1
		WHERE contact_id = $2 AND list_id NOT IN (SELECT list_id FROM list_members WHERE contact_id = $1)`,
		primaryID, duplicateID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM list_members WHERE contact_id = $1`, duplicateID); err != nil {
		return nil, err
	}
	// Propriedades personalizadas: a entidade fica em custom_properties, e só
	// migram as que o principal ainda não tem preenchidas.
	if err := mergeCustomValues(tx, "contacts", primaryID, duplicateID); err != nil {
		return nil, err
	}
	// Os anexos do duplicado seguem para o principal.
	if _, err := tx.Exec(`UPDATE attachments SET entity_id = $1 WHERE entity = $2 AND entity_id = $3`,
		primaryID, AttachContact, duplicateID); err != nil {
		return nil, err
	}

	// Preenche os vazios do principal com os dados do duplicado.
	merged := *primary
	fillEmpty(&merged.FirstName, duplicate.FirstName)
	fillEmpty(&merged.LastName, duplicate.LastName)
	fillEmpty(&merged.Email, duplicate.Email)
	fillEmpty(&merged.Phone, duplicate.Phone)
	fillEmpty(&merged.JobTitle, duplicate.JobTitle)
	fillEmpty(&merged.Source, duplicate.Source)
	if merged.CompanyID == nil {
		merged.CompanyID = duplicate.CompanyID
	}
	if merged.OwnerID == nil {
		merged.OwnerID = duplicate.OwnerID
	}

	if _, err := tx.Exec(`
		UPDATE contacts SET first_name = $1, last_name = $2, email = $3, phone = $4,
		       job_title = $5, source = $6, company_id = $7, owner_id = $8, updated_at = NOW()
		WHERE id = $9`,
		merged.FirstName, merged.LastName, merged.Email, merged.Phone, merged.JobTitle,
		merged.Source, merged.CompanyID, merged.OwnerID, primaryID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM contacts WHERE id = $1`, duplicateID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ContactByID(db, primaryID)
}

// MergeCompanies move tudo do duplicado para o principal e apaga o duplicado.
func MergeCompanies(db *sql.DB, primaryID, duplicateID int64) (*Company, error) {
	if primaryID == duplicateID {
		return nil, fmt.Errorf("escolha duas empresas diferentes")
	}

	primary, err := CompanyByID(db, primaryID)
	if err != nil {
		return nil, err
	}
	duplicate, err := CompanyByID(db, duplicateID)
	if err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for _, table := range companyRefs {
		if _, err := tx.Exec(fmt.Sprintf(
			`UPDATE %s SET company_id = $1 WHERE company_id = $2`, table), primaryID, duplicateID); err != nil {
			return nil, err
		}
	}
	if err := mergeCustomValues(tx, "companies", primaryID, duplicateID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE attachments SET entity_id = $1 WHERE entity = $2 AND entity_id = $3`,
		primaryID, AttachCompany, duplicateID); err != nil {
		return nil, err
	}

	merged := *primary
	fillEmpty(&merged.Name, duplicate.Name)
	fillEmpty(&merged.Domain, duplicate.Domain)
	fillEmpty(&merged.Phone, duplicate.Phone)
	fillEmpty(&merged.Industry, duplicate.Industry)
	fillEmpty(&merged.City, duplicate.City)
	fillEmpty(&merged.State, duplicate.State)
	fillEmpty(&merged.ECNumber, duplicate.ECNumber)
	fillEmpty(&merged.EconomicGroup, duplicate.EconomicGroup)
	fillEmpty(&merged.CNPJ, duplicate.CNPJ)
	fillEmpty(&merged.Representative, duplicate.Representative)
	fillEmpty(&merged.Instagram, duplicate.Instagram)
	fillEmpty(&merged.AnticipationMode, duplicate.AnticipationMode)
	if merged.OwnerID == nil {
		merged.OwnerID = duplicate.OwnerID
	}
	if merged.AccreditedAt == nil {
		merged.AccreditedAt = duplicate.AccreditedAt
	}
	if merged.MachinesCount == 0 {
		merged.MachinesCount = duplicate.MachinesCount
	}
	if len(merged.Products) == 0 {
		merged.Products = duplicate.Products
	}

	if _, err := tx.Exec(`
		UPDATE companies SET name = $1, domain = $2, phone = $3, industry = $4, city = $5,
		       state = $6, owner_id = $7, ec_number = $8, economic_group = $9, cnpj = $10,
		       accredited_at = $11, representative = $12, instagram = $13,
		       machines_count = $14, anticipation_mode = $15, updated_at = NOW()
		WHERE id = $16`,
		merged.Name, merged.Domain, merged.Phone, merged.Industry, merged.City, merged.State,
		merged.OwnerID, merged.ECNumber, merged.EconomicGroup, merged.CNPJ, merged.AccreditedAt,
		merged.Representative, merged.Instagram, merged.MachinesCount, merged.AnticipationMode,
		primaryID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM companies WHERE id = $1`, duplicateID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return CompanyByID(db, primaryID)
}

// mergeCustomValues leva os valores de propriedades personalizadas do duplicado
// para o principal, mantendo o que o principal já tinha preenchido.
func mergeCustomValues(tx *sql.Tx, entity string, primaryID, duplicateID int64) error {
	if _, err := tx.Exec(`
		UPDATE custom_property_values v SET record_id = $1
		FROM custom_properties p
		WHERE v.property_id = p.id AND p.entity = $3 AND v.record_id = $2
		  AND NOT EXISTS (
		      SELECT 1 FROM custom_property_values x
		      WHERE x.property_id = v.property_id AND x.record_id = $1)`,
		primaryID, duplicateID, entity); err != nil {
		return err
	}
	_, err := tx.Exec(`
		DELETE FROM custom_property_values v
		USING custom_properties p
		WHERE v.property_id = p.id AND p.entity = $2 AND v.record_id = $1`,
		duplicateID, entity)
	return err
}

// fillEmpty completa o destino apenas quando ele está vazio.
func fillEmpty(dst *string, src string) {
	if strings.TrimSpace(*dst) == "" {
		*dst = src
	}
}
