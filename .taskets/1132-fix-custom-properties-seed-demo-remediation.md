---
id: 1132
title: "[Fix] Custom properties: seed demo definitions + values (make tailorization visible)"
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
tags: [custom-properties, metadata, demo, seed, remediation]
category: engineering
group: lecrm-custom-properties-ux
order: 1
remediates: 20260530-112845-0655
plan: true
---

## Remediation task #1132

Closes the three gaps the original task `20260530-112845-0655` left. The seed
itself was correct and already DB-verified; the missing pieces were push +
deploy-verify + live HTTP confirmation. All done — full log lives in the backing
tasket `20260530-112845-0655` under "## Remediation (task #1132 — 2026-05-30)".

### What was fixed

1. **Pushed to remote.** `origin/main` advanced `9346a6a1..5ceec683` (fast-forward).
   Includes the two custom-property commits: `f4867e01` (seed) + `f5e459cf` (status flip).
2. **Deploy / idempotent seed re-apply** on demo schema
   `workspace_99433671a69342869dbcd2c25f1d902f` — every INSERT `0 0` (no-op).
   Data-only seed → no API rebuild needed; `lecrm-api` already serves it. 5 defs / 16 values.
3. **Live HTTP verification** on `https://demo.lecrm.gbconsult.me`:
   - `GET /v1/metadata/definitions` → 5 defs (2 contact + 3 deal), French, mixed types incl. enums.
   - `GET /v1/deals/{id}/properties` → populated, e.g. `{"canal_signature":"Visio","probabilite":30,"source_du_lead":"Site web"}`.
   - `GET /v1/contacts/{id}/properties` → populated, e.g. `{"canal_prefere":"Téléphone","fonction":"Gérante"}`.

Values are served via the dedicated `/v1/{entity}/{id}/properties` sub-resource
(workspace-scoped metadata route, no RBAC gate) — the canonical shape the web
custom-properties editor reads — not embedded in the RBAC-gated CRM payload.
