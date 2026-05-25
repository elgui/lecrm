-- contacts.sql — CRUD queries for the workspace-scoped contacts table.
--
-- Queries run with search_path = workspace_<uuid>, so table names are unqualified.
-- Cursor-based pagination: pass NULL cursor_created_at for the first page.

-- name: ListContacts :many
SELECT id, first_name, last_name, email, phone, company_id, owner_id, created_at, updated_at
FROM contacts
WHERE
  sqlc.arg('cursor_created_at')::timestamptz IS NULL
  OR created_at < sqlc.arg('cursor_created_at')::timestamptz
  OR (created_at = sqlc.arg('cursor_created_at')::timestamptz AND id < sqlc.arg('cursor_id')::uuid)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit')::int;

-- name: GetContact :one
SELECT id, first_name, last_name, email, phone, company_id, owner_id, created_at, updated_at
FROM contacts
WHERE id = $1;

-- name: CreateContact :one
INSERT INTO contacts (first_name, last_name, email, phone, company_id, owner_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, first_name, last_name, email, phone, company_id, owner_id, created_at, updated_at;

-- name: UpdateContact :one
UPDATE contacts
SET first_name = COALESCE(NULLIF(sqlc.arg('first_name')::text, ''), first_name),
    last_name  = COALESCE(NULLIF(sqlc.arg('last_name')::text, ''), last_name),
    email      = sqlc.arg('email'),
    phone      = sqlc.arg('phone'),
    company_id = sqlc.arg('company_id'),
    owner_id   = sqlc.arg('owner_id'),
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING id, first_name, last_name, email, phone, company_id, owner_id, created_at, updated_at;

-- name: DeleteContact :exec
DELETE FROM contacts WHERE id = $1;

-- name: CountContacts :one
SELECT count(*) FROM contacts;
