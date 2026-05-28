---
id: 20260524-1406-river-advisory-locks-schema-switch-safety
title: "River advisory locks for schema-switch safety"
status: done
priority: p3
created: 2026-05-24
updated: 2026-05-25
done: 2026-05-25
category: project
group: council-architecture-hardening
group_order: 40
order: 7
plan: true
tags: [background-jobs, river, multi-tenant, reliability]
---

# River advisory locks for schema-switch safety

## Pre-flight: Verify Previous Tasket

Before starting, verify Tasket 6 ("PgBouncer + connection pool strategy") completed:

1. `ls deploy/compose/pgbouncer.yml || grep -c 'search_path' apps/api/internal/db/` -- pooling approach present
2. `cd apps/api && go test -race -count=1 ./...` -- all tests pass
3. `git log --oneline -10 | grep -i "pool\|pgbouncer\|connection"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

Marcus (Engineer) identified that River background jobs interacting with schema-per-tenant have an unaddressed failure mode: a job that fails mid-execution after switching search_path leaves the connection in an ambiguous state. The current test suite doesn't cover this because the happy path always completes cleanly.

Additionally, the council recommended wrapping River behind an interface (Serena: "40 lines today buys infinite optionality"). If River proves unstable at scale, the interface enables a swap to Temporal or another job system without rewriting business logic.

**Timeline:** Edge case under concurrent load, not v0 critical. Should be validated before 10+ clients with concurrent background jobs.

Source of truth: `docs/council-architecture-review-2026-05-24.md`
Working directory: `/home/gui/Projects/leCRM`

## Approach

Three deliverables:

1. **Advisory locks around schema transitions:** Before switching search_path in a job worker, acquire a Postgres advisory lock keyed on workspace_id. This prevents concurrent jobs for the same workspace from racing on the connection's search_path state.

2. **Connection state cleanup on job failure:** Use `defer` to reset search_path (or close/discard the connection) if a job panics or errors mid-execution. Ensure no connection is returned to the pool with a stale search_path.

3. **Job runner interface:** Define a `JobRunner` interface that River implements. Business logic depends on the interface, not on River directly. If River needs to be replaced, only the adapter changes.

## Steps

1. Create `apps/api/internal/jobs/runner.go`:
   ```go
   type JobRunner interface {
       Enqueue(ctx context.Context, job Job) error
       Start(ctx context.Context) error
       Stop(ctx context.Context) error
   }
   
   type Job interface {
       Kind() string
       WorkspaceID() uuid.UUID
   }
   ```
2. Create `apps/api/internal/jobs/river_adapter.go`:
   - Implements JobRunner using River
   - Wraps River's client with the advisory lock pattern
3. Update `apps/api/internal/jobs/workspace_job.go`:
   - Before setting search_path: `SELECT pg_advisory_lock(hashtext($1))` with workspace_id as key
   - After job completion (success or failure): `SELECT pg_advisory_unlock(hashtext($1))`
   - Use `defer` for the unlock to handle panics
   - On connection error during unlock: discard the connection (don't return to pool)
4. Add connection state verification after job completion:
   - Query `SHOW search_path` after each job
   - If search_path is not the expected default, log a warning and reset it
   - This is defense-in-depth, not the primary mechanism
5. Write tests for failure scenarios:
   - Test: job panics mid-execution → connection search_path is reset
   - Test: job errors after setting search_path → advisory lock released
   - Test: concurrent jobs for same workspace → serialized (not racing)
   - Test: concurrent jobs for different workspaces → parallel (not blocked)
6. Load test with 50 concurrent tenants:
   - Enqueue 100 jobs across 50 workspaces simultaneously
   - Verify no cross-tenant data access
   - Verify no connection pool exhaustion
   - Verify advisory locks are released (no lock leak)
7. Document the job runner interface and its guarantees

## Done When

- [ ] Advisory locks prevent concurrent schema-switch races per workspace
- [ ] Failed/panicked jobs always release their advisory lock (defer pattern)
- [ ] Connection search_path is always reset after job completion (success or failure)
- [ ] No connection is returned to pool with stale search_path
- [ ] `JobRunner` interface defined — business logic depends on interface, not River
- [ ] `RiverAdapter` implements `JobRunner`
- [ ] Test: panic mid-job → lock released, connection discarded
- [ ] Test: concurrent same-workspace jobs → serialized
- [ ] Test: concurrent different-workspace jobs → parallel
- [ ] Load test: 100 jobs across 50 workspaces completes without lock leaks

## Completion Verification

1. `ls apps/api/internal/jobs/runner.go` -- interface exists
2. `ls apps/api/internal/jobs/river_adapter.go` -- adapter exists
3. `grep -c 'pg_advisory_lock' apps/api/internal/jobs/workspace_job.go` -- advisory locks present
4. `cd apps/api && go test -race -count=1 ./internal/jobs/...` -- all job tests pass
5. Commit: `feat(jobs): add advisory locks and JobRunner interface for schema-switch safety`

## References

- `apps/api/internal/jobs/` — current workspace job pattern
- `apps/api/internal/db/db.go` — connection pool
- `docs/council-architecture-review-2026-05-24.md` — council review
- River docs: github.com/riverqueue/river
- PostgreSQL advisory locks: pg_advisory_lock/pg_advisory_unlock
