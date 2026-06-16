---
id: 20260531-145435-9f97
title: "L1: Reports must render real seeded data on demo workspaces (or be hidden/stubbed for the demo)"
status: done
priority: p1
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 3
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why
On `demo.lecrm.gbconsult.me` the **Reports** nav item dead-ends to "Reports unavailable — Embedded reporting is not configured on this server." (screenshot `08-reports.png`). A first-class nav item returning an error is a trust-killer in Léo's tour. The Reports route is now BUILT (cube-frame, ~194 lines) — so this is a **config/seeding gap on staging**, not a build.

## Decide, then do ONE
- **(a) Make it real:** configure embedded reporting (Cube embed token + dashboards) and seed at least one dashboard for the 3 demo workspaces (`demo`, `bistrot-halles`, `menuiserie-vasseur`) so Reports renders live data. See `apps/web/src/routes/reports/$workspaceId.tsx` + the embed-token endpoint + `docs/INFRASTRUCTURE.md` / `deploy/`.
- **(b) Stub it honestly for the demo:** hide the Reports nav item, or replace the error with a branded "Rapports — bientôt disponibles" placeholder (same honest-placeholder spirit as the AI seat).

## Scope guardrail
Confirm whether Cube is deployable on staging cheaply BEFORE committing to (a). If not cheap, ship (b) — never leave the red error in the demo.

## Acceptance
- Clicking Reports during the demo shows either real seeded dashboards or an honest branded placeholder — never the "not configured" error.
