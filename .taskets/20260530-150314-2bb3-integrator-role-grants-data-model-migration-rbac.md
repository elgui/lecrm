---
id: 20260530-150314-2bb3
title: Integrator role + grants data model (migration + RBAC)
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
tags: [lecrm, integrator, rbac, multi-tenant, auth]
category: project
group: lecrm-integrator-switching
order: 1
plan: true
---

# Integrator role + grants data model (migration + RBAC)

## Context
First slice of the **integrator workspace-switching** feature: let Léo (GB Consult integrator) switch between client workspaces to administrate their accounts. Design locked with Guillaume:
- **Switch mechanism:** re-auth per subdomain via the existing Authentik SSO redirect. The ADR-009 §5.2 per-subdomain session-cookie boundary (`Domain=<slug>.<tld>`, never wildcard) is **untouched**.
- **Integrator identity:** a *distinct, non-billable* `integrator` principal (new role) — owner-equivalent capabilities, hidden from the client member list, actions tagged in `core.audit_log`.
- **Grant flow:** auto-grant on provision via an **email-keyed pending grant**, because at provision time Léo has no `core.users` row yet (that row is created on his first OIDC login, keyed by `(issuer, sub)`). First login materializes the grant into a real `integrator` membership.

This tasket lays the data-model foundation: the new role and the `integrator_grants` table. No login or UI behavior changes yet (those are taskets 3 and 4).

Working directory: `/home/gui/Projects/leCRM`.

## Approach
- New SQL migration under `packages/db/migrations/` (follow the numbering/idempotency style of `0002_identity.sql` and `0004_workspaces_admin_email_registry.sql`).
- Extend `core.workspace_members.role` CHECK from `('owner','admin','member')` to include `'integrator'`.
- Create `core.integrator_grants` — the email-keyed pending-grant table that decouples "who may administrate workspace X" from "has this human logged in yet".
- Extend the Go RBAC role enum in `apps/api/internal/rbac/role.go` with `RoleIntegrator` at the top of the total order, owner-equivalent capabilities.

## Steps
1. Create migration `packages/db/migrations/00NN_integrator_role_and_grants.sql` (pick the next free number), wrapped in `BEGIN;`/`COMMIT;`:
   - Drop & re-add the role CHECK: `ALTER TABLE core.workspace_members DROP CONSTRAINT IF EXISTS workspace_members_role_check; ALTER TABLE core.workspace_members ADD CONSTRAINT workspace_members_role_check CHECK (role IN ('owner','admin','member','integrator'));` (confirm the existing constraint name via `\d core.workspace_members` — it may be auto-named).
   - `CREATE TABLE IF NOT EXISTS core.integrator_grants ( workspace_id UUID NOT NULL REFERENCES core.workspaces(id) ON DELETE CASCADE, email TEXT NOT NULL, granted_by TEXT NOT NULL DEFAULT '', granted_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (workspace_id, lower(email)) );` — note PK on `(workspace_id, lower(email))` requires the expression form; if Postgres rejects a functional PK, use `UNIQUE INDEX ON core.integrator_grants (workspace_id, lower(email))` + a surrogate or `(workspace_id, email)` PK with a citext/lower-index. Verify what applies cleanly.
   - `CREATE INDEX IF NOT EXISTS integrator_grants_email_idx ON core.integrator_grants (lower(email));`
   - `ALTER TABLE core.integrator_grants OWNER TO lecrm_provisioner;`
   - Grant the API role read access: `GRANT SELECT ON core.integrator_grants TO <lecrm-api app role>;` (find the app-role name — search migrations for the `GRANT ... TO` that gives `lecrm-api` access to `core.users`/`core.workspace_members`).
2. `apps/api/internal/rbac/role.go`:
   - Add `RoleIntegrator` as the highest constant (after `RoleOwner`).
   - Add `RoleIntegrator: "integrator"` to `roleNames`.
   - Add the `"integrator"` case to `ParseRole`.
   - Update `PermissionsFor` so `RoleIntegrator` yields owner-equivalent capabilities (CanRead/CanWrite/CanManageMembers/CanManageTokens/CanDeleteWorkspace all true). **Note (reviewable):** integrator currently gets workspace-delete; flag in the PR description for Guillaume.
   - Update the package doc comment's "Capability summary" to mention integrator.
3. Unit tests in `apps/api/internal/rbac/`: `ParseRole("integrator")` → `(RoleIntegrator, true)`; `RoleIntegrator.AtLeast(RoleOwner)` → true; `PermissionsFor(RoleIntegrator)` → all-true bundle; round-trip `RoleIntegrator.String()` == `"integrator"`.

## Done When
- [ ] Migration applies cleanly against a fresh local Postgres (and is idempotent on re-run).
- [ ] `core.workspace_members` accepts `role = 'integrator'`; `core.integrator_grants` exists with the lower(email) uniqueness + index, owned by `lecrm_provisioner`, SELECT granted to the API role.
- [ ] `apps/api/internal/rbac` tests green.
- [ ] `golangci-lint run` and `go vet ./...` clean.

## Completion Verification
1. `ls packages/db/migrations/ | grep integrator` -- migration file exists
2. Apply migrations locally (per the project's migrate runner, e.g. `cmd/lecrm-migrate`) -- no error
3. `cd apps/api && go test ./internal/rbac/... -count=1` -- tests pass
4. `cd apps/api && go vet ./... && golangci-lint run` -- clean
5. Commit: `feat(db,rbac): integrator role + core.integrator_grants table`

## References
- `packages/db/migrations/0002_identity.sql` — workspace_members / users schema + ownership/grant conventions
- `packages/db/migrations/0004_workspaces_admin_email_registry.sql` — creator_email/admin_email columns
- `apps/api/internal/rbac/role.go` — role hierarchy to extend
- `docs/adr/ADR-009-stack-and-license.md` §5.2 — cookie scoping (must remain untouched)
- `docs/adr/ADR-001-tenancy-model.md` — schema-per-tenant
- Prior tasket `20260529-1103-leo-access-smoke-test-handoff` — current manual `INSERT INTO core.workspace_members` workflow this feature automates
