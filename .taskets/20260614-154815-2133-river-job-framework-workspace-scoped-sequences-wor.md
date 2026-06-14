---
id: 20260614-154815-2133
title: river job framework + workspace-scoped sequences workers
status: later
priority: p2
created: 2026-06-14
updated: 2026-06-14
tags: [sequences, v1, river, jobs]
category: project
group: lecrm-v1-build
group_order: 300
order: 3
plan: true
---

# river job framework + workspace-scoped sequences workers

## Why
ADR-004 rev 2 §3 defines four tenant-scoped river job types that drive the sequences state machine. Jobs live in the per-workspace `river_<workspace_base36>` schema (ADR-009 §8.3); a worker acquires a workspace-scoped pgxpool before executing. This tasket stands up the job framework and the worker transaction contract that §4–§8 build on.

## Steps
1. Define the four job arg structs (IDs only, no PII): `sequences.enroll {contact_id, sequence_id}`, `sequences.send_step {enrollment_id, step_index, idempotency_key}`, `sequences.poll_reply {enrollment_id}`, `sequences.finalize {enrollment_id, terminal_state, reason}`.
2. Register river workers with per-type retry policies (enroll 3, send_step 5→`failed`, poll_reply 3, finalize 3) and `UniqueOpts{ByArgs: true}` (finalize also by-state).
3. Implement the worker entry contract `ctx, tx, release, err := workspaceCtx.AcquireTx(ctx, args.WorkspaceID); defer release()` mirroring `apps/api/internal/jobs/workspace.go`; all DB + audit writes go through `tx`.
4. Implement the `send_step` idempotency key `sha256(workspace_id:enrollment_id:step_index:attempt_epoch)`; `attempt_epoch` increments only on explicit supersede, NOT on river-internal retries (the partial unique index from order:2 guarantees one active row).
5. Unit-test the key formula across edge cases (`sequences/idempotency_test.go`).

## Done when
- Four job types register and run against a per-workspace river schema.
- Worker AcquireTx contract enforced; a mis-routed job cannot touch another tenant's schema (Postgres role barrier).
- Idempotency-key unit test passes.

## References
- ADR-004 rev 2 §3 (jobs + idempotency)
- ADR-009 §8.3 (river tenancy)
- `apps/api/internal/jobs/workspace.go` (AcquireTx pattern)
- Depends on order:2 (schema)
