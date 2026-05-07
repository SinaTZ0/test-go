-- name: ListTodos :many
SELECT id, title, description, completed, created_at, updated_at
FROM todos
ORDER BY id;

-- name: GetTodo :one
SELECT id, title, description, completed, created_at, updated_at
FROM todos
WHERE id = sqlc.arg(id);

-- name: CreateTodo :one
INSERT INTO todos (
	title,
	description,
	completed,
	created_at,
	updated_at
) VALUES (
	sqlc.arg(title),
	sqlc.arg(description),
	sqlc.arg(completed),
	NOW(),
	NOW()
)
RETURNING id, title, description, completed, created_at, updated_at;

-- name: ReplaceTodo :one
UPDATE todos
SET
	title = sqlc.arg(title),
	description = sqlc.arg(description),
	completed = sqlc.arg(completed),
	updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING id, title, description, completed, created_at, updated_at;

-- name: UpdateTodo :one
UPDATE todos
SET
	title = COALESCE(sqlc.narg(title), title),
	description = COALESCE(sqlc.narg(description), description),
	completed = COALESCE(sqlc.narg(completed), completed),
	updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING id, title, description, completed, created_at, updated_at;

-- name: DeleteTodo :execrows
DELETE FROM todos
WHERE id = sqlc.arg(id);