---
id: 20260530-112845-eebb
title: 'custom-properties: type-aware value inputs (enum dropdown, date picker, number)'
status: done
priority: P2
created: 2026-05-30T11:12:08Z
updated: 2026-05-31T07:30:00Z
tags: [custom-properties, frontend]
group: null
effort: S
---

# custom-properties: type-aware value inputs (enum dropdown, date picker, number)

## Context
The custom-properties editor renders every property as a plain text
input regardless of declared type. A `date` property should show a date
picker, an `enum` a dropdown, a `number` a numeric input, a `boolean` a
checkbox. Without this, the demo's "custom fields" story looks unfinished.

## Acceptance criteria
- [x] `date` properties render a date picker
- [x] `enum` properties render a dropdown (allowed_values)
- [x] `number` properties render a numeric input
- [x] `boolean` properties render a checkbox
- [x] Values round-trip correctly (save + reload)
- [x] Read-only mode respected (members)

## Resolution (2026-05-31)
Already implemented in
`apps/web/src/components/custom-properties-editor.tsx` (lines 111-149):
`boolean` → checkbox, `enum` → `<select>` over `allowed_values`,
`number`/`date` → typed `<Input>`, `text`/`json` → text input. The
`coerce()` helper round-trips typed values on save; `canWrite` gates
every control (members are read-only). Verified against the live code
during the 2026-05-31 demo-polish audit — closing as done.
