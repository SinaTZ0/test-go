package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const testDatabaseURL = "postgres://user:password@localhost:5432/go-todo"

func TestTodoAPI(t *testing.T) {
	ctx := context.Background()
	dsn := testDatabaseURL
	if envDSN := os.Getenv("DATABASE_URL"); envDSN != "" {
		dsn = envDSN
	}

	app := newTestApp(t, ctx, dsn)
	defer app.Close()

	resetTodos(t, ctx, dsn)

	t.Run("health", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("crud lifecycle", func(t *testing.T) {
		resetTodos(t, ctx, dsn)

		created := createTodo(t, app, `{"title":"Write tests","completed":false}`)
		if created.ID == 0 {
			t.Fatal("expected todo id to be set")
		}

		listRec := httptest.NewRecorder()
		listReq := httptest.NewRequest(http.MethodGet, "/todos", nil)
		app.ServeHTTP(listRec, listReq)

		if listRec.Code != http.StatusOK {
			t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
		}

		var listBody struct {
			Data []Todo `json:"data"`
		}
		decodeBody(t, listRec.Body.Bytes(), &listBody)
		if len(listBody.Data) != 1 {
			t.Fatalf("list length = %d, want 1", len(listBody.Data))
		}

		getRec := httptest.NewRecorder()
		getReq := httptest.NewRequest(http.MethodGet, "/todos/1", nil)
		app.ServeHTTP(getRec, getReq)
		if getRec.Code != http.StatusOK {
			t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
		}

		replaceRec := httptest.NewRecorder()
		replaceReq := httptest.NewRequest(http.MethodPut, "/todos/1", bytes.NewBufferString(`{"title":"Ship docs","description":"Published version","completed":true}`))
		replaceReq.Header.Set("Content-Type", "application/json")
		app.ServeHTTP(replaceRec, replaceReq)
		if replaceRec.Code != http.StatusOK {
			t.Fatalf("put status = %d, want %d", replaceRec.Code, http.StatusOK)
		}

		var replaced Todo
		decodeBody(t, replaceRec.Body.Bytes(), &replaced)
		if replaced.Title != "Ship docs" || !replaced.Completed {
			t.Fatalf("unexpected replaced todo: %+v", replaced)
		}

		updateRec := httptest.NewRecorder()
		updateReq := httptest.NewRequest(http.MethodPatch, "/todos/1", bytes.NewBufferString(`{"completed":true}`))
		updateReq.Header.Set("Content-Type", "application/json")
		app.ServeHTTP(updateRec, updateReq)
		if updateRec.Code != http.StatusOK {
			t.Fatalf("patch status = %d, want %d", updateRec.Code, http.StatusOK)
		}

		var updated Todo
		decodeBody(t, updateRec.Body.Bytes(), &updated)
		if !updated.Completed {
			t.Fatal("expected completed todo after patch")
		}

		deleteRec := httptest.NewRecorder()
		deleteReq := httptest.NewRequest(http.MethodDelete, "/todos/1", nil)
		app.ServeHTTP(deleteRec, deleteReq)
		if deleteRec.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusNoContent)
		}
	})

	t.Run("validation", func(t *testing.T) {
		resetTodos(t, ctx, dsn)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/todos", bytes.NewBufferString(`{"title":""}`))
		req.Header.Set("Content-Type", "application/json")
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

func newTestApp(t *testing.T, ctx context.Context, dsn string) *App {
	t.Helper()

	app, err := NewApp(ctx, dsn)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	return app
}

func resetTodos(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `TRUNCATE TABLE todos RESTART IDENTITY`); err != nil {
		t.Fatalf("reset todos: %v", err)
	}
}

func createTodo(t *testing.T, app *App, body string) Todo {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/todos", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var todo Todo
	decodeBody(t, rec.Body.Bytes(), &todo)
	return todo
}

func decodeBody(t *testing.T, data []byte, target any) {
	t.Helper()

	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}
