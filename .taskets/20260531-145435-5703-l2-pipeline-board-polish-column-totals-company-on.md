---
id: 20260531-145435-5703
title: "L2: Pipeline board polish — column € totals, company on cards, closing-soon cue, scroll affordance"
status: done
priority: p2
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 6
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why
The pipeline kanban (screenshot `07-pipeline-kanban.png`) shows columns with a **count only** (no summed €), **info-starved cards** (title/amount/date only — no company/contact), the **5th column cut off** with weak horizontal-scroll affordance, and ISO dates. It looks like a card wall, not a sales cockpit.

## What to do (in `apps/web/src/routes/pipeline/$workspaceId.tsx` + card/board components)
1. Per-column **€ total** in the column header alongside the count.
2. **Company name** on each deal card (`text-sm` muted, under the title).
3. **Urgency cue** — a colored dot/badge when the expected close date is within 14 days (red if overdue, keep existing overdue logic).
4. **French dates** on cards (shared formatter from tasket #5).
5. Clearer **horizontal-scroll affordance** so the 5th column is discoverable (fade/edge gradient, scroll hint, or responsive column sizing).

## Scope guardrail
Do NOT split the combined "Gagné / Perdu" column or rebuild the board — those are deferred nice-to-haves. Keep drag-drop behavior intact (click-vs-drag 4px threshold).

## Acceptance
- Columns show € totals; cards show company + French date + closing-soon cue; the board's full width is discoverable.
- `tsc`, `eslint`, `vitest` green in `apps/web`.
