-- tasks.sql — workspace-scoped task list with due dates.
--
-- Run with search_path = workspace_<uuid>. Reminder scheduling happens
-- via River when due_date is set (apps/api/internal/jobs/task_reminder.go).

-- name: ListTasksByAssignee :many
SELECT id, title, description, entity_type, entity_id, assignee_id,
       due_date, completed_at, created_at, updated_at
FROM tasks
WHERE
  (sqlc.narg('assignee_id')::uuid IS NULL
    OR assignee_id = sqlc.narg('assignee_id')::uuid)
  AND (sqlc.arg('entity_type')::text = ''
    OR (entity_type = sqlc.arg('entity_type')::text
        AND entity_id = sqlc.narg('entity_id')::uuid))
ORDER BY
  completed_at NULLS FIRST,
  due_date NULLS LAST,
  created_at DESC
LIMIT sqlc.arg('page_limit')::int;

-- name: GetTask :one
SELECT id, title, description, entity_type, entity_id, assignee_id,
       due_date, completed_at, created_at, updated_at
FROM tasks
WHERE id = $1;

-- name: CreateTask :one
INSERT INTO tasks (title, description, entity_type, entity_id, assignee_id, due_date)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, title, description, entity_type, entity_id, assignee_id,
          due_date, completed_at, created_at, updated_at;

-- name: UpdateTask :one
UPDATE tasks
SET title       = sqlc.arg('title')::text,
    description = sqlc.arg('description'),
    entity_type = sqlc.arg('entity_type'),
    entity_id   = sqlc.arg('entity_id'),
    assignee_id = sqlc.arg('assignee_id'),
    due_date    = sqlc.arg('due_date'),
    updated_at  = now()
WHERE id = sqlc.arg('id')::uuid
RETURNING id, title, description, entity_type, entity_id, assignee_id,
          due_date, completed_at, created_at, updated_at;

-- name: ToggleTaskCompletion :one
UPDATE tasks
SET completed_at = CASE WHEN completed_at IS NULL THEN now() ELSE NULL END,
    updated_at   = now()
WHERE id = $1
RETURNING id, title, description, entity_type, entity_id, assignee_id,
          due_date, completed_at, created_at, updated_at;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;
