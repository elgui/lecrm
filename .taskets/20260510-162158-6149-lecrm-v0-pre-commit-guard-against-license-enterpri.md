---
id: 20260510-162158-6149
title: leCRM v0 — Pre-commit guard against @license Enterprise modifications
status: deleted
priority: p2
created: 2026-05-10
updated: 2026-05-10
tags: [compliance, tooling, v0]
category: project
review: "superseded-by: docs/adr/ADR-008-clean-room-reimplementation.md + docs/adr/ADR-009-stack-and-license.md. Twenty-fork-specific work; no analogue under clean-room Apache 2.0 build. Cleared by housekeeping in tasket 20260510-202450-b844 Part A on 2026-05-10."
group: lecrm-v0-build
order: 8
plan: true
---

# leCRM v0 — Pre-commit guard

## Why this tasket exists
`packages/twenty-server/src/engine/gbconsult/ENTERPRISE_FILES.list` enumerates 297 upstream files marked `@license Enterprise`. ADR-002 TO RESOLVE item 2 specifies a pre-commit hook that fails any commit modifying a file on that list (or adding a new `@license Enterprise` header outside upstream-rebase commits). The guard defends against accidental redistribution of commercial-licensed code under our AGPL terms.

## Done criteria
- [ ] `.husky/pre-commit` (or equivalent) script that:
    - Reads `packages/twenty-server/src/engine/gbconsult/ENTERPRISE_FILES.list`.
    - Diffs the staged file list against the inventory.
    - Fails if any staged file path appears in the list AND the commit is not tagged with `[upstream-rebase]` (or similar marker).
- [ ] CI workflow that re-checks the same invariant on every PR (defence-in-depth).
- [ ] Inventory-regeneration script (`scripts/regenerate-enterprise-list.sh`) included in the upstream-rebase runbook.
