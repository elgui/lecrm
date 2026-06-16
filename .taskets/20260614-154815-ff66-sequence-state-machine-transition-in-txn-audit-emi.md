---
id: 20260614-154815-ff66
title: Sequence state machine + Transition() + in-txn audit emission
status: done
priority: p2
created: 2026-06-14
updated: 2026-06-15
tags: [sequences, v1, state-machine, audit]
category: project
group: lecrm-v1-build
group_order: 300
order: 4
plan: true
done: 2026-06-15
---

# Sequence state machine + Transition() + in-txn audit emission

## Why
ADR-004 rev 2 §2 + §6: all state changes flow through one Go function `sequences.Transition(ctx, tx, enrollmentID, to, reason)` that locks the row, validates the transition, updates side-effect columns, emits the audit row in the SAME transaction (ADR-009 §7.2 fail-closed), and enqueues the next job. This is the heart of the engine.

## Steps
1. Encode the state set + in-code transition table: `enrolled → step_sent → waiting_reply → reply_received / ooo_detected / failed`, plus terminal `bounced / unsubscribed / suppressed / completed`.
2. Implement `Transition`: `SELECT … FOR UPDATE` the enrollment; validate `from→to`; update `state`, `last_transition_at`, side-effect cols (`reply_message_id`, `ooo_returns_at`, `next_action_at`); emit one `audit_log` row in `tx`; enqueue the next river job when implied.
3. Invalid-transition handling: panic in dev/test; in prod return 500 and emit `sequences.transition.invalid` (retention class auth/1y).
4. Emit the §6 event catalogue (`sequences.enrolled/step_sent/reply_received/ooo_detected/failed/bounced/unsubscribed`) with the right fields + `actor_type` (`human_api` UI, `mcp_agent` agent, `internal_service` system).
5. Tests: valid-path transitions; invalid-transition panic; audit-row-in-same-tx (a rollback drops both the state change and the audit row).

## Done when
- All transitions go through `Transition`; no direct `UPDATE enrollments SET state`.
- Every transition emits exactly one in-txn `audit_log` row with the correct `actor_type`.
- Invalid transitions panic (dev) / 500 + audit (prod).

## References
- ADR-004 rev 2 §2 (state machine), §6 (audit emission)
- ADR-009 §7.2 (fail-closed audit), §4.1 (actor_type claim)
- ADR-007 §3 (audit catalogue)
- Depends on order:2, order:3
