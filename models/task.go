package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

var TaskTypes = []string{"ligacao", "email", "reuniao", "tarefa"}

type Task struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Type        string     `json:"type"`
	Priority    string     `json:"priority"`
	DueDate     *time.Time `json:"due_date"`
	CompletedAt *time.Time `json:"completed_at"`
	OwnerID     *int64     `json:"owner_id"`
	OwnerName   string     `json:"owner_name,omitempty"`
	ContactID   *int64     `json:"contact_id"`
	ContactName string     `json:"contact_name,omitempty"`
	CompanyID   *int64     `json:"company_id"`
	DealID      *int64     `json:"deal_id"`
	ProjectID   *int64     `json:"project_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type TaskFilter struct {
	OwnerID   int64
	ContactID int64
	CompanyID int64
	DealID    int64
	ProjectID int64
	Status    string // pendente | concluida | atrasada
	Pagination
}

const taskSelect = `
	SELECT t.id, t.title, COALESCE(t.description,''), t.type, t.priority, t.due_date, t.completed_at,
	       t.owner_id, COALESCE(u.name,''),
	       t.contact_id, COALESCE(ct.first_name || ' ' || COALESCE(ct.last_name,''), ''),
	       t.company_id, t.deal_id, t.project_id, t.created_at, t.updated_at
	FROM tasks t
	LEFT JOIN users u ON u.id = t.owner_id
	LEFT JOIN contacts ct ON ct.id = t.contact_id`

func scanTask(row interface{ Scan(...any) error }) (*Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.Title, &t.Description, &t.Type, &t.Priority, &t.DueDate, &t.CompletedAt,
		&t.OwnerID, &t.OwnerName, &t.ContactID, &t.ContactName, &t.CompanyID, &t.DealID, &t.ProjectID,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.ContactName = strings.TrimSpace(t.ContactName)
	return &t, nil
}

func ListTasks(db *sql.DB, f TaskFilter) ([]Task, int, error) {
	f.Normalize()

	where := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.OwnerID > 0 {
		add("t.owner_id = $%d", f.OwnerID)
	}
	if f.ContactID > 0 {
		add("t.contact_id = $%d", f.ContactID)
	}
	if f.CompanyID > 0 {
		add("t.company_id = $%d", f.CompanyID)
	}
	if f.DealID > 0 {
		add("t.deal_id = $%d", f.DealID)
	}
	if f.ProjectID > 0 {
		add("t.project_id = $%d", f.ProjectID)
	}
	switch f.Status {
	case "pendente":
		where = append(where, "t.completed_at IS NULL")
	case "concluida":
		where = append(where, "t.completed_at IS NOT NULL")
	case "atrasada":
		where = append(where, "t.completed_at IS NULL AND t.due_date < NOW()")
	}
	cond := strings.Join(where, " AND ")

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM tasks t WHERE `+cond, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, f.Offset())
	rows, err := db.Query(fmt.Sprintf(
		`%s WHERE %s ORDER BY t.completed_at NULLS FIRST, t.due_date NULLS LAST, t.id DESC LIMIT $%d OFFSET $%d`,
		taskSelect, cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *t)
	}
	return list, total, rows.Err()
}

func TaskByID(db *sql.DB, id int64) (*Task, error) {
	row := db.QueryRow(taskSelect+` WHERE t.id = $1`, id)
	return scanTask(row)
}

func CreateTask(db *sql.DB, t *Task) error {
	if t.Type == "" {
		t.Type = "tarefa"
	}
	if t.Priority == "" {
		t.Priority = "media"
	}
	return db.QueryRow(`
		INSERT INTO tasks (title, description, type, priority, due_date, owner_id, contact_id, company_id, deal_id, project_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`,
		t.Title, t.Description, t.Type, t.Priority, t.DueDate, t.OwnerID, t.ContactID, t.CompanyID, t.DealID, t.ProjectID,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func UpdateTask(db *sql.DB, t *Task) error {
	_, err := db.Exec(`
		UPDATE tasks SET title = $1, description = $2, type = $3, priority = $4, due_date = $5,
		       owner_id = $6, contact_id = $7, company_id = $8, deal_id = $9, project_id = $10, updated_at = NOW()
		WHERE id = $11`,
		t.Title, t.Description, t.Type, t.Priority, t.DueDate, t.OwnerID, t.ContactID, t.CompanyID, t.DealID, t.ProjectID, t.ID)
	return err
}

// ToggleTask marca ou desmarca a conclusão da tarefa.
func ToggleTask(db *sql.DB, id int64, done bool) error {
	if done {
		_, err := db.Exec(`UPDATE tasks SET completed_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
		return err
	}
	_, err := db.Exec(`UPDATE tasks SET completed_at = NULL, updated_at = NOW() WHERE id = $1`, id)
	return err
}

func DeleteTask(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM tasks WHERE id = $1`, id)
	return err
}
