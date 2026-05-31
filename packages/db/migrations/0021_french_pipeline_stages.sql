-- 0021_french_pipeline_stages.sql — French labels for the gbconsult-default
-- pipeline (tasket 0077, lecrm-demo-polish).
--
-- WHY THIS EXISTS
-- ---------------
-- The gbconsult-default template seeded English stage names (Discovery,
-- Qualified, Proposal Sent, Negotiation, Closed-Won/Lost) while every deal
-- title, custom field and the ICP are French — a FR/EN mix that reads as
-- unfinished to the French-SMB ICP. This migration:
--   1. Re-defines core.lecrm_provision_workspace_with_registry so NEW
--      workspaces seed French labels.
--   2. Back-fills EXISTING workspace schemas (renaming the template alone
--      does not retro-update already-provisioned tenants such as the demo).
--
-- ORDERING / NUMBERING (was 0020 — collided with
-- 0020_restore_registry_input_validation.sql)
-- ---------------------------------------------------------------------------
-- This is numbered 0021 so it applies AFTER 0020_restore_registry_input_
-- validation.sql. The migrator (apps/migrate) applies *.sql in alphanumeric
-- filename order and tracks applied files by name, so two files numbered 0020
-- would be an ambiguous/duplicate version. Because BOTH that migration and
-- this one CREATE OR REPLACE the same function, the LAST one applied wins —
-- so this one must (a) run last and (b) carry the validation guards itself,
-- or it would silently re-drop them.
--
-- CANONICAL FUNCTION BODY — KEEP IN SYNC
-- ---------------------------------------------------------------------------
-- This file now holds the AUTHORITATIVE definition of
-- core.lecrm_provision_workspace_with_registry. Its body is
-- 0020_restore_registry_input_validation.sql's body (= the 0006 input-
-- validation guards on top of the 0017 grant/search_path body) with ONLY the
-- pipeline seed labels changed to French. Any future change to provisioning
-- logic (grants, audit row, atomicity, validation) MUST be mirrored here, or
-- the next CREATE OR REPLACE will lose it. The up-front validation block is
-- asserted by apps/admin/internal/tenant/definer_hardening_test.go
-- (TestRegistryRejectsInvalidSlug / TestRegistryRejectsInvalidEmail /
-- TestRegistryRejectsNullSlug) — dropping it here turns those tests red.
--
-- DESIGN NOTES
-- ------------
-- * Stage IDs and order_index are NOT touched — deals reference stage_id
--   (a UUID), so no FK breaks and every deal keeps its column.
-- * The combined "Closed-Won/Lost" stage becomes a single "Gagné / Perdu"
--   stage (stage count stays 5). Splitting Won vs Lost into two distinct
--   stages is the more correct model (Leo treats them as distinct) but
--   changes the stage count — flagged for Guillaume, not done here.
-- * The back-fill renames ONLY stages whose current name matches a known
--   English label, so tenants that customized their pipeline are untouched.
--   Re-running on already-French data is a no-op (idempotent).
--
-- References:
--   packages/db/migrations/0006_security_definer_hardening.sql — original guards
--   packages/db/migrations/0017_app_role.sql — added grant, dropped guards
--   packages/db/migrations/0020_restore_registry_input_validation.sql — restored guards
--   apps/admin/internal/tenant/definer_hardening_test.go — the assertions

BEGIN;

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
    -- French labels (0021) — see file header.
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
          ('Découverte', 1),
          ('Qualifié', 2),
          ('Proposition envoyée', 3),
          ('Négociation', 4),
          ('Gagné / Perdu', 5)$f$, v_role_name);
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

-- Back-fill: rename the English gbconsult-default labels to French in every
-- already-provisioned gbconsult-default workspace schema. Scoped two ways so
-- it never touches a pipeline it didn't seed:
--   1. Only workspaces whose provisioning_features_applied includes
--      "gbconsult-default-v1" (the feature this template stamps) are visited —
--      bootstrap/other-template tenants are skipped entirely.
--   2. The UPDATE matches on the exact old English label, so a gbconsult-default
--      tenant that later customised a stage name is still left alone.
-- Idempotent on already-French data. IDs + order_index are preserved, so deals
-- keep their stage_id (no FK break).
DO $$
DECLARE
  ws RECORD;
  rename_sql CONSTANT TEXT := $f$
    UPDATE %I.pipeline_stages SET name = CASE name
      WHEN 'Discovery'       THEN 'Découverte'
      WHEN 'Qualified'       THEN 'Qualifié'
      WHEN 'Proposal Sent'   THEN 'Proposition envoyée'
      WHEN 'Negotiation'     THEN 'Négociation'
      WHEN 'Closed-Won/Lost' THEN 'Gagné / Perdu'
      ELSE name
    END
    WHERE name IN ('Discovery','Qualified','Proposal Sent','Negotiation','Closed-Won/Lost')
  $f$;
BEGIN
  FOR ws IN
    SELECT role_name FROM core.workspaces
    WHERE provisioning_features_applied @> '["gbconsult-default-v1"]'::jsonb
  LOOP
    IF EXISTS (
      SELECT 1 FROM information_schema.tables
      WHERE table_schema = ws.role_name AND table_name = 'pipeline_stages'
    ) THEN
      EXECUTE format(rename_sql, ws.role_name);
    END IF;
  END LOOP;
END$$;

COMMIT;
