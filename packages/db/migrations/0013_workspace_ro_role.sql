-- 0013_workspace_ro_role.sql — per-workspace read-only role for Cube.dev.
--
-- Sprint 12 (Cube.dev backend, ADR-009 §9): the embedded reporting
-- surface needs a Postgres role per workspace that can SELECT but not
-- mutate. Cube connects as a single shared user (lecrm_cube_reader)
-- and issues `SET LOCAL ROLE workspace_<id>_ro` per query, driven by
-- the workspace_id claim in the embed JWT.
--
-- What this migration adds:
--   1. core.lecrm_cube_reader role (idempotent) — the single LOGIN role
--      Cube uses. It has membership in every per-workspace RO role and
--      assumes one via SET ROLE per query.
--   2. core.lecrm_provision_workspace extended to also create
--      workspace_<id>_ro and GRANT it to lecrm_cube_reader.
--
-- The RO role:
--   - LOGIN disabled (NOLOGIN). Membership-only — only reachable via
--     SET ROLE from lecrm_cube_reader. Removes a class of credential-
--     leak attack: there is no password to compromise.
--   - GRANT USAGE on the workspace schema.
--   - GRANT SELECT on all current tables + ALTER DEFAULT PRIVILEGES so
--     future tables are SELECT-able without re-running the migration.
--   - statement_timeout=15s, lock_timeout=2s — dashboards must be quick
--     and non-blocking; a long-running report cannot starve writes.
--
-- ADR-001 schema-per-tenant compliance: the RO role is scoped to a
-- single workspace schema; it has no privileges on other workspaces or
-- on the core schema.
--
-- This file CREATE OR REPLACEs core.lecrm_provision_workspace; the body
-- is the union of every prior migration's table list (companies,
-- contacts, deals, objects, custom_property_definitions, sync_connections,
-- external_entity_mappings, email_suppression, email_send_log) plus the
-- new step 15 that provisions the RO role. Keeping every step in a
-- single CREATE OR REPLACE preserves the existing rebuild-from-scratch
-- semantics: a fresh DB applying migrations in order ends with the
-- complete schema, and re-running any single migration is idempotent.
--
-- References:
--   ADR-009 §9 — Cube.dev as v0 dashboard bridge
--   ADR-001 — schema-per-tenant
--   packages/db/migrations/0012_email_suppression.sql — previous body

BEGIN;

-- ============================================================
-- 1. core.lecrm_cube_reader — singleton LOGIN role for Cube.
-- ============================================================
-- Created at the DB layer once (not per workspace). Password is set at
-- deploy time via ALTER ROLE outside the migration: this file is run
-- under postgres superuser at fresh-DB init and we do not want a
-- migration to contain a literal credential. The deploy/compose stack
-- runs an ALTER ROLE statement using LECRM_CUBE_DB_PASSWORD from the
-- environment.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'lecrm_cube_reader') THEN
    EXECUTE 'CREATE ROLE lecrm_cube_reader LOGIN PASSWORD NULL CONNECTION LIMIT 50';
  END IF;
  -- Tight defaults: 15s statement, 2s lock. Reporting queries must not
  -- pile up behind writes or hold long transactions.
  EXECUTE 'ALTER ROLE lecrm_cube_reader SET statement_timeout = ''15s''';
  EXECUTE 'ALTER ROLE lecrm_cube_reader SET lock_timeout = ''2s''';
  EXECUTE 'ALTER ROLE lecrm_cube_reader SET idle_in_transaction_session_timeout = ''10s''';
  -- search_path includes core only so Cube can read pipeline_stages, etc.
  -- via SET ROLE — the per-workspace RO role layers its own schema on top.
  EXECUTE 'ALTER ROLE lecrm_cube_reader SET search_path = core, pg_catalog';
END$$;

-- Grant lecrm_cube_reader to lecrm_provisioner so the SECURITY DEFINER
-- function can GRANT each per-workspace RO role to it without superuser.
GRANT lecrm_cube_reader TO lecrm_provisioner;

-- ============================================================
-- 2. core.lecrm_provision_workspace(UUID) — extended for RO role.
-- ============================================================
CREATE OR REPLACE FUNCTION core.lecrm_provision_workspace(p_workspace_id UUID)
  RETURNS TEXT
  LANGUAGE plpgsql
  SECURITY DEFINER
  SET search_path = core, pg_catalog
AS $func$
DECLARE
  v_role_name    TEXT;
  v_ro_role_name TEXT;
  v_password     TEXT;
  v_river_name   TEXT;
BEGIN
  IF p_workspace_id IS NULL THEN
    RAISE EXCEPTION 'p_workspace_id must not be NULL'
      USING ERRCODE = 'invalid_parameter_value';
  END IF;
  IF p_workspace_id = '00000000-0000-0000-0000-000000000000'::uuid THEN
    RAISE EXCEPTION 'p_workspace_id must not be the zero UUID'
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  v_role_name    := 'workspace_' || lower(replace(p_workspace_id::text, '-', ''));
  v_ro_role_name := v_role_name || '_ro';
  v_password     := encode(gen_random_bytes(32), 'base64');
  v_river_name   := 'river_' || lower(replace(p_workspace_id::text, '-', ''));

  -- 1. Owner role.
  BEGIN
    EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L CONNECTION LIMIT 10', v_role_name, v_password);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;
  BEGIN
    EXECUTE format('GRANT %I TO lecrm_provisioner', v_role_name);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;
  EXECUTE format('ALTER ROLE %I SET search_path = %I, public', v_role_name, v_role_name);
  EXECUTE format('ALTER ROLE %I SET statement_timeout = ''30s''', v_role_name);
  EXECUTE format('ALTER ROLE %I SET lock_timeout = ''5s''', v_role_name);
  EXECUTE format('ALTER ROLE %I SET work_mem = ''16MB''', v_role_name);

  -- 2. Schema.
  BEGIN
    EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_role_name, v_role_name);
  EXCEPTION
    WHEN duplicate_schema THEN NULL;
  END;

  -- 3. Owner grants.
  EXECUTE format('GRANT USAGE, CREATE ON SCHEMA %I TO %I', v_role_name, v_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT ALL ON TABLES TO %I',
    v_role_name, v_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT ALL ON SEQUENCES TO %I',
    v_role_name, v_role_name);

  -- 4. Lateral-expansion mitigation.
  EXECUTE format('REVOKE CREATE ON SCHEMA public FROM %I', v_role_name);
  EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA public FROM %I', v_role_name);

  -- 5. River-job schema.
  BEGIN
    EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_river_name, v_role_name);
  EXCEPTION
    WHEN duplicate_schema THEN NULL;
  END;

  -- 6. Custom-property metadata table (ADR-010 §4).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.custom_property_definitions (
      id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      parent_type     text NOT NULL CHECK (parent_type IN ('contact', 'deal')),
      property_key    text NOT NULL,
      property_type   text NOT NULL CHECK (property_type IN ('string', 'number', 'boolean', 'enum', 'date')),
      allowed_values  jsonb NULL,
      required        boolean NOT NULL DEFAULT false,
      created_at      timestamptz NOT NULL DEFAULT now(),
      UNIQUE (parent_type, property_key)
    )$f$, v_role_name);

  -- 7. JSONB-primary object storage table + indexes (ADR-010 §3).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.objects (
      id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      object_type   text NOT NULL,
      parent_type   text NULL,
      parent_id     uuid NULL,
      data          jsonb NOT NULL DEFAULT '{}'::jsonb,
      created_at    timestamptz NOT NULL DEFAULT now(),
      updated_at    timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS objects_type_parent_idx ON %I.objects (object_type, parent_type, parent_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS objects_data_gin_idx ON %I.objects USING gin (data)',
    v_role_name);

  -- 8. Companies table.
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.companies (
      id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      name       text NOT NULL,
      domain     text,
      industry   text,
      size       text CHECK (size IN ('1-10','11-50','51-200','201-1000','1000+')),
      owner_id   uuid,
      created_at timestamptz NOT NULL DEFAULT now(),
      updated_at timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name);

  -- 9. Contacts table.
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.contacts (
      id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      first_name  text NOT NULL,
      last_name   text NOT NULL,
      email       text,
      phone       text,
      company_id  uuid REFERENCES %I.companies(id) ON DELETE SET NULL,
      owner_id    uuid,
      created_at  timestamptz NOT NULL DEFAULT now(),
      updated_at  timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS contacts_company_id_idx ON %I.contacts (company_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS contacts_email_idx ON %I.contacts (email) WHERE email IS NOT NULL',
    v_role_name);

  -- 10. Deals table.
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.deals (
      id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      title               text NOT NULL,
      amount              numeric(12,2),
      currency            char(3) DEFAULT 'EUR',
      stage_id            uuid,
      contact_id          uuid REFERENCES %I.contacts(id) ON DELETE SET NULL,
      company_id          uuid REFERENCES %I.companies(id) ON DELETE SET NULL,
      owner_id            uuid,
      expected_close_date date,
      closed_at           timestamptz,
      created_at          timestamptz NOT NULL DEFAULT now(),
      updated_at          timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name, v_role_name, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS deals_stage_id_idx ON %I.deals (stage_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS deals_contact_id_idx ON %I.deals (contact_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS deals_expected_close_date_idx ON %I.deals (expected_close_date) WHERE expected_close_date IS NOT NULL',
    v_role_name);

  -- 11. Sync connections — per-tenant provider lifecycle (ADR-011 §3).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.sync_connections (
      id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      provider_id  text NOT NULL,
      status       text NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','active','paused','error','revoked','disconnected')),
      settings     jsonb NOT NULL DEFAULT '{}'::jsonb,
      sync_cursor  jsonb,
      last_sync_at timestamptz,
      last_error   text,
      created_at   timestamptz NOT NULL DEFAULT now(),
      updated_at   timestamptz NOT NULL DEFAULT now(),
      UNIQUE (provider_id)
    )$f$, v_role_name);

  -- 12. External entity mappings — external ID ↔ leCRM entity (ADR-011 §4).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.external_entity_mappings (
      id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      provider_id    text NOT NULL,
      external_id    text NOT NULL,
      entity_type    text NOT NULL,
      entity_id      uuid NOT NULL,
      last_synced_at timestamptz NOT NULL DEFAULT now(),
      meta           jsonb NOT NULL DEFAULT '{}'::jsonb,
      created_at     timestamptz NOT NULL DEFAULT now(),
      UNIQUE (provider_id, external_id)
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS ext_map_entity_idx ON %I.external_entity_mappings (entity_type, entity_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS ext_map_provider_idx ON %I.external_entity_mappings (provider_id, external_id)',
    v_role_name);

  -- 13. Email suppression list — ADR-003 §Mitigations item 3.
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.email_suppression (
      id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      email          text NOT NULL UNIQUE,
      reason         text NOT NULL CHECK (reason IN ('hard_bounce','blocked','complaint','unsubscribed')),
      suppressed_at  timestamptz NOT NULL DEFAULT now(),
      created_at     timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS email_suppression_at_idx ON %I.email_suppression (suppressed_at DESC)',
    v_role_name);

  -- 14. Email send log — 7-day rolling bounce-rate alarm (ADR-003 §Mitigations item 4).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.email_send_log (
      id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      message_id      text,
      recipient_email text NOT NULL,
      sent_at         timestamptz NOT NULL DEFAULT now(),
      bounced_at      timestamptz,
      bounce_kind     text CHECK (bounce_kind IN ('hard_bounce','complaint'))
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS email_send_log_sent_at_idx ON %I.email_send_log (sent_at DESC)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS email_send_log_message_id_idx ON %I.email_send_log (message_id) WHERE message_id IS NOT NULL',
    v_role_name);

  -- 15. Per-workspace READ-ONLY role for Cube.dev (ADR-009 §9).
  --     NOLOGIN — only reachable via SET ROLE from lecrm_cube_reader.
  BEGIN
    EXECUTE format('CREATE ROLE %I NOLOGIN', v_ro_role_name);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;
  BEGIN
    EXECUTE format('GRANT %I TO lecrm_provisioner', v_ro_role_name);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;

  -- Allow lecrm_cube_reader to assume this RO role via SET ROLE.
  BEGIN
    EXECUTE format('GRANT %I TO lecrm_cube_reader', v_ro_role_name);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;

  -- Tight session settings on the RO role; persisted via ALTER ROLE
  -- and inherited when lecrm_cube_reader does SET ROLE.
  EXECUTE format('ALTER ROLE %I SET search_path = %I, core, pg_catalog', v_ro_role_name, v_role_name);
  EXECUTE format('ALTER ROLE %I SET statement_timeout = ''15s''', v_ro_role_name);
  EXECUTE format('ALTER ROLE %I SET lock_timeout = ''2s''', v_ro_role_name);

  -- Strict privileges: USAGE + SELECT on the workspace schema only.
  -- No CREATE, no INSERT/UPDATE/DELETE, no access to other workspaces.
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO %I', v_role_name, v_ro_role_name);
  EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA %I TO %I', v_role_name, v_ro_role_name);
  EXECUTE format('GRANT SELECT ON ALL SEQUENCES IN SCHEMA %I TO %I', v_role_name, v_ro_role_name);

  -- Default privileges: future tables created by the owner are
  -- automatically SELECT-able by the RO role. Issued FOR ROLE the
  -- schema owner so the ACL applies to objects they create.
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA %I GRANT SELECT ON TABLES TO %I',
    v_role_name, v_role_name, v_ro_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA %I GRANT SELECT ON SEQUENCES TO %I',
    v_role_name, v_role_name, v_ro_role_name);

  -- Explicitly REVOKE any inherited write privilege the RO role might
  -- pick up via PUBLIC. Belt-and-suspenders — PUBLIC has no INSERT on
  -- these tables by default, but a future migration that loosens grants
  -- must not silently re-open writes through the RO role.
  EXECUTE format('REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON ALL TABLES IN SCHEMA %I FROM %I',
    v_role_name, v_ro_role_name);

  RETURN v_role_name;
END;
$func$;

ALTER FUNCTION core.lecrm_provision_workspace(UUID) OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace(UUID) FROM PUBLIC;

-- ============================================================
-- 3. Backfill: provision RO role for existing workspaces.
-- ============================================================
-- Re-run the (now extended) provisioning function for every workspace
-- already in core.workspaces. The function is idempotent — duplicate
-- role / schema creates are swallowed; only the RO-role block is new.
DO $$
DECLARE
  ws RECORD;
BEGIN
  FOR ws IN SELECT id FROM core.workspaces LOOP
    PERFORM core.lecrm_provision_workspace(ws.id);
  END LOOP;
END$$;

COMMIT;
