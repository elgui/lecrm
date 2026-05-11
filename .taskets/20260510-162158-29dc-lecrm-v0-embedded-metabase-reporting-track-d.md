---
id: 20260510-162158-29dc
title: leCRM v0 — Embedded Metabase reporting (Track D)
status: later
priority: p2
created: 2026-05-10
updated: 2026-05-11
tags: [reporting, metabase, v0]
category: project
group: lecrm-v0-build
order: 2
plan: true
---

# leCRM v0 — Dashboard reporting iframe bridge (Cube.dev primary, Metabase fallback)

## Why this tasket exists

v0 needs a basic per-client reporting surface (deal counts, activity volume, pipeline funnels) without building dashboards in-app. Per [ADR-009](docs/adr/ADR-009-stack-and-license.md) §9, **Cube.dev iframe is the named v0 bridge**. The original plan was self-hosted Metabase pointed at Twenty's Postgres via a Twenty SDK extension — that delivery vector is dead (no Twenty fork, no SDK extension).

The new shape is an iframe-embeddable dashboard with workspace-scoped read access to the per-workspace Postgres schema, surfaced inside the React frontend under a "Reports" route.

**This tasket is downstream of [b844](20260510-202450-b844-lecrm-v0-twenty-fork-tasket-housekeeping-week-1-sc.md) (scaffolding) — start after the scaffold is up.**

## Re-scoped done criteria

- [ ] **Decide Cube.dev vs Metabase** at kickoff (re-scope review). Cube.dev is the ADR-009 §9 named choice (semantic-layer + iframe + LLM-ready for v2 "ask your CRM"). Metabase is the faster-to-scope alternative if Cube.dev integration cost exceeds 3 days. Record the decision as a comment on this tasket.
- [ ] Per-workspace **read-only** Postgres role (`workspace_<id>_ro`) with `SELECT` only on the workspace schema. Created via an extension to `lecrm_provision_workspace` (a sibling SECURITY DEFINER function or an addition to the primary one).
- [ ] Cube.dev (or Metabase) container in `deploy/compose/`, scoped to ~512 MB memory; reads from Postgres as the per-workspace read-only role.
- [ ] Signed iframe embed URL with workspace_id parameter; signature verified by the dashboard service.
- [ ] React frontend `apps/web/src/routes/reports/$workspaceId.tsx` mounts the iframe with the right URL.
- [ ] Baseline dashboard: deals by stage, deals by owner, recent activities, conversion funnel.

## Out of scope

- LLM-driven "ask your CRM" dashboard interface (v2 work).
- Custom-object reporting beyond the standard contacts/deals/companies model (waits on ADR-010 metadata engine, week 5).

## References

- [ADR-009](docs/adr/ADR-009-stack-and-license.md) §9 (Cube.dev as v0 dashboard bridge).
- [ADR-001](docs/adr/ADR-001-tenancy-model.md) (workspace schema isolation) — read-only role for Cube.dev/Metabase is a v0 addition.
