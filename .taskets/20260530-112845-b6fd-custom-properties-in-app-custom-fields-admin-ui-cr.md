---
id: 20260530-112845-b6fd
title: "Custom properties: in-app Custom Fields admin UI (create/list/delete definitions)"
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
tags: [custom-properties, metadata, frontend, settings, moat]
category: engineering
group: lecrm-custom-properties-ux
order: 2
plan: true
---

## Context

The metadata API exposes definition management (`POST /v1/metadata/definitions` create, `GET ...?parent_type=` list, `DELETE /v1/metadata/definitions/{id}`) but there is **no frontend for it** — Settings only has Workspace + Members (`apps/web/src/routes/settings/`). Definitions can today only be created via CLI/API (tasket `731a` deferred the admin UI to "v1+"). Verified 2026-05-30.

## Why

This is the capability Guillaume flagged as the missing piece for Leo's first impression: the ability to **create a custom field in-app, live**. For a HubSpot integrator, "add any field, any type, per object, in seconds, no engineer" is the tailorization-moat demo moment. Without it the feature looks like a fixed schema.

## Approach

1. New Settings sub-page "Custom Fields" (`apps/web/src/routes/settings/custom-fields.tsx`), styled like `settings/members.tsx`. Tab/select for object type (Contact / Deal).
2. List existing definitions for the selected type (`GET /v1/metadata/definitions?parent_type=`). Show key, type, required, allowed_values; delete action (`DELETE .../{id}`) with confirm.
3. Create form (`POST /v1/metadata/definitions`): property_key, property_type (string/number/boolean/enum/date/json), required toggle, and an allowed_values editor shown only for `enum`. Client-validate key format; surface API 400s (duplicate key → 409).
4. Add a `use-metadata-definitions.ts` hook (mirror `use-deals.ts` `useDealProperties` pattern) with TanStack Query; invalidate on create/delete so the editor + tables pick up new fields.
5. RBAC: gate the page + mutations to **admin+** (`RoleAdmin`) so Leo (now admin on demo) can use it; confirm against `apps/api/internal/rbac/role.go`. DECISION to confirm with Guillaume: admin vs owner-only for schema changes.

## Done When

- [ ] Settings > Custom Fields lists, creates, and deletes definitions for Contact + Deal
- [ ] Enum type captures allowed_values; create errors (dup key) surfaced
- [ ] New field appears in the record detail editor without reload (cache invalidation)
- [ ] admin+ gated; `go build ./...` + `bun run typecheck` clean
- [ ] Live demo: create a field as `leo`, see it on a record

## References

- `apps/api/internal/metadata/handlers.go` (`ListDefinitions`/`CreateDefinition`/`DeleteDefinition`, routes at lines 44-52)
- `apps/web/src/routes/settings/members.tsx` (settings sub-page + mutation pattern), `__root.tsx` (route mount)
- `apps/web/src/hooks/use-deals.ts` (`useDealProperties`/`useUpdateDealProperties` pattern), `lib/types.ts` (`CustomPropertyDefinition`)
- `apps/api/internal/rbac/role.go` (member<admin<owner)
- Depends on order:1 (seeded definitions help verify); enhanced by order:3 (type-aware inputs)
