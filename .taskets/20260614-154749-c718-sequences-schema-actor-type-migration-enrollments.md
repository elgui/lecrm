---
id: 20260614-154749-c718
title: Sequences schema + actor_type migration (enrollments, enrollment_steps)
status: done
priority: p2
created: 2026-06-14
updated: 2026-06-15
done: 2026-06-15
tags: [sequences, v1, schema, migration]
category: project
group: lecrm-v1-build
group_order: 300
order: 2
plan: true
---

# Sequences schema + actor_type migration (enrollments, enrollment_steps)

## Why
v1 sequences need a durable Postgres state foundation. ADR-004 rev 2 §1 specifies two per-workspace tables (`enrollments`, `enrollment_steps`) plus the `enrollment_state` / `step_send_state` enums, appended to `core.lecrm_provision_workspace` as steps 13–14 (after the ADR-011 connector steps 11–12). The Brandur-style partial unique index is the durable at-most-once backstop for step sends. Separately, `audit_log` has no `actor_type` column today (ADR-004 rev 2 §Q6 / S1) — the sequences engine is the first hard requirement for it, so it MUST land in the same migration before the sequences package can merge.

This is the foundation tasket; everything else in `lecrm-v1-build` depends on it.

## Steps
1. Author the Atlas migration appending to `core.lecrm_provision_workspace` (the function is FULLY RESTATED per-migration — copy the current body verbatim and add to it; dropping a feature silently regresses it — see the provision-function restatement footgun).
2. Create enums `enrollment_state` ('enrolled','step_sent','waiting_reply','reply_received','ooo_detected','failed','bounced','unsubscribed','suppressed','completed') and `step_send_state` ('pending','sent','delivered','bounced','cancelled','superseded').
3. Create `enrollments` + `enrollment_steps` exactly per ADR-004 rev 2 §1 (incl. `brevo_message_id`, `rfc_message_id`, `idempotency_key`, supporting indexes).
4. Create `uniq_enrollment_step_active ON enrollment_steps (enrollment_id, step_index) WHERE state NOT IN ('cancelled','superseded')`.
5. Make `email_suppression` provisioning `CREATE TABLE IF NOT EXISTS`-idempotent in the same step.
6. Add `audit_log.actor_type text NOT NULL DEFAULT 'human_api'` in the same migration (ADR-007 follow-up TO RESOLVE-14).
7. Generate sqlc bindings for the new tables.
8. Integration test: provision a fresh workspace; assert tables, enums, the partial index, and the `audit_log.actor_type` column all exist.

## Done when
- A fresh-workspace provision yields `enrollments` + `enrollment_steps` + both enums + `uniq_enrollment_step_active`.
- `audit_log` has `actor_type` (default `'human_api'`).
- sqlc bindings compile; the provisioning integration test passes.
- Diff the restated provision function against the prior migration — no pre-existing feature dropped.

## References
- ADR-004 rev 2 §1 (schema), §Q6 / S1 (actor_type column)
- ADR-007 follow-up TO RESOLVE-14
- Memory: provision-function restatement footgun
- Parent: `20260510-162158-aa6f`
