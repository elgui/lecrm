---
id: 20260524-1400-slug-tombstoning-reserved-blocklist
title: "Slug tombstoning + reserved blocklist"
status: done
priority: p0
created: 2026-05-24
updated: 2026-05-25
done: 2026-05-25
category: project
group: council-architecture-hardening
group_order: 40
order: 1
plan: true
tags: [security, multi-tenant, wildcard-tls, phishing-prevention]
---

# Slug tombstoning + reserved blocklist

## Context

The PAI Council architecture review (2026-05-24) identified slug recycling as the ONLY security gap with no retroactive fix. When a tenant is deleted or churns, their subdomain (e.g., `acme.lecrm.fr`) must never be re-registered by another party. The wildcard TLS cert (`*.lecrm.fr`) means a recycled slug immediately gets a valid padlock in the browser — making credential phishing against former tenant employees trivially convincing.

**Council consensus:** All 4 members (Architect, Engineer, Security, Researcher) agreed this is the single highest-priority fix. Ship this week.

Source of truth: `docs/council-architecture-review-2026-05-24.md`
Branch: `main`
Working directory: `/home/gui/Projects/leCRM`

## Approach

Two changes: one migration + one middleware modification.

**Migration (0005):**
- Add `tombstoned_at TIMESTAMPTZ DEFAULT NULL` column to `core.workspaces`
- Create `core.reserved_slugs` table (slug TEXT PRIMARY KEY, reason TEXT, created_at TIMESTAMPTZ)
- Add unique partial index: `CREATE UNIQUE INDEX idx_workspaces_slug_active ON core.workspaces (slug) WHERE tombstoned_at IS NULL`
- Seed reserved_slugs with common dangerous values: `admin`, `api`, `auth`, `www`, `mail`, `smtp`, `ftp`, `ns1`, `ns2`, `localhost`, `test`, `staging`, `prod`, `app`

**Workspace middleware:**
- In `apps/api/internal/workspace/resolver.go`: after resolving slug, check `tombstoned_at IS NULL` in the query
- If slug is tombstoned or reserved, return HTTP 404 (not 403 — don't leak that the workspace existed)

**Admin CLI:**
- In `apps/admin/internal/tenant/`: add a `tombstone` subcommand that sets `tombstoned_at = NOW()` instead of deleting
- Modify `create.go` to check both `core.workspaces` (including tombstoned) and `core.reserved_slugs` before provisioning

## Steps

1. Create `packages/db/migrations/0005_slug_tombstoning.sql`:
   - `ALTER TABLE core.workspaces ADD COLUMN tombstoned_at TIMESTAMPTZ DEFAULT NULL`
   - `CREATE TABLE core.reserved_slugs (slug TEXT PRIMARY KEY, reason TEXT NOT NULL, created_at TIMESTAMPTZ DEFAULT NOW())`
   - Unique partial index on active slugs
   - Seed blocklist
2. Update `packages/db/queries/workspaces.sql` — modify `GetWorkspaceBySlug` to add `AND tombstoned_at IS NULL`
3. Regenerate sqlc: `cd apps/api && sqlc generate`
4. Update `apps/api/internal/workspace/resolver.go` to use the updated query
5. Add `TombstoneWorkspace` and `IsSlugAvailable` queries
6. Add `tenant tombstone --slug <slug>` subcommand to admin CLI
7. Update `tenant create` to reject tombstoned/reserved slugs with clear error message
8. Add test: attempt to resolve a tombstoned workspace slug → expect 404
9. Add test: attempt to create a workspace with a reserved slug → expect error
10. Run existing test suite to confirm no regressions

## Done When

- [ ] Migration 0005 applies cleanly on fresh DB and on top of existing 0004
- [ ] `GetWorkspaceBySlug` excludes tombstoned workspaces
- [ ] Reserved slugs (admin, api, auth, www, etc.) cannot be provisioned
- [ ] Tombstoned slugs cannot be re-provisioned
- [ ] `lecrm-admin tenant tombstone --slug X` sets tombstoned_at without deleting data
- [ ] Workspace middleware returns 404 for tombstoned/reserved slugs
- [ ] Existing integration tests pass (no regression)
- [ ] New tests cover tombstoning and reserved slug scenarios

## Completion Verification

1. `ls packages/db/migrations/0005_slug_tombstoning.sql` -- migration exists
2. `grep -c 'tombstoned_at' packages/db/queries/workspaces.sql` -- query updated
3. `grep -c 'reserved_slugs' packages/db/migrations/0005_slug_tombstoning.sql` -- table created
4. `cd apps/api && go test -race -count=1 ./internal/workspace/...` -- tests pass
5. `cd apps/admin && go test -race -count=1 ./internal/tenant/...` -- tests pass
6. Commit: `feat(security): add slug tombstoning and reserved blocklist`

## References

- `docs/council-architecture-review-2026-05-24.md` — council review
- `packages/db/migrations/0004_workspaces_admin_email_registry.sql` — current latest migration
- `apps/api/internal/workspace/resolver.go` — workspace resolution
- `apps/admin/internal/tenant/create.go` — tenant creation
- RFC 9700 (OAuth 2.0 Security BCP) — subdomain takeover warnings
