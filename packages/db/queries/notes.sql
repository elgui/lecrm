-- notes.sql — workspace-scoped user-authored notes on entities.
--
-- Run with search_path = workspace_<uuid>. Authoring is enforced at
-- the handler layer (only the author may UPDATE; author or admin may
-- DELETE — admin check sits in HTTP middleware, Sprint 7).

-- name: ListNotesByEntity :many
SELECT id, entity_type, entity_id, body, author_id, created_at, updated_at
FROM notes
WHERE entity_type = sqlc.arg('entity_type')::text
  AND entity_id   = sqlc.arg('entity_id')::uuid
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit')::int;

-- name: GetNote :one
SELECT id, entity_type, entity_id, body, author_id, created_at, updated_at
FROM notes
WHERE id = $1;

-- name: CreateNote :one
INSERT INTO notes (entity_type, entity_id, body, author_id)
VALUES ($1, $2, $3, $4)
RETURNING id, entity_type, entity_id, body, author_id, created_at, updated_at;

-- name: UpdateNote :one
UPDATE notes
SET body       = sqlc.arg('body')::text,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid
RETURNING id, entity_type, entity_id, body, author_id, created_at, updated_at;

-- name: DeleteNote :exec
DELETE FROM notes WHERE id = $1;
