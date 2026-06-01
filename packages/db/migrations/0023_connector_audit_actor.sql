-- 0023_connector_audit_actor.sql — admit the 'connector' actor_type in the
-- canonical audit trail (closes the connector-event fail-closed prod bug).
--
-- WHY THIS EXISTS
-- ---------------
-- The connector event-ingestion path (POST /v1/connectors/{source}/events,
-- apps/api/internal/crm/connectors.go) writes audit rows with
-- actor_type = 'connector' via capability.EmitAudit, e.g.:
--   * connector.contact.enriched        (connectors.go:299)
--   * connector.<invitation event>      (connectors.go:413)
--   * connector.<reply event>           (connectors.go:443)
--
-- core.audit_log.actor_type carries a CHECK constraint (from 0001_init.sql,
-- extended by 0019 to admit 'integrator') that does NOT admit 'connector'.
-- Because EmitAudit is fail-closed (a mutation that cannot be audit-logged is
-- rejected and the surrounding WriteTx rolls back — ADR-009 §7.2), EVERY
-- connector event currently violates the CHECK (SQLSTATE 23514) and rolls back
-- in production: chatboting candidate-enrichment and invitation-claim events
-- silently fail. This migration extends the CHECK to admit 'connector'.
--
-- WHY OPTION (1) — EXTEND THE CHECK (vs. remap connector → internal_service)
-- -------------------------------------------------------------------------
-- 'connector' is already a first-class, attribution-bearing actor in the
-- per-workspace activities tables: the provisioning function's activities CHECK
-- admits it (0015 / 0022_dedup_no_merge_rules.sql:295). Service tokens are
-- minted with ActorType "connector" (auth.CreateServiceTokenInput). Remapping
-- connector audit writes to 'internal_service' for the central trail would
-- DROP that attribution and desync the central audit from the entity timeline,
-- defeating ADR-007's audit catalogue. Admitting 'connector' in core.audit_log
-- restores parity: the same actor appears in both the entity timeline and the
-- security-relevant central audit. (Mirrors the 0019 decision for 'integrator'.)
--
-- SCOPE
-- -----
-- Only core.audit_log is altered. The per-workspace activities CHECK already
-- admits 'connector', so no per-schema sweep is needed (unlike 0019).
--
-- IDEMPOTENCY
-- -----------
-- The CHECK on core.audit_log is column-level and unnamed in 0001, so
-- PostgreSQL auto-named it audit_log_actor_type_check. DROP IF EXISTS + ADD is
-- idempotent: a re-run drops the (already connector-admitting) constraint and
-- recreates the identical definition.
--
-- References:
--   packages/db/migrations/0001_init.sql:82 — core.audit_log + original CHECK
--   packages/db/migrations/0019_integrator_audit_actor.sql — 'integrator' add
--   packages/db/migrations/0022_dedup_no_merge_rules.sql:295 — per-workspace
--     activities CHECK already admits 'connector'
--   apps/api/internal/crm/connectors.go:299,413,443 — connector audit writes
--   apps/api/capability/capability.go — EmitAudit (fail-closed) + ActorType*
--   docs/adr/ADR-007 — audit catalogue; ADR-009 §7.2 — fail-closed audit

BEGIN;

ALTER TABLE core.audit_log
  DROP CONSTRAINT IF EXISTS audit_log_actor_type_check;
ALTER TABLE core.audit_log
  ADD CONSTRAINT audit_log_actor_type_check
  CHECK (actor_type IN ('human_api', 'mcp_agent', 'internal_service', 'system', 'integrator', 'connector')
         OR actor_type IS NULL);

COMMIT;
