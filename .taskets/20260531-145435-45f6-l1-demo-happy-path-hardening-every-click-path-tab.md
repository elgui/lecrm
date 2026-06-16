---
id: 20260531-145435-45f6
title: "L1: Demo happy-path hardening — every click-path tab populated, no console errors / 500s"
status: done
priority: p1
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 4
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why
Léo will click every tab in at least one workspace. Any empty table, "0 contacts", Lorem, console error, or 500 on the happy path silently ends the deal. Seed **depth over breadth**.

## What to do
Walk Léo's path across the 3 seeded workspaces (`demo`, `bistrot-halles`, `menuiserie-vasseur`):
dashboard → contacts → contact detail → deals → deal detail → pipeline → tasks → settings.
- Every list populated with believable French data; notes/tasks present on at least some records.
- No empty states on the demo path, no `0` headline stats, no placeholder text.
- Open browser devtools and confirm **no console errors and no failed/500 requests** on the happy path.
- Fix or seed whatever is thin. Record what you checked.

## Acceptance
- A written walkthrough note (in this tasket or a report) confirming each tab populated + clean console across the 3 workspaces.
