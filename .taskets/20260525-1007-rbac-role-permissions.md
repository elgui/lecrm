---
id: 20260525-1007-rbac-role-permissions
title: "Multi-user RBAC with role-based permissions"
status: done
priority: p1
created: 2026-05-25
updated: 2026-05-28
done: 2026-05-28
category: project
group: crm-frontend-rbac-export
group_order: 200
order: 1
plan: true
tags: [auth, rbac, permissions, sprint-8]
---

# Multi-user RBAC with role-based permissions

## Shipped Status (verified 2026-05-28)

The DB foundation and member-bootstrap helper exist; the authorization
layer, member-management endpoints, and frontend gating do not.

**Already in `main`:**

- `core.workspace_members` table with `role` column accepting
  `'owner' | 'admin' | 'member'` — `packages/db/migrations/0002_identity.sql:35`.
- `auth.Store.EnsureMember` — `apps/api/internal/auth/store.go:65` —
  inserts a row at default role `'member'` on first workspace touch.

Verify the shipped surface:

```bash
grep -q "CREATE TABLE IF NOT EXISTS core.workspace_members" \
  packages/db/migrations/0002_identity.sql
grep -q "EnsureMember" apps/api/internal/auth/store.go
```

**Residual scope** (what this tasket should actually deliver):

1. `RequireRole(minRole)` middleware injecting role into request context.
2. Member management endpoints — `GET/POST/PATCH/DELETE /v1/workspace/members*`
   and `GET /v1/workspace/me` (current-user role + permissions).
3. Apply RBAC to existing CRM routes (read = `member+`, write = `admin+`,
   member-mgmt = `owner` only). Routes are mounted via
   `crm.Handler.RegisterRoutes` in `apps/api/internal/crm/handlers.go:34`
   — wrap them in `r.Group` with the new middleware.
4. RBAC regression suite — 15+ tests across role × endpoint matrix; reuse
   `apps/api/internal/testfixtures/tenantpair/` for two-workspace setup.
5. Frontend gating — hide unauthorized controls; `/settings/members`
   route for owners only.

## Pre-flight (run before residual work)

```bash
export PATH=$PATH:/usr/local/go/bin
test -f apps/api/internal/crm/handlers.go && \
  grep -q "RegisterRoutes" apps/api/internal/crm/handlers.go
test -f apps/api/internal/auth/store.go && \
  grep -q "EnsureMember" apps/api/internal/auth/store.go
(cd apps/api && go build ./... && go test ./...)
```

**If any check fails, STOP and report. Do not proceed.**

## Context

Feature 7 of v0: multi-user with role-based permissions. The `core.workspace_members` table already exists (from 0002_identity.sql) with a `role` column accepting `'owner'`, `'admin'`, `'member'`. This tasket implements the authorization middleware and member management endpoints.

Sprint 8 work per `docs/sprint-plan.md`.

Source of truth: `docs/sprint-plan.md` Sprint 8
Working directory: `/home/gui/Projects/leCRM`

## Steps

1. RBAC middleware:
   - After auth middleware resolves user, look up their role in `workspace_members`
   - Inject role into request context
   - Create `RequireRole(minRole)` middleware factory:
     - `member`: can read all entities
     - `admin`: can read + create + update + delete entities
     - `owner`: admin + manage workspace members + manage service tokens + delete workspace
   - Apply to route groups: read routes = member+, write routes = admin+, member mgmt = owner

2. Member management endpoints:
   - `GET /v1/workspace/members` — list members with roles
   - `POST /v1/workspace/members/invite` — invite by email (creates pending membership + sends invite email placeholder)
   - `PATCH /v1/workspace/members/:id/role` — change role (owner only, can't demote self)
   - `DELETE /v1/workspace/members/:id` — remove member (owner only, can't remove self)

3. Self-service:
   - `GET /v1/workspace/me` — current user's role and permissions
   - Useful for frontend to conditionally show/hide admin controls

4. RBAC regression test suite (non-negotiable category (b) per test-strategy):
   - Test matrix: 3 roles × every protected endpoint
   - Test: member can GET /v1/contacts but not POST
   - Test: admin can POST /v1/contacts
   - Test: member can't DELETE /v1/workspace/members/:id
   - Test: owner can change roles
   - Test: owner can't demote themselves
   - Test: owner can't remove themselves
   - Test: service token actor_type respects RBAC (connector token scoped to write-only)
   - Minimum: 15+ RBAC tests covering the full matrix

5. Frontend updates:
   - Conditionally show/hide edit/delete buttons based on role
   - Member management page under /settings/members (owner only)
   - Invite form + pending invitations list

## Done When

- [ ] RBAC middleware enforces role hierarchy (member < admin < owner)
- [ ] Member management CRUD endpoints work
- [ ] Owner can invite, change roles, remove members
- [ ] Members can't perform admin or owner actions
- [ ] RBAC regression suite has 15+ tests covering the role × endpoint matrix
- [ ] Frontend hides unauthorized controls based on role
- [ ] All tests pass

## Completion Verification

1. `grep -c 'RequireRole\|rbac' apps/api/internal/http/` -- RBAC middleware present
2. `cd apps/api && go test -race -count=1 ./...` -- all tests pass including RBAC suite
3. Commit: `feat(auth): multi-user RBAC with role-based permissions (Sprint 8)`

## References

- `packages/db/migrations/0002_identity.sql` — workspace_members table with role column
- `apps/api/internal/workspace/middleware.go` — workspace context (inject role here)
- `docs/test-strategy.md` — non-negotiable test category (b) RBAC
- `docs/sprint-plan.md` Sprint 8
