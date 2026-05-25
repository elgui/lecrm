-- 0009_metadata_json_type.sql — add 'json' to property_type CHECK constraint.
--
-- Sprint 5 feature 6: the chatboting connector (ADR-011) needs to store
-- structured payloads (scoring_breakdown, enrichment data) as custom
-- properties. Adding 'json' to the property_type enum enables this.
--
-- Two changes:
--   1. The provisioning function gets 'json' in its CHECK clause so
--      new workspace schemas include the type from first provision.
--   2. Existing workspace schemas are updated in-place: the old CHECK
--      constraint is dropped and replaced with one that includes 'json'.
--
-- References:
--   docs/adr/ADR-010-metadata-engine.md §4a
--   docs/adr/ADR-011-chatboting-connector-boundary.md

BEGIN;

-- Update the provisioning function to include 'json' in property_type CHECK.
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

  -- 6. Custom-property metadata table (ADR-010 §4) — now includes 'json'
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.custom_property_definitions (
      id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      parent_type     text NOT NULL CHECK (parent_type IN ('contact', 'deal')),
      property_key    text NOT NULL,
      property_type   text NOT NULL CHECK (property_type IN ('string', 'number', 'boolean', 'enum', 'date', 'json')),
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

  RETURN v_role_name;
END;
$func$;

ALTER FUNCTION core.lecrm_provision_workspace(UUID) OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace(UUID) FROM PUBLIC;

-- Patch existing workspace schemas: drop old CHECK and add new one with 'json'.
-- Uses the workspace registry to find all schemas, then alters each one.
DO $migrate$
DECLARE
  v_schema TEXT;
  v_conname TEXT;
BEGIN
  FOR v_schema IN SELECT role_name FROM core.workspaces WHERE tombstoned_at IS NULL
  LOOP
    SELECT conname INTO v_conname
      FROM pg_constraint c
      JOIN pg_class r ON c.conrelid = r.oid
      JOIN pg_namespace n ON r.relnamespace = n.oid
      WHERE n.nspname = v_schema
        AND r.relname = 'custom_property_definitions'
        AND c.contype = 'c'
        AND pg_get_constraintdef(c.oid) LIKE '%property_type%';

    IF v_conname IS NOT NULL THEN
      EXECUTE format('ALTER TABLE %I.custom_property_definitions DROP CONSTRAINT %I', v_schema, v_conname);
      EXECUTE format($f$
        ALTER TABLE %I.custom_property_definitions
          ADD CONSTRAINT %I CHECK (property_type IN ('string', 'number', 'boolean', 'enum', 'date', 'json'))
      $f$, v_schema, v_conname);
    END IF;
  END LOOP;
END
$migrate$;

COMMIT;
