---
id: 20260528-123820-e629
title: leCRM v0 — Cube.dev frontend (Reports route + 4 baseline dashboards)
status: next
priority: p1
created: 2026-05-28
updated: 2026-05-28
tags: [reporting, cube-dev, frontend, dashboards, v0]
category: project
group: lecrm-v0-sprint-12
group_order: 12
order: 2
plan: true
---

# leCRM v0 — Cube.dev frontend (Reports route + 4 baseline dashboards)

## Why

Frontend half of sprint-12. Depends on `12a` (backend: RO role, Cube container, signed-embed endpoint) having shipped. Surfaces baseline dashboards inside the React app under a workspace-scoped "Reports" route.

## Done criteria

- [ ] **React route** `apps/web/src/routes/reports/$workspaceId.tsx` (TanStack Router, matches existing pattern). Loader fetches embed JWT from `POST /api/v1/reports/embed-token`.
- [ ] **Cube.dev iframe** mounted with the signed JWT in the query string; resize listener for responsive height.
- [ ] **4 baseline dashboards** authored as Cube schema queries + rendered in the iframe:
  1. **Deals by stage** (count, grouped by `deal_stage`).
  2. **Deals by owner** (count + total value, top 10).
  3. **Recent activities** (last 30 days, by type, time series).
  4. **Conversion funnel** (deals progressing through stages).
- [ ] **Empty-state UX** when the workspace has zero deals/activities (no broken charts).
- [ ] **Loading + error states** on the iframe wrapper (token-mint failure, Cube unreachable).
- [ ] **Tests**: route loader test (token fetch happy path + 403 path); component test for empty/loading/error states.

## Pre-flight (verify before starting)

1. `ls deploy/compose/cube.yml` — 12a Cube container config exists.
2. `curl -X POST http://localhost:8080/api/v1/reports/embed-token` — 12a signed-embed endpoint responds.
3. `ls deploy/cube/schema/` — Cube measures/dimensions defined.
4. If any check fails, STOP — 12a has not shipped yet.

## Out of scope

- Custom-object dashboards beyond the standard contacts/deals/companies/activities (waits on ADR-010 metadata-engine work).
- LLM-driven "ask your CRM" UX (v2).
- Dashboard authoring UI for end-users (v2; v0 ships fixed dashboards only).

## References

- Sibling tasket: 12a backend (id `20260510-162158-29dc`).
- ADR-009 §9 (Cube.dev as v0 dashboard bridge).
