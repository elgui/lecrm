-- 0027_gmail_mailbox_index.sql — cross-workspace mailbox→(workspace,user) index
-- for Gmail reply detection (ADR-004 rev 2 §4, taskets 5078/5b07).
--
-- WHY THIS EXISTS
-- ---------------
-- A Gmail Pub/Sub push (POST /v1/webhooks/gmail/push) arrives OUTSIDE any
-- workspace context — its body carries only {emailAddress, historyId}. The push
-- handler must resolve that emailAddress to the (workspace_id, user_id) whose
-- connection owns the mailbox BEFORE it can enqueue a per-workspace poll job
-- (gmailreply.ConnectionResolver). The authoritative per-tenant connection row
-- lives in the workspace's own schema (workspace_<hex>.sync_connections,
-- migration 0011) — unreachable without already knowing the workspace. This
-- table is the global, core-schema index that breaks that chicken-and-egg:
-- email_address → (workspace_id, user_id), maintained at connect/seed time
-- alongside the per-workspace sync_connections row.
--
-- It mirrors the existing core control-plane registry pattern (core.workspaces,
-- 0001): a provisioner-owned core table that lecrm_api reads. The push handler
-- runs on the main lecrm_api pool (never as a workspace role), so lecrm_api is
-- the only role that needs access.
--
-- v1 assumption: one Gmail mailbox maps to exactly one (workspace, user) — the
-- rep who granted OAuth (ADR-009 §9, Gmail-first). The PRIMARY KEY on
-- email_address encodes that. A future multi-workspace-per-mailbox model would
-- relax the PK; not needed at v1.
--
-- References:
--   docs/adr/ADR-004-rev2-sequences-architecture.md §4 (Gmail reply detection)
--   apps/api/internal/sequences/gmailreply/resolver.go — ConnectionResolver seam
--   apps/api/internal/sequences/gmailreply/cursor.go — per-workspace sync_connections
--   packages/db/migrations/0001_init.sql — core.workspaces (the pattern mirrored)
--   packages/db/migrations/0017_app_role.sql — lecrm_api core grants

BEGIN;

CREATE TABLE IF NOT EXISTS core.gmail_mailbox_index (
  email_address  text        PRIMARY KEY,
  workspace_id   uuid        NOT NULL REFERENCES core.workspaces(id) ON DELETE CASCADE,
  user_id        uuid        NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);

-- Secondary lookups by workspace (e.g. listing a workspace's connected
-- mailboxes, or cascade housekeeping). The email lookup the push handler does
-- is served by the PRIMARY KEY.
CREATE INDEX IF NOT EXISTS gmail_mailbox_index_workspace_idx
  ON core.gmail_mailbox_index (workspace_id);

-- Provisioner-owned like every other core control-plane table (0001), so the
-- ownership/grant story matches core.workspaces / core.audit_log.
ALTER TABLE core.gmail_mailbox_index OWNER TO lecrm_provisioner;

-- lecrm_api resolves the index on the main pool. 0017 granted DML on all core
-- tables that existed AT THAT TIME; this table is new and created by the
-- migration runner (postgres superuser at initdb), so the 0017 ALTER DEFAULT
-- PRIVILEGES FOR ROLE lecrm_provisioner does not cover it — grant explicitly.
GRANT SELECT, INSERT, UPDATE, DELETE ON core.gmail_mailbox_index TO lecrm_api;

COMMIT;
