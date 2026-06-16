-- deploy/seed/gmail-demo-connection.sql — register a Gmail mailbox connection
-- for reply detection (ADR-004 rev 2 §4). Idempotent. Writes BOTH:
--   1. the per-workspace sync_connections row (workspace_<hex>.sync_connections),
--      which the poll/watch-renew workers read; and
--   2. the cross-workspace core index (core.gmail_mailbox_index, migration 0027),
--      which the push handler resolves email → (workspace, user) against.
--
-- This is the staging-seed shortcut that stands in for the in-product
-- "Connect Gmail" OAuth flow (deferred). Run it ONCE the rep's OAuth refresh
-- token has been captured + rendered (see ops/runbooks/gmail-oauth-pubsub-setup.md).
--
-- Run as a superuser (or a role that can write the workspace schema + core),
-- e.g. against the staging superuser DSN on the box:
--
--   psql "$SUPERUSER_DSN" \
--     -v ws='<workspace_uuid>' \
--     -v usr='<user_uuid>' \
--     -v email='rep@example.com' \
--     -f deploy/seed/gmail-demo-connection.sql
--
-- The workspace + user UUIDs come from core.workspaces / workspace_members; the
-- email is the connected mailbox (must match the OAuth grant + the rendered
-- manifest filename <ws>/<usr>.yaml).

\set ON_ERROR_STOP on

-- 1. Per-workspace connection row. Point search_path at the workspace schema
--    (workspace_<hex>) so the unqualified table resolves there.
SELECT set_config('search_path', 'workspace_' || replace(:'ws', '-', ''), false);

INSERT INTO sync_connections (provider_id, status, settings)
VALUES ('gmail', 'active', jsonb_build_object('user_id', :'usr', 'email_address', :'email'))
ON CONFLICT (provider_id) DO UPDATE
  SET status = 'active', settings = EXCLUDED.settings, updated_at = now();

-- 2. Cross-workspace index (qualified — independent of search_path).
INSERT INTO core.gmail_mailbox_index (email_address, workspace_id, user_id)
VALUES (:'email', :'ws'::uuid, :'usr'::uuid)
ON CONFLICT (email_address) DO UPDATE
  SET workspace_id = EXCLUDED.workspace_id, user_id = EXCLUDED.user_id, updated_at = now();
