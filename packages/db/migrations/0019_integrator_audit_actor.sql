-- 0019_integrator_audit_actor.sql — tag integrator-actor writes in the
-- canonical audit trail (Sprint: lecrm-integrator-switching, step 3/6).
--
-- WHY THIS EXISTS
-- ---------------
-- Login-time elevation (tasket 3) materializes GB Consult's integrator as an
-- 'integrator' member and the rbac layer now stamps the session principal's
-- actor_type as 'integrator'. core.audit_log.actor_type carries a CHECK
-- constraint (from 0001_init.sql) that admits only the original four actor
-- types, so an integrator-attributed audit row would be REJECTED by the
-- fail-closed EmitAudit path (apps/api/capability/capability.go). This
-- migration extends that CHECK to admit 'integrator'.
--
-- SCOPE
-- -----
-- Only core.audit_log is altered — it is the security-relevant audit trail
-- (ADR-009 §7.2). The per-workspace activities table's actor_type CHECK
-- (created per-schema by core.lecrm_provision_workspace) is deliberately NOT
-- touched here: it is the entity timeline, not the audit trail, and altering
-- it would require redefining the provisioning function plus a cross-schema
-- sweep. emitEntityActivity maps integrator → human_api for that timeline
-- instead (see apps/api/capability/capability.go).
--
-- The CHECK on core.audit_log is column-level and unnamed in 0001, so
-- PostgreSQL auto-named it audit_log_actor_type_check. DROP IF EXISTS + ADD is
-- idempotent: a re-run drops the (already integrator-admitting) constraint and
-- recreates the identical definition.
--
-- References:
--   packages/db/migrations/0001_init.sql — core.audit_log + original CHECK
--   packages/db/migrations/0018_integrator_role_and_grants.sql — grants table
--   apps/api/internal/rbac/middleware.go — sets actor_type 'integrator'
--   apps/api/capability/capability.go — EmitAudit / emitEntityActivity
--   docs/adr/ADR-009-stack-and-license.md §7.2 — audit trail

BEGIN;

ALTER TABLE core.audit_log
  DROP CONSTRAINT IF EXISTS audit_log_actor_type_check;
ALTER TABLE core.audit_log
  ADD CONSTRAINT audit_log_actor_type_check
  CHECK (actor_type IN ('human_api', 'mcp_agent', 'internal_service', 'system', 'integrator')
         OR actor_type IS NULL);

COMMIT;
