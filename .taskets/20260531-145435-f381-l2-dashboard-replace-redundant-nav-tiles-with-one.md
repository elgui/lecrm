---
id: 20260531-145435-f381
title: "L2: Dashboard — replace redundant nav tiles with one live 'what needs attention' section"
status: done
priority: p2
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 7
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why
The dashboard (screenshots `02-dashboard.png`, `15-integrator-dashboard.png`) has 4 stat cards, then **3 nav tiles that duplicate the sidebar**, then empty white space. It's a launcher, not a cockpit — the #1 reason a client logs in twice and never returns.

## What to do (in `apps/web/src/routes/index.tsx`)
1. Remove the 3 redundant bottom nav tiles (Contacts/Companies/Deals — the sidebar already does this).
2. Add **one live "what needs my attention" section**: tasks due, deals closing within 14 days, and/or recent activity. Use existing hooks (tasks, deals). Keep it calm and scannable; seeded demo data must make it look alive.

## Scope guardrail
Don't over-build an activity-feed engine — a simple "due / closing soon / recent" composed from existing queries is enough for the demo. **Check the existing tasket** `20260530-111108-f84f-demo-dashboard-replace-bare-nav-cards-with-live-st…` and FOLD this into it / coordinate rather than duplicating.

## Acceptance
- Dashboard shows a useful live section instead of redundant tiles + whitespace; no duplication of sidebar nav.
- `tsc`, `eslint`, `vitest` green in `apps/web`.
