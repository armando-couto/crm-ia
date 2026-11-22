// Package banco conecta ao PostgreSQL do serviço de dados e aplica as
// migrações embutidas (só as tabelas de controle: os esquemas de serviço são
// construídos e trocados pela carga).
package banco

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migracoes/*.sql
var migracoes embed.FS

// Conectar abre um pool e espera o banco responder. parametros vão para a
// sessão de cada conexão (jit, work_mem, statement_timeout).
func Conectar(ctx context.Context, url string, conexoes int32, parametros map[string]string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	cfg.MaxConns = conexoes
	for k, v := range parametros {
		cfg.ConnConfig.RuntimeParams[k] = v
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var ultimo error
	for i := 0; i < 30; i++ {
		if ultimo = pool.Ping(ctx); ultimo == nil {
			return pool, nil
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	pool.Close()
	return nil, fmt.Errorf("PostgreSQL não respondeu: %w", ultimo)
}

// Migrar aplica, em ordem, as migrações ainda não executadas. Migração já
// aplicada é imutável: mudança de esquema é sempre um arquivo novo.
func Migrar(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS migracoes (versao TEXT PRIMARY KEY, aplicada_em TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entradas, err := migracoes.ReadDir("migracoes")
	if err != nil {
		return err
	}
	nomes := make([]string, 0, len(entradas))
	for _, e := range entradas {
		nomes = append(nomes, e.Name())
	}
	sort.Strings(nomes)
	for _, nome := range nomes {
		var existe bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM migracoes WHERE versao=$1)`, nome).Scan(&existe); err != nil {
			return err
		}
		if existe {
			continue
		}
		sql, err := migracoes.ReadFile("migracoes/" + nome)
		if err != nil {
			return err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migração %s: %w", nome, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO migracoes (versao) VALUES ($1)`, nome); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
