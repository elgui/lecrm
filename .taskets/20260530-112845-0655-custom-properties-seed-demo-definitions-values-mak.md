---
id: 20260530-112845-0655
title: "Custom properties: seed demo definitions + values (make tailorization visible)"
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
tags: [custom-properties, metadata, demo, seed, moat]
category: engineering
group: lecrm-custom-properties-ux
order: 1
plan: true
---

## Context

leCRM's custom-properties ("tailorization") capability is fully built at the API/data layer (metadata engine, ADR-010): `GET/POST/DELETE /v1/metadata/definitions`, `GET/PUT /v1/{contacts,deals}/{id}/properties`, types string/number/boolean/enum/date/json, fail-closed audit, JSONB + GIN. The frontend `custom-properties-editor.tsx` already renders/edits values for defined properties. BUT the demo workspace has **zero definitions seeded**, so every record shows "No custom properties defined for this workspace" — the single most important differentiator for a HubSpot integrator (Leo) is currently invisible.

## Why

Custom fields are THE thing a CRM integrator evaluates (it's his daily HubSpot work) and leCRM's stated moat is tailorization. Seeding a few realistic French custom fields + values lights up the EXISTING editor on every record with ZERO new app code — the highest bang-for-buck step to make the demo compelling. Fast win; do this first. See [[project_lecrm_strategic_moat]].

## Approach

1. Add custom-property DEFINITIONS to the demo workspace (per-workspace `custom_property_definitions` table; match the shape used by `apps/api/internal/metadata/definitions.go`). Suggested, French, realistic:
   - Deal: `source_du_lead` (enum: Site web / Recommandation / Salon / LinkedIn), `probabilite` (number, %), `canal_signature` (string)
   - Contact: `fonction` (string), `canal_prefere` (enum: Email / Telephone / WhatsApp)
2. POPULATE values on the 6 seeded deals + 10 contacts (the `objects` JSONB store, keyed by parent_type+parent_id — match `apps/api/internal/metadata/set.go`). Prefer seeding **through the API** (`PUT /v1/{entity}/{id}/properties`) so it exercises validation/audit; or extend `deploy/seed/demo.sql` idempotently if SQL is simpler.
3. Re-verify on `demo.lecrm.gbconsult.me`: contact + deal detail show populated custom fields instead of the empty-state message.

## Done When

- [ ] Demo workspace has >=3 Deal + >=2 Contact custom-property definitions (French, mixed types incl. an enum)
- [ ] Seeded deals/contacts show populated custom-property values in the detail UI
- [ ] Seeding path is idempotent and committed (seed script or documented API calls)

## References

- `apps/api/internal/metadata/definitions.go`, `set.go`, `handlers.go` (storage shape, endpoints)
- `apps/web/src/components/custom-properties-editor.tsx` (consumes definitions+values)
- `deploy/seed/demo.sql` (existing idempotent seed; demo schema = `core.workspaces.role_name WHERE slug='demo'`)
- Live host `51.77.146.49`, container `lecrm-postgres`, db `lecrm`
