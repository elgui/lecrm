---
id: 20260530-150314-be6c
title: Auto-grant on provision + lecrm-admin integrator command
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
tags: [lecrm, integrator, rbac, multi-tenant, auth]
category: project
group: lecrm-integrator-switching
order: 2
plan: true
---

# Auto-grant on provision + `lecrm-admin integrator` command

## Pre-flight: Verify Previous Tasket
Before starting, verify Tasket 1 ("Integrator role + grants data model") completed:
1. `ls packages/db/migrations/ | grep integrator` -- migration exists
2. `psql ... -c "\d core.integrator_grants"` (or the project's DB shell) -- table exists with `(workspace_id, lower(email))` uniqueness
3. `cd apps/api && go test ./internal/rbac/... -count=1` -- integrator RBAC tests pass
4. `git log --oneline -10 | grep -i integrator` -- Tasket 1 commit exists

**If any check fails, STOP and report. Do not proceed.**

## Context
Slice 2 of integrator workspace-switching. Tasket 1 created `core.integrator_grants`. This tasket writes grants:
- **Auto-grant on provision:** when a tenant is created with `--owner-email` (Léo's email, stored as `creator_email`), also insert an `integrator_grant` for that workspace, so the tenant is switch-able the moment it's provisioned — *before* Léo has ever logged into it.
- **Explicit grant CLI:** a `lecrm-admin integrator grant|revoke|list` command for tenants Léo did not provision.

Working directory: `/home/gui/Projects/leCRM`.

## Approach
- `apps/admin/internal/tenant/create.go`: the provision paths (`createFresh`/`createUpsert` via `callWrapper`, and `createForceRecreate` which already holds a `pgx.Tx`). After the workspace row is provisioned and the workspace UUID is known, if `creatorEmail != ""`, `INSERT INTO core.integrator_grants (workspace_id, email, granted_by) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`. For `createForceRecreate` do it inside the existing tx (same atomicity guarantee); for the conn paths, exec on the same `conn` right after `callWrapper`.
- New subcommand wired in `apps/admin/cmd/lecrm-admin/main.go` (urfave/cli), backed by a new `apps/admin/internal/integrator/` package (or extend `internal/tenant`): `grant --slug --email`, `revoke --slug --email`, `list [--slug]`. Resolve `--slug` → `workspace_id` via `core.workspaces`. `granted_by` = `LECRM_OPERATOR_EMAIL` env (same convention the audit path uses) or a flag.
- Keep error-surface style consistent with existing `tenant` StructErr kinds.

## Steps
1. Add an `insertIntegratorGrant(ctx, q Querier, workspaceID uuid.UUID, email, grantedBy string)` helper (works with both `*pgx.Conn` and `pgx.Tx`).
2. Call it from all three provision paths when `creatorEmail` is non-empty. Add a stdout line e.g. `[PROVISION] integrator grant: leo@vernayo.com`.
3. Implement `integrator grant|revoke|list`:
   - `grant`: validate slug, resolve workspace_id, upsert grant.
   - `revoke`: `DELETE FROM core.integrator_grants WHERE workspace_id=$1 AND lower(email)=lower($2)`.
   - `list`: print grants (optionally filtered by `--slug`), joined to `core.workspaces.slug`.
4. Update `docs/integrator-handoff.md`: document the auto-grant behavior and the new `gb-tenant integrator …` commands (the alias is `gb-tenant`).
5. Tests: a Postgres-backed test (testcontainers / the existing admin test harness) asserting provision-with-owner-email writes a grant, and grant/revoke/list round-trip.

## Done When
- [ ] `lecrm-admin tenant create --slug X --owner-email leo@vernayo.com` writes a matching `core.integrator_grants` row in the same transaction as provisioning.
- [ ] `lecrm-admin integrator grant|revoke|list` work end-to-end against local/staging Postgres.
- [ ] `docs/integrator-handoff.md` documents both.
- [ ] `go vet ./...` + `golangci-lint run` clean; admin tests green.

## Completion Verification
1. `cd apps/admin && go test ./... -count=1` -- tests pass
2. Manual: provision a throwaway slug with `--owner-email`, then `lecrm-admin integrator list --slug <slug>` shows the grant
3. `grep -n "integrator" docs/integrator-handoff.md` -- docs updated
4. `cd apps/admin && go vet ./... && golangci-lint run` -- clean
5. Commit: `feat(admin): auto-grant integrator on provision + integrator grant/revoke/list`

## References
- `apps/admin/internal/tenant/create.go` — provision paths (createFresh/createUpsert/createForceRecreate)
- `apps/admin/cmd/lecrm-admin/main.go` — CLI command wiring
- `docs/integrator-handoff.md` — operator guide to update
- Tasket 1 (this group) — `core.integrator_grants` schema
