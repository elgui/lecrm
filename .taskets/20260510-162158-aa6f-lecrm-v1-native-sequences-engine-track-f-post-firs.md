---
id: 20260510-162158-aa6f
title: leCRM v1 — Native sequences engine (Track F, post-first-client)
status: now
priority: p2
created: 2026-05-10
updated: 2026-06-15
tags: [sequences, email, deliverability, v1]
category: project
group: lecrm-v1-build
group_order: 300
order: 1
plan: true
role: epic
unparked: 2026-06-14 — v1-readiness gates closed (kickoff tasket 20260528-142652-8580)
---

# leCRM v1 — Native sequences engine (post-first-paying-client)

## Why this tasket exists

v0 ships transactional email only (tasket 499c). v1 (post-first-client revenue, target 2026 Q4) adds the native sequences engine: enrollment, multi-step send, reply detection, OOO classification, complaint suppression. Reply detection is the deliverability moat — clients with active sequences need to know within 5 minutes when a prospect replies.

Re-scoped 2026-05-10 against the locked Go + Postgres + river stack (ADR-009). Original NestJS + BullMQ plan is superseded.

**Unparked 2026-06-14** — the "post-first-paying-client" gate was dropped (Guillaume's 2026-06-14 decision to decouple v1 from any first-client milestone); v1 kicks off now that the v1-readiness technical gates are closed. The "post-first-client" wording in the title/heading is historical. See kickoff tasket `20260528-142652-8580`.

## Re-scoped done criteria

**v1 scope is locked to [ADR-004 rev 2](docs/adr/ADR-004-rev2-sequences-architecture.md). Reply detection is Gmail-only at v1 — the Brevo inbound parse catch-all is deferred (Professional-tier-gated ~€500/mo; see [ADR-003 Addendum A2026-06-14](docs/adr/ADR-003-email-provider-brevo.md), Option D).**

- [ ] `enrollments` + `enrollment_steps` DDL appended to the per-workspace provisioning function, with the Brandur-style partial unique index `uniq_enrollment_step_active` on `(enrollment_id, step_index)` (ADR-004 rev 2 §1).
- [ ] Pre-req migration: add `actor_type` column to `audit_log` (`text NOT NULL DEFAULT 'human_api'`) in the same Atlas migration as the sequences tables — must land before the sequences package merges (ADR-004 rev 2 §Q6 / S1; ADR-007 follow-up TO RESOLVE-14).
- [ ] Four river job types — `sequences.enroll`, `sequences.send_step`, `sequences.poll_reply`, `sequences.finalize` — tenant-scoped per ADR-009 §8.3 (ADR-004 rev 2 §3).
- [ ] river-backed sequence state machine: `enrolled → step_sent → waiting_reply → reply_received / ooo_detected / failed` (+ terminal `bounced / unsubscribed / suppressed / completed`). Idempotency keyed on `(enrollment_id, step_index)` via the partial unique index above + river `UniqueOpts{ByArgs}` (ADR-004 rev 2 §2–3).
- [ ] **Reply detection — Gmail Pub/Sub Watch (PRIMARY, v1)**: per connected workspace user, per-workspace OAuth secret in SOPS, `users.watch()` → push subscription → `sequences.poll_reply`, daily `gmail.watch_renew` job (ADR-004 rev 2 §4; tasket 1023).
- [ ] OOO classifier (rules + Haiku at v1; v1.1 ML upgrade if signal warrants) — ADR-004 rev 2 §5.
- [ ] GlockApps pre-flight content scoring; score <7/10 blocks activation with admin override. v1 default is manual (`ops/scripts/glockapps-preflight.sh`); API-automated integration is an open question (ADR-004 rev 2 §Q2).
- [ ] All sequence state transitions emit `sequences.*` audit-log events **in the same transaction** with the `actor_type` claim from ADR-007 §3 (ADR-004 rev 2 §6).

**Deferred (explicitly not v1):**

- [ ] ~~Brevo inbound parse webhook (`apps/api/internal/email/brevo/inbound.go`)~~ — **DEFERRED**. The generic `reply@<client-domain>` catch-all; provider undecided (Postfix self-host €0 / Mailjet Parse ~$17/mo / CloudMailin — costed in ADR-003 Finding 3). Build only when a paying client needs it.
- [ ] Microsoft Graph subscriptions for Outlook users (v1.1 — Gmail-first per ADR-009 §9).
- [ ] IMAP IDLE fallback (v1.1).

## Open dependencies

_All readiness-gate dependencies closed as of 2026-06-14 (v1-readiness plan-group). None remain open:_

- ✅ **Brevo plan tier** — RESOLVED. Inbound parse is Professional-gated (~€500/mo) and not bought; v1 ships Gmail-only reply detection (tasket `2702`, [ADR-003 Addendum A2026-06-14](docs/adr/ADR-003-email-provider-brevo.md)).
- ✅ **ADR-004 re-issue** — DONE. [ADR-004 rev 2](docs/adr/ADR-004-rev2-sequences-architecture.md) re-issued against the Go + river stack (commit `a24f67a8`); rev 1 superseded.

## References

- [ADR-004](docs/adr/ADR-004-sequences-architecture.md) (sequences architecture — substantive intent survives; implementation surface re-scoped to Go + river).
- [ADR-003](docs/adr/ADR-003-email-provider-brevo.md) (Brevo).
- [ADR-009](docs/adr/ADR-009-stack-and-license.md) §2.4 (idempotency), §8.3 (river job tenancy).
