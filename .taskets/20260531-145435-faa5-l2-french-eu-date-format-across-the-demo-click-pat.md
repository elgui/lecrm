---
id: 20260531-145435-faa5
title: "L2: French + EU date format across the demo click-path"
status: done
priority: p2
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 5
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why
UI chrome is **English** ("New contact", "Manage your contacts and relationships", "Save changes", "Open pipeline") over **French** data — jarring for a French-market product (all screenshots). Dates are inconsistent: lists show US `M/D/YYYY` (`5/29/2026`) while pipeline cards show ISO (`2026-07-13`).

## What to do
1. Localize the **chrome strings on the click-path screens** to French (dashboard, contacts, companies, deals, pipeline, tasks, settings, detail pages, page headers, buttons, empty/loaded states).
2. Format **all dates** as `JJ/MM/AAAA` (or `13 juil.` style for cards) via a shared formatter, applied to lists and pipeline cards.

## Scope guardrail
**Demo-critical slice only.** Do NOT build a full i18n framework (i18next, locale files, language switcher) for this — hardcode/centralize French strings for the click-path. Full i18n is a separate post-demo decision.

## Acceptance
- No visible English chrome on the demo click-path; all dates in EU format.
- `tsc`, `eslint`, `vitest` green in `apps/web`.
