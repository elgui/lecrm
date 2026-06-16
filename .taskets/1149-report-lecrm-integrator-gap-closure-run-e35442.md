---
id: 1149
title: "[Report] lecrm-integrator-gap-closure run e35442"
status: done
updated: 2026-06-01
done: 2026-06-01
priority: p2
created: 2026-06-01
tags: [automation, report, lecrm, integrator-gap]
category: tooling
group: lecrm-integrator-gap-closure
order: 4
plan: true
---

## Report deliverable

Evidence-based report committed at
`docs/automation/automation-report-ga-20260601-e35442.md` (commit `c461059e`,
branch `auto/lecrm-integrator-gap-closure-20260601`). Supersedes the 12:05
mid-run draft (`f48f612d`).

### Summary

Run `ga-20260601-e35442` — **2 / 3 feature steps truly complete**, each backed by
a real commit AND re-verified (including the build-tagged integration tests the
per-step verifiers never actually ran).

- **Step 1 — report engine** (`cbb18013`): ✅ done. `go build` + Go unit tests +
  `tsc --noEmit` all clean.
- **Step 3 — dedup / record merge** (`b8a25224` + fix `82844ade`): ✅ done.
  Dedup integration suite (`-tags integration`, real testcontainers PG) **passes**
  — exact-email match, relation re-pointing, no-merge rule, audit, cross-tenant.
- **Step 2 — CSV import** (`a4b8fb7a`): ❌ **false completion / runtime-broken.**
  Compiles and unit-tests green, but **7/7 import integration tests FAIL**: routes
  are registered as static literals (`/v1/import/contacts/analyze`, …) while the
  handler reads `chi.URLParam(r, "entity")` — always `""` — so every
  analyze/preview/commit returns `404 unknown import entity`. Dead end-to-end (the
  React wizard hits the same paths). The verifier's "integration tests verified"
  claim was false; those tests are gated behind `//go:build integration` and were
  never executed.

### Key correction vs. run metadata

Both signals were wrong: the task-body stats (`1 done / 2 blocked`) were **stale**
(pre-remediation), and the per-step verifiers were **over-optimistic** on step 2
(green untagged build ≠ working feature). Truth: 2/3 done, 1/3 broken.

### Recommendations (in report §6)

1. Fix CSV import: declare the `{entity}` route param the handler already reads
   (~3 lines in `handlers.go`); re-run `go test -tags integration -run TestImport`.
2. Fix the secondary harness 401 (inject RBAC principal into `pipelineTestEnv`,
   like `82844ade` did for the dedup harness).
3. Add `go test -tags integration` to the per-step verification gate — both false
   signals trace to it never being run.
4. Re-queue CSV import as a `[Fix]` task; steps 1 & 3 need no rework.

### Build status at report time

`go build ./...` EXIT 0 · Go unit tests pass (crm 99) · `tsc --noEmit` EXIT 0 ·
dedup integration PASS · import integration 7/7 FAIL · web `vitest`
environment-blocked (WASM OOM on host, not a code defect).
