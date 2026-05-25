-- companies.sql — CRUD queries for the workspace-scoped companies table.
--
-- Queries run with search_path = workspace_<uuid>, so table names are unqualified.
-- Cursor-based pagination: pass NULL cursor_created_at for the first page.

-- name: ListCompanies :many
SELECT id, name, domain, industry, size, owner_id, created_at, updated_at
FROM companies
WHERE
  sqlc.arg('cursor_created_at')::timestamptz IS NULL
  OR created_at < sqlc.arg('cursor_created_at')::timestamptz
  OR (created_at = sqlc.arg('cursor_created_at')::timestamptz AND id < sqlc.arg('cursor_id')::uuid)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit')::int;

-- name: GetCompany :one
SELECT id, name, domain, industry, size, owner_id, created_at, updated_at
FROM companies
WHERE id = $1;

-- name: CreateCompany :one
INSERT INTO companies (name, domain, industry, size, owner_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, name, domain, industry, size, owner_id, created_at, updated_at;

-- name: UpdateCompany :one
UPDATE companies
SET name       = COALESCE(NULLIF(sqlc.arg('name')::text, ''), name),
    domain     = sqlc.arg('domain'),
    industry   = sqlc.arg('industry'),
    size       = sqlc.arg('size'),
    owner_id   = sqlc.arg('owner_id'),
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING id, name, domain, industry, size, owner_id, created_at, updated_at;

-- name: DeleteCompany :exec
DELETE FROM companies WHERE id = $1;

-- name: CountCompanies :one
SELECT count(*) FROM companies;
