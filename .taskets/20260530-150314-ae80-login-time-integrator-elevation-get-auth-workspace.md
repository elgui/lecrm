---
id: 20260530-150314-ae80
title: Login-time integrator elevation + GET /auth/workspaces endpoint
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
tags: [lecrm, integrator, rbac, multi-tenant, auth]
category: project
group: lecrm-integrator-switching
order: 3
plan: true
---

# Login-time integrator elevation + `GET /auth/workspaces` endpoint

## Pre-flight: Verify Previous Tasket
Before starting, verify Tasket 2 ("Auto-grant on provision + integrator command") completed:
1. `cd apps/admin && go test ./... -count=1` -- admin tests pass
2. Manual/staging: an `integrator_grants` row exists for a provisioned tenant (`lecrm-admin integrator list`)
3. `git log --oneline -15 | grep -i "integrator"` -- Tasket 2 commit exists

**If any check fails, STOP and report. Do not proceed.**

## Context
Slice 3 of integrator workspace-switching — the API behavior that turns a pending grant into a live integrator session and lets the frontend enumerate a user's switch-able workspaces.

Two pieces:
1. **Login elevation:** at `/auth/callback`, after `UpsertUser`, if `core.integrator_grants` has a row for `(this workspace, lower(email))`, ensure membership as `'integrator'` instead of `'member'` — and **never downgrade** an existing higher role.
2. **`GET /auth/workspaces`:** a session-scoped sibling of `/auth/me` returning the UNION of the user's `core.workspace_members` (by `user_id`) and `core.integrator_grants` (by `lower(email)`), joined to `core.workspaces` (`tombstoned_at IS NULL`). This is what makes a freshly-provisioned, never-logged-into tenant appear in Léo's switcher immediately.

Working directory: `/home/gui/Projects/leCRM`.

## Security invariants (do not violate)
- `/auth/workspaces` returns **only the authenticated user's own** access — no slug enumeration, no cross-user data. It reads the `core` schema via the auth Store's pool (same path as `UpsertUser`/`EnsureMember`), NOT a workspace role connection.
- The ADR-009 §5.2 cookie boundary is unchanged — this endpoint does not mint cross-workspace sessions; it just lists targets the frontend will full-navigate to (each gets its own scoped cookie after SSO).

## Approach
- `apps/api/internal/auth/store.go`:
  - Add `IntegratorGrantExists(ctx, workspaceID uuid.UUID, email string) (bool, error)` (match on `lower(email)`).
  - Generalize membership write: add `EnsureMemberWithRole(ctx, workspaceID, userID, role)` that inserts the role and on conflict **only upgrades** (keeps the max of existing/new in the hierarchy — do the max in Go via `rbac.ParseRole`, or `DO UPDATE ... WHERE` guarding against downgrade). Keep `EnsureMember` as a thin wrapper passing `member`.
  - Add `ListAccessibleWorkspaces(ctx, userID uuid.UUID, email string) ([]AccessibleWorkspace, error)` — `SELECT slug, role` from members-by-user UNION grants-by-lower(email), joined to `core.workspaces` where `tombstoned_at IS NULL`; dedup by workspace preferring the membership role if present else `'integrator'`.
- `apps/api/internal/auth/handlers.go`:
  - In `Callback`, compute the join role: `role := "member"; if integrator-grant exists { role = "integrator" }`, then `EnsureMemberWithRole(...)`.
  - Add `Workspaces` handler for `GET /auth/workspaces`: decode session (same as `/auth/me`), call `ListAccessibleWorkspaces`, return `[]{slug, role, url}` where `url` is built from the configured Domain TLD (reuse the cookie-scope/DomainTLD config so the frontend doesn't have to guess) — e.g. `https://<slug>.<tld>/`. Register the route next to `/auth/me`.
- Audit + member-list hygiene:
  - Tag integrator-actor writes in `core.audit_log` (set `actor_type`/role to reflect integrator where the audit path records the principal — locate where audit rows are written with the rbac Principal).
  - Exclude `role = 'integrator'` rows from `GET /v1/workspace/members` listing (so Léo isn't counted as a client seat / shown to the client). Find the members list handler/store (`apps/api/internal/members`).

## Steps
1. Store methods: `IntegratorGrantExists`, `EnsureMemberWithRole` (+ keep `EnsureMember`), `ListAccessibleWorkspaces` (+ `AccessibleWorkspace` type).
2. Wire elevation into `Callback`; confirm no-downgrade behavior with a test.
3. Add `GET /auth/workspaces` handler + route registration; build URLs from DomainTLD config.
4. Exclude integrator rows from the members listing; tag integrator actions in audit.
5. Tests using the existing 2-tenant cross-tenant fixture:
   - integrator-granted user logging into workspace A lands as `integrator`; the same user is NOT auto-elevated in workspace B where no grant exists.
   - `/auth/workspaces` returns A (and any other granted-but-unvisited tenant) for that user, and nothing belonging to other users.
   - members listing for a workspace omits integrator rows.

## Done When
- [ ] Integrator login lands as `integrator`; existing higher roles never downgraded.
- [ ] `GET /auth/workspaces` returns the correct union including freshly-provisioned (never-logged-into) tenants, scoped to the caller only.
- [ ] Integrator excluded from `GET /v1/workspace/members`; integrator actions tagged in audit.
- [ ] Cross-tenant isolation test green; `go vet` + `golangci-lint` clean.

## Completion Verification
1. `cd apps/api && go test ./internal/auth/... ./internal/members/... -count=1` -- pass
2. `cd apps/api && go test ./... -run CrossTenant -count=1` -- isolation fixture green
3. Manual against staging: log in as Léo on a granted demo subdomain → `/auth/me` shows role integrator; `GET /auth/workspaces` lists his tenants
4. `cd apps/api && go vet ./... && golangci-lint run` -- clean
5. Commit: `feat(api,auth): integrator login elevation + GET /auth/workspaces`

## References
- `apps/api/internal/auth/store.go` — UpsertUser/EnsureMember/GetUserProfile to extend
- `apps/api/internal/auth/handlers.go` — Callback + /auth/me (sibling pattern for /auth/workspaces)
- `apps/api/internal/auth/cookie.go` — DomainTLD/CookieScope config to build switch URLs
- `apps/api/internal/members/` — members listing to filter
- `apps/api/internal/rbac/role.go` — ParseRole for no-downgrade max
- `docs/adr/ADR-009-stack-and-license.md` §5.2 — cookie boundary (unchanged)
