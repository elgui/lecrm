---
id: 20260528-142535-c8a3
title: Re-issue ADR-004 (sequences architecture) for Go + river stack
status: later
priority: p2
created: 2026-05-28
updated: 2026-05-28
tags: [adr, sequences, v1-readiness, doc]
category: project
group: lecrm-v1-readiness
group_order: 80
order: 1
plan: true
---

## Why

ADR-004 (`docs/adr/ADR-004-sequences-architecture.md`) was written against the NestJS + BullMQ stack. ADR-009 superseded the runtime to Go + river. The architectural *intent* of ADR-004 (durable state machine, reply correlation on `messageId`) survives, but the implementation surface needs to be re-issued before v1 sequences code starts.

## Steps

1. Read the current ADR-004 cover-to-cover. Tag what survives vs. what is superseded.
2. Read ADR-009 (stack/license), §2.4 (idempotency: Brandur-style partial unique index on `(enrollment_id, step_index)`), §8.3 (river job tenancy), §9 (Gmail-first deferral of Outlook).
3. Read ADR-011 (chatboting connector boundary) — sequences will produce connector-attributed activities; the actor_type/source_system fields must match.
4. Draft replacement ADR-004 rev. 2 covering:
   - `enrollments` and `enrollment_steps` table shape (per-workspace schema).
   - river job types: `sequences.enroll`, `sequences.send_step`, `sequences.poll_reply`, `sequences.finalize`.
   - State machine: `ENROLLED → STEP_SENT → WAITING_REPLY → REPLY_RECEIVED | OOO_DETECTED | FAILED`.
   - Idempotency key shape and the partial unique index DDL.
   - Brevo inbound parse webhook → `apps/api/internal/email/brevo/inbound.go` → river enqueue.
   - Gmail Pub/Sub Watch fan-in (per-workspace OAuth secret in SOPS).
   - Audit-log emission per state transition (ties to `actor_type` from ADR-007 §3).
5. Open-questions section for: OOO classifier baseline (rules vs ML), GlockApps preflight integration point, suppression-list propagation.
6. Mark the old ADR-004 as `Status: Superseded by ADR-004-rev2 (2026-XX-XX)` with a forward pointer.

## Done when

- `docs/adr/ADR-004-sequences-architecture.md` (or `-rev2.md`) reflects the Go + river runtime end-to-end.
- The old ADR has a `Superseded by` line at the top.
- The new ADR references the locked stack (ADR-009) and the connector boundary (ADR-011) concretely.

## Verification

```bash
grep -q "river" docs/adr/ADR-004*.md
grep -q "Superseded by\|superseded" docs/adr/ADR-004-sequences-architecture.md
grep -q "enrollment_id, step_index" docs/adr/ADR-004*.md
```

## References

- `docs/adr/ADR-004-sequences-architecture.md` (current — to be superseded)
- `docs/adr/ADR-009-stack-and-license.md` §2.4, §8.3, §9
- `docs/adr/ADR-011-chatboting-connector-boundary.md`
- `docs/adr/ADR-003-email-provider-brevo.md` (Brevo plan-tier context)
- Existing v1 tasket: `.taskets/20260510-162158-aa6f-lecrm-v1-native-sequences-engine-track-f-post-firs.md`
