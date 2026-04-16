package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxRequestBodyBytes = 1 << 20

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

type Todo struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Completed   bool      `json:"completed"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateTodoRequest struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Completed   bool   `json:"completed"`
}

type UpdateTodoRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Completed   *bool   `json:"completed,omitempty"`
}

type TodoStore struct {
	pool *pgxpool.Pool
}

func NewTodoStore(ctx context.Context, dsn string) (*TodoStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &TodoStore{pool: pool}
	if err := store.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func (s *TodoStore) Close() {
	s.pool.Close()
}

func (s *TodoStore) ensureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, todoTableSchema)
	if err != nil {
		return fmt.Errorf("ensure todos table: %w", err)
	}

	return nil
}

func (s *TodoStore) List(ctx context.Context) ([]Todo, error) {
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

func (s *TodoStore) Create(ctx context.Context, req CreateTodoRequest) (Todo, error) {
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

func (s *TodoStore) Replace(ctx context.Context, id int64, req CreateTodoRequest) (Todo, bool, error) {
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

func (s *TodoStore) Get(ctx context.Context, id int64) (Todo, bool, error) {
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

func (s *TodoStore) Update(ctx context.Context, id int64, req UpdateTodoRequest) (Todo, bool, error) {
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

func (s *TodoStore) Delete(ctx context.Context, id int64) (bool, error) {
	result, err := s.pool.Exec(ctx, `DELETE FROM todos WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete todo: %w", err)
	}

	return result.RowsAffected() > 0, nil
}

type App struct {
	store *TodoStore
	mux   *http.ServeMux
}

func NewApp(ctx context.Context, dsn string) (*App, error) {
	store, err := NewTodoStore(ctx, dsn)
	if err != nil {
		return nil, err
	}

	app := &App{
		store: store,
		mux:   http.NewServeMux(),
	}

	app.mux.HandleFunc("GET /healthz", app.handleHealth)
	app.mux.HandleFunc("GET /todos", app.handleListTodos)
	app.mux.HandleFunc("POST /todos", app.handleCreateTodo)
	app.mux.HandleFunc("GET /todos/{id}", app.handleGetTodo)
	app.mux.HandleFunc("PUT /todos/{id}", app.handleReplaceTodo)
	app.mux.HandleFunc("PATCH /todos/{id}", app.handleUpdateTodo)
	app.mux.HandleFunc("DELETE /todos/{id}", app.handleDeleteTodo)

	return app, nil
}

func (a *App) Close() error {
	a.store.Close()
	return nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleListTodos(w http.ResponseWriter, r *http.Request) {
	todos, err := a.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list todos")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": todos})
}

func (a *App) handleCreateTodo(w http.ResponseWriter, r *http.Request) {
	var req CreateTodoRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "title is required")
		return
	}

	todo, err := a.store.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create todo")
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/todos/%d", todo.ID))
	writeJSON(w, http.StatusCreated, todo)
}

func (a *App) handleGetTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseTodoID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	todo, ok, err := a.store.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load todo")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "todo not found")
		return
	}

	writeJSON(w, http.StatusOK, todo)
}

func (a *App) handleReplaceTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseTodoID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	var req CreateTodoRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "title is required")
		return
	}

	todo, ok, err := a.store.Replace(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to replace todo")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "todo not found")
		return
	}

	writeJSON(w, http.StatusOK, todo)
}

func (a *App) handleUpdateTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseTodoID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	var req UpdateTodoRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.Title == nil && req.Description == nil && req.Completed == nil {
		writeError(w, http.StatusBadRequest, "validation_error", "at least one field must be provided")
		return
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "title cannot be empty")
		return
	}

	todo, ok, err := a.store.Update(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update todo")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "todo not found")
		return
	}

	writeJSON(w, http.StatusOK, todo)
}

func (a *App) handleDeleteTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseTodoID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	deleted, err := a.store.Delete(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete todo")
		return
	}

	if !deleted {
		writeError(w, http.StatusNotFound, "not_found", "todo not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	defer body.Close()

	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}

func parseTodoID(raw string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, errors.New("missing todo id")
	}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("todo id must be a positive integer")
	}

	return id, nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, code, message string) {
	writeJSON(w, statusCode, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func scanTodo(scanner interface{ Scan(dest ...any) error }) (Todo, error) {
	var todo Todo
	if err := scanner.Scan(&todo.ID, &todo.Title, &todo.Description, &todo.Completed, &todo.CreatedAt, &todo.UpdatedAt); err != nil {
		return Todo{}, err
	}

	return todo, nil
}
