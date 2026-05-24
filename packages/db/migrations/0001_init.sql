-- 0001_init.sql — leCRM bootstrap schema and workspace provisioning.
--
-- Establishes the global control plane in the `core` schema, the
-- `lecrm_provisioner` role (Tier-0 secret, annual rotation per ADR-007
-- follow-up), and the SECURITY DEFINER function `lecrm_provision_workspace`
-- that creates a per-workspace Postgres role + schema + per-tenant
-- `river_*` schema in a single transaction.
--
-- The function body is the verbatim ADR-009 §2.1 specification with one
-- adjustment for idempotency: the application-level expectation per
-- ADR-009 §2.1 is "catches `duplicate_object` for the role and `42P06`
-- for the schema and treats them as success on a known workspace_id".
-- We implement that idempotency inside the function itself by wrapping
-- each DDL step in BEGIN/EXCEPTION blocks so the function is safe to
-- re-invoke on the same UUID. This preserves the one-transaction
-- guarantee while pushing the duplicate-object handling out of every
-- caller.
--
-- References:
--   ADR-009 §2.1 — function body and Tier-0 role status
--   ADR-001     — schema-per-tenant primitive
--   ADR-007 §3  — audit_log catalogue (security.workspace_id_mismatch
--                 lives here; the schema is created here, emissions wired
--                 in Sprint 7)

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS core;

-- The lecrm_provisioner role owns the SECURITY DEFINER function and is
-- the only role with DDL privileges in production. The application
-- (`lecrm-api`) runs as a separate application role with zero DDL.
-- See ADR-009 §2.1, §8.2 (three binaries, not one).
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lecrm_provisioner') THEN
    EXECUTE format(
      'CREATE ROLE lecrm_provisioner LOGIN CREATEROLE PASSWORD %L',
      encode(gen_random_bytes(32), 'base64')
    );
  END IF;
END
$$;

-- Allow lecrm_provisioner to CREATE SCHEMA in the current database.
-- The function body's `CREATE SCHEMA ... AUTHORIZATION` runs with the
-- function owner's (lecrm_provisioner's) privileges per SECURITY DEFINER.
DO $$
BEGIN
  EXECUTE format('GRANT CREATE ON DATABASE %I TO lecrm_provisioner', current_database());
END
$$;

-- Allow lecrm_provisioner to CREATE objects inside the core schema
-- (functions, tables, etc.). Required so subsequent migrations applied by
-- lecrm-migrate as the lecrm_provisioner role (per deploy/compose/migrate.yml)
-- can add SECURITY DEFINER wrappers and registry tables in core. Idempotent.
GRANT CREATE ON SCHEMA core TO lecrm_provisioner;

-- core.workspaces — global workspace registry. Tenant data lives in
-- per-workspace schemas; this table is the only place workspace_id is
-- enumerated.
CREATE TABLE IF NOT EXISTS core.workspaces (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug        TEXT UNIQUE NOT NULL,
  role_name   TEXT UNIQUE NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- core.audit_log — ADR-007 §3 catalogue. Fail-closed: a mutation that
-- cannot be audit-logged MUST be rejected (ADR-009 §7.2). Workspace
-- column is nullable for security events that fire before workspace
-- resolution (e.g. security.workspace_id_mismatch).
CREATE TABLE IF NOT EXISTS core.audit_log (
  id                BIGSERIAL PRIMARY KEY,
  occurred_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  event             TEXT        NOT NULL,
  workspace_id      UUID        REFERENCES core.workspaces(id),
  actor_type        TEXT        CHECK (actor_type IN ('human_api', 'mcp_agent', 'internal_service', 'system') OR actor_type IS NULL),
  actor_user_id     UUID,
  actor_ip          INET,
  request_id        UUID,
  payload           JSONB       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS audit_log_workspace_occurred_idx
  ON core.audit_log (workspace_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS audit_log_event_occurred_idx
  ON core.audit_log (event, occurred_at DESC);

-- lecrm_provision_workspace(p_workspace_id) — single SQL call provisions
-- a workspace per ADR-009 §2.1.
--
-- The function body matches ADR-009 §2.1 lines 63-106 with idempotency
-- baked in via per-step EXCEPTION blocks so callers do not need to
-- catch `duplicate_object` / `42P06`. Returns the role name in both the
-- fresh and re-invocation cases.
CREATE OR REPLACE FUNCTION core.lecrm_provision_workspace(p_workspace_id UUID)
  RETURNS TEXT
  LANGUAGE plpgsql
  SECURITY DEFINER
  SET search_path = pg_catalog, public
AS $func$
DECLARE
  v_role_name   TEXT := 'workspace_' || lower(replace(p_workspace_id::text, '-', ''));
  v_password    TEXT := encode(gen_random_bytes(32), 'base64');
  v_river_name  TEXT := 'river_' || lower(replace(p_workspace_id::text, '-', ''));
BEGIN
  -- 1. Role
  BEGIN
    EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L CONNECTION LIMIT 10', v_role_name, v_password);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;
  -- On PG16+ the CREATEROLE owner is auto-granted the new role with
  -- INHERIT/SET options. On PG15 the grant is not implicit, so we
  -- request it explicitly; the statement is idempotent and harmless on
  -- PG16+. This is what lets lecrm_provisioner subsequently execute
  -- `CREATE SCHEMA ... AUTHORIZATION %I` for the workspace role below.
  BEGIN
    EXECUTE format('GRANT %I TO lecrm_provisioner', v_role_name);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;
  EXECUTE format('ALTER ROLE %I SET search_path = %I, public', v_role_name, v_role_name);
  EXECUTE format('ALTER ROLE %I SET statement_timeout = ''30s''', v_role_name);
  EXECUTE format('ALTER ROLE %I SET lock_timeout = ''5s''', v_role_name);
  EXECUTE format('ALTER ROLE %I SET work_mem = ''16MB''', v_role_name);

  -- 2. Schema
  BEGIN
    EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_role_name, v_role_name);
  EXCEPTION
    WHEN duplicate_schema THEN NULL;
  END;

  -- 3. Grants on the workspace's own schema
  EXECUTE format('GRANT USAGE, CREATE ON SCHEMA %I TO %I', v_role_name, v_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT ALL ON TABLES TO %I',
    v_role_name, v_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT ALL ON SEQUENCES TO %I',
    v_role_name, v_role_name);

  -- 4. Lateral-expansion mitigation: revoke all default access to public
  EXECUTE format('REVOKE CREATE ON SCHEMA public FROM %I', v_role_name);
  EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA public FROM %I', v_role_name);

  -- 5. River-job schema for this workspace
  BEGIN
    EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_river_name, v_role_name);
  EXCEPTION
    WHEN duplicate_schema THEN NULL;
  END;

  -- Password is intentionally NOT returned by this function. A sibling
  -- function (to be added when secrets-manifest plumbing lands in
  -- Sprint 3 per tasket 20260510-162158-1023) will rotate-and-capture
  -- the role password and write it to the SOPS-encrypted secret
  -- manifest.
  RETURN v_role_name;
END;
$func$;

-- The function is owned by lecrm_provisioner; only the provisioner can
-- create roles/schemas, so SECURITY DEFINER carries those privileges to
-- any caller granted EXECUTE.
ALTER FUNCTION core.lecrm_provision_workspace(UUID) OWNER TO lecrm_provisioner;

-- The application role does not have EXECUTE — only `cmd/lecrm-migrate`
-- (running as lecrm_provisioner directly) calls this function.
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace(UUID) FROM PUBLIC;

-- lecrm_provisioner needs USAGE on `core` to invoke functions it owns
-- (owning the function does not imply USAGE on the schema), and it
-- owns and operates the control-plane tables in `core`.
GRANT USAGE ON SCHEMA core TO lecrm_provisioner;
ALTER TABLE core.workspaces  OWNER TO lecrm_provisioner;
ALTER TABLE core.audit_log   OWNER TO lecrm_provisioner;

COMMIT;
