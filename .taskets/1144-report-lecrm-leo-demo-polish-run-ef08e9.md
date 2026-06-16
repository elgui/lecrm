---
id: 1144
title: "[Report] lecrm-leo-demo-polish run ef08e9"
status: done
updated: 2026-05-31
done: 2026-05-31
priority: p2
created: 2026-05-31
tags: [automation, report, lecrm, demo, polish, leo]
category: tooling
group: lecrm-leo-demo-polish
order: 12
plan: true
---

## Report deliverable

Evidence-based report committed at
`docs/automation/automation-report-ga-20260531-ef08e9.md` (commit `8081053a`,
branch `auto/lecrm-leo-demo-polish-20260531`).

### Summary

Run `ga-20260531-ef08e9` — **3 / 11 tasks truly complete**, each backed by a real
commit *and* re-verified (clean `tsc`, clean `eslint`, test files present).
**0 false completions.** Run stalled at task 4.

- **#d396 — Contacts list company name + relationships** ✅ — commit `f6cf8501`
  (5 files, +125/−1; `use-companies.test.ts` +3 tests).
- **#04a1 — One Save per record + humanized custom-field labels** ✅ — commit
  `2b0b73cb` (13 files, +490/−173; new `record-save-bar`, `use-custom-property-form` + tests).
- **#9f97 — Reports "coming soon" honest placeholder** ✅ — commit `6ab8ad7d`
  (4 files, +180/−36; new `reports.ts` + tests). Correctly chose honesty over fake data.

### Failed / blocked

- **#45f6 (task 4) — Demo happy-path hardening** ❌ FAILED — needed a running dev
  env (PostgreSQL role `gui` missing) + interactive browser walkthrough; no commits.
  Correctly *not* marked done.
- **Tasks 5–11 (`#faa5 #5703 #f381 #1bb4 #dca9 #657d #1763`)** ⛔ cascade-blocked
  on the task-4 dependency chain; never attempted. Most are code/UI work that does
  not genuinely depend on task 4 — recommend decoupling and re-running headlessly.

### Build status (re-verified in worktree)

- TypeScript: `tsc --noEmit -p tsconfig.app.json` → exit 0.
- Lint: `eslint src` → exit 0.
- Web `vitest` OOMs under the host 6 GB vmem cap (WASM) — environmental, not a code
  defect; in-run each step reported its tests passing (69 → 79 → 85, +19 new).
- Working tree clean; all run commits present on `auto/lecrm-leo-demo-polish-20260531`.

### Recommendations

1. Re-run tasks 5–9 + 11 with the dependency chain decoupled from `#45f6` (they are
   `tsc`/`eslint`-gated UI work, not dependent on a seeded demo DB).
2. Provision the dev env (PostgreSQL `gui` role, migrations, demo seed, dev server)
   before retrying `#45f6` — it needs a headed/interactive loop.
3. Standardize web tests off the WASM/OOM path (`bun test` or raise the vmem cap)
   so future runs get a clean automated signal.
