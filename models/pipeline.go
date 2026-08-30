package models

import (
	"database/sql"
)

type Pipeline struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	Position   int             `json:"position"`
	DealsCount int             `json:"deals_count"`
	Stages     []PipelineStage `json:"stages"`
}

type PipelineStage struct {
	ID          int64  `json:"id"`
	PipelineID  int64  `json:"pipeline_id"`
	Name        string `json:"name"`
	Position    int    `json:"position"`
	Probability int    `json:"probability"`
	IsWon       bool   `json:"is_won"`
	IsLost      bool   `json:"is_lost"`
	DealsCount  int    `json:"deals_count"`
}

func ListPipelines(db *sql.DB) ([]Pipeline, error) {
	rows, err := db.Query(`
		SELECT p.id, p.name, p.position,
		       (SELECT COUNT(*) FROM deals d WHERE d.pipeline_id = p.id)
		FROM pipelines p ORDER BY p.position, p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pipelines := []Pipeline{}
	for rows.Next() {
		var p Pipeline
		if err := rows.Scan(&p.ID, &p.Name, &p.Position, &p.DealsCount); err != nil {
			return nil, err
		}
		p.Stages = []PipelineStage{}
		pipelines = append(pipelines, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stageRows, err := db.Query(`
		SELECT s.id, s.pipeline_id, s.name, s.position, s.probability, s.is_won, s.is_lost,
		       (SELECT COUNT(*) FROM deals d WHERE d.stage_id = s.id)
		FROM pipeline_stages s ORDER BY s.position, s.id`)
	if err != nil {
		return nil, err
	}
	defer stageRows.Close()

	for stageRows.Next() {
		var s PipelineStage
		if err := stageRows.Scan(&s.ID, &s.PipelineID, &s.Name, &s.Position, &s.Probability,
			&s.IsWon, &s.IsLost, &s.DealsCount); err != nil {
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

// ReorderStages aplica a nova ordem das fases: as posições seguem a ordem
// dos IDs informados. Só fases do próprio pipeline são aceitas.
func ReorderStages(db *sql.DB, pipelineID int64, stageIDs []int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for position, stageID := range stageIDs {
		res, err := tx.Exec(`UPDATE pipeline_stages SET position = $1 WHERE id = $2 AND pipeline_id = $3`,
			position, stageID, pipelineID)
		if err != nil {
			tx.Rollback()
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			tx.Rollback()
			return sql.ErrNoRows
		}
	}
	return tx.Commit()
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
