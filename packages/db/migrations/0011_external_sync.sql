-- 0011_external_sync.sql — external-system sync infrastructure (ADR-011).
--
-- Adds two tables to each workspace schema:
--   sync_connections: per-tenant provider connection lifecycle
--   external_entity_mappings: external ID ↔ leCRM entity lookup
--
-- These tables power the sync.Engine abstraction. Gmail is the first
-- provider (v0, read-only import); the schema supports arbitrary
-- future connectors (Shopify, Outlook, etc.) without DDL changes.
--
-- References:
--   docs/adr/ADR-011-external-system-sync.md
--   apps/api/internal/sync/ — Go interface definitions

BEGIN;

CREATE OR REPLACE FUNCTION core.lecrm_provision_workspace(p_workspace_id UUID)
  RETURNS TEXT
  LANGUAGE plpgsql
  SECURITY DEFINER
  SET search_path = core, pg_catalog
AS $func$
DECLARE
  v_role_name   TEXT;
  v_password    TEXT;
  v_river_name  TEXT;
BEGIN
  IF p_workspace_id IS NULL THEN
    RAISE EXCEPTION 'p_workspace_id must not be NULL'
      USING ERRCODE = 'invalid_parameter_value';
  END IF;
  IF p_workspace_id = '00000000-0000-0000-0000-000000000000'::uuid THEN
    RAISE EXCEPTION 'p_workspace_id must not be the zero UUID'
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  v_role_name  := 'workspace_' || lower(replace(p_workspace_id::text, '-', ''));
  v_password   := encode(gen_random_bytes(32), 'base64');
  v_river_name := 'river_' || lower(replace(p_workspace_id::text, '-', ''));

  -- 1. Role
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

  -- 4. Lateral-expansion mitigation
  EXECUTE format('REVOKE CREATE ON SCHEMA public FROM %I', v_role_name);
  EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA public FROM %I', v_role_name);

  -- 5. River-job schema
  BEGIN
    EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_river_name, v_role_name);
  EXCEPTION
    WHEN duplicate_schema THEN NULL;
  END;

  -- 6. Custom-property metadata table (ADR-010 §4)
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

  -- 8. Companies table
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

  -- 9. Contacts table
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

  -- 10. Deals table
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

  -- 11. Sync connections — per-tenant provider lifecycle (ADR-011 §3)
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

  -- 12. External entity mappings — external ID ↔ leCRM entity (ADR-011 §4)
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

  RETURN v_role_name;
END;
$func$;

ALTER FUNCTION core.lecrm_provision_workspace(UUID) OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace(UUID) FROM PUBLIC;

COMMIT;
