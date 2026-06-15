-- 0026_sequence_audit_emit_fn.sql — same-tx audit emission for the sequences
-- state machine (ADR-004 rev 2 §2/§6, ADR-009 §7.2 fail-closed).
--
-- WHY THIS EXISTS
-- ---------------
-- ADR-004 rev 2 §2 requires every sequences state transition to emit one
-- core.audit_log row IN THE SAME TRANSACTION as the state change (fail-closed:
-- a state change that cannot be audited must roll back, ADR-009 §7.2).
--
-- The sequences workers open that transaction as the WORKSPACE ROLE
-- (workspace_<hex>) via db.TenantPool.AcquireTx — the role barrier is the
-- primary tenant-isolation control (ADR-009 §8.3). But core.audit_log is owned
-- by lecrm_provisioner and only lecrm_api (the main API pool) holds core
-- grants; a workspace role has NO access to schema core at all. So a direct
--   INSERT INTO core.audit_log ...
-- from inside a worker-role transaction fails with "permission denied for
-- schema core" — capability.EmitAudit works only because the capability
-- Service runs on the main lecrm_api pool, never as a workspace role.
--
-- THE MECHANISM
-- -------------
-- core.lecrm_emit_audit(...) is a SECURITY DEFINER function owned by
-- lecrm_provisioner (the owner of core.audit_log). A workspace-role
-- transaction calls it WITHIN its own transaction:
--
--   SELECT core.lecrm_emit_audit($event, $workspace_id, $actor_type,
--                                $actor_user_id, $payload);
--
-- Because it is SECURITY DEFINER, the INSERT executes with the definer's
-- (lecrm_provisioner's) privileges, so the row lands even though the calling
-- workspace role cannot touch core directly. Because the call runs inside the
-- caller's transaction, the audit row and the state change commit or roll back
-- together — the §2/§7.2 fail-closed guarantee. This is the same idiom as
-- core.lecrm_provision_workspace (SECURITY DEFINER carrying provisioner
-- privileges to a less-privileged caller).
--
-- TENANT ISOLATION — the constrained entry point + session_user guard
-- -------------------------------------------------------------------
-- The workspace role gains NO direct access to core.audit_log (no SELECT,
-- UPDATE, DELETE, or raw INSERT) — only this single, append-only,
-- column-constrained entry point. To stop tenant A forging an audit row
-- attributed to tenant B, the function asserts that a WORKSPACE-role caller
-- may only emit for its OWN workspace: the workspace_id must hash to the
-- caller's role name (same formula as the provisioner). session_user (the
-- authenticated login role — workspace_<hex> on a worker connection) is used,
-- not current_user, because inside a SECURITY DEFINER function current_user is
-- the definer (lecrm_provisioner), whereas session_user remains the caller.
-- Privileged service roles (lecrm_api, lecrm_provisioner) legitimately write
-- for every workspace and are exempt from the guard.
--
-- WHY GRANT TO PUBLIC INSTEAD OF RESTATING THE PROVISION FUNCTION
-- --------------------------------------------------------------
-- New workspaces must "inherit" the ability to emit audit. Rather than add a
-- per-role GRANT inside core.lecrm_provision_workspace — which is FULLY
-- RESTATED by every migration that touches it and is the single largest
-- copy-paste-regression footgun in this tree (see 0024/0025 notes; CLAUDE.md
-- "Read before Write") — this migration grants USAGE on core + EXECUTE on the
-- one guarded function to PUBLIC. PUBLIC covers all current AND future roles
-- automatically, so new workspace roles inherit it with zero provision-fn
-- churn. The grant is safe: USAGE on schema core conveys NO table access
-- (every core table still requires its own privileges, which workspace roles
-- do not have), and the only PUBLIC-executable core function besides this one
-- is none — the provisioning functions REVOKE ALL FROM PUBLIC. The real
-- isolation control is the session_user guard above, not the breadth of the
-- EXECUTE grant.
--
-- References:
--   docs/adr/ADR-004-rev2-sequences-architecture.md §2 (Transition), §6 (audit)
--   docs/adr/ADR-009-stack-and-license.md §7.2 (fail-closed), §8.3 (tenancy)
--   apps/api/internal/db/tenant_pool.go — AcquireTx (worker-role tx)
--   apps/api/internal/sequences/transition.go — the sole caller
--   apps/api/capability/capability.go — EmitAudit (main-pool counterpart)

BEGIN;

CREATE OR REPLACE FUNCTION core.lecrm_emit_audit(
  p_event         text,
  p_workspace_id  uuid,
  p_actor_type    text,
  p_actor_user_id uuid,
  p_payload       jsonb
)
  RETURNS void
  LANGUAGE plpgsql
  SECURITY DEFINER
  SET search_path = core, pg_catalog
AS $func$
DECLARE
  v_expected_role text;
BEGIN
  IF p_event IS NULL OR p_event = '' THEN
    RAISE EXCEPTION 'audit event must not be empty'
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  -- Tenant-isolation guard: a workspace_<hex> caller may emit ONLY for its own
  -- workspace. session_user is the authenticated login role (stable across the
  -- SECURITY DEFINER boundary, unlike current_user). The '\_' escapes the LIKE
  -- wildcard so only the literal "workspace_" prefix matches.
  IF session_user LIKE 'workspace\_%' THEN
    v_expected_role := 'workspace_' || lower(replace(p_workspace_id::text, '-', ''));
    IF session_user <> v_expected_role THEN
      RAISE EXCEPTION
        'audit workspace_id % does not match caller role %', p_workspace_id, session_user
        USING ERRCODE = 'insufficient_privilege';
    END IF;
  END IF;

  -- Mirror capability.EmitAudit's default: an unset actor attributes to
  -- 'human_api' (the column's own DEFAULT, applied here for an explicit empty
  -- string). The actor_type CHECK on the column is the source of truth for
  -- valid values; an invalid one rolls the caller's tx back, fail-closed.
  INSERT INTO core.audit_log (event, workspace_id, actor_type, actor_user_id, payload)
  VALUES (
    p_event,
    p_workspace_id,
    COALESCE(NULLIF(p_actor_type, ''), 'human_api'),
    p_actor_user_id,
    COALESCE(p_payload, '{}'::jsonb)
  );
END;
$func$;

ALTER FUNCTION core.lecrm_emit_audit(text, uuid, text, uuid, jsonb)
  OWNER TO lecrm_provisioner;

-- Reset to a known grant state, then open the two minimal privileges every
-- caller (workspace roles via AcquireTx, lecrm_api via the main pool) needs:
-- USAGE on schema core to resolve the qualified name, and EXECUTE on the
-- function. Neither conveys any access to core TABLES.
REVOKE ALL ON FUNCTION core.lecrm_emit_audit(text, uuid, text, uuid, jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION core.lecrm_emit_audit(text, uuid, text, uuid, jsonb) TO PUBLIC;
GRANT USAGE ON SCHEMA core TO PUBLIC;

COMMIT;
