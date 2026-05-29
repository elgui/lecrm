-- 0016_service_tokens.sql — Sprint 7 (tasket 20260525-1006).
--
-- Workspace-scoped Bearer service tokens (ADR-009 §4.1, ADR-011 §6).
--
-- Tokens carry the plaintext form `lecrm_<workspace_slug>_<base64url(32 random bytes)>`
-- and are persisted ONLY as an argon2id hash. The plaintext is shown
-- once at creation time and never again.
--
-- The token row is owned by the workspace (ON DELETE CASCADE) and
-- carries an actor_type drawn from the same catalogue as
-- core.audit_log.actor_type:
--
--   human_api        — a developer / API user invoking on behalf of self
--   mcp_agent        — an MCP agent acting on a workspace
--   internal_service — internal lecrm component (cron, worker)
--   connector        — chatboting / external integration (per ADR-011 §6)
--
-- Scopes default to `["*"]` (full access). Scope vocabulary is
-- application-defined; the database only stores the JSONB blob.
--
-- Lookup happens in the application layer: argon2 verify is expensive
-- and cannot be done in SQL, so the middleware loads all candidate
-- rows by prefix (workspace slug embedded in plaintext) and verifies
-- each hash. With per-workspace token counts in the dozens this is
-- fine; if a workspace ever pushes past a few hundred tokens we add
-- a token_id-prefix index.
--
-- Owned by lecrm_provisioner (same as core.audit_log + idempotency_keys).

BEGIN;

CREATE TABLE IF NOT EXISTS core.service_tokens (
  id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id   UUID        NOT NULL REFERENCES core.workspaces(id) ON DELETE CASCADE,
  name           TEXT        NOT NULL,
  token_hash     TEXT        NOT NULL,
  actor_type     TEXT        NOT NULL CHECK (actor_type IN ('human_api','mcp_agent','internal_service','connector')),
  scopes         JSONB       NOT NULL DEFAULT '["*"]'::jsonb,
  expires_at     TIMESTAMPTZ,
  last_used_at   TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS service_tokens_workspace_id_idx
  ON core.service_tokens (workspace_id);

-- Active-token lookups filter `expires_at IS NULL OR expires_at > now()`.
-- now() is STABLE (not IMMUTABLE), so it cannot appear in a partial-index
-- predicate (Postgres rejects it: "functions in index predicate must be
-- marked IMMUTABLE"). Index (workspace_id, expires_at) instead and let the
-- planner apply the time filter at query time with now() as a runtime value.
CREATE INDEX IF NOT EXISTS service_tokens_workspace_active_idx
  ON core.service_tokens (workspace_id, expires_at);

ALTER TABLE core.service_tokens OWNER TO lecrm_provisioner;

COMMIT;
