-- deals.sql — CRUD queries for the workspace-scoped deals table.
--
-- Queries run with search_path = workspace_<uuid>, so table names are unqualified.
-- Cursor-based pagination: pass NULL cursor_created_at for the first page.

-- name: ListDeals :many
SELECT id, title, amount, currency, stage_id, contact_id, company_id,
       owner_id, expected_close_date, closed_at, created_at, updated_at
FROM deals
WHERE
  sqlc.arg('cursor_created_at')::timestamptz IS NULL
  OR created_at < sqlc.arg('cursor_created_at')::timestamptz
  OR (created_at = sqlc.arg('cursor_created_at')::timestamptz AND id < sqlc.arg('cursor_id')::uuid)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit')::int;

-- name: GetDeal :one
SELECT id, title, amount, currency, stage_id, contact_id, company_id,
       owner_id, expected_close_date, closed_at, created_at, updated_at
FROM deals
WHERE id = $1;

-- name: CreateDeal :one
INSERT INTO deals (title, amount, currency, stage_id, contact_id, company_id, owner_id, expected_close_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, title, amount, currency, stage_id, contact_id, company_id,
          owner_id, expected_close_date, closed_at, created_at, updated_at;

-- name: UpdateDeal :one
UPDATE deals
SET title               = COALESCE(NULLIF(sqlc.arg('title')::text, ''), title),
    amount              = sqlc.arg('amount'),
    currency            = sqlc.arg('currency'),
    stage_id            = sqlc.arg('stage_id'),
    contact_id          = sqlc.arg('contact_id'),
    company_id          = sqlc.arg('company_id'),
    owner_id            = sqlc.arg('owner_id'),
    expected_close_date = sqlc.arg('expected_close_date'),
    updated_at          = now()
WHERE id = sqlc.arg('id')
RETURNING id, title, amount, currency, stage_id, contact_id, company_id,
          owner_id, expected_close_date, closed_at, created_at, updated_at;

-- name: DeleteDeal :exec
DELETE FROM deals WHERE id = $1;

-- name: CountDeals :one
SELECT count(*) FROM deals;

-- name: ListDealsByStage :many
SELECT id, title, amount, currency, stage_id, contact_id, company_id,
       owner_id, expected_close_date, closed_at, created_at, updated_at
FROM deals
WHERE stage_id = $1
ORDER BY created_at DESC;

-- name: UpdateDealStage :one
UPDATE deals
SET stage_id   = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, title, amount, currency, stage_id, contact_id, company_id,
          owner_id, expected_close_date, closed_at, created_at, updated_at;
