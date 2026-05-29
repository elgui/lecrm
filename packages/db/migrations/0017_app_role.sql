-- 0017_app_role.sql — create the lecrm-api application Postgres role.
--
-- WHY THIS EXISTS
-- ---------------
-- The runtime (`cmd/lecrm-api`) authenticates to Postgres as the role in
-- LECRM_DATABASE_URL — `lecrm_api` (see deploy/.env.* and
-- ops/connection-pooling.md). 0002_identity.sql §16 noted the application
-- role would be "created in a later migration"; that migration was never
-- written, so a fresh database had no `lecrm_api` role and the API could
-- not start. This migration closes that gap.
--
-- ACCESS MODEL (matches the implemented capability layer)
-- ------------------------------------------------------
-- The human REST path runs every tenant query through the single
-- control-plane pool (role = lecrm_api) with `SET LOCAL search_path` to
-- the workspace schema and NO `SET ROLE` (apps/api/capability/capability.go
-- ReadTx/WriteTx; principalFrom leaves ReadRole empty). So lecrm_api needs
-- *direct* DML on every workspace schema's tables, plus DML on the core
-- control-plane tables it manages (users, members, tokens, audit, …).
-- Tenant isolation on this path is enforced by the application pinning
-- search_path correctly + RBAC — not by per-role DB grants. (The MCP read
-- path additionally uses ReadTxAsRole → SET LOCAL ROLE workspace_<id>_ro;
-- that constrained read role already exists from 0015 and is unchanged.)
--
-- All 18 per-workspace tables are owned by lecrm_provisioner (the
-- SECURITY DEFINER that creates them), so lecrm_provisioner can GRANT them
-- to lecrm_api. New workspaces are covered by hooking the grant into
-- core.lecrm_provision_workspace_with_registry (the single provisioning
-- entry point used by apps/migrate and apps/admin); existing workspaces are
-- back-filled at the bottom of this file.
--
-- PASSWORD: created LOGIN with no password here. The password is set
-- out-of-band at deploy time from LECRM_PGBOUNCER_AUTH_PASS — mirroring the
-- lecrm_cube_reader pattern in 0013 — so no secret is baked into version
-- control. See deploy/postgres/initdb/zz-bootstrap.sh (portable boot) or the
-- runbook in deploy/README.md.
--
-- FOLLOW-UP (not blocking v0 staging): tighten core grants — lecrm_api only
-- needs SELECT on core.workspaces (the registry is written by the
-- provisioner, not the app). Broad core DML is accepted for the disposable
-- staging stopgap and revisited at the Hetzner migration.
--
-- References:
--   ops/connection-pooling.md            — lecrm_api as the control-plane role
--   apps/api/capability/capability.go    — ReadTx/WriteTx (search_path, no SET ROLE)
--   packages/db/migrations/0002_identity.sql §16 — the deferred "later migration"
--   packages/db/migrations/0013_workspace_ro_role.sql — out-of-band password pattern
--   packages/db/migrations/0015_…         — per-workspace _ro grant block mirrored below

BEGIN;

-- 1. The application role. LOGIN, no DDL, no superuser. Password set
--    out-of-band (see header). Idempotent: re-running this migration (or
--    the migrate-runner re-applying it) must not fail.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lecrm_api') THEN
    -- CONNECTION LIMIT generous: one control-plane pool (10) + the bounded
    -- Go TenantPool, all multiplexed through this one login role.
    EXECUTE 'CREATE ROLE lecrm_api LOGIN CONNECTION LIMIT 100';
  END IF;
END
$$;

-- Defence-in-depth timeouts (mirror the workspace-role tuning in 0015).
ALTER ROLE lecrm_api SET statement_timeout = '30s';
ALTER ROLE lecrm_api SET lock_timeout = '5s';
ALTER ROLE lecrm_api SET idle_in_transaction_session_timeout = '30s';

-- 2. Core control-plane grants. lecrm_api reads/writes the global tables
--    (users upsert, workspace_members, service_tokens, idempotency_keys,
--    session_revocations, audit_log, …). USAGE on core lets it reach them.
GRANT USAGE ON SCHEMA core TO lecrm_api;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA core TO lecrm_api;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA core TO lecrm_api;
-- Future core tables (owned by lecrm_provisioner via later migrations) are
-- auto-granted. Initdb applies migrations as the postgres superuser, but the
-- workspace-data tables in core are provisioner-owned; cover both grantors.
ALTER DEFAULT PRIVILEGES FOR ROLE lecrm_provisioner IN SCHEMA core
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO lecrm_api;
ALTER DEFAULT PRIVILEGES FOR ROLE lecrm_provisioner IN SCHEMA core
  GRANT USAGE, SELECT ON SEQUENCES TO lecrm_api;

-- 3. Per-workspace grant helper. Grants lecrm_api full DML on one workspace
--    schema. SECURITY DEFINER + owned by lecrm_provisioner so it runs with
--    the table owner's privileges (can GRANT the provisioner-owned tables
--    and set the provisioner's default privileges for that schema). Mirrors
--    the per-workspace _ro grant block in 0015, but with DML for the app role.
CREATE OR REPLACE FUNCTION core.lecrm_grant_app_role(p_schema TEXT)
  RETURNS void
  LANGUAGE plpgsql
  SECURITY DEFINER
  SET search_path = core, pg_catalog
AS $func$
BEGIN
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO lecrm_api', p_schema);
  EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %I TO lecrm_api', p_schema);
  EXECUTE format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA %I TO lecrm_api', p_schema);
  -- Tables added to this schema later (e.g. pipeline_stages from the
  -- gbconsult-default template, or a future migration's CREATE TABLE) are
  -- provisioner-owned; auto-grant them to lecrm_api.
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES FOR ROLE lecrm_provisioner IN SCHEMA %I GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO lecrm_api',
    p_schema);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES FOR ROLE lecrm_provisioner IN SCHEMA %I GRANT USAGE, SELECT ON SEQUENCES TO lecrm_api',
    p_schema);
END;
$func$;

ALTER FUNCTION core.lecrm_grant_app_role(TEXT) OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_grant_app_role(TEXT) FROM PUBLIC;

-- 4. Wire the grant into the provisioning entry point so every NEW workspace
--    grants lecrm_api automatically. This is a verbatim copy of the 0004
--    body (the canonical with_registry wrapper) with a single added line:
--    `PERFORM core.lecrm_grant_app_role(v_role_name)` after the schema and
--    its tables have been created. Keeping the rest byte-identical preserves
--    AC-T1/AC-F3 (atomicity + bit-identical re-runs).
CREATE OR REPLACE FUNCTION core.lecrm_provision_workspace_with_registry(
  p_workspace_id  UUID,
  p_slug          TEXT,
  p_admin_email   TEXT,
  p_creator_email TEXT,
  p_template      TEXT
)
  RETURNS TEXT
  LANGUAGE plpgsql
  SECURITY DEFINER
  SET search_path = pg_catalog, public
AS $func$
DECLARE
  v_role_name TEXT := 'workspace_' || lower(replace(p_workspace_id::text, '-', ''));
  v_features  JSONB := CASE
                         WHEN p_template = '' OR p_template IS NULL THEN '[]'::jsonb
                         ELSE jsonb_build_array(p_template || '-v1')
                       END;
  v_inserted  INT := 0;
BEGIN
  -- Step 1. Call the existing provisioning function. Creates role +
  -- workspace_<uuid> schema + river_<uuid> schema + all entity tables.
  -- Silent-idempotent on re-invocation.
  PERFORM core.lecrm_provision_workspace(p_workspace_id);

  -- Step 1b (0017). Grant the application role DML on the freshly created
  -- workspace schema so the API (role = lecrm_api) can read/write tenant
  -- data via search_path. Idempotent — GRANT is a no-op if already held.
  PERFORM core.lecrm_grant_app_role(v_role_name);

  -- Step 2. UPSERT core.workspaces. ON CONFLICT (id) DO NOTHING — preserves
  -- the no-op semantics on --upsert. If the slug is already taken by a
  -- DIFFERENT UUID, the UNIQUE (slug) constraint raises unique_violation
  -- here and the whole transaction (including the role/schema created
  -- above) rolls back. AC-T1 atomicity test exploits this.
  INSERT INTO core.workspaces
    (id, slug, role_name, admin_email, creator_email, provisioning_features_applied)
  VALUES
    (p_workspace_id, p_slug, v_role_name, p_admin_email, p_creator_email, v_features)
  ON CONFLICT (id) DO NOTHING;
  GET DIAGNOSTICS v_inserted = ROW_COUNT;

  -- The role_name in the registry is the source of truth; re-fetch it in
  -- case ON CONFLICT (id) DO NOTHING skipped our INSERT (a previous
  -- provision recorded it already).
  SELECT role_name INTO v_role_name FROM core.workspaces WHERE id = p_workspace_id;

  -- Step 3 + 4: audit row and pipeline seed only on fresh provision.
  -- Repeat calls (--upsert path against the same UUID) skip these so the
  -- DB state stays bit-identical, satisfying AC-F3.
  IF v_inserted = 1 THEN
    -- Step 3. Audit row in the same transaction (ADR-007 fail-closed contract).
    INSERT INTO core.audit_log (event, workspace_id, actor_type, payload)
    VALUES (
      'workspace.provisioned',
      p_workspace_id,
      'system',
      jsonb_build_object(
        'slug', p_slug,
        'admin_email', p_admin_email,
        'creator_email', p_creator_email,
        'template', p_template
      )
    );

    -- Step 4. Pipeline-stages table + seed for the named template.
    IF p_template = 'gbconsult-default' THEN
      EXECUTE format($f$
        CREATE TABLE IF NOT EXISTS %I.pipeline_stages (
          id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          name        TEXT NOT NULL UNIQUE,
          order_index INT  NOT NULL,
          created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
        )$f$, v_role_name);

      EXECUTE format($f$
        INSERT INTO %I.pipeline_stages (name, order_index) VALUES
          ('Discovery', 1),
          ('Qualified', 2),
          ('Proposal Sent', 3),
          ('Negotiation', 4),
          ('Closed-Won/Lost', 5)$f$, v_role_name);
    ELSIF p_template = '' OR p_template IS NULL THEN
      -- Bootstrap path (apps/migrate provision-workspace) — no template,
      -- no pipeline seed. provisioning_features_applied stays '[]'::jsonb.
      NULL;
    ELSE
      RAISE EXCEPTION 'unknown template: %', p_template
        USING HINT = 'Known templates for v0: gbconsult-default. Pass empty string for the bootstrap path.';
    END IF;
  END IF;

  RETURN v_role_name;
END;
$func$;

ALTER FUNCTION core.lecrm_provision_workspace_with_registry(UUID, TEXT, TEXT, TEXT, TEXT)
  OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace_with_registry(UUID, TEXT, TEXT, TEXT, TEXT)
  FROM PUBLIC;

-- 5. Back-fill: grant lecrm_api on every workspace schema that already
--    exists (pre-0017 provisions). No-op on a fresh database.
DO $$
DECLARE
  ws RECORD;
BEGIN
  FOR ws IN SELECT role_name FROM core.workspaces LOOP
    PERFORM core.lecrm_grant_app_role(ws.role_name);
  END LOOP;
END$$;

COMMIT;
