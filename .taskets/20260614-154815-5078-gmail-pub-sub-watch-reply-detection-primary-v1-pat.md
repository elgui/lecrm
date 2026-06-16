---
id: 20260614-154815-5078
title: Gmail Pub/Sub Watch reply detection (PRIMARY v1 path)
status: later
priority: p2
created: 2026-06-14
updated: 2026-06-14
tags: [sequences, v1, gmail, reply-detection]
category: project
group: lecrm-v1-build
group_order: 300
order: 5
plan: true
---

# Gmail Pub/Sub Watch reply detection (PRIMARY v1 path)

## Why
ADR-004 rev 2 §4: reply detection at v1 is Gmail-only (ADR-009 §9 Gmail-first; Brevo inbound DEFERRED per ADR-003 Addendum A2026-06-14). The rep sends from their own mailbox, so Gmail Pub/Sub Watch covers ~95% of SMB sequences. This is THE v1 reply-detection surface — there is no Brevo `inbound.go` at v1.

## Steps
1. Per-workspace-user Google OAuth grant — scopes `gmail.readonly` + `gmail.send` (verify whether `gmail.modify` is actually needed — S3; smaller scope eases OAuth review, tasket bf09). Refresh tokens SOPS-age encrypted at `secrets/oauth/gmail/<workspace_id>/<user_id>.yaml` (ADR-007).
2. `users.watch()` → Pub/Sub topic `projects/lecrm-prod/topics/gmail-inbox-events`; single push subscription → `https://api.lecrm.fr/v1/webhooks/gmail/push`.
3. Push handler: validate the Google-signed JWT; resolve `email_address → workspace+user`; enqueue `sequences.poll_reply`.
4. `poll_reply` worker: `users.history.list(historyId=last_history_id)`; for each new INBOX message extract `In-Reply-To` / `References`; match `enrollment_steps.rfc_message_id` (indexed); on match call `Transition(waiting_reply → reply_received | ooo_detected)` per the OOO classifier (order:6).
5. Daily renewal river job `gmail.watch_renew` (`0 4 * * *`) — Gmail watch expires every 7 days; renew daily for margin.
6. Persist `last_history_id` per connection; handle history-gap (full re-sync) safely.

## Done when
- A reply to a sent step is detected and transitions the enrollment within the reply window.
- Watch auto-renews daily; expiry does not drop detection.
- OAuth refresh tokens are SOPS-encrypted at the documented path.

## References
- ADR-004 rev 2 §4 (Gmail path), S3 (scope minimisation)
- ADR-009 §9 (Gmail-first); ADR-003 Addendum A2026-06-14 (Brevo inbound deferred)
- tasket 1023 (per-workspace OAuth secret), tasket bf09 (OAuth review)
- Depends on order:3, order:4
