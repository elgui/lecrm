-- 0018_integrator_role_and_grants.sql — data-model foundation for the
-- integrator workspace-switching feature (Sprint: lecrm-integrator-switching,
-- step 1/8).
--
-- WHY THIS EXISTS
-- ---------------
-- GB Consult's integrator (Léo) needs to switch into a client workspace and
-- administrate it as an owner-equivalent — but as a DISTINCT, NON-BILLABLE
-- principal that is hidden from the client member list and whose actions are
-- tagged in core.audit_log. This migration lays the two pieces of state that
-- foundation needs:
--
--   1. A new 'integrator' membership role, accepted by
--      core.workspace_members.role (login-time elevation, tasket 3, will
--      materialize an integrator row here).
--
--   2. core.integrator_grants — an EMAIL-KEYED pending-grant table. At
--      provision time the integrator has no core.users row yet (that row is
--      created on his first OIDC login, keyed by (issuer, sub)). We therefore
--      record "this email may administrate workspace X" decoupled from "has
--      this human logged in yet". First login (tasket 3) reads this table by
--      lower(email) and materializes a real core.workspace_members row.
--
-- No login or UI behaviour changes here — those are taskets 3 and 4.
--
-- ACCESS MODEL
-- ------------
-- integrator_grants is provisioner-owned (writes happen on the provision /
-- lecrm-admin path, tasket 2). The application role (lecrm_api) only needs
-- SELECT — login-time elevation reads the pending grant. Migrations apply as
-- the postgres superuser in CI (testcontainers init scripts) and as
-- lecrm_provisioner in prod, so the table is not auto-covered by the
-- ALTER DEFAULT PRIVILEGES FOR ROLE lecrm_provisioner block in 0017; the
-- SELECT grant below is therefore explicit.
--
-- References:
--   packages/db/migrations/0002_identity.sql — workspace_members / users schema
--   packages/db/migrations/0004_workspaces_admin_email_registry.sql — admin_email registry
--   packages/db/migrations/0017_app_role.sql — lecrm_api application role + core grants
--   apps/api/internal/rbac/role.go — RoleIntegrator (extended in the same tasket)
--   docs/adr/ADR-009-stack-and-license.md §5.2 — per-subdomain cookie boundary (untouched)

BEGIN;

-- 1. Extend the workspace_members role CHECK to admit 'integrator'.
--    0002 created the check inline and unnamed; PostgreSQL auto-names a
--    single-column table CHECK as <table>_<column>_check, i.e.
--    workspace_members_role_check. DROP IF EXISTS + ADD is idempotent: on
--    re-run the named constraint already admits 'integrator', the DROP
--    removes it, and the ADD recreates the identical definition.
ALTER TABLE core.workspace_members
  DROP CONSTRAINT IF EXISTS workspace_members_role_check;
ALTER TABLE core.workspace_members
  ADD CONSTRAINT workspace_members_role_check
  CHECK (role IN ('owner', 'admin', 'member', 'integrator'));

-- 2. Email-keyed pending-grant table. PRIMARY KEY cannot be a functional
--    expression in PostgreSQL, so case-insensitive uniqueness is enforced by
--    a UNIQUE INDEX on (workspace_id, lower(email)) instead of a PK.
CREATE TABLE IF NOT EXISTS core.integrator_grants (
  workspace_id UUID        NOT NULL REFERENCES core.workspaces(id) ON DELETE CASCADE,
  email        TEXT        NOT NULL,
  granted_by   TEXT        NOT NULL DEFAULT '',  -- who created the grant (admin actor / ''=system)
  granted_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One pending grant per (workspace, email), case-insensitive. This is the
-- conflict target the provision / lecrm-admin path (tasket 2) UPSERTs against.
CREATE UNIQUE INDEX IF NOT EXISTS integrator_grants_workspace_email_uk
  ON core.integrator_grants (workspace_id, lower(email));

-- Lookup by email across workspaces: login-time elevation (tasket 3) resolves
-- every pending grant for the freshly-authenticated human's email.
CREATE INDEX IF NOT EXISTS integrator_grants_email_idx
  ON core.integrator_grants (lower(email));

ALTER TABLE core.integrator_grants OWNER TO lecrm_provisioner;

-- 3. The API role only reads pending grants (login-time elevation). Writes
--    stay on the provisioner / admin path.
GRANT SELECT ON core.integrator_grants TO lecrm_api;

COMMIT;
