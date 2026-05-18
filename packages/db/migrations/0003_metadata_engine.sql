-- 0003_metadata_engine.sql — extend lecrm_provision_workspace for ADR-010 metadata tables.
--
-- Adds two DDL steps to core.lecrm_provision_workspace:
--   Step 6 — custom_property_definitions: type-safety metadata table (ADR-010 §4)
--   Step 7 — objects: JSONB-primary storage table + two indexes (ADR-010 §3)
--
-- Both tables are created inside the workspace schema (same AUTHORIZATION as the
-- workspace role) in a single transaction, consistent with ADR-009 §2.1 requirement
-- that provisioning be atomic. IF NOT EXISTS preserves idempotency on re-invocation.
--
-- References:
--   ADR-010 §3 (objects schema), §4 (custom_property_definitions schema), §TO RESOLVE-1
--   ADR-009 §2.1 (provisioning function, single-transaction requirement)
--   packages/db/migrations/0001_init.sql (function being extended)

BEGIN;

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

  -- 6. Custom-property metadata table (ADR-010 §4)
  --    Defines the type contract for custom properties on Contact and Deal objects.
  --    sqlc handles this static schema cleanly; the API layer validates every
  --    custom-property write against this table (fail-closed: bad payload → 400).
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

  -- 7. JSONB-primary object storage table + indexes (ADR-010 §3)
  --    Generic per-workspace store for all custom-property data. parent_id is not
  --    a DB FK (parent_type selects the target table dynamically; PG FKs cannot
  --    conditionally target multiple tables). Application enforces referential
  --    integrity; a post-v0 janitor job will detect orphan rows.
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

  -- Password is intentionally NOT returned by this function. A sibling
  -- function (to be added when secrets-manifest plumbing lands in
  -- Sprint 3 per tasket 20260510-162158-1023) will rotate-and-capture
  -- the role password and write it to the SOPS-encrypted secret
  -- manifest.
  RETURN v_role_name;
END;
$func$;

-- Ownership and ACL are preserved from 0001_init.sql; CREATE OR REPLACE
-- does not reset these, but we re-apply them to be explicit and safe on
-- environments where this migration runs without the prior function owner
-- already being lecrm_provisioner.
ALTER FUNCTION core.lecrm_provision_workspace(UUID) OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace(UUID) FROM PUBLIC;

COMMIT;
