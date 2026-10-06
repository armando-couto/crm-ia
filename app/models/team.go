package models

import (
	"database/sql"
	"time"
)

type Team struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	MembersCount int       `json:"members_count"`
	CreatedAt    time.Time `json:"created_at"`
}

func ListTeams(db *sql.DB) ([]Team, error) {
	rows, err := db.Query(`
		SELECT t.id, t.name,
		       (SELECT COUNT(*) FROM users u WHERE u.team_id = t.id AND u.active),
		       t.created_at
		FROM teams t ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teams := []Team{}
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.MembersCount, &t.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}

func CreateTeam(db *sql.DB, t *Team) error {
	return db.QueryRow(`INSERT INTO teams (name) VALUES ($1) RETURNING id, created_at`,
		t.Name).Scan(&t.ID, &t.CreatedAt)
}

func UpdateTeam(db *sql.DB, t *Team) error {
	_, err := db.Exec(`UPDATE teams SET name = $1 WHERE id = $2`, t.Name, t.ID)
	return err
}

// DeleteTeam remove a equipe; os usuários ficam sem equipe (ON DELETE SET NULL).
func DeleteTeam(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM teams WHERE id = $1`, id)
	return err
}
