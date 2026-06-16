---
id: 20260614-154815-5078
title: Gmail Pub/Sub reply detection — human setup checkpoint (OAuth + GCP)
status: now
priority: p2
created: 2026-06-14
updated: 2026-06-15
tags: [sequences, v1, gmail, reply-detection, human-gate]
category: project
group: lecrm-v1-build
group_order: 300
order: 5
plan: true
role: epic
gate: human
gate_reason: Gmail OAuth grant + GCP Pub/Sub topic/push subscription must be set up by a human
---

# Gmail Pub/Sub reply detection — human setup checkpoint (OAuth + GCP)

> **Model-A split (2026-06-14).** This tasket was a hybrid (human OAuth/GCP setup *then* handler
> code). It is now a **pure human checkpoint** (`gate: human` + `role: epic`): the automator spawns
> no worker and a "Run All" holds here, with no timeout, until a human does the external setup
> below and continues. The handler/worker code moved to the dependent tasket **order:6**
> (`20260614-154815-5b07`, `depends_on: [20260614-154815-5078]`).

## Why
ADR-004 rev 2 §4: reply detection at v1 is Gmail-only (ADR-009 §9 Gmail-first; Brevo inbound DEFERRED per ADR-003 Addendum A2026-06-14). The rep sends from their own mailbox, so Gmail Pub/Sub Watch covers ~95% of SMB sequences. This is THE v1 reply-detection surface — there is no Brevo `inbound.go` at v1. The handler code (push endpoint, `poll_reply` worker, daily renewal) cannot function until the OAuth grant and the Pub/Sub topic/subscription exist — so that external setup is a human gate that **precedes** the code tasket.

## Human setup (do these, then continue the gate)
1. Per-workspace-user Google OAuth grant — scopes `gmail.readonly` + `gmail.send` (verify whether `gmail.modify` is actually needed — S3; smaller scope eases OAuth review, tasket bf09). Refresh tokens SOPS-age encrypted at `secrets/oauth/gmail/<workspace_id>/<user_id>.yaml` (ADR-007).
2. `users.watch()` → Pub/Sub topic `projects/lecrm-prod/topics/gmail-inbox-events`; single push subscription → `https://api.lecrm.fr/v1/webhooks/gmail/push`.

## Done when (the human continues)
- A connected workspace user has granted OAuth, with refresh tokens SOPS-encrypted at the documented path.
- The Pub/Sub topic + push subscription exist and point at the webhook endpoint.
- → **Continue the gate** from the dashboard (or `POST /api/automation/{run_id}/continue-gate`); the dependent handler-code tasket (order:6) then runs.

## References
- ADR-004 rev 2 §4 (Gmail path), S3 (scope minimisation)
- ADR-009 §9 (Gmail-first); ADR-003 Addendum A2026-06-14 (Brevo inbound deferred)
- tasket 1023 (per-workspace OAuth secret), tasket bf09 (OAuth review)
- Handler code: order:6 `20260614-154815-5b07` (Gmail push handler + `poll_reply` worker), depends on this checkpoint
- Depends on order:3, order:4
