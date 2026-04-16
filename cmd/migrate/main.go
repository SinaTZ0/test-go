package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

const migrateSchema = `
CREATE TABLE IF NOT EXISTS todos (
	id BIGSERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	completed BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
)
`

func main() {
	var dsn string
	flag.StringVar(&dsn, "dsn", os.Getenv("DATABASE_URL"), "Postgres connection string")
	flag.Parse()

	if dsn == "" {
		dsn = "postgres://user:password@localhost:5432/go-todo"
	}

	if err := runMigration(context.Background(), dsn); err != nil {
		log.Fatal(err)
	}

	fmt.Println("migration complete")
}

func runMigration(ctx context.Context, dsn string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	if _, err := pool.Exec(ctx, migrateSchema); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}

	return nil
}
