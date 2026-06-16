---
id: 20260531-145435-04a1
title: "L1: Merge split Save changes / Save properties into one Save + humanize custom-field labels"
status: done
priority: p1
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 2
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why
Every record detail page (contact, deal, company) has **two separate forms with two buttons** — "Save changes" (core fields) and "Save properties" (custom fields) (screenshots `05-contact-detail.png`, `11-deal-detail.png`). It's a data-loss trap: edit the phone number, scroll, edit a custom property, hit "Save properties", and the phone change is lost. Custom fields also render **raw snake_case keys** as labels (`canal_prefere`, `source_du_lead`, `probabilite`, `canal_signature`) under a generic "Custom Properties" header — reads like a DB dump.

## What to do
1. Merge into a **single save action** per detail page — one "Enregistrer" button that fires both the core-field and custom-property mutations (Winston: cheap to unify on the frontend; a transactional backend unify is the larger optional version). Disable the button when nothing is dirty.
2. Surface **human display names** for custom fields instead of snake_case keys. If the field definition lacks a `display_name`, add one (migration + custom-fields admin form) and fall back to a prettified key. Give the section a French title (e.g. "Informations complémentaires").

## Acceptance
- One save button per detail page; editing core + custom fields and saving once persists both; no lost edits.
- Custom field labels are human-readable, not snake_case.
- `tsc`, `eslint`, `vitest` green in `apps/web`.
