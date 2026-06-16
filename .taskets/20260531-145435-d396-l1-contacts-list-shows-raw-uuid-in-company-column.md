---
id: 20260531-145435-d396
title: "L1: Contacts list shows raw UUID in Company column → show company name + surface relationships"
status: done
priority: p1
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 1
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why
The Contacts list — the most-used screen — renders a **truncated raw company UUID** (`c0000000…`) in the COMPANY column for ~7 of 9 rows instead of the company name (screenshot `04-contacts-list.png`). It reads like alpha software and is the first thing Léo sees when he clicks Contacts.

## What to do
1. Fix the contacts list query/render so `company_id` resolves to the company **name** in the COMPANY column. Winston's read: likely a server-side query-embedding / join fix, small. Check the contacts list hook (`apps/web/src/hooks/use-contacts.ts`) and the list route (`apps/web/src/routes/contacts/index.tsx`) and the API contact list handler.
2. While here, surface the **invisible relationships** (screenshots `05-contact-detail.png`, `11-deal-detail.png`): show the linked **company** on the contact detail, and **company + primary contact** on the deal detail. Display-only links — NOT a new association editor.

## Scope guardrail
Only do the relationship-surfacing if the contact↔company / deal↔company / deal↔contact FKs already exist in the schema. **Confirm in the schema first.** If the associations don't exist yet, ship ONLY the list-column name fix here and split relationship-surfacing into a follow-up tasket.

## Acceptance
- Contacts list COMPANY column shows readable company names (or a clean "—" when none), never a UUID.
- (If FKs exist) contact detail shows its company; deal detail shows company + contact.
- `tsc --noEmit -p tsconfig.app.json`, `eslint src`, `vitest run` green in `apps/web`.
