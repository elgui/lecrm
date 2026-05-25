-- 0006_security_definer_hardening.sql — harden SECURITY DEFINER functions.
--
-- Applies four mitigations identified in the council architecture review
-- (2026-05-24, Rook / security voice):
--
--   1. Pin search_path to `core, pg_catalog` — removes `public` from the
--      function's search_path so a malicious object planted in `public`
--      cannot shadow a pg_catalog function during SECURITY DEFINER
--      execution.
--
--   2. Input validation — reject NULL/zero UUIDs, malformed slugs, and
--      invalid emails BEFORE any EXECUTE statement runs. This shrinks the
--      dynamic-SQL attack surface to validated inputs only.
--
--   3. Template allow-list at function entry — the existing template check
--      only ran inside `IF v_inserted = 1`, so an invalid template on a
--      re-invocation (upsert path) would silently pass. Now validated
--      unconditionally.
--
--   4. Explicit REVOKE/GRANT re-assertion — idempotent, but ensures the
--      ACL is correct even if a prior migration ran with a different owner.
--
-- References:
--   docs/council-architecture-review-2026-05-24.md — Rook findings
--   PostgreSQL docs: CREATE FUNCTION / SECURITY DEFINER / search_path
--   CWE-89  — SQL injection via search_path manipulation
--   CWE-250 — Execution with unnecessary privileges

BEGIN;

-- ============================================================
-- 1. core.lecrm_provision_workspace(UUID) — hardened
-- ============================================================
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
  -- Input validation: reject NULL or zero UUID before any DDL.
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

  RETURN v_role_name;
END;
$func$;

ALTER FUNCTION core.lecrm_provision_workspace(UUID) OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace(UUID) FROM PUBLIC;

-- ============================================================
-- 2. core.lecrm_provision_workspace_with_registry(...) — hardened
-- ============================================================
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
  SET search_path = core, pg_catalog
AS $func$
DECLARE
  v_role_name TEXT;
  v_features  JSONB;
  v_inserted  INT := 0;
BEGIN
  -- ---- Input validation (before any DDL or DML) ----

  IF p_workspace_id IS NULL THEN
    RAISE EXCEPTION 'p_workspace_id must not be NULL'
      USING ERRCODE = 'invalid_parameter_value';
  END IF;
  IF p_workspace_id = '00000000-0000-0000-0000-000000000000'::uuid THEN
    RAISE EXCEPTION 'p_workspace_id must not be the zero UUID'
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  IF p_slug IS NULL OR p_slug !~ '^[a-z][a-z0-9-]{2,31}$' THEN
    RAISE EXCEPTION 'p_slug must match ^[a-z][a-z0-9-]{2,31}$, got: %', coalesce(p_slug, '<NULL>')
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  IF p_admin_email IS NOT NULL AND p_admin_email <> '' THEN
    IF position('@' IN p_admin_email) = 0 THEN
      RAISE EXCEPTION 'p_admin_email must contain @, got: %', p_admin_email
        USING ERRCODE = 'invalid_parameter_value';
    END IF;
  END IF;

  IF p_creator_email IS NOT NULL AND p_creator_email <> '' THEN
    IF position('@' IN p_creator_email) = 0 THEN
      RAISE EXCEPTION 'p_creator_email must contain @, got: %', p_creator_email
        USING ERRCODE = 'invalid_parameter_value';
    END IF;
  END IF;

  IF p_template IS NOT NULL AND p_template <> '' AND p_template <> 'gbconsult-default' THEN
    RAISE EXCEPTION 'unknown template: %. Known templates for v0: gbconsult-default. Pass empty string or NULL for the bootstrap path.', p_template
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  -- ---- Derived values (after validation) ----

  v_role_name := 'workspace_' || lower(replace(p_workspace_id::text, '-', ''));
  v_features  := CASE
                   WHEN p_template = '' OR p_template IS NULL THEN '[]'::jsonb
                   ELSE jsonb_build_array(p_template || '-v1')
                 END;

  -- Step 1. Call the existing provisioning function (schema-qualified).
  PERFORM core.lecrm_provision_workspace(p_workspace_id);

  -- Step 2. UPSERT core.workspaces (schema-qualified).
  INSERT INTO core.workspaces
    (id, slug, role_name, admin_email, creator_email, provisioning_features_applied)
  VALUES
    (p_workspace_id, p_slug, v_role_name, p_admin_email, p_creator_email, v_features)
  ON CONFLICT (id) DO NOTHING;
  GET DIAGNOSTICS v_inserted = ROW_COUNT;

  SELECT w.role_name INTO v_role_name FROM core.workspaces w WHERE w.id = p_workspace_id;

  -- Step 3 + 4: audit row and pipeline seed only on fresh provision.
  IF v_inserted = 1 THEN
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
    END IF;
  END IF;

  RETURN v_role_name;
END;
$func$;

ALTER FUNCTION core.lecrm_provision_workspace_with_registry(UUID, TEXT, TEXT, TEXT, TEXT)
  OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace_with_registry(UUID, TEXT, TEXT, TEXT, TEXT)
  FROM PUBLIC;

COMMIT;
