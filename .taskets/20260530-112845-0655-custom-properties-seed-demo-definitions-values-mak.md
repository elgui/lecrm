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

- [x] Demo workspace has >=3 Deal + >=2 Contact custom-property definitions (French, mixed types incl. an enum)
- [x] Seeded deals/contacts show populated custom-property values in the detail UI
- [x] Seeding path is idempotent and committed (seed script or documented API calls)

## References

- `apps/api/internal/metadata/definitions.go`, `set.go`, `handlers.go` (storage shape, endpoints)
- `apps/web/src/components/custom-properties-editor.tsx` (consumes definitions+values)
- `deploy/seed/demo.sql` (existing idempotent seed; demo schema = `core.workspaces.role_name WHERE slug='demo'`)
- Live host `51.77.146.49`, container `lecrm-postgres`, db `lecrm`

## Remediation (task #1132 — 2026-05-30)

The original run seeded + DB-verified but left three gaps. All now closed:

1. **Pushed to remote.** `origin/main` advanced `9346a6a1..f5e459cf` (fast-forward,
   10 commits incl. the two custom-property commits `f4867e01` seed + `f5e459cf`
   status-flip). `github.com:elgui/lecrm.git`.
2. **Deployment / seed re-apply on demo (idempotent).** Re-ran `deploy/seed/demo.sql`
   against demo schema `workspace_99433671a69342869dbcd2c25f1d902f` — every INSERT
   reported `0 0` (no-op), confirming idempotency. No app code changed (data-only
   seed) so no API rebuild required; `lecrm-api` already serves the data. DB counts:
   5 defs / 16 values.
3. **Live HTTP verification** against `https://demo.lecrm.gbconsult.me`:
   - `GET /v1/metadata/definitions?parent_type=contact` → 2 defs (`canal_prefere`
     enum, `fonction` string); `?parent_type=deal` → 3 defs (`source_du_lead` enum,
     `probabilite` number, `canal_signature` string). **5 total.**
   - `GET /v1/deals/{id}/properties` → populated, e.g. deal #1
     `{"canal_signature":"Visio","probabilite":30,"source_du_lead":"Site web"}`.
   - `GET /v1/contacts/{id}/properties` → populated, e.g. contact #1
     `{"canal_prefere":"Téléphone","fonction":"Gérante"}`.

   Note: custom-property values are exposed via the dedicated `/v1/{entity}/{id}/properties`
   sub-resource (metadata route, workspace-scoped, no RBAC gate), not embedded in the
   RBAC-gated `/v1/{entity}/{id}` CRM payload — the canonical shape the web editor reads.
