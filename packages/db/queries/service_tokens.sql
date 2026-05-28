-- service_tokens.sql — workspace-scoped Bearer service tokens
-- (ADR-009 §4.1, Sprint 7 tasket 20260525-1006).
--
-- The table lives in `core` (not the workspace schema) so a single
-- application connection can authenticate Bearer requests before the
-- per-workspace search_path is set. Every row carries workspace_id and
-- the application is responsible for scoping reads.

-- name: ListServiceTokensByWorkspace :many
SELECT id, workspace_id, name, actor_type, scopes, expires_at, last_used_at, created_at
FROM core.service_tokens
WHERE workspace_id = $1
ORDER BY created_at DESC;

-- name: ListServiceTokenCandidatesForVerify :many
-- Returns all active (non-expired) tokens for a workspace, including
-- the argon2id hash so the application layer can verify a candidate
-- plaintext against each row. Per-workspace token counts are small
-- (dozens) so the linear verify is acceptable.
SELECT id, workspace_id, name, token_hash, actor_type, scopes, expires_at
FROM core.service_tokens
WHERE workspace_id = $1
  AND (expires_at IS NULL OR expires_at > now());

-- name: CreateServiceToken :one
INSERT INTO core.service_tokens (workspace_id, name, token_hash, actor_type, scopes, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, workspace_id, name, actor_type, scopes, expires_at, last_used_at, created_at;

-- name: DeleteServiceToken :execrows
DELETE FROM core.service_tokens
WHERE workspace_id = $1 AND id = $2;

-- name: TouchServiceTokenLastUsed :exec
UPDATE core.service_tokens
SET last_used_at = now()
WHERE id = $1;
