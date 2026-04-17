package main

import (
	"context"

	todoapp "github.com/SinaTZ0/test-go/internal/todo"
)

// Todo mirrors the application todo model for compatibility with existing tests.
type Todo = todoapp.Todo

// CreateTodoRequest mirrors the request body used to create or replace a todo.
type CreateTodoRequest = todoapp.CreateTodoRequest

// UpdateTodoRequest mirrors the request body used for partial todo updates.
type UpdateTodoRequest = todoapp.UpdateTodoRequest

// TodoStore exposes the store type for compatibility with existing tests.
type TodoStore = todoapp.Store

// App exposes the HTTP application type for compatibility with existing tests.
type App = todoapp.App

// NewTodoStore opens a todo store backed by the supplied Postgres connection string.
func NewTodoStore(ctx context.Context, dsn string) (*TodoStore, error) {
	return todoapp.NewStore(ctx, dsn)
}

// NewApp opens a todo store and returns a ready-to-serve HTTP application.
func NewApp(ctx context.Context, dsn string) (*App, error) {
	store, err := todoapp.NewStore(ctx, dsn)
	if err != nil {
		return nil, err
	}

	return todoapp.NewApp(store)
}
