package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	todoapp "github.com/SinaTZ0/test-go/internal/todo"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var databaseURL string
	flag.StringVar(&databaseURL, "database-url", os.Getenv("DATABASE_URL"), "Postgres connection string")
	flag.Parse()

	if databaseURL == "" {
		databaseURL = "postgres://user:password@localhost:5432/go-todo"
	}

	if err := runMigration(context.Background(), databaseURL); err != nil {
		log.Fatal(err)
	}

	fmt.Println("migration complete")
}

func runMigration(ctx context.Context, databaseURL string) error {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	schema, err := todoapp.LoadSchemaSQL()
	if err != nil {
		return fmt.Errorf("load schema: %w", err)
	}

	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}

	return nil
}
