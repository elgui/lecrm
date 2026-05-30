---
id: 1133
title: "[Fix] Custom properties: in-app Custom Fields admin UI (create/list/delete definitions)"
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
tags: [custom-properties, metadata, frontend, settings, eslint, remediation]
category: engineering
group: lecrm-custom-properties-ux
order: 2
remediates: 20260530-112845-b6fd
plan: true
---

## Remediation task #1133

Closes the gaps the original task `20260530-112845-b6fd` left: an unresolved
ESLint failure and uncommitted/unpushed code. The admin-UI code itself was
sound; the blockers were tooling + git.

### Root cause of the ESLint failure

`apps/web` had `"lint": "eslint ."` in package.json but **no ESLint config
file at all**. Under ESLint v9 (flat-config era) that aborts with
`Oops! Something went wrong! ... ESLint couldn't find an eslint.config.(js|mjs|cjs) file`
— the truncated "Oops!" error the verifier saw. It was never a violation in
the new `.tsx`/`.ts` files; the linter simply never ran.

### What was fixed

1. **Added `apps/web/eslint.config.js`** (flat config) so `eslint .` runs:
   - `@eslint/js` recommended + `typescript-eslint` recommended (TS/TSX parse).
   - Stable `react-hooks` rules only (`rules-of-hooks` error, `exhaustive-deps`
     warn) — deliberately not the experimental React-Compiler ruleset, which
     this codebase predates and which would flag legitimate pre-existing
     patterns (`window.location` redirect, form-sync `setState` in effects).
   - Node globals for build scripts/config files; `require()` allowed in
     configs (tailwind.config.ts). Vitest globals for tests;
     `no-constant-binary-expression` off in tests (the `cn()` suite
     intentionally passes `false && 'b'`).
   - New devDeps: `typescript-eslint`, `eslint-plugin-react-hooks`, `globals`.
2. **`npm run lint` → 0 problems** (was: hard error). `tsc --noEmit` clean.
   `api` `internal/http` + `internal/metadata` Go tests green.
3. **Committed + pushed.** `870ce9e0` (lint config) plus the previously
   uncommitted-then-committed step-2 UI commit `e39282df` reached
   `origin/main` (`42372591..870ce9e0`).

### Checklist verification (semantic review of `custom-fields.tsx`)

Live browser test not run in-session (vite build is WASM-OOM blocked; the new
frontend is not yet deployed). Verified by code instead:

- **Lists / creates / deletes definitions** — `useDefinitions` table +
  `CreateFieldForm` (`useCreateDefinition`, key-format validation) +
  `onDelete` (`useDeleteDefinition`) guarded by `window.confirm`.
- **Enum values** — `allowedValuesRaw` editor rendered only when
  `propertyType === 'enum'`; create blocked if the enum has no values.
- **Appears in record detail** — create/delete mutations invalidate the
  `['metadata','definitions']` prefix, the same query key
  `useContactDefinitions` / `useDealDefinitions` read, so record-detail
  editors pick up new fields with no reload.
- **Contact/Deal tabs** + API-error surfacing (`{"error":...}` body parsed;
  dup key → 409).

Endpoints (`GET/POST /v1/metadata/definitions`, `DELETE …/{id}`) match the
existing hooks and the wired `metadata.Handler` route group.
