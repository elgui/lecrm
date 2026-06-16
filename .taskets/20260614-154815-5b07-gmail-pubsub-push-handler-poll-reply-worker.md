---
id: 20260614-154815-5b07
title: Gmail Pub/Sub push handler + poll_reply worker (reply detection code)
status: done
priority: p2
created: 2026-06-14
updated: 2026-06-15
done: 2026-06-15
tags: [sequences, v1, gmail, reply-detection]
category: project
group: lecrm-v1-build
group_order: 300
order: 6
plan: true
depends_on: [20260614-154815-5078]
---

# Gmail Pub/Sub push handler + poll_reply worker (reply detection code)

> **Model-A split (2026-06-14).** The handler/worker code for Gmail reply detection, split out
> from the order:5 hybrid (`20260614-154815-5078`). It `depends_on` that **human setup
> checkpoint** — a "Run All" only dispatches this worker *after* the human has done the OAuth
> grant + GCP Pub/Sub topic/subscription and continued the gate. See the checkpoint tasket for
> the external setup steps and ADR references.

## Why
This is the automatable half of Gmail-only v1 reply detection (ADR-004 rev 2 §4). The OAuth grant and the Pub/Sub topic/push subscription are created by a human at order:5; with those in place, this tasket builds the code that turns inbound Gmail events into enrollment transitions.

## Steps
1. **Push handler** (`POST /v1/webhooks/gmail/push`): validate the Google-signed JWT; resolve `email_address → workspace+user`; enqueue `sequences.poll_reply`.
2. **`poll_reply` worker**: `users.history.list(historyId=last_history_id)`; for each new INBOX message extract `In-Reply-To` / `References`; match `enrollment_steps.rfc_message_id` (indexed); on match call `Transition(waiting_reply → reply_received | ooo_detected)` per the OOO classifier (order:7).
3. **Daily renewal** river job `gmail.watch_renew` (`0 4 * * *`) — Gmail watch expires every 7 days; renew daily for margin.
4. **Persist `last_history_id`** per connection; handle history-gap (full re-sync) safely.

## Done when
- A reply to a sent step is detected and transitions the enrollment within the reply window.
- Watch auto-renews daily; expiry does not drop detection.
- The push handler rejects requests without a valid Google-signed JWT.

## References
- Human setup checkpoint: order:5 `20260614-154815-5078` (OAuth grant + Pub/Sub topic/subscription) — **must be continued before this runs**
- ADR-004 rev 2 §4 (Gmail path); ADR-007 (OAuth secret storage)
- OOO classifier: order:7 `20260614-154815-a81e` (the `Transition()` target on match)
