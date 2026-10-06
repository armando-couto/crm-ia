package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ListRules define os filtros de uma lista dinâmica.
type ListRules struct {
	LifecycleStage string `json:"lifecycle_stage,omitempty"`
	OwnerID        int64  `json:"owner_id,omitempty"`
	Source         string `json:"source,omitempty"`
}

type ContactList struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"` // estatica | dinamica
	Rules     *ListRules `json:"rules,omitempty"`
	CreatedBy *int64     `json:"created_by"`
	Members   int        `json:"members_count"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func ListContactLists(db *sql.DB) ([]ContactList, error) {
	rows, err := db.Query(`
		SELECT l.id, l.name, l.kind, l.rules, l.created_by,
		       (SELECT COUNT(*) FROM list_members m WHERE m.list_id = l.id),
		       l.created_at, l.updated_at
		FROM lists l ORDER BY l.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lists := []ContactList{}
	for rows.Next() {
		l, err := scanContactList(rows)
		if err != nil {
			return nil, err
		}
		lists = append(lists, *l)
	}
	return lists, rows.Err()
}

func scanContactList(row interface{ Scan(...any) error }) (*ContactList, error) {
	var l ContactList
	var rules sql.NullString
	if err := row.Scan(&l.ID, &l.Name, &l.Kind, &rules, &l.CreatedBy, &l.Members, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, err
	}
	if rules.Valid && rules.String != "" {
		var r ListRules
		if err := json.Unmarshal([]byte(rules.String), &r); err == nil {
			l.Rules = &r
		}
	}
	return &l, nil
}

func ContactListByID(db *sql.DB, id int64) (*ContactList, error) {
	row := db.QueryRow(`
		SELECT l.id, l.name, l.kind, l.rules, l.created_by,
		       (SELECT COUNT(*) FROM list_members m WHERE m.list_id = l.id),
		       l.created_at, l.updated_at
		FROM lists l WHERE l.id = $1`, id)
	return scanContactList(row)
}

func CreateContactList(db *sql.DB, l *ContactList) error {
	var rules any
	if l.Rules != nil {
		b, err := json.Marshal(l.Rules)
		if err != nil {
			return err
		}
		rules = string(b)
	}
	return db.QueryRow(`
		INSERT INTO lists (name, kind, rules, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		l.Name, l.Kind, rules, l.CreatedBy,
	).Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt)
}

func UpdateContactList(db *sql.DB, l *ContactList) error {
	var rules any
	if l.Rules != nil {
		b, err := json.Marshal(l.Rules)
		if err != nil {
			return err
		}
		rules = string(b)
	}
	_, err := db.Exec(`UPDATE lists SET name = $1, rules = $2, updated_at = NOW() WHERE id = $3`,
		l.Name, rules, l.ID)
	return err
}

func DeleteContactList(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM lists WHERE id = $1`, id)
	return err
}

func AddListMember(db *sql.DB, listID, contactID int64) error {
	_, err := db.Exec(`
		INSERT INTO list_members (list_id, contact_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, listID, contactID)
	return err
}

func RemoveListMember(db *sql.DB, listID, contactID int64) error {
	_, err := db.Exec(`DELETE FROM list_members WHERE list_id = $1 AND contact_id = $2`, listID, contactID)
	return err
}

// ListContactsOfList resolve os contatos da lista: estática via list_members,
// dinâmica aplicando as regras como filtro normal de contatos.
func ListContactsOfList(db *sql.DB, l *ContactList, p Pagination) ([]Contact, int, error) {
	if l.Kind == "dinamica" {
		f := ContactFilter{Pagination: p}
		if l.Rules != nil {
			f.LifecycleStage = l.Rules.LifecycleStage
			f.OwnerID = l.Rules.OwnerID
			f.Source = l.Rules.Source
		}
		return ListContacts(db, f)
	}

	p.Normalize()
	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM list_members WHERE list_id = $1`, l.ID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := db.Query(contactSelect+`
		JOIN list_members m ON m.contact_id = c.id
		WHERE m.list_id = $1
		ORDER BY c.first_name
		LIMIT $2 OFFSET $3`, l.ID, p.PerPage, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	contacts := []Contact{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, 0, err
		}
		contacts = append(contacts, *c)
	}
	return contacts, total, rows.Err()
}
