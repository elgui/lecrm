---
id: 20260525-1003-entity-rest-handlers-crud
title: "Contact + Company + Deal REST handlers with CRUD"
status: done
priority: p0
created: 2026-05-25
updated: 2026-05-28
done: 2026-05-28
category: project
group: crm-crud-complete
group_order: 60
order: 1
plan: true
tags: [crm, rest, handlers, audit, sprint-6-7]
---

# Contact + Company + Deal REST handlers with CRUD

## Shipped Status (verified 2026-05-28)

The base CRUD surface was delivered by PR#5 — do NOT rewrite it. Residual
scope: idempotency-key support and an explicit audit-log fail-closed layer.

**Already in `main`** (commits `1d2ec9a1` / `03646d2a` / `c0af3972`):

- 15 endpoints live in `apps/api/internal/crm/handlers.go` (5 per entity,
  registered via `Handler.RegisterRoutes` at line 34).
- Cursor pagination — `encodeCursor` / `decodeCursor` (line ~225) using
  opaque base64(JSON({id, created_at})).
- sqlc bindings — `packages/db/queries/{contacts,companies,deals}.sql`
  generated into `apps/api/internal/sqlcgen/`.
- Workspace-scoped DB access via `Handler.ws()` (line 269) + `readTx` /
  write tx helpers; cross-tenant isolation enforced by per-workspace
  Postgres role + `SET LOCAL search_path`.
- Frontend wiring + types — `apps/web/src/hooks/use-deals.ts` etc.

Verify the shipped surface:

```bash
export PATH=$PATH:/usr/local/go/bin
grep -cE "r\.(Get|Post|Put|Delete)\(" apps/api/internal/crm/handlers.go   # expect ≥15
test -f packages/db/queries/contacts.sql && \
  test -f packages/db/queries/companies.sql && \
  test -f packages/db/queries/deals.sql
(cd apps/api && go build ./... && go test ./internal/crm/...)
```

**Residual scope** (what this tasket should actually deliver):

1. `Idempotency-Key` header on `POST` routes — no `idempotency_keys`
   table exists; `grep -rn idempotency_keys apps/api/ packages/db/` is empty.
2. Fail-closed audit-log transactions on every mutation — there is no
   `apps/api/internal/audit/` package; current handlers don't emit
   `contact.created` / `contact.updated` / `contact.deleted` events.
3. Soft-delete decision — current `deleteRow` (handlers.go:452) is a
   hard `DELETE`. Decide soft vs. hard; document the choice; if soft,
   add `deleted_at` columns + filter on read paths.
4. *Note:* `PATCH /v1/deals/:id/stage` is **out of scope** here —
   covered by tasket `1005` (Pipeline Kanban).

If the verification block above passes, start at "Residual scope" item 1.
Otherwise STOP and report which check failed.

## Context

Entity tables and sqlc queries exist from the previous group. This tasket implements the full REST API surface: 5 endpoints per entity (list, get, create, update, delete) with audit logging, idempotency, and cursor pagination.

Sprint 6-7 work per `docs/sprint-plan.md`.

Source of truth: `docs/sprint-plan.md` Sprint 6-7
Working directory: `/home/gui/Projects/leCRM`

## Approach

Follow the existing handler patterns in `apps/api/internal/http/server.go` (Chi router assembly). Each entity gets its own handler file. All mutations are audit-logged via the fail-closed pattern from `apps/api/internal/metadata/`.

## Steps

1. Create handler files:
   - `apps/api/internal/http/contacts.go` — ContactHandler struct + route registration
   - `apps/api/internal/http/companies.go` — CompanyHandler
   - `apps/api/internal/http/deals.go` — DealHandler

2. Implement per entity (contacts as example, same for companies/deals):
   - `GET /v1/contacts` — list, opaque base64 cursor pagination (encode last-seen ID+created_at)
   - `GET /v1/contacts/:id` — get by UUID, 404 if not found
   - `POST /v1/contacts` — create, validate required fields, `Idempotency-Key` header support
   - `PUT /v1/contacts/:id` — full update, 404 if not found, validate
   - `DELETE /v1/contacts/:id` — soft or hard delete (decide: soft with deleted_at, or hard)

3. Idempotency-Key handling (ADR-009 §4):
   - Table: `idempotency_keys` (key TEXT, workspace_id UUID, response_status INT, response_body JSONB, created_at, expires_at)
   - On POST: if key exists and not expired, return cached response
   - On POST: if key is new, execute + store result
   - TTL: 24h, cleanup via River job

4. Cursor pagination:
   - Encode: base64(json({id, created_at})) as opaque cursor
   - Decode: extract last-seen values, use in WHERE clause
   - Default page size: 50, max 200
   - Response: `{data: [...], next_cursor: "...", has_more: bool}`

5. Audit logging on all mutations:
   - Wrap mutation + audit in a single transaction
   - Event types: `contact.created`, `contact.updated`, `contact.deleted` (same for companies/deals)
   - Actor from session context (user_id + actor_type)
   - Payload: diff of changed fields (old vs new for updates)
   - Fail-closed: if audit write fails, mutation rolls back

6. Deal-specific: stage association
   - `POST /v1/deals` accepts `stage_id` (defaults to first pipeline stage)
   - `PATCH /v1/deals/:id/stage` — dedicated stage-transition endpoint (creates activity)

7. Register routes in `apps/api/internal/http/server.go`:
   - Mount under workspace-scoped route group (after workspace middleware)
   - All handlers use workspace-scoped DB connection

8. Write integration tests:
   - Test: CRUD lifecycle for each entity (create → read → update → delete → 404)
   - Test: cursor pagination with 100+ records
   - Test: Idempotency-Key replay returns cached response
   - Test: cross-tenant isolation (workspace A's contacts invisible from B)
   - Test: audit log entry created on every mutation

## Done When

- [ ] 5 REST endpoints per entity (list, get, create, update, delete) working
- [ ] All mutations audit-logged (fail-closed)
- [ ] Idempotency-Key header prevents duplicate creates
- [ ] Cursor pagination works with opaque cursors
- [ ] Cross-tenant isolation verified (integration test)
- [ ] Deal stage transitions create activity log entries
- [ ] `golangci-lint` clean
- [ ] All tests pass

## Completion Verification

1. `ls apps/api/internal/http/contacts.go apps/api/internal/http/companies.go apps/api/internal/http/deals.go` -- handlers exist
2. `cd apps/api && go test -race -count=1 ./internal/http/...` -- handler tests pass
3. `cd apps/api && go test -race -count=1 ./...` -- all tests pass
4. Commit: `feat(api): Contact, Company, Deal REST handlers with audit + pagination (Sprint 6-7)`

## References

- `apps/api/internal/http/server.go` — Chi router assembly
- `apps/api/internal/metadata/fail_closed_test.go` — fail-closed audit pattern
- `apps/api/internal/workspace/middleware.go` — workspace-scoped connection
- `docs/sprint-plan.md` Sprint 7 — REST handlers, audit, idempotency
- ADR-009 §4 — Idempotency-Key, cursor pagination
