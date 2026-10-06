package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
)

//go:embed sql/*.sql
var files embed.FS

// Run aplica, em ordem, todas as migrations SQL ainda não executadas.
// O controle de versão fica na tabela schema_migrations.
func Run(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	if err != nil {
		return fmt.Errorf("criando schema_migrations: %w", err)
	}

	entries, err := files.ReadDir("sql")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var exists bool
		err = db.QueryRow(`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		content, err := files.ReadFile("sql/" + name)
		if err != nil {
			return err
		}

		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
