---
id: 20260525-1001-custom-properties-crud-wired
title: "Custom properties CRUD wired to Contact + Deal"
status: done
priority: p0
created: 2026-05-25
updated: 2026-05-28
done: 2026-05-25
category: project
group: crm-entity-foundation
group_order: 100
order: 2
plan: true
tags: [crm, metadata, custom-properties, jsonb, sprint-5]
---

# Custom properties CRUD wired to Contact + Deal

## Pre-flight: Verify Previous Tasket

Before starting, verify Tasket 1 ("Contact + Company + Deal domain models") completed:

1. `ls packages/db/queries/contacts.sql` -- contacts query file exists
2. `cd apps/api && go build ./...` -- compiles with generated sqlc code
3. `git log --oneline -10 | grep -i "contact\|company\|deal\|entity"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

The metadata engine (ADR-010) exists — `objects` and `custom_property_definitions` tables are provisioned per workspace. The `Get`, `Set`, and `Find` functions exist in `apps/api/internal/metadata/`. But no REST endpoints expose them, and the `property_type` CHECK constraint doesn't yet include `json` (needed for ADR-011 chatboting connector payloads like `scoring_breakdown`).

This is Sprint 5 work (feature 6: custom properties on Contact + Deal).

Source of truth: `docs/sprint-plan.md` Sprint 5, `docs/adr/ADR-010-metadata-engine.md`
Working directory: `/home/gui/Projects/leCRM`

## Approach

Wire the existing metadata engine to REST endpoints. Add `json` property type. Build the JSONB regression test suite (non-negotiable test category (c) per test strategy).

## Steps

1. Add `json` to the `property_type` CHECK constraint:
   - New migration: `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... CHECK (property_type IN ('string','number','boolean','enum','date','json'))`
   - Update provisioning function to include `json` in CHECK for new workspaces
   - Update `apps/api/internal/metadata/` validation logic to handle `json` type:
     - Accept valid JSON objects/arrays
     - If `allowed_values` contains a JSON Schema, validate against it
     - Fail-closed: invalid JSON → 400

2. Implement custom property definition management endpoints:
   - `GET /v1/metadata/definitions?parent_type=contact` — list definitions for an entity type
   - `POST /v1/metadata/definitions` — create a new definition (parent_type, property_key, property_type, allowed_values, required)
   - `DELETE /v1/metadata/definitions/:id` — remove a definition (cascade: remove matching data from objects?)

3. Implement custom property CRUD on entities:
   - `GET /v1/contacts/:id/properties` — calls `metadata.Get(ctx, "contact", id)`
   - `PUT /v1/contacts/:id/properties` — calls `metadata.Set(ctx, "contact", id, data)`
   - `GET /v1/deals/:id/properties` — same for deals
   - `PUT /v1/deals/:id/properties` — same for deals

4. Ensure all custom property writes are audit-logged (fail-closed):
   - `metadata.Set` already wraps write + audit in a single transaction (verified by `fail_closed_test.go`)
   - Verify the event type is `metadata.property.upsert` per ADR-007

5. JSONB regression test suite (minimum 8 tests per test-strategy §4.3):
   - Test: concurrent Set on same parent_id → last-write-wins, no corruption
   - Test: Set with property_key not in definitions → 400
   - Test: Set with wrong property_type (number value for string def) → 400
   - Test: Set with enum value not in allowed_values → 400
   - Test: Set with `json` type → valid JSON accepted, invalid rejected
   - Test: Find with GIN @> containment → correct results
   - Test: Find across 100+ objects → GIN index used (EXPLAIN check)
   - Test: Delete definition → subsequent Set with that key → 400
   - Test: Cross-tenant isolation → workspace A's properties invisible from workspace B

6. Cache `custom_property_definitions` per workspace (the council flagged re-querying on every Set):
   - In-process LRU cache with 5-minute TTL
   - Invalidate on definition CREATE/DELETE
   - Bounded: max 50 entries (≤30 properties per workspace × small workspace count)

## Done When

- [ ] `json` property type accepted in custom_property_definitions
- [ ] REST endpoints for definition management work (GET/POST/DELETE)
- [ ] REST endpoints for property CRUD on contacts and deals work
- [ ] All property writes are audit-logged (fail-closed test passes)
- [ ] JSONB regression suite has ≥8 tests, all passing
- [ ] Definition cache reduces DB round-trips on Set (benchmark or log)
- [ ] `golangci-lint` clean

## Completion Verification

1. `grep -c "'json'" packages/db/migrations/` -- json type in migration
2. `grep -c 'definitions' apps/api/internal/http/` -- definition endpoints registered
3. `cd apps/api && go test -race -count=1 ./internal/metadata/...` -- ≥8 tests pass
4. `cd apps/api && go test -race -count=1 ./...` -- all tests pass
5. Commit: `feat(metadata): wire custom properties CRUD to Contact + Deal, add json type (Sprint 5)`

## References

- `apps/api/internal/metadata/` — existing Get/Set/Find functions
- `apps/api/internal/metadata/fail_closed_test.go` — existing fail-closed test
- `docs/adr/ADR-010-metadata-engine.md` §4a — `json` property type amendment
- `docs/adr/ADR-011-chatboting-connector-boundary.md` — connector payloads need `json` type
- `docs/test-strategy.md` §4.3 — non-negotiable JSONB regression category
