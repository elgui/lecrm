---
id: 20260515-192005-6fe3
title: leCRM v0 — Extend lecrm_provision_workspace for ADR-010 metadata tables (Sprint 4)
status: done
priority: p1
created: 2026-05-15
updated: 2026-05-18
tags: [sprint-4, adr-010, metadata-engine, provisioning]
category: engineering
group: lecrm-v0-sprint-4
group_order: 4
order: 1
plan: true
done: 2026-05-18
---

## Read this cold — full context inline

Extends `core.lecrm_provision_workspace` (ADR-009 §2.1) to create the workspace's `objects` and `custom_property_definitions` tables + indexes per ADR-010 §3-4 alongside the schema and role. ADR-010 §TO RESOLVE-1.

## Why this exists

ADR-010 commits JSONB-primary on a generic `objects` table per workspace schema with a sibling `custom_property_definitions` metadata table for type safety. These tables must exist at workspace provisioning time, not at first-use — first-use creation would (a) create a thundering-herd-on-first-property-create UX bug and (b) leave tenants in an undefined state if the application crashes between provisioning and first metadata write.

ADR-009 §2.1 requires provisioning be a single transaction (the SECURITY DEFINER function). The extension stays inside that same function — no new provisioning step, just additional DDL inside the existing one-transaction guarantee.

## Prerequisite (DOR)

- Sprint 3 Database/Tenancy track `done`: `cmd/lecrm-migrate` end-to-end provisioning works (Sprint 3 tasket order=3).
- ADR-010 committed (done at commit `e875fb8`).
- `packages/db/migrations/0001_init.sql` (current provisioning function) shipped (commit `611baca`).

## Approach

### A. New migration `packages/db/migrations/0003_metadata_engine.sql`
Extends `core.lecrm_provision_workspace` via `CREATE OR REPLACE FUNCTION` — keeps the function body's first ~5 steps unchanged (role + schema + grants + river schema) and appends two new DDL steps:

```sql
-- Step 6: Custom-property metadata table (ADR-010 §4)
EXECUTE format($f$
  CREATE TABLE %I.custom_property_definitions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_type     text NOT NULL CHECK (parent_type IN ('contact', 'deal')),
    property_key    text NOT NULL,
    property_type   text NOT NULL CHECK (property_type IN ('string', 'number', 'boolean', 'enum', 'date')),
    allowed_values  jsonb NULL,
    required        boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (parent_type, property_key)
  )$f$, v_role_name);

-- Step 7: Custom-property storage table (ADR-010 §3)
EXECUTE format($f$
  CREATE TABLE %I.objects (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    object_type   text NOT NULL,
    parent_type   text NULL,
    parent_id     uuid NULL,
    data          jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
  )$f$, v_role_name);

EXECUTE format('CREATE INDEX objects_type_parent_idx ON %I.objects (object_type, parent_type, parent_id)', v_role_name);
EXECUTE format('CREATE INDEX objects_data_gin_idx ON %I.objects USING gin (data)', v_role_name);
```

Idempotency preserved via the existing per-step EXCEPTION blocks (re-invocation catches `duplicate_object` / `42P06`).

### B. End-to-end test (extends Sprint 3 testcontainers test)
1. Provision a workspace via `cmd/lecrm-migrate provision-workspace --slug=acme`.
2. Assert: `acme.objects` exists with both indexes; `acme.custom_property_definitions` exists with unique constraint.
3. Re-provision: idempotent, no error.

## Done When

- [ ] `packages/db/migrations/0003_metadata_engine.sql` committed
- [ ] `core.lecrm_provision_workspace` provisions both tables + indexes
- [ ] Testcontainers end-to-end test asserts table+index presence + idempotency
- [ ] Apply migration to all existing workspaces via Atlas sweep (`parallel + on_error = CONTINUE`)

## References

- ADR-010 §3 (objects schema), §4 (custom_property_definitions schema), §TO RESOLVE-1
- ADR-009 §2.1 (provisioning function), §2.4 (Atlas v1.0 sweep model)
- `packages/db/migrations/0001_init.sql` (function to extend)
- Sprint 3 Database/Tenancy track tasket (provides the testcontainers harness this consumes)
