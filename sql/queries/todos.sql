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
	sqlc.narg(description),
	sqlc.arg(completed),
	NOW(),
	NOW()
)
RETURNING id, title, description, completed, created_at, updated_at;

-- name: ReplaceTodo :one
UPDATE todos
SET
	title = sqlc.arg(title),
	description = sqlc.narg(description),
	completed = sqlc.arg(completed),
	updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING id, title, description, completed, created_at, updated_at;

-- name: UpdateTodo :one
UPDATE todos
SET
	title = COALESCE(sqlc.narg(title), title),
	description = CASE WHEN sqlc.arg(description_set)::bool THEN sqlc.narg(description) ELSE description END,
	completed = COALESCE(sqlc.narg(completed), completed),
	updated_at = NOW()
WHERE id = sqlc.arg(id)
RETURNING id, title, description, completed, created_at, updated_at;

-- name: DeleteTodo :execrows
DELETE FROM todos
WHERE id = sqlc.arg(id);