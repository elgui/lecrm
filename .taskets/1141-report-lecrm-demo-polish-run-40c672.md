---
id: 1141
title: "[Report] lecrm-demo-polish run 40c672"
status: done
updated: 2026-05-31
done: 2026-05-31
priority: p2
created: 2026-05-31
tags: [automation, report, lecrm, demo, polish]
category: tooling
group: lecrm-demo-polish
order: 5
plan: true
---

## Report deliverable

Evidence-based report committed at
`docs/automation/automation-report-ga-20260531-40c672.md` (commit `cf57bbf6`).

### Summary

Run `ga-20260531-40c672` shipped the demo-polish slice — **4/4 tasks truly
complete**, each backed by a real commit *and* re-verified (clean `tsc`, clean
`go build`, 11/11 new unit tests via `bun test`). **0 errored / 0 blocked /
0 skipped. 0 false completions.** 1 of 3 allowed remediations injected and closed.

- **#f84f — live dashboard KPI stats** ✅ — commit `def2c1ce` (3 web files);
  `tsc` exit 0, `dashboard-stats.test.ts` green.
- **#0077 — French pipeline stage names** ✅ — commit `e411e415` (13 files,
  Go + web + migration `0020`); `go build ./apps/{api,admin,mcp}/...` exit 0,
  `format.test.ts` (+49 cases) green.
- **#37fc — Authentik login branding script** ✅ — commit `f2a3f3a9`
  (`scripts/authentik-brand-lecrm.py`, 135 lines). Labelled `partial_success`
  in-run because the script was committed but not yet executed on the live host;
  correctly remediated rather than falsely credited.
- **#1140 — [Fix] execute + verify branding** ✅ — commit `8d6c7162`; ran the
  script on `51.77.146.49`, verified title/logo/favicon/flow via API + HTML.
  Doc-only commit because the functional change lives on the live Authentik host.

### Remediations

- `#1140` (execute Authentik branding script) — **success**; closes the #37fc gap.
  Verified the live login page renders leCRM branding.

### Build status (verified in worktree)

- TypeScript: `tsc --noEmit -p tsconfig.app.json` → exit 0.
- New unit tests: `bun test format.test.ts dashboard-stats.test.ts` → 11 pass / 0 fail.
- Go: `go build ./apps/api/... ./apps/admin/... ./apps/mcp/...` → exit 0.
- Web full `vite build` / `vitest` OOM on the host 6 GB vmem cap (WASM) — env
  limitation, not a code defect; type + behavioral correctness covered by tsc + bun.
- Working tree clean; all 4 run commits present on `auto/lecrm-demo-polish-20260531`.

### Recommendations

1. Run `vite build` once on a host without the vmem cap to produce the SPA bundle.
2. Re-run `scripts/authentik-brand-lecrm.py` (idempotent) after any Authentik upgrade —
   branding is live-host state, not tracked by the repo.
3. Confirm migration `0020_french_pipeline_stages.sql` is applied to staging DB.
4. (Infra) Raise the vmem cap or standardize web CI on `bun test` so future runs get a
   clean automated signal instead of a manual workaround.
