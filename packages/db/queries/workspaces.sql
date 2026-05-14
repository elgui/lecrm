-- workspaces.sql — typed queries against core.workspaces.
--
-- ListWorkspacesForTest is the Week-2 Go-ramp checkpoint Test 1 surface
-- (ADR-009 §1.1). It returns the first ten rows; production callers
-- always scope by workspace_id resolved from the request subdomain and
-- do NOT enumerate the table.

-- name: ListWorkspacesForTest :many
SELECT id, slug, created_at
FROM core.workspaces
ORDER BY created_at ASC
LIMIT 10;

-- name: GetWorkspaceBySlug :one
SELECT id, slug, role_name, created_at, updated_at
FROM core.workspaces
WHERE slug = $1;
