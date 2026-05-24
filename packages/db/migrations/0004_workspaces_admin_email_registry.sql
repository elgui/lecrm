-- 0004_workspaces_admin_email_registry.sql — extend core.workspaces and add the
-- atomic provisioning wrapper for the integrator handoff (Story 8.1).
--
-- This migration adds three columns to core.workspaces (admin_email,
-- creator_email, provisioning_features_applied) and defines a new SECURITY
-- DEFINER function core.lecrm_provision_workspace_with_registry that composes
-- in ONE transaction:
--   (a) UPSERT into core.workspaces (with the new columns)
--   (b) core.lecrm_provision_workspace(p_workspace_id) — existing function,
--       creates role + workspace_<uuid> schema + river_<uuid> schema +
--       metadata-engine tables (extended in 0003)
--   (c) INSERT into core.audit_log with event = 'workspace.provisioned'
--   (d) CREATE TABLE pipeline_stages in workspace_<uuid> + INSERT 5 default
--       stages of the named template
--
-- The wrapper is the atomicity boundary required by AC-T1 (Story 8.1):
-- partial provisioning is not an acceptable failure mode in front of the
-- first Design Partner. Both apps/admin (tenant create) and apps/migrate
-- (provision-workspace) call this wrapper — no duplicate Go provisioning
-- code (D11). For the bootstrap path apps/migrate passes empty admin_email,
-- empty creator_email, and empty template (no pipeline seed).
--
-- References:
--   Story 8.1 (T1, AC-T1, AC-F1..F5, AC-I-05..I-13, D9, D11, D12)
--   ADR-009 §2.1 — single-transaction provisioning contract
--   ADR-010 (Sprint 9) — replaces hardcoded gbconsult-default with metadata registry
--   packages/db/migrations/0001_init.sql:59-86 — core.workspaces + core.audit_log
--   packages/db/migrations/0003_metadata_engine.sql — existing function extension

BEGIN;

-- 1. Extend core.workspaces with the three Story 8.1 columns. Single
--    migration (council Round 3 verdict: 3-of-4 against two-step). Foundation
--    story; zero live rows expected. ADD COLUMN IF NOT EXISTS preserves
--    idempotency on re-apply.
ALTER TABLE core.workspaces
  ADD COLUMN IF NOT EXISTS admin_email TEXT NOT NULL DEFAULT '';
ALTER TABLE core.workspaces
  ADD COLUMN IF NOT EXISTS creator_email TEXT NOT NULL DEFAULT '';
ALTER TABLE core.workspaces
  ADD COLUMN IF NOT EXISTS provisioning_features_applied JSONB NOT NULL DEFAULT '[]'::jsonb;

-- 2. Commit-time backfill notice (the bootstrap path apps/migrate passes
--    empty admin_email by design, so empty values are accepted; the loud
--    assertion belongs in a future migration once we have live rows that
--    SHOULD have admin emails). For 8.1 this is informational only.
DO $$
DECLARE
  v_count INT;
BEGIN
  SELECT count(*) INTO v_count FROM core.workspaces WHERE admin_email = '';
  IF v_count > 0 THEN
    RAISE NOTICE 'core.workspaces: % rows with empty admin_email (bootstrap path is allowed in 8.1)', v_count;
  END IF;
END $$;

-- 3. lecrm_provision_workspace_with_registry — the atomic provisioning
--    wrapper. SECURITY DEFINER so callers (apps/admin, apps/migrate) only
--    need EXECUTE; the function runs with lecrm_provisioner's privileges
--    and inherits the caller's transaction.
--
-- Idempotency contract:
--   - p_workspace_id is the natural key. On a fresh provision the
--     caller mints a new UUIDv7. On --upsert against an existing slug the
--     caller looks up the existing workspace UUID and passes it here.
--   - INSERT INTO core.workspaces ... ON CONFLICT (id) DO NOTHING preserves
--     the "underlying DB state unchanged" guarantee from AC-F3 / AC-I-10.
--   - The audit row and pipeline seed are written ONLY when this call
--     newly inserted a workspaces row. Repeat calls are silent no-ops.
--
-- Transactional contract (AC-T1):
--   All six writes — role, schema, river schema, workspaces row, audit
--   row, pipeline seed — commit together or nothing commits. Postgres
--   CREATE ROLE / CREATE SCHEMA are transactional in PG13+; SECURITY
--   DEFINER functions inherit the caller's transaction, so a failure at
--   any step (e.g. UNIQUE (slug) violation, pipeline-stages collision)
--   triggers a full rollback.
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
  -- workspace_<uuid> schema + river_<uuid> schema + metadata-engine tables
  -- (custom_property_definitions, objects). Silent-idempotent on re-invocation.
  PERFORM core.lecrm_provision_workspace(p_workspace_id);

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
    -- actor_type = 'system' because this is internal infrastructure, not a
    -- human or MCP action. The CLI surfaces actor identity in the payload.
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
    -- Templates are hardcoded for v0; ADR-010 (Sprint 9) replaces this with
    -- a metadata-engine-backed registry.
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

-- Ownership and ACL: the wrapper runs as lecrm_provisioner (Tier-0). Only
-- granted callers may EXECUTE; PUBLIC has none.
ALTER FUNCTION core.lecrm_provision_workspace_with_registry(UUID, TEXT, TEXT, TEXT, TEXT)
  OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace_with_registry(UUID, TEXT, TEXT, TEXT, TEXT)
  FROM PUBLIC;

COMMIT;
