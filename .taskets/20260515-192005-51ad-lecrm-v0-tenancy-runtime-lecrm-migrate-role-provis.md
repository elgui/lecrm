---
id: 20260515-192005-51ad
title: "leCRM v0 — Tenancy runtime: lecrm-migrate + role provisioning + river worker pattern (Sprint 3)"
status: done
priority: p1
created: 2026-05-15
updated: 2026-05-18
done: 2026-05-18
tags: [sprint-3, tenancy, database, river, lecrm-migrate]
category: engineering
group: lecrm-v0-sprint-3
group_order: 3
order: 3
plan: true
---

## Read this cold — full context inline

Sprint 3 Database/Tenancy track. Wires the SECURITY DEFINER provisioning function (ADR-009 §2.1, already implemented in `packages/db/migrations/0001_init.sql` per commit `611baca`) into a runnable Compose pre-deploy job, and scaffolds the `river` worker pattern that acquires workspace-scoped Postgres connections before any data operation.

## Why this exists

ADR-009 §8.2 names three binaries: `cmd/lecrm-api` (runtime, app role, no DDL), `cmd/lecrm-mcp` (MCP adapter, constrained role), `cmd/lecrm-migrate` (DDL-capable, runs as `lecrm_provisioner`). Pre-deploy ordering: `migrate exits 0` → `api starts`. Currently `apps/migrate/` exists but its `cmd/` runner does not wrap the provisioning function end-to-end.

ADR-009 §8.3 binds river to workspace-scoped operation: tables in per-tenant `river_<workspace_base36>` schema, workers acquire a workspace-scoped Postgres connection (via `SET ROLE` or by authenticating as that role) BEFORE any data operation. Without this scaffold, the river worker pattern leaks across tenants — a v0 hard fail.

## Prerequisite (DOR)

- Wk-2 scaffolding live (commit `f69d24a`: apps/api with workspace middleware + sqlcgen substrate).
- SECURITY DEFINER provisioning function exists in `packages/db/migrations/0001_init.sql` per commit `611baca`. Function: `core.lecrm_provision_workspace(p_workspace_id UUID) → role_name TEXT`.
- Secrets baseline live: `lecrm_provisioner` credential in SOPS secret manifest per tasket `20260510-162158-1023`.
- Cross-tenant isolation test fixture (sibling tasket, order=5 in this group) lands in parallel — it consumes the endpoints this tasket produces.

## Approach

### A. `cmd/lecrm-migrate` Compose pre-deploy job
1. Implement `apps/migrate/cmd/lecrm-migrate/main.go` with two operations:
   - `apply`: runs Atlas migrations (versioned SQL in `packages/db/migrations/`) against the `core` schema. Honors `parallel + on_error = CONTINUE` per ADR-009 §2.4.
   - `provision-workspace --slug=<slug>`: calls `core.lecrm_provision_workspace(<uuid>)` and writes the returned role + slug back to `core.workspaces`. Idempotent on re-invocation.
2. Run as `lecrm_provisioner` role (Tier-0 secret per ADR-007). Connection string from `LECRM_PROVISIONER_DSN` env.
3. Compose pre-deploy job in `deploy/compose/`: ordering depends on api service (`depends_on: { migrate: { condition: service_completed_successfully } }`).

### B. End-to-end provisioning test
1. testcontainers-go spins up Postgres 17 with `0001_init.sql` applied.
2. Call `cmd/lecrm-migrate provision-workspace --slug=acme` → assert:
   - `core.workspaces` row exists with `slug = 'acme'`, `role_name = 'workspace_<base36-uuid>'`.
   - Postgres role exists with `LOGIN`, `CONNECTION LIMIT 10`, `search_path` set per ADR-009 §2.1.
   - Workspace schema exists with role as `AUTHORIZATION`.
   - `river_<base36-uuid>` schema exists with the same `AUTHORIZATION`.
   - Re-invocation succeeds without error (idempotency).

### C. River worker scaffold
1. `apps/api/internal/jobs/` package with a generic `RunWorkspaceJob[T any]` helper:
   - Takes a `workspace_id` and a job func.
   - Connects to Postgres using the workspace's role (resolved via `core.workspaces.role_name` + credentials lookup from secrets).
   - Verifies `current_setting('search_path')` is the workspace's schema (defense-in-depth) before invoking the job func.
2. Wire one no-op probe job (`probe_workspace_connectivity`) that confirms the pattern end-to-end.

## Done When

- [ ] `apps/migrate/cmd/lecrm-migrate/main.go` implements both `apply` and `provision-workspace` operations
- [ ] End-to-end testcontainers test passes for fresh + idempotent re-invocation paths
- [ ] `apps/api/internal/jobs/` workspace-scoped worker pattern implemented + probe job runs against a provisioned workspace
- [ ] Per-tenant `river_<workspace_base36>` schema populated by provisioning + visible to the probe job
- [ ] Compose service ordering documented in `deploy/compose/`

## References

- ADR-009 §2.1 (SECURITY DEFINER function), §8.2 (three binaries), §8.3 (river tenant-scoping)
- `packages/db/migrations/0001_init.sql` (function to wrap)
- Tasket `20260510-162158-1023` (secrets baseline; provides `lecrm_provisioner` DSN)
- Sibling Sprint 3 tasket (order=5) — cross-tenant isolation test fixture (consumes the endpoints this produces)
- Commit `f69d24a` (Wk-2 ramp checkpoint scaffolding)
