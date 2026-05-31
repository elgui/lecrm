---
id: 20260530-111108-f84f
title: "Demo dashboard: replace bare nav-cards with live stats (counts + pipeline value)"
status: later
priority: p2
created: 2026-05-30
updated: 2026-05-30
tags: [demo, frontend, first-impression, polish]
category: engineering
group: lecrm-demo-polish
group_order: 250
order: 1
plan: true
---

## Context

The demo (`https://demo.lecrm.gbconsult.me`) landing screen at `/` renders only three navigation cards (Contacts / Companies / Deals) under a "Dashboard" heading - no data. Verified live 2026-05-30. First screen Leo and future clients see after login.

## Why

A bare dashboard undersells a CRM that is actually populated (4 companies, 10 contacts, 6 deals, ~158k EUR pipeline in the seed). For a CRM integrator evaluating the tool, an empty-feeling landing reads as unfinished. Highest-leverage cosmetic fix for the first impression. Not a blocker - polish.

## Approach

1. Add summary tiles with REAL numbers: total contacts, companies, open deals (count), open pipeline value (sum of amounts where stage != Closed-Won/Lost); optional deals-by-stage breakdown.
2. Source from existing list endpoints or add a lightweight `GET /v1/dashboard/summary` aggregate. Workspace-scoped (any member can read).
3. Keep/fold the nav cards; match existing shadcn/ui styling in `apps/web`.

## Done When

- [ ] Dashboard shows live counts + open pipeline value matching seeded demo data
- [ ] Numbers workspace-scoped, update with data
- [ ] `go build ./...` + `bun run typecheck` clean; no layout regression on `/`

## References

- `apps/web` dashboard/index route (the "Dashboard" heading + 3 cards)
- `apps/api/internal/crm` list handlers (counts/sums)
- `deploy/seed/demo.sql` for expected numbers
