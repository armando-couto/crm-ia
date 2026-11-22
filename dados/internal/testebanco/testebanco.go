// Package testebanco cria o banco dos testes de integração: PostgreSQL de
// verdade (CRMIA_TEST_PG; padrão o PostgreSQL local de desenvolvimento), um
// banco por pacote para os testes rodarem em paralelo. A contagem, a
// paginação e a troca de esquema vivem no SQL: um banco de mentira não
// provaria nada.
package testebanco

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Criar apaga e recria o banco nome e devolve a URL dele. ok=false quando
// não há PostgreSQL de teste (os testes de integração são pulados).
func Criar(nome string) (url string, ok bool) {
	admin := os.Getenv("CRMIA_TEST_PG")
	if admin == "" {
		admin = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "AVISO: PostgreSQL de teste indisponível, pulando os testes de integração:", err)
		return "", false
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `DROP DATABASE IF EXISTS `+nome+` WITH (FORCE)`); err != nil {
		fmt.Fprintln(os.Stderr, "não consegui apagar o banco de teste:", err)
		os.Exit(1)
	}
	if _, err := conn.Exec(ctx, `CREATE DATABASE `+nome); err != nil {
		fmt.Fprintln(os.Stderr, "não consegui criar o banco de teste:", err)
		os.Exit(1)
	}
	cfg := conn.Config()
	antigo := "/" + cfg.Database
	i := strings.Index(admin, antigo+"?")
	if i < 0 {
		if !strings.HasSuffix(admin, antigo) {
			fmt.Fprintln(os.Stderr, "CRMIA_TEST_PG precisa terminar no nome do banco administrativo")
			os.Exit(1)
		}
		return strings.TrimSuffix(admin, antigo) + "/" + nome, true
	}
	return admin[:i] + "/" + nome + admin[i+len(antigo):], true
}
