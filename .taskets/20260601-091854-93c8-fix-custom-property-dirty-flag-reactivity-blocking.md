---
id: 20260601-091854-93c8
title: Fix custom-property dirty-flag reactivity blocking PR #8 web CI
status: done
priority: p1
created: 2026-06-01
updated: 2026-06-01
done: 2026-06-01
tags: [lecrm, demo, ux, frontend, ci, pr8]
category: project
group: lecrm-leo-demo-polish-fixups
order: 1
plan: true
---

> Follow-up from the PR #8 review (group `lecrm-leo-demo-polish`). New group so we don't append to the in-flight automation group.
> Frontend: `apps/web` (React 19 + TanStack Query). Read before Write; scope `git add` to files you change.

## Why — PR #8 CI is RED on exactly one check
On PR #8 head `c3e6ebec6`, every CI check passes EXCEPT `web-test+typecheck`, which fails on a single test:
`apps/web/src/hooks/use-custom-property-form.test.tsx:133` — *"adopts a clean baseline once the refetch catches up to the saved edit"* (expects `isDirty` to become `false`, receives `true`). `tsc` and `eslint` are clean; 106/107 tests pass. **This is the only thing blocking merge.** Do NOT merge PR #8 until this check is green.

## Root cause (verified)
The review-fix commit added a reseed guard to `apps/web/src/hooks/use-custom-property-form.ts` (lines ~81-102). The "adopt a clean baseline" branch:
```
if (currentJson === seededJson) {
  baselineRef.current = seededJson;   // mutates a REF only
  return;                             // no setState -> no re-render
}
```
`baselineRef` is a `useRef`, and `isDirty` is computed during render as `JSON.stringify(form) !== baselineRef.current`. Mutating the ref without any state update means the component never re-renders, so `isDirty` is not recomputed and stays `true` after a save settles / refetch catches up. The new test correctly catches this (its `waitFor` never sees `isDirty` flip to false). In the running app the symptom is milder (a save triggers re-renders through the mutation state), but the dirty/"Enregistré" indicator can lag — and CI is red regardless.

## Fix (pick one; minimal-diff preferred)
- **Preferred:** make the baseline a piece of STATE, not a ref, so adopting a new baseline triggers a render and `isDirty` recomputes:
  `const [baseline, setBaseline] = React.useState('{}')` and `setBaseline(seededJson)` in the adopt branches; derive `const isDirty = JSON.stringify(form) !== baseline`. Keep the effect deps `[definitions, values]` (don't add `baseline`) to avoid a loop.
- **Alternative (smaller):** in the `currentJson === seededJson` branch, also call `setForm(seeded)` so a render occurs (new object reference; identical content; `isDirty` then evaluates false). The effect deps are `[definitions, values]`, so this does not re-trigger the effect — no loop.

Preserve the genuinely-correct behaviour the commit added: a mid-edit refetch with DIFFERENT content must NOT clobber an in-progress draft (the test at `:99` "does not clobber an in-progress edit" must keep passing).

## Acceptance
- `cd apps/web && node_modules/.bin/vitest run` → all pass (incl. both `use-custom-property-form` tests at :99 and :120).
- `node_modules/.bin/tsc --noEmit -p tsconfig.app.json` and `node_modules/.bin/eslint src` → clean.
- PR #8 `web-test+typecheck` check goes green; then PR #8 is mergeable (the other 3 review findings — unified-save atomicity, full company-name map, reports error→placeholder — were verified fixed in `c3e6ebec6`).

## Note for whoever merges
After PR #8 merges, the staging host still needs a manual rebuild (`docker compose ... up -d --build api`) and the new `deploy/seed/demo*.sql` applied to the staging DB for the live demo to reflect it.
