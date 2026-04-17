package todo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const todoTableSchema = `
CREATE TABLE IF NOT EXISTS todos (
	id BIGSERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	completed BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
)
`

// Store wraps the Postgres connection pool used by the todo application.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore opens a Postgres-backed todo store and ensures the schema exists.
func NewStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &Store{pool: pool}
	if err := store.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) ensureSchema(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, todoTableSchema); err != nil {
		return fmt.Errorf("ensure todos table: %w", err)
	}

	return nil
}

// List returns all todos ordered by their identifier.
func (s *Store) List(ctx context.Context) ([]Todo, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, title, description, completed, created_at, updated_at FROM todos ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}
	defer rows.Close()

	todos := make([]Todo, 0)
	for rows.Next() {
		todo, err := scanTodo(rows)
		if err != nil {
			return nil, fmt.Errorf("scan todo: %w", err)
		}
		todos = append(todos, todo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate todos: %w", err)
	}

	sort.Slice(todos, func(i, j int) bool {
		return todos[i].ID < todos[j].ID
	})

	return todos, nil
}

// Create inserts a new todo and returns the persisted record.
func (s *Store) Create(ctx context.Context, req CreateTodoRequest) (Todo, error) {
	now := time.Now().UTC()
	row := s.pool.QueryRow(ctx, `
		INSERT INTO todos (title, description, completed, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, title, description, completed, created_at, updated_at
	`, strings.TrimSpace(req.Title), strings.TrimSpace(req.Description), req.Completed, now, now)

	todo, err := scanTodo(row)
	if err != nil {
		return Todo{}, fmt.Errorf("create todo: %w", err)
	}

	return todo, nil
}

// Replace updates all mutable todo fields and returns the persisted record.
func (s *Store) Replace(ctx context.Context, id int64, req CreateTodoRequest) (Todo, bool, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE todos
		SET title = $2, description = $3, completed = $4, updated_at = NOW()
		WHERE id = $1
		RETURNING id, title, description, completed, created_at, updated_at
	`, id, strings.TrimSpace(req.Title), strings.TrimSpace(req.Description), req.Completed)

	todo, err := scanTodo(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Todo{}, false, nil
		}
		return Todo{}, false, fmt.Errorf("replace todo: %w", err)
	}

	return todo, true, nil
}

// Get fetches a todo by identifier.
func (s *Store) Get(ctx context.Context, id int64) (Todo, bool, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, title, description, completed, created_at, updated_at
		FROM todos
		WHERE id = $1
	`, id)

	todo, err := scanTodo(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Todo{}, false, nil
		}
		return Todo{}, false, fmt.Errorf("get todo: %w", err)
	}

	return todo, true, nil
}

// Update applies a partial update to a todo and returns the persisted record.
func (s *Store) Update(ctx context.Context, id int64, req UpdateTodoRequest) (Todo, bool, error) {
	setClauses := make([]string, 0, 3)
	args := []any{id}
	argPos := 2

	if req.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argPos))
		args = append(args, strings.TrimSpace(*req.Title))
		argPos++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argPos))
		args = append(args, strings.TrimSpace(*req.Description))
		argPos++
	}
	if req.Completed != nil {
		setClauses = append(setClauses, fmt.Sprintf("completed = $%d", argPos))
		args = append(args, *req.Completed)
		argPos++
	}

	if len(setClauses) == 0 {
		return Todo{}, false, errors.New("at least one field must be provided")
	}

	query := fmt.Sprintf(`
		UPDATE todos
		SET %s, updated_at = NOW()
		WHERE id = $1
		RETURNING id, title, description, completed, created_at, updated_at
	`, strings.Join(setClauses, ", "))

	row := s.pool.QueryRow(ctx, query, args...)
	todo, err := scanTodo(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Todo{}, false, nil
		}
		return Todo{}, false, fmt.Errorf("update todo: %w", err)
	}

	return todo, true, nil
}

// Delete removes a todo and reports whether a row was deleted.
func (s *Store) Delete(ctx context.Context, id int64) (bool, error) {
	result, err := s.pool.Exec(ctx, `DELETE FROM todos WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete todo: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

func scanTodo(scanner interface{ Scan(dest ...any) error }) (Todo, error) {
	var todo Todo
	if err := scanner.Scan(&todo.ID, &todo.Title, &todo.Description, &todo.Completed, &todo.CreatedAt, &todo.UpdatedAt); err != nil {
		return Todo{}, err
	}

	return todo, nil
}
