---
id: 20260510-162158-aa6f
title: leCRM v1 — Native sequences engine (Track F, post-first-client)
status: later
priority: p2
created: 2026-05-10
updated: 2026-05-14
tags: [sequences, email, deliverability, v1]
category: project
group: lecrm-v1-build
group_order: 20
order: 1
plan: true
---

# leCRM v1 — Native sequences engine (post-first-paying-client)

## Why this tasket exists

v0 ships transactional email only (tasket 499c). v1 (post-first-client revenue, target 2026 Q4) adds the native sequences engine: enrollment, multi-step send, reply detection, OOO classification, complaint suppression. Reply detection is the deliverability moat — clients with active sequences need to know within 5 minutes when a prospect replies.

Re-scoped 2026-05-10 against the locked Go + Postgres + river stack (ADR-009). Original NestJS + BullMQ plan is superseded.

## Re-scoped done criteria

- [ ] Brevo inbound parse webhook → `apps/api/internal/email/brevo/inbound.go` → river job → sequences state machine.
- [ ] Gmail Pub/Sub Watch subscription per CRM user (per-workspace OAuth secret in SOPS manifest, see tasket 1023).
- [ ] Microsoft Graph subscriptions for Outlook users (v1.1 — deferred behind Gmail-first per ADR-009 §9).
- [ ] IMAP IDLE fallback (v1.1).
- [ ] river-backed sequence state machine with states: `ENROLLED → STEP_SENT → WAITING_REPLY → REPLY_RECEIVED / OOO_DETECTED / FAILED`. Idempotency keyed on `(enrollment_id, step_index)` via the Brandur-style partial unique index per ADR-009 §2.4.
- [ ] OOO classifier (rules-based at v1; v1.1 ML upgrade if signal warrants).
- [ ] Pre-flight content scoring via GlockApps API; score <7/10 blocks activation with admin override.
- [ ] All sequence state transitions emit audit-log events with `actor_type` claim from ADR-007 §3.

## Open dependencies

- Brevo plan tier confirmed (Starter → Standard → Business per phase). Inbound parse webhook may have plan-tier gating; sales-email response (tasket 499c) clarifies.
- ADR-004 (sequences architecture) needs re-issue against the Go + river stack — the original NestJS + BullMQ plan is superseded but ADR-004's *architectural intent* (state-machine over a durable queue, reply correlation on `messageId`) survives.

## References

- [ADR-004](docs/adr/ADR-004-sequences-architecture.md) (sequences architecture — substantive intent survives; implementation surface re-scoped to Go + river).
- [ADR-003](docs/adr/ADR-003-email-provider-brevo.md) (Brevo).
- [ADR-009](docs/adr/ADR-009-stack-and-license.md) §2.4 (idempotency), §8.3 (river job tenancy).
