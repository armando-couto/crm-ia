package models

import (
	"database/sql"
)

type Pipeline struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	Position int             `json:"position"`
	Stages   []PipelineStage `json:"stages"`
}

type PipelineStage struct {
	ID          int64  `json:"id"`
	PipelineID  int64  `json:"pipeline_id"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	Probability int    `json:"probability"`
	IsWon       bool   `json:"is_won"`
	IsLost      bool   `json:"is_lost"`
}

func ListPipelines(db *sql.DB) ([]Pipeline, error) {
	rows, err := db.Query(`SELECT id, name, position FROM pipelines ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pipelines := []Pipeline{}
	for rows.Next() {
		var p Pipeline
		if err := rows.Scan(&p.ID, &p.Name, &p.Position); err != nil {
			return nil, err
		}
		p.Stages = []PipelineStage{}
		pipelines = append(pipelines, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stageRows, err := db.Query(`
		SELECT id, pipeline_id, name, position, probability, is_won, is_lost
		FROM pipeline_stages ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer stageRows.Close()

	for stageRows.Next() {
		var s PipelineStage
		if err := stageRows.Scan(&s.ID, &s.PipelineID, &s.Name, &s.Position, &s.Probability, &s.IsWon, &s.IsLost); err != nil {
			return nil, err
		}
		for i := range pipelines {
			if pipelines[i].ID == s.PipelineID {
				pipelines[i].Stages = append(pipelines[i].Stages, s)
			}
		}
	}
	return pipelines, stageRows.Err()
}

func StageByID(db *sql.DB, id int64) (*PipelineStage, error) {
	var s PipelineStage
	err := db.QueryRow(`
		SELECT id, pipeline_id, name, position, probability, is_won, is_lost
		FROM pipeline_stages WHERE id = $1`, id,
	).Scan(&s.ID, &s.PipelineID, &s.Name, &s.Position, &s.Probability, &s.IsWon, &s.IsLost)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func CreatePipeline(db *sql.DB, p *Pipeline) error {
	return db.QueryRow(`INSERT INTO pipelines (name, position) VALUES ($1, $2) RETURNING id`,
		p.Name, p.Position).Scan(&p.ID)
}

func UpdatePipeline(db *sql.DB, p *Pipeline) error {
	_, err := db.Exec(`UPDATE pipelines SET name = $1, position = $2 WHERE id = $3`, p.Name, p.Position, p.ID)
	return err
}

func DeletePipeline(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM pipelines WHERE id = $1`, id)
	return err
}

func CreateStage(db *sql.DB, s *PipelineStage) error {
	return db.QueryRow(`
		INSERT INTO pipeline_stages (pipeline_id, name, position, probability, is_won, is_lost)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		s.PipelineID, s.Name, s.Position, s.Probability, s.IsWon, s.IsLost).Scan(&s.ID)
}

func UpdateStage(db *sql.DB, s *PipelineStage) error {
	_, err := db.Exec(`
		UPDATE pipeline_stages SET name = $1, position = $2, probability = $3, is_won = $4, is_lost = $5
		WHERE id = $6`,
		s.Name, s.Position, s.Probability, s.IsWon, s.IsLost, s.ID)
	return err
}

func DeleteStage(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM pipeline_stages WHERE id = $1`, id)
	return err
}
