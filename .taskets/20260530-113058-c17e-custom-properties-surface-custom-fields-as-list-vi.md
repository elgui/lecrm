---
id: 20260530-113058-c17e
title: "Custom properties: surface custom fields as list-view columns (+ minor shadcn input polish)"
status: done
priority: p2
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
tags: [custom-properties, frontend, list-view, demo]
category: engineering
group: lecrm-custom-properties-ux
order: 3
plan: true
---

## Context

Custom-property VALUE editing is already type-aware: `apps/web/src/components/custom-properties-editor.tsx` renders boolean -> checkbox, enum -> select (from `allowed_values`), number -> number, date -> date input, json/string -> text, with coercion-on-save matching `apps/api/internal/metadata/set.go`. Verified 2026-05-30. (This replaces the superseded "type-aware inputs" tasket, which was based on a wrong reading.)

The remaining gap: custom fields are only visible on the DETAIL pages. They do NOT appear anywhere in the LIST views (`apps/web/src/routes/deals/index.tsx`, `contacts/index.tsx` show only built-in columns).

## Why

For the tailorization moat to land in a demo, a CRM integrator (Leo) should SEE custom fields in the table at a glance - e.g. a "Source du lead" column on Deals - not only after opening each record. This is the visible payoff of the seed (order:1) and the in-app field creator (order:2). Lower priority polish that completes the story.

## Approach

1. Surface 1-2 workspace custom fields as columns in the Deals list (and optionally Contacts list). Pull definitions (`GET /v1/metadata/definitions?parent_type=deal`) + per-row values; render the value with type formatting (enum label, formatted date, etc.).
2. Avoid N+1: prefer a list response that includes custom props, or batch-fetch values for the page. Check whether the existing list endpoint can include properties before adding per-row fetches.
3. Optional: let the user pick which custom field(s) to show as columns (small selector); otherwise show a sensible default (first 1-2 defs).

## Minor input polish (optional, same tasket)

- enum native `<select>` -> shadcn `Select`; boolean raw checkbox -> shadcn `Switch`; date `<input type=date>` -> the date control used on deal detail. Purely cosmetic; the inputs already function.

## Done When

- [ ] At least one custom field renders as a column in the Deals list (type-formatted)
- [ ] No N+1 regression on the list view (batched or included in list response)
- [ ] `bun run typecheck` + existing web tests pass

## References

- `apps/web/src/routes/deals/index.tsx`, `contacts/index.tsx` (list tables)
- `apps/web/src/components/custom-properties-editor.tsx` (already type-aware; reuse formatting)
- `apps/api/internal/metadata/handlers.go` (definitions + properties endpoints)
- Depends on order:1 (seeded fields to show) and order:2 (fields created in-app)
