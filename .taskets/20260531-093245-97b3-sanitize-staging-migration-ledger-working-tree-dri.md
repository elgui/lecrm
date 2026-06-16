---
id: 20260531-093245-97b3
title: Sanitize staging migration ledger + working-tree drift after PR #7 deploy
status: done
priority: p2
created: 2026-05-31
updated: 2026-05-31
tags: [tech-debt, migrations, deploy, hygiene]
category: engineering
done: 2026-05-31
---

## Context

PR #7 (`auto/lecrm-demo-polish`, merge commit `457715c5`) was merged and deployed to the live demo (demo.lecrm.gbconsult.me, host `vps-25b8e3b3` / 51.77.146.49) on 2026-05-31. During the deploy, several **pre-existing** inconsistencies were surfaced and worked around but NOT fully resolved. This tasket tracks cleaning them up so the staging DB and the deploy-source checkout are reproducible and a future `migrate apply` is safe.

## What needs sanitizing

### 1. Staging `core.schema_migrations` ledger is behind
It records through `0017_app_role.sql` plus `0021_french_pipeline_stages.sql` (inserted manually after applying 0021 via psql during the deploy). `0018_integrator_role_and_grants.sql` and `0019_integrator_audit_actor.sql` are NOT recorded — the automation run applied things via direct data-fixes instead of the migrate-runner.
**Risk:** a future blanket `lecrm-migrate apply` (migrate.yml mounts the host migrations dir) would attempt to (re)apply 0018, 0019, AND the untracked `0020_restore_registry_input_validation.sql`.
**Action:** verify whether 0018/0019 are actually applied to the staging schema; if applied, backfill `schema_migrations` to match reality; if not, apply them properly via the runner.

### 2. Untracked migration `0020_restore_registry_input_validation.sql`
Still sitting uncommitted in `packages/db/migrations/` on the deploy host. It restores the input-validation guards that 0017 dropped. **Note:** `0021_french_pipeline_stages.sql` (now merged + applied to staging) ALREADY carries those same guards, so the function on staging is now guarded (verified `has_slug_guard=t`).
**Action:** decide its fate — either commit it (numbered `<= 0020` so 0021 stays the last definition of `core.lecrm_provision_workspace_with_registry`) or drop it as superseded by 0021. Coordinate with its author.

### 3. Working-tree drift on the live deploy host (`/home/gui/Projects/leCRM`)
5 modified tracked `.taskets/` files plus several untracked files (taskets, `deploy/leo-demo-invite.eml/.html`, the restore migration). None are build-relevant and they were intentionally left untouched (not ours to revert per CLAUDE.md), but the tree should eventually be reconciled so it is clean for reproducible deploys.

### 4. Pre-existing flaky test
`apps/admin/internal/tenant` `TestCreateFresh` intermittently fails with `tuple concurrently updated (SQLSTATE XX000)` because CI runs admin test packages in parallel against one shared Postgres and concurrent provisioning calls race on shared role/catalog tuples.
**Action:** serialize migration/provisioning in the test harness or isolate the DB per package.

## Done When
- [x] staging `core.schema_migrations` reflects reality for 0018/0019 (verified applied, or applied + recorded)
- [x] `0020_restore_registry_input_validation.sql` is either committed (`<= 0020`) or removed as superseded by 0021, with the decision recorded
- [ ] deploy-host working tree reconciled to a clean state (drift committed or cleared by its owner)
- [x] `TestCreateFresh` flakiness fixed or a tracked follow-up filed (follow-up filed: `20260531-095802-6a34-fix-testcreatefresh-flakiness-serialize-provisioni.md`)

## Resolution Log — 2026-05-31 (session on `vps-25b8e3b3`)

**Items #1 + #2 DONE. Items #3 (tree drift) and #4 (flaky test) deferred as tracked follow-ups (scoping decision: do the reproducibility-critical DB work on-host now).**

### Item #1 — ledger vs reality (one assumption corrected)
The tasket assumed 0018/0019 were applied "via direct data-fixes" but just not recorded.
**That was wrong** — they were **not applied at all**. Pre-session probe of the live staging schema:
- `core.integrator_grants` — **MISSING**
- `workspace_members_role_check` — did NOT admit `'integrator'`
- `audit_log_actor_type_check` — did NOT admit `'integrator'`

So the fix was to **apply** them (not backfill the ledger). Done via the **real migrate-runner**
(`apps/migrate/cmd/lecrm-migrate apply`, the canonical codepath — `migrator.Apply`), built from
source and pointed at the staging DB (`127.0.0.1:54320`, superuser DSN — the privilege level this
host has always applied migrations at; `lecrm_provisioner` owns the core tables so end-state
ownership is identical either way). Result: `applied: 2, failures: 0` (0018, 0019); 0021 correctly
skipped (already recorded → **not clobbered**).

Post-apply verification (all green):
- ledger = `0001..0019, 0021` (matches migrations dir exactly)
- `core.integrator_grants` exists, owned by `lecrm_provisioner`, `lecrm_api` has SELECT
- both CHECK constraints now admit `'integrator'`
- provision fn retains French labels (`Découverte`) **and** slug guard → 0021 intact
- **re-run `apply` = `applied: 0`** → a future blanket `lecrm-migrate apply` is now a clean no-op

### Item #2 — fate of `0020_restore_registry_input_validation.sql`: **DROPPED as superseded**
Decision: **drop** (deleted the untracked file from `packages/db/migrations/`). Rationale:
- It was **never committed** to git (commit `cb760dfc` renamed the french migration `0020→0021` and
  folded the same guards into 0021, making 0020_restore redundant).
- 0021 fully supersedes it: same input-validation guards **+** French pipeline labels.
- It was an active **landmine**: it carried the *old English* labels and `CREATE OR REPLACE`s the
  provision fn, so a future runner (which would have applied `0018→0019→0020`, 0021 already recorded
  → skipped) would have silently reverted the demo pipeline to English. Removing it eliminates that.

### Still open
- **#3 working-tree drift** — left as-is per CLAUDE.md ("don't revert drift you didn't create"); the
  only change *this* session made to the tree is the `rm` of the untracked 0020 file (intended).
- **#4 `TestCreateFresh` flakiness** — not fixed here; **follow-up tasket filed**
  `20260531-095802-6a34` (root cause: parallel-package `go test` against one shared CI Postgres
  races on catalog DDL → XX000; fix options: `-p 1`, xact advisory lock, or DB-per-package).

## References
- Merge commit `457715c5` (PR #7); migration `packages/db/migrations/0021_french_pipeline_stages.sql`
- `deploy/compose/migrate.yml` (migrate-runner mounts host migrations dir); `apps/migrate/internal/migrator/migrator.go` (applies `*.sql` in filename order, tracks by name)
- `CLAUDE.md`: working-tree-drift + "this checkout is the deploy source" guardrails
