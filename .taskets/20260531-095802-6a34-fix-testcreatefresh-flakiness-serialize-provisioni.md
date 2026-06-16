---
id: 20260531-095802-6a34
title: Fix TestCreateFresh flakiness — serialize provisioning in admin integration suite
status: done
priority: p2
created: 2026-05-31
updated: 2026-05-31
tags: [tech-debt, testing, ci, flaky-test]
category: engineering
done: 2026-05-31
---

Follow-up split out from tasket 20260531-093245-97b3 (sanitize-staging-migration-ledger) item #4. The DB-ledger items of that tasket are done and verified; this is the remaining deferred item.

## Problem
`apps/admin/internal/tenant` `TestCreateFresh` (apps/admin/internal/tenant/create_test.go:20) intermittently fails with `tuple concurrently updated (SQLSTATE XX000)`.

## Root cause
CI runs `go test -race -count=1 -timeout 5m -tags=integration ./...` for apps/admin (.github/workflows/ci.yml:111) against a SINGLE shared postgres:16 service container (DSN `postgres://postgres:postgres@localhost:5432/postgres`, ci.yml:86). `go test ./...` runs packages in parallel by default, and several integration test packages each call the provisioning path concurrently. Provisioning does shared-catalog DDL (CREATE ROLE, schema creation via `core.lecrm_provision_workspace_with_registry`), so two concurrent provisions race on shared role/catalog tuples → XX000 `tuple concurrently updated`. This is a test-harness concurrency bug, not a product bug.

## Fix options (pick one)
1. **Simplest:** run the admin integration suite with `-p 1` (serialize package execution) in ci.yml. Small CI slowdown, zero code change.
2. **Keep parallelism:** wrap the provisioning DDL in the test harness (apps/admin/internal/tenant/testhelper_test.go `newConn`/`ensureMigrationsApplied` + the provision call) with an xact advisory lock (`pg_advisory_xact_lock`) so concurrent provisions serialize on catalog mutation while packages still run in parallel.
3. **Heavier:** isolate a database per test package (CREATE DATABASE per package / template DB) so there is no shared catalog to race on.

Recommendation: option 1 for an immediate green CI; option 2 if preserving package-level parallelism matters.

## Done when
- TestCreateFresh (and the sibling tenant integration tests) no longer flake under the CI parallel-package run — verified by repeated runs (e.g. `-count=5` or several CI runs) showing no XX000.
- The chosen approach is documented in the test harness or ci.yml with a one-line why.

## References
- apps/admin/internal/tenant/create_test.go:20 (TestCreateFresh)
- apps/admin/internal/tenant/testhelper_test.go (shared-DB harness, ensureMigrationsApplied)
- .github/workflows/ci.yml:86,107-111 (shared postgres service + go test invocation)
- Parent tasket: .taskets/20260531-093245-97b3-sanitize-staging-migration-ledger-working-tree-dri.md (item #4)
