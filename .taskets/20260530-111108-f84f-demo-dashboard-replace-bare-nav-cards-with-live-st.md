---
id: 20260530-111108-f84f
title: "Demo dashboard: replace bare nav-cards with live stats (counts + pipeline value)"
status: done
priority: p2
created: 2026-05-30
updated: 2026-05-31
done: 2026-05-31
tags: [demo, frontend, first-impression, polish]
category: engineering
group: lecrm-demo-polish
group_order: 250
order: 1
plan: true
---

## Context

The demo (`https://demo.lecrm.gbconsult.me`) landing screen at `/` shows three navigation cards (Contacts / Companies / Deals) under a "Dashboard" heading. First screen Leo and future clients see after login.

**Scope reduced 2026-05-31 (post UI-polish ship, commit 30d88b54, live on demo).** The card LAYOUT/STYLING is now DONE — `apps/web/src/routes/index.tsx` renders polished design-system cards (tinted icon chips, hover lift, arrow affordance). Remaining work is **data-wiring ONLY** — no visual design needed: drop live numbers into (or alongside) the existing cards. Effort M → S.

## Why

A bare dashboard undersells a CRM that is actually populated (4 companies, 10 contacts, 6 deals, ~158k EUR pipeline in the seed). For a CRM integrator evaluating the tool, an empty-feeling landing reads as unfinished. Highest-leverage cosmetic fix for the first impression. Not a blocker - polish.

## Approach

1. Add REAL numbers: total contacts, companies, open deals (count), open pipeline value (sum of amounts where stage != Closed-Won/Lost); optional deals-by-stage breakdown / weighted forecast / recent activity.
2. Source from existing list endpoints or add a lightweight `GET /v1/dashboard/summary` aggregate. Workspace-scoped (any member can read).
3. Styling is already in place — extend the existing `index.tsx` cards (or add a stat row above them) using the shipped design system; do NOT redesign.

## Done When

- [ ] Dashboard shows live counts + open pipeline value matching seeded demo data
- [ ] Numbers workspace-scoped, update with data
- [ ] `go build ./...` + `bun run typecheck` clean; no layout regression on `/`

## References

- `apps/web` dashboard/index route (the "Dashboard" heading + 3 cards)
- `apps/api/internal/crm` list handlers (counts/sums)
- `deploy/seed/demo.sql` for expected numbers
