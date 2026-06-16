---
id: 20260531-145435-1bb4
title: "L2: Visual craft pass — typography hierarchy, wordmark, integrator context banner, switcher label"
status: done
priority: p2
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 8
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why
The UI is competent but flat — it looks built, not crafted (all screenshots). Specific gaps: flat typography, an auto-generated-looking wordmark, and — critically for the integrator — **no clear signal that Léo is administering a CLIENT's data** (screenshots `15`, `16`: the dashboard still says "your workspace" and the only cue is a truncated footer label "GB Consult · admini…").

## What to do
1. **Typography hierarchy:** boost stat values (`text-3xl font-semibold`), give sidebar section headers (CRM / WORKSPACE / CONFIGURE) more presence (small, tracked, slightly warmer gray), `font-medium` nav items vs `font-normal` subtext.
2. **Wordmark:** replace the blue-square + white-"I" with a confident leCRM wordmark for the demo.
3. **Integrator context banner (safety + clarity):** when role is `integrator` in the current workspace, show a persistent, unmissable banner / top-bar accent — e.g. "Bistrot des Halles — vous administrez ce compte" (Chrome-incognito style), not just the footer label. See `apps/web/src/components/WorkspaceSwitcher.tsx` and the root layout `apps/web/src/routes/__root.tsx`.
4. Fix the **truncated switcher button label** ("GB Consult · admini…").
5. Trim the leftover **"Powered by authentik"** footer on the already-branded login (login branding shipped in tasket `37fc`).

## Acceptance
- Clear visual hierarchy; a real wordmark; an unmissable "you are administering {client}" signal for the integrator; no truncated switcher label.
- `tsc`, `eslint`, `vitest` green in `apps/web`.
