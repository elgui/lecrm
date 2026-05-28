---
id: 20260528-145729-904c
title: [Report] lecrm-v1-readiness run 2c8481
status: done
priority: p1
created: 2026-05-28
updated: 2026-05-28
category: tooling
group: lecrm-v1-readiness
order: 6
---

## Report deliverable

Evidence-based report committed at `automation-report-ga-20260528-2c8481.md`.

Summary:
- Run window 2026-05-28 14:30 → 14:55 UTC.
- 1 substantive deliverable landed: ADR-004 rev2 (Go + river sequences) — commit `a24f67a8`.
- Bookkeeping commit `dd1dfabc` flipped the remediation tasket #1120 to done.
- 3 v1-readiness gate steps were skipped before they ran (v0 ship-gate verification, Brevo plan tier, v1 kickoff signal) — these need re-queueing in the next automation run.
- 0 false completions, 0 failures, 0 blocked.
- Build green across apps/api, apps/admin, apps/migrate. Working tree clean.
- Both `partial_success` verdicts in the progress JSON were caused by snapshot-before-commit timing — both files are in git on `main`.
