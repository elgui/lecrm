-- 0014_idempotency_keys.sql — Sprint 7 (tasket 20260525-1003).
--
-- ADR-009 §4 Idempotency-Key contract:
--   Any POST that carries `Idempotency-Key: <opaque>` MUST replay the
--   first response for that key within a 24h window. Replays return
--   the cached status + body verbatim and add `Idempotency-Replayed: true`.
--
-- Storage shape — keyed on (workspace_id, key) so the same opaque key
-- from two different tenants does NOT collide and cannot be used as an
-- enumeration oracle across tenants.
--
-- Owned by lecrm_provisioner, same as core.audit_log. The application
-- pool connects with provisioner-level privileges and can INSERT/SELECT
-- directly; per-workspace roles do NOT have direct access (writes go
-- through the API's writeTx wrapper, which holds the provisioner conn).
--
-- Cleanup: a daily River job (Sprint 8+) sweeps rows where
-- expires_at < now() — for v0 the table is small and unpruned-stale
-- rows are harmless because the read query filters by `expires_at > now()`.

BEGIN;

CREATE TABLE IF NOT EXISTS core.idempotency_keys (
  key             TEXT        NOT NULL,
  workspace_id    UUID        NOT NULL REFERENCES core.workspaces(id) ON DELETE CASCADE,
  method          TEXT        NOT NULL,
  path            TEXT        NOT NULL,
  response_status INT         NOT NULL,
  response_body   BYTEA       NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at      TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (workspace_id, key)
);

CREATE INDEX IF NOT EXISTS idempotency_keys_expires_at_idx
  ON core.idempotency_keys (expires_at);

ALTER TABLE core.idempotency_keys OWNER TO lecrm_provisioner;

COMMIT;
