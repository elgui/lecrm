# Connection Pooling Strategy

## Problem

Schema-per-tenant with per-workspace Postgres roles creates two scaling ceilings:

1. **Connection count**: Each `RunWorkspaceJob` call opens a fresh `pgx.Connect()`
   — no pooling. With `CONNECTION LIMIT 10` per workspace role and ~150
   max_connections on a 12GB VPS, the ceiling hits at ~15 tenants.

2. **Prepared-statement cache**: Each schema prefix generates distinct query
   fingerprints. pgx's prepared-statement cache grows N×T (unique queries ×
   tenant count). At 50 tenants this is 50× the memory of a single-schema design.

## Solution: Two-Layer Pooling

```
                     ┌─────────────────────────────┐
                     │  API (pgx v5, SimpleProtocol)│
                     └─────────┬───────────────────┘
                               │
            ┌──────────────────┼──────────────────────┐
            │                  │                      │
   Control-plane pool    TenantPool (Go)         [optional]
   (pgxpool, 10 conns)   ≤20 sub-pools           PgBouncer
   core.* tables          ≤3 conns each           :6432
            │              LRU eviction               │
            └──────────────────┼──────────────────────┘
                               │
                     ┌─────────┴───────────────────┐
                     │     PostgreSQL 17 :5432      │
                     │     max_connections ≈ 150    │
                     └─────────────────────────────┘
```

### Layer 1: Go-level TenantPool (always active)

`apps/api/internal/db/tenant_pool.go`

- Maintains up to **MaxPools** (default 20) pgxpool.Pool instances
- Each sub-pool has **MaxConnsPerPool** (default 3) connections
- LRU eviction when pool count exceeds limit
- All pools use `QueryExecModeSimpleProtocol` — zero prepared statements
- Total connection budget: 10 (control) + 60 (20×3 tenant) = **70 max**

### Layer 2: PgBouncer (production, ≥10 tenants)

`deploy/compose/pgbouncer.yml`

- `pool_mode = transaction` — connections returned after each transaction
- `server_reset_query = DEALLOCATE ALL` — clears any prepared statements
- `MAX_DB_CONNECTIONS = 50` — hard cap on Postgres-side connections
- API connects to PgBouncer (:6432) instead of Postgres (:5432) directly

## Connection Budget (12GB VPS)

| Component | Connections | Notes |
|-----------|------------|-------|
| Control-plane pool | 10 | core.* tables, pgxpool |
| TenantPool (20 active of N tenants) | 60 | 20 pools × 3 conns, LRU eviction |
| Migrations / admin | 5 | lecrm-migrate, ad-hoc |
| **Total** | **75** | Well within 150 max_connections |

At 50 tenants: only 20 pools are active at any time. The 30 least-recently-used
tenant pools are evicted (closed). When a request arrives for an evicted tenant,
a new pool is created and the new LRU victim is evicted.

## SimpleProtocol (pgx v5)

Both `db.Open` and `TenantPool` set:

```go
cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
```

This sends queries as simple text protocol instead of the extended protocol
with prepared statements. Effect:

- **Zero** prepared-statement cache — memory is constant regardless of tenant count
- Compatible with PgBouncer transaction mode (no PARSE/BIND/EXECUTE sequence)
- Minor performance cost (text vs. binary encoding) — negligible for our query patterns

## Deployment

### Without PgBouncer (≤10 tenants)

No changes needed. TenantPool handles connection bounding in Go.

```
LECRM_DATABASE_URL=postgres://lecrm_api:***@localhost:5432/lecrm
```

### With PgBouncer (≥10 tenants)

```bash
docker compose -f deploy/compose/postgres.yml \
               -f deploy/compose/pgbouncer.yml up -d
```

Point the API at PgBouncer:

```
LECRM_DATABASE_URL=postgres://lecrm_api:***@localhost:6432/lecrm
```

## Health Checks

- `db.HealthCheck(ctx, pool)` — verifies control-plane pool with SELECT 1
- `db.TenantPoolHealthy(tp)` — verifies pool count is within budget
- PgBouncer healthcheck in compose: TCP probe on :6432

## Migration from RunWorkspaceJob

`RunWorkspaceJob` (in `apps/api/internal/jobs/`) opens a raw `pgx.Connect()`
per invocation — no pooling. For new code, prefer `TenantPool.RunInWorkspace()`:

```go
// Before (unbounded connections):
result, err := jobs.RunWorkspaceJob(ctx, resolver, creds, wsID, fn)

// After (bounded, pooled):
err := tenantPool.RunInWorkspace(ctx, wsID, func(ctx context.Context, conn *pgxpool.Conn) error {
    // use conn for queries
    return nil
})
```

`RunWorkspaceJob` is retained for backward compatibility during migration.
