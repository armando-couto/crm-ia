package models

import (
	"database/sql"
	"slices"
	"time"
)

var ProjectStatuses = []string{"ativo", "concluido", "arquivado"}

func ValidProjectStatus(s string) bool {
	return slices.Contains(ProjectStatuses, s)
}

type Project struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	DueDate     *time.Time `json:"due_date"`
	OwnerID     *int64     `json:"owner_id"`
	OwnerName   string     `json:"owner_name,omitempty"`
	TasksTotal  int        `json:"tasks_total"`
	TasksDone   int        `json:"tasks_done"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

const projectSelect = `
	SELECT p.id, p.name, COALESCE(p.description,''), p.status, p.due_date,
	       p.owner_id, COALESCE(u.name,''),
	       (SELECT COUNT(*) FROM tasks t WHERE t.project_id = p.id),
	       (SELECT COUNT(*) FROM tasks t WHERE t.project_id = p.id AND t.completed_at IS NOT NULL),
	       p.created_at, p.updated_at
	FROM projects p
	LEFT JOIN users u ON u.id = p.owner_id`

func scanProject(row interface{ Scan(...any) error }) (*Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Status, &p.DueDate,
		&p.OwnerID, &p.OwnerName, &p.TasksTotal, &p.TasksDone, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func ListProjects(db *sql.DB, status string) ([]Project, error) {
	query := projectSelect
	args := []any{}
	if status != "" {
		query += ` WHERE p.status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY p.updated_at DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	return list, rows.Err()
}

func ProjectByID(db *sql.DB, id int64) (*Project, error) {
	row := db.QueryRow(projectSelect+` WHERE p.id = $1`, id)
	return scanProject(row)
}

func CreateProject(db *sql.DB, p *Project) error {
	if p.Status == "" {
		p.Status = "ativo"
	}
	return db.QueryRow(`
		INSERT INTO projects (name, description, status, due_date, owner_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`,
		p.Name, p.Description, p.Status, p.DueDate, p.OwnerID,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func UpdateProject(db *sql.DB, p *Project) error {
	_, err := db.Exec(`
		UPDATE projects SET name = $1, description = $2, status = $3, due_date = $4, owner_id = $5, updated_at = NOW()
		WHERE id = $6`,
		p.Name, p.Description, p.Status, p.DueDate, p.OwnerID, p.ID)
	return err
}

func DeleteProject(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM projects WHERE id = $1`, id)
	return err
}
