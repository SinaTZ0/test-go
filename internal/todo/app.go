package todo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const maxRequestBodyBytes = 1 << 20

// App wires the todo store to HTTP handlers.
type App struct {
	store *Store
	mux   *http.ServeMux
}

// NewApp constructs an HTTP application backed by the supplied store.
func NewApp(store *Store) (*App, error) {
	if store == nil {
		return nil, errors.New("store is required")
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

// Close releases the application's underlying resources.
func (a *App) Close() error {
	a.store.Close()
	return nil
}

// ServeHTTP dispatches requests to the application's router.
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
