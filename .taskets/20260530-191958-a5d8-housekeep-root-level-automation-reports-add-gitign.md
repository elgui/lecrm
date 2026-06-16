---
id: 20260530-191958-a5d8
title: Housekeep root-level automation reports + add gitignore guard
status: done
priority: p3
created: 2026-05-30
updated: 2026-05-30
tags: [housekeeping, automation, repo-hygiene]
category: tooling
done: 2026-05-30
---

## Goal
The repository root has accumulated ~15 committed `automation-report-ga-*.md` files from past Group Automator runs. PR #6 (merged as `0da561b3`) established the right convention by relocating its own run report to `docs/automation/`, but the pre-existing reports were never moved. Clean them up and prevent recurrence.

## Background
- PR #6 moved `automation-report-ga-20260530-dcfb6a.md` → `docs/automation/`.
- Still at repo root: `automation-report-ga-20260510-d4a8ff.md` through `automation-report-ga-20260530-9c5aa0.md` (~15 files).
- These are process artifacts, not source; they clutter the root and rot over time.

## Scope / Acceptance criteria
- [ ] `git mv` all remaining root-level `automation-report-ga-*.md` files into `docs/automation/` (match PR #6's convention).
- [ ] Prevent recurrence: either add a `.gitignore` rule for root-level `automation-report-ga-*.md` AND/OR fix the Group Automator so it writes future run reports under `docs/automation/` directly. Prefer fixing the source (automator) over ignoring.
- [ ] `grep -rn 'automation-report-ga' --include='*.md' --include='*.go' --include='*.json'` to confirm nothing references the old root paths after the move.
- [ ] Working tree clean; no code behavior change.

## Notes
- Low priority, pure repo hygiene. Safe to batch with other housekeeping.
- Out of scope: the separate finding that `main` CI is currently red on pre-existing golangci-lint debt + `definer_hardening`/`tombstone` integration tests (track separately if desired).

## References
- `docs/automation/` — destination dir, convention set by PR #6
- merge commit `0da561b3` — PR #6 (lecrm-integrator-switching)
