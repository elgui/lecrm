---
id: 20260530-112845-eebb
title: "[SUPERSEDED] Custom properties: type-aware inputs (already implemented; replaced by list-columns tasket)"
status: superseded
priority: p1
created: 2026-05-30
updated: 2026-05-31
tags: [custom-properties, frontend, ux, polish]
category: engineering
group: lecrm-custom-properties-ux
order: 3
plan: true
---

## Context

`apps/web/src/components/custom-properties-editor.tsx` historically rendered EVERY custom property as a text or number `<Input>` regardless of declared type — enums free-text, dates text, booleans text. (Belongs to group `lecrm-custom-properties-ux`, NOT `lecrm-demo-polish` — noted here because it surfaced during the 2026-05-31 demo-polish audit.)

## Why

For the tailorization demo to feel real (not a JSON-blob hack), the value inputs must respect the declared type: an enum a dropdown of its allowed_values, a date a date picker, a boolean a checkbox/switch. The polish that makes "custom fields" read as a first-class feature to a CRM integrator.

## Resolution (verified 2026-05-31)

**Already implemented** — `custom-properties-editor.tsx` (lines 111-149)
renders per `def.property_type`: `boolean` → checkbox, `enum` →
`<select>` over `allowed_values`, `number`/`date` → typed `<Input>`,
`text`/`json` → text input. The `coerce()` helper round-trips typed
values on save; `canWrite` gates every control (members read-only).
Required fields show an asterisk. The optional list-view-column sub-goal
was split into its own tasket (`...-c17e surface custom fields as list
view columns`) — hence **superseded**, not merely done.

## Done When (all met)

- [x] Enum/date/boolean custom props render as Select/date-picker/checkbox and save correctly
- [x] Required + enum constraints enforced client-side; values round-trip through the API
- [x] `bun run typecheck` + existing web tests pass (55/55 green 2026-05-31)

## References

- `apps/web/src/components/custom-properties-editor.tsx` (current type-aware impl)
- `apps/api/internal/metadata/set.go` + coercion (type coercion contract)
- List-columns follow-up: `20260530-113058-c17e-custom-properties-surface-custom-fields-as-list-vi.md`
