---
id: 20260525-1000-contact-company-deal-domain-models
title: "Contact + Company + Deal domain models and sqlc queries"
status: done
priority: p0
created: 2026-05-25
updated: 2026-05-28
done: 2026-05-25
category: project
group: crm-entity-foundation
group_order: 100
order: 1
plan: true
tags: [crm, entities, sqlc, migration, sprint-4]
---

# Contact + Company + Deal domain models and sqlc queries

## Context

leCRM's infrastructure is solid (provisioning, auth, workspace isolation, metadata engine, admin CLI, CI) but has zero CRM entity implementation. Every remaining v0 feature — pipeline Kanban, activity log, RBAC, export, Gmail sync, and the chatboting connector (ADR-011) — depends on Contact, Company, and Deal tables existing.

This is Sprint 4 work per `docs/sprint-plan.md`. The three entities are feature 1 of the 8 v0 features.

Source of truth: `docs/sprint-plan.md` Sprint 4
Working directory: `/home/gui/Projects/leCRM`

## Approach

Create the three core CRM entity tables in a new migration, write sqlc query files for CRUD, and extend the provisioning function to create these tables in each workspace schema.

All tables live in `workspace_<uuid>` schema (per ADR-001 schema-per-tenant). No cross-schema foreign keys. `owner_id` references `core.users.id` conceptually but is not a DB FK (cross-schema FK would violate isolation). Application enforces referential integrity.

## Steps

1. Create `packages/db/migrations/0005_crm_entities.sql` (or next available number):
   ```sql
   -- Companies
   CREATE TABLE workspace_<role>.companies (
     id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
     name text NOT NULL,
     domain text,
     industry text,
     size text CHECK (size IN ('1-10','11-50','51-200','201-1000','1000+')),
     owner_id uuid,
     created_at timestamptz NOT NULL DEFAULT now(),
     updated_at timestamptz NOT NULL DEFAULT now()
   );

   -- Contacts
   CREATE TABLE workspace_<role>.contacts (
     id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
     first_name text NOT NULL,
     last_name text NOT NULL,
     email text,
     phone text,
     company_id uuid REFERENCES companies(id) ON DELETE SET NULL,
     owner_id uuid,
     created_at timestamptz NOT NULL DEFAULT now(),
     updated_at timestamptz NOT NULL DEFAULT now()
   );

   -- Deals
   CREATE TABLE workspace_<role>.deals (
     id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
     title text NOT NULL,
     amount numeric(12,2),
     currency char(3) DEFAULT 'EUR',
     stage_id uuid REFERENCES pipeline_stages(id),
     contact_id uuid REFERENCES contacts(id) ON DELETE SET NULL,
     company_id uuid REFERENCES companies(id) ON DELETE SET NULL,
     owner_id uuid,
     expected_close_date date,
     closed_at timestamptz,
     created_at timestamptz NOT NULL DEFAULT now(),
     updated_at timestamptz NOT NULL DEFAULT now()
   );
   ```
   These are created inside the provisioning function via `EXECUTE format(...)`, matching the pattern from 0003 (metadata engine tables).

2. Add indexes:
   - `contacts(company_id)` for company-to-contacts lookup
   - `contacts(email)` for dedup/search
   - `deals(stage_id)` for pipeline grouping
   - `deals(contact_id)` for contact-to-deals lookup
   - `deals(expected_close_date)` for forecast queries

3. Write sqlc query files:
   - `packages/db/queries/contacts.sql` — ListContacts (cursor-paginated), GetContact, CreateContact, UpdateContact, DeleteContact, CountContacts
   - `packages/db/queries/companies.sql` — same pattern
   - `packages/db/queries/deals.sql` — same pattern + ListDealsByStage, UpdateDealStage

4. Run `cd apps/api && sqlc generate` — verify generated Go code compiles

5. Create `apps/api/internal/domain/` package (or use the sqlcgen structs directly if sufficient):
   - Contact, Company, Deal types with JSON tags
   - Validate function per type (email format, required fields)

6. Extend `core.lecrm_provision_workspace` to create these tables via `CREATE OR REPLACE FUNCTION` in the migration

7. Verify: provision a new workspace → all 3 tables + pipeline_stages exist

8. Run existing test suite to confirm no regressions on provisioning

## Done When

- [ ] Migration applies cleanly on fresh DB and on top of existing migrations
- [ ] Provisioning function creates contacts, companies, deals tables in new workspaces
- [ ] sqlc generates typed Go code for all CRUD queries
- [ ] `cd apps/api && go build ./...` compiles clean
- [ ] Existing provisioning tests pass (no regression)
- [ ] New test: provision workspace → verify all 3 tables exist with correct columns

## Completion Verification

1. `ls packages/db/migrations/0005_crm_entities.sql` -- migration exists
2. `grep -c 'contacts\|companies\|deals' packages/db/queries/*.sql` -- query files exist
3. `ls apps/api/internal/sqlcgen/contacts.sql.go` -- sqlc generated
4. `cd apps/api && go build ./...` -- compiles
5. `cd apps/api && go test -race -count=1 ./...` -- all tests pass
6. Commit: `feat(db): add Contact, Company, Deal entity tables and sqlc queries (Sprint 4)`

## References

- `docs/sprint-plan.md` — Sprint 4 work items
- `packages/db/migrations/0003_metadata_engine.sql` — pattern for extending provisioning function
- `packages/db/migrations/0004_workspaces_admin_email_registry.sql` — pipeline_stages table (deals FK target)
- `apps/api/internal/sqlcgen/` — existing generated code
- `packages/db/queries/` — existing query files
