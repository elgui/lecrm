---
id: 20260601-110828-2b50
title: [Integrator gap] Custom report builder + surface Cube.dev reporting on the demo
status: done
priority: p1
created: 2026-06-01
updated: 2026-06-01
done: 2026-06-01
tags: [reporting, cube, integrator-gap, leo, custom-properties]
category: project
group: lecrm-integrator-gap-closure
order: 1
plan: true
---

## Context

Anticipating Léo's HubSpot reflexes (he comes from ChefCheffe, where his recurring
MRR — 300→800 €/mo, vault tasket #402 — came from a **custom monthly "Analyse
Produits" report** HubSpot could not produce natively: N-1 vs current vs objective,
mix CA par catégorie, Client 360). Custom reporting is the integrator's stickiest,
most lucrative deliverable.

leCRM already has the **backend** for this: Cube.dev wiring + 4 baseline dashboards
were shipped (taskets `20260510-162158-29dc`, `20260528-123820-e629`). **But** the
Reports route is currently stubbed to an honest "coming soon" placeholder on the
demo (tasket `20260531-145435-9f97` chose honesty over fake data), so a visitor sees
nothing — and there is **no self-serve custom-report builder** for per-client KPIs.

This is the highest-ROI gap: the heavy lifting (Cube backend) is done; what's missing
is surfacing it on real data + a thin custom-definition layer.

## Goal

Make reporting **live on the demo** for the 3 seeded workspaces, and add a minimal
**custom report builder** so an integrator can define a client-specific report
(metric × dimension × period, with an N-1 comparison column) without code.

## Steps

1. **Un-stub the Reports route** and wire it to the live Cube.dev models against real
   seeded data; confirm it renders for all 3 demo workspaces (`demo`,
   `bistrot-halles`, `menuiserie-vasseur`) with no empty states / 500s.
2. **Saved custom reports (per workspace):** a builder that lets the user pick
   - a **metric** (count, sum of deal amount, win rate),
   - a **dimension** (pipeline stage, owner, company, custom property),
   - a **period** (month/quarter/year),
   - and toggle an **N-1 comparison** column (current vs same period last year).
   Persist definitions per workspace (reuse the metadata/JSONB pattern, ADR-010).
3. **Respect tenant isolation + custom properties** — reports must read only the
   caller's workspace schema and be able to group by custom-property definitions.
4. Gate on `tsc --noEmit -p tsconfig.app.json`, `eslint src`, `vitest run` (web) and
   `go build ./apps/...` if backend touched.

## Done When

- [ ] Reports route renders real seeded data on all 3 demo workspaces (no placeholder, no empty table).
- [ ] A user can create, save, and re-open a custom report (metric × dimension × period).
- [ ] The N-1 comparison column renders correctly.
- [ ] Reports are workspace-scoped (cross-tenant test green) and can group by a custom property.
- [ ] tsc + eslint + vitest green; commit scoped to the report feature only.

## References

- Cube.dev backend: taskets `20260510-162158-29dc`, `20260528-123820-e629`
- Reports stub decision: tasket `20260531-145435-9f97`
- `docs/ICP-ARCHETYPE.md` (custom reporting is P-later but is Léo's MRR engine)
- ADR-010 metadata-engine / JSONB pattern for storing report definitions
- vault evidence of the deliverable shape: tasket #402, #729 (Analyse Produits spec)
