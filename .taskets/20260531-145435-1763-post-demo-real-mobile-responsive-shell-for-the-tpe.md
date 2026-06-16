---
id: 20260531-145435-1763
title: "Post-demo: real mobile responsive shell for the TPE client (bottom tab nav)"
status: done
priority: p3
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 11
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why
VERIFIED BROKEN (screenshots `18-mobile-dashboard.png`, `19-mobile-pipeline.png`): the fixed `w-64` sidebar stays visible at 390px and squeezes content to a ~130px sliver; no hamburger/drawer. The TPE client lives on their phone — mobile matters a lot for the CLIENT persona. (The integrator app stays **desktop-only** — do not build mobile settings/switcher.)

## What to do (post-demo — NOT in the Léo demo critical path)
- Replace the fixed sidebar at mobile breakpoints with a **bottom tab bar**: Contacts, Pipeline, Tasks, + a central "+" FAB → bottom sheet for create (Nouveau contact / Nouveau deal / Enregistrer un appel).
- **Contact list** → full-width two-line rows (name + company), avatar left, chevron right — no table columns; pinned search.
- **Pipeline** → swipeable snap-scroll columns; touch-tune dnd-kit drag (the part that won't go responsive for free — Winston).
- Relative French dates ("dans 14 jours"); minimal create modal (custom fields live in detail, not create).

## Scope
Winston tagged the responsive shell **M** effort; pipeline touch-tuning is the extra cost. **Captured because the user flagged mobile as very important for TPE** — sequence it after the demo ships. Integrator surfaces stay desktop-only.

## Acceptance
- Client-facing screens are usable and pleasant at 390px with bottom-tab nav; integrator console unchanged.
