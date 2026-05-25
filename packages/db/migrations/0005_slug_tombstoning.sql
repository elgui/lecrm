-- 0005_slug_tombstoning.sql — slug tombstoning and reserved blocklist.
--
-- When a tenant is deleted or churns, their subdomain (e.g. acme.lecrm.fr)
-- must never be re-registered by another party. The wildcard TLS cert
-- (*.lecrm.fr) means a recycled slug gets a valid padlock — making
-- credential phishing against former tenant employees trivially convincing.
--
-- This migration:
--   (a) Adds tombstoned_at to core.workspaces (soft-delete marker)
--   (b) Creates core.reserved_slugs (infrastructure/dangerous slug blocklist)
--   (c) Adds a partial unique index so only active slugs enforce uniqueness
--
-- References:
--   council-architecture-review-2026-05-24 — slug recycling = highest-priority fix
--   RFC 9700 (OAuth 2.0 Security BCP) — subdomain takeover warnings

BEGIN;

-- 1. Add tombstoned_at column. NULL = active, non-NULL = tombstoned.
ALTER TABLE core.workspaces
  ADD COLUMN IF NOT EXISTS tombstoned_at TIMESTAMPTZ DEFAULT NULL;

-- 2. Partial unique index: only active (non-tombstoned) workspaces must
--    have unique slugs. Tombstoned workspaces keep their slug as a record
--    but don't block the namespace (the reserved_slugs table does that).
--    The original UNIQUE constraint on slug remains for backwards compat
--    with existing rows; this index is the forward-looking enforcement.
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_slug_active
  ON core.workspaces (slug) WHERE tombstoned_at IS NULL;

-- 3. Reserved slugs table: infrastructure names that must never be provisioned.
CREATE TABLE IF NOT EXISTS core.reserved_slugs (
  slug       TEXT PRIMARY KEY,
  reason     TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE core.reserved_slugs OWNER TO lecrm_provisioner;

-- 4. Seed the blocklist with dangerous infrastructure names.
INSERT INTO core.reserved_slugs (slug, reason) VALUES
  ('admin',     'infrastructure — admin panel'),
  ('api',       'infrastructure — API endpoint'),
  ('auth',      'infrastructure — authentication'),
  ('www',       'infrastructure — web root'),
  ('mail',      'infrastructure — email'),
  ('smtp',      'infrastructure — email relay'),
  ('ftp',       'infrastructure — file transfer'),
  ('ns1',       'infrastructure — nameserver'),
  ('ns2',       'infrastructure — nameserver'),
  ('localhost',  'infrastructure — loopback'),
  ('test',      'infrastructure — testing'),
  ('staging',   'infrastructure — staging environment'),
  ('prod',      'infrastructure — production alias'),
  ('app',       'infrastructure — application root'),
  ('status',    'infrastructure — status page'),
  ('grafana',   'infrastructure — monitoring'),
  ('metrics',   'infrastructure — monitoring'),
  ('internal',  'infrastructure — internal services'),
  ('cdn',       'infrastructure — content delivery'),
  ('assets',    'infrastructure — static assets'),
  ('static',    'infrastructure — static files'),
  ('websocket', 'infrastructure — websocket endpoint'),
  ('ws',        'infrastructure — websocket shorthand'),
  ('wss',       'infrastructure — secure websocket'),
  ('dashboard', 'infrastructure — dashboard'),
  ('console',   'infrastructure — console'),
  ('panel',     'infrastructure — control panel'),
  ('login',     'infrastructure — login page'),
  ('signup',    'infrastructure — signup page'),
  ('register',  'infrastructure — registration'),
  ('docs',      'infrastructure — documentation')
ON CONFLICT (slug) DO NOTHING;

COMMIT;
