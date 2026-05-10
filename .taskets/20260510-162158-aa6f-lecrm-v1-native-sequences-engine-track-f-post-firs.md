---
id: 20260510-162158-aa6f
title: leCRM v1 — Native sequences engine (Track F, post-first-client)
status: later
priority: p2
created: 2026-05-10
updated: 2026-05-10
tags: [sequences, email, deliverability, v1]
category: project
group: lecrm-v0-build
order: 5
plan: true
---

# leCRM v1 — Native sequences engine (Track F)

## Why this tasket exists
v0 ships with transactional email only. v1 (post-first-client revenue, target 2026 Q4) adds the native sequences engine: enrollment, multi-step send, reply detection, OOO classification, complaint suppression. Reply detection is the deliverability moat — clients with active sequences need to know within 5 minutes when a prospect replies.

Reference: ADR-004 (`docs/adr/ADR-004-sequences-architecture.md`), ARCHITECTURE.md §5.3.

## Done criteria
- [ ] Brevo inbound parse webhook → BullMQ `email-event` queue → sequences state machine.
- [ ] Gmail Pub/Sub Watch subscription per CRM user; Microsoft Graph subscriptions for Outlook users.
- [ ] IMAP IDLE fallback for non-Google/non-Microsoft mailboxes.
- [ ] BullMQ-backed sequence state machine with states: ENROLLED → STEP_SENT → WAITING_REPLY → REPLY_RECEIVED / OOO_DETECTED / FAILED. Idempotency keyed on `enrollment_id × step_index`.
- [ ] OOO classifier (rules-based, then v1.1 ML upgrade): pre-filter before notifying CRM user.
- [ ] Pre-flight content scoring via GlockApps API; score <7/10 blocks activation with admin override.

## Open dependencies
- Brevo plan tier confirmed (Starter → Standard → Business per phase). Inbound parse webhook may have plan-tier gating; sales-email response (Track B) clarifies.
