---
id: 20260524-1405-pgbouncer-connection-pool-strategy
title: "PgBouncer + connection pool strategy"
status: done
priority: p3
created: 2026-05-24
updated: 2026-05-25
done: 2026-05-25
category: project
group: council-architecture-hardening
group_order: 40
order: 6
plan: true
tags: [infrastructure, postgresql, performance, scaling]
---

# PgBouncer + connection pool strategy

## Pre-flight: Verify Previous Tasket

Before starting, verify Tasket 5 ("Session revocation mechanism") completed:

1. `grep -c 'revoke\|revocation' apps/api/internal/auth/handlers.go` -- revocation endpoints present
2. `cd apps/api && go test -race -count=1 ./internal/auth/...` -- tests pass
3. `git log --oneline -10 | grep -i "revocation\|revoke"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

The council identified two related scaling concerns for 10+ tenants:

1. **Connection ceiling:** Current design implicitly assumes session-mode connections. On a 12GB RAM VPS, PostgreSQL's max_connections is realistically 100-150. With schema-per-tenant and 10 connections per workspace role (CONNECTION LIMIT 10 in provisioning), 15 tenants = 150 connections = ceiling hit.

2. **Prepared-statement cache bloat:** Per-schema SQL generates distinct query fingerprints (each schema prefix is a different query text). pgx's prepared-statement cache grows N×T where N = unique queries and T = tenant count. At 50 tenants this is 50x the memory of a single-schema design.

**Council disagreement:** Ava (Researcher) says PgBouncer in transaction mode solves this. Marcus (Engineer) says PgBouncer transaction mode is incompatible with prepared statements and that DEALLOCATE ALL is needed. Both are partially right — pgx v5 documents the pattern with `pool_mode=transaction` + `server_reset_query=DEALLOCATE ALL`.

**Timeline:** Not urgent at 3-5 tenants. Blocking at 10+. Should be implemented before the 10th client onboards.

Source of truth: `docs/council-architecture-review-2026-05-24.md`
Working directory: `/home/gui/Projects/leCRM`

## Approach

Add PgBouncer between the API application and PostgreSQL:

```
API (pgx v5) → PgBouncer (transaction mode) → PostgreSQL 17
```

Key configuration:
- `pool_mode = transaction` — connections returned to pool after each transaction
- `server_reset_query = DEALLOCATE ALL` — clears prepared statements between transactions
- pgx config: disable implicit prepared statements (`PreferSimpleProtocol: true` or explicit `DEALLOCATE`)
- Per-workspace connections: PgBouncer pool per database user (workspace role), or single pool with SET ROLE

**Alternative approach (if PgBouncer adds too much complexity):**
- search_path middleware: `SET search_path = <workspace_schema>, public` at the start of each request
- Single database role for the API (not per-workspace role for reads)
- Keep per-workspace roles for writes only (via RunWorkspaceJob pattern)

## Steps

1. Research and decide: PgBouncer vs search_path middleware
   - Read pgx v5 docs on PgBouncer compatibility
   - Evaluate: does the existing `RunWorkspaceJob` pattern work with PgBouncer?
   - Decision criteria: if PgBouncer config takes >4h, use search_path middleware
2. If PgBouncer route:
   - Add `deploy/compose/pgbouncer.yml` with PgBouncer 1.22+
   - Configure: `pool_mode=transaction`, `server_reset_query=DEALLOCATE ALL`
   - Configure per-user pools (one per workspace role) or auth_query for dynamic users
   - Update `apps/api/internal/db/db.go`: connect to PgBouncer port, not Postgres directly
   - Update pgx pool config: `ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol`
   - Health check: verify PgBouncer is healthy before API starts
3. If search_path middleware route:
   - Create `apps/api/internal/db/tenant_conn.go`:
     - Acquire connection from pool
     - `SET search_path = <workspace_schema>, public`
     - Execute query
     - Reset search_path (or return connection — pool reset handles it)
   - This eliminates per-schema query fingerprints (all queries use the same text)
4. Load test with simulated 50 tenants:
   - Verify connection count stays within bounds
   - Verify prepared-statement memory doesn't grow linearly with tenant count
   - Verify no cross-tenant data leakage under concurrent load
5. Update documentation with the chosen approach

## Done When

- [ ] Connection count is bounded regardless of tenant count (not N×10 per tenant)
- [ ] Prepared-statement cache memory doesn't grow linearly with tenant count
- [ ] No cross-tenant data leakage under concurrent load (verified by test)
- [ ] API still works with RunWorkspaceJob pattern (per-workspace role for writes)
- [ ] Health checks verify pooler health before API accepts traffic
- [ ] Load test passes with 50 simulated tenants on a 12GB VPS memory budget
- [ ] Existing integration tests pass

## Completion Verification

1. `ls deploy/compose/pgbouncer.yml || grep -c 'search_path' apps/api/internal/db/` -- one approach implemented
2. `cd apps/api && go test -race -count=1 ./...` -- all tests pass
3. Commit: `feat(infra): add connection pooling strategy for multi-tenant scale`

## References

- `apps/api/internal/db/db.go` — current pool configuration (MaxConns=10, MaxLifetime=1h)
- `apps/api/internal/jobs/` — RunWorkspaceJob per-tenant connection pattern
- `packages/db/migrations/0001_init.sql` — workspace role CONNECTION LIMIT 10
- `docs/council-architecture-review-2026-05-24.md` — council review
- pgx v5 PgBouncer docs: PreferSimpleProtocol, QueryExecModeSimpleProtocol
- PgBouncer docs: server_reset_query, pool_mode
