package todo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SinaTZ0/test-go/internal/todo/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps the Postgres connection pool used by the todo application.
type Store struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewStore opens a Postgres-backed todo store and ensures the schema exists.
func NewStore(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &Store{pool: pool, queries: db.New(pool)}
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
	schema, err := LoadSchemaSQL()
	if err != nil {
		return fmt.Errorf("load todos schema: %w", err)
	}

	if _, err := s.pool.Exec(ctx, string(schema)); err != nil {
		return fmt.Errorf("ensure todos table: %w", err)
	}

	return nil
}

// List returns all todos ordered by their identifier.
func (s *Store) List(ctx context.Context) ([]Todo, error) {
	records, err := s.queries.ListTodos(ctx)
	if err != nil {
		return nil, fmt.Errorf("list todos: %w", err)
	}

	todos := make([]Todo, 0, len(records))
	for _, record := range records {
		todos = append(todos, fromDBTodo(record))
	}

	return todos, nil
}

// Create inserts a new todo and returns the persisted record.
func (s *Store) Create(ctx context.Context, req CreateTodoRequest) (Todo, error) {
	record, err := s.queries.CreateTodo(ctx, db.CreateTodoParams{
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description),
		Completed:   req.Completed,
	})
	if err != nil {
		return Todo{}, fmt.Errorf("create todo: %w", err)
	}

	return fromDBTodo(record), nil
}

// Replace updates all mutable todo fields and returns the persisted record.
func (s *Store) Replace(ctx context.Context, id int64, req CreateTodoRequest) (Todo, bool, error) {
	record, err := s.queries.ReplaceTodo(ctx, db.ReplaceTodoParams{
		ID:          id,
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description),
		Completed:   req.Completed,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Todo{}, false, nil
		}
		return Todo{}, false, fmt.Errorf("replace todo: %w", err)
	}

	return fromDBTodo(record), true, nil
}

// Get fetches a todo by identifier.
func (s *Store) Get(ctx context.Context, id int64) (Todo, bool, error) {
	record, err := s.queries.GetTodo(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Todo{}, false, nil
		}
		return Todo{}, false, fmt.Errorf("get todo: %w", err)
	}

	return fromDBTodo(record), true, nil
}

// Red
// Update applies a partial update to a todo and returns the persisted record.
func (s *Store) Update(ctx context.Context, id int64, req UpdateTodoRequest) (Todo, bool, error) {
	if req.Title == nil && req.Description == nil && req.Completed == nil {
		return Todo{}, false, errors.New("at least one field must be provided")
	}

	params := db.UpdateTodoParams{
		Completed: req.Completed,
		ID:        id,
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		params.Title = &title
	}

	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		params.Description = &description
	}

	record, err := s.queries.UpdateTodo(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Todo{}, false, nil
		}
		return Todo{}, false, fmt.Errorf("update todo: %w", err)
	}

	return fromDBTodo(record), true, nil
}

// Blue
// Update applies a partial update to a todo and returns the persisted record.
// func (s *Store) Update(ctx context.Context, id int64, req UpdateTodoRequest) (Todo, bool, error) {
// 	if req.Title == nil && req.Description == nil && req.Completed == nil {
// 		return Todo{}, false, errors.New("at least one field must be provided")
// 	}

// 	params := db.UpdateTodoParams{ID: id}
// 	if req.Title != nil {
// 		params.Title = pgtype.Text{String: strings.TrimSpace(*req.Title), Valid: true}
// 	}

// 	if req.Description != nil {
// 		params.Description = pgtype.Text{String: strings.TrimSpace(*req.Description), Valid: true}
// 	}

// 	if req.Completed != nil {
// 		params.Completed = pgtype.Bool{Bool: *req.Completed, Valid: true}
// 	}

// 	record, err := s.queries.UpdateTodo(ctx, params)
// 	if err != nil {
// 		if errors.Is(err, pgx.ErrNoRows) {
// 			return Todo{}, false, nil
// 		}
// 		return Todo{}, false, fmt.Errorf("update todo: %w", err)
// 	}

// 	return fromDBTodo(record), true, nil
// }

// Delete removes a todo and reports whether a row was deleted.
func (s *Store) Delete(ctx context.Context, id int64) (bool, error) {
	rowsAffected, err := s.queries.DeleteTodo(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete todo: %w", err)
	}

	return rowsAffected > 0, nil
}

func fromDBTodo(record db.Todo) Todo {
	return Todo{
		ID:          record.ID,
		Title:       record.Title,
		Description: record.Description,
		Completed:   record.Completed,
		CreatedAt:   record.CreatedAt.Time,
		UpdatedAt:   record.UpdatedAt.Time,
	}
}
