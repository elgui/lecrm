---
id: 20260602-170533-83e0
title: [Tooling] Wire the integration suite (-tags integration) into CI
status: done
priority: p2
created: 2026-06-02
updated: 2026-06-04
tags: [lecrm, ci, integration-tests, tech-debt]
category: tooling
done: 2026-06-04
---

## Why

CI (`.github/workflows/ci.yml`) currently builds/tests only the untagged unit suite and NEVER runs `go test -tags integration ./...`. A green untagged build is NOT evidence a feature works end-to-end — this is exactly how the connector-audit prod bug (core.audit_log rejecting `actor_type='connector'`, fail-closed rollback of every connector event) and the earlier CSV-import 404 slipped past CI while unit tests stayed green.

This is run-report recommendation #3 (`docs/automation/automation-report-ga-20260601-e35442.md`).

## Done when

- [ ] A CI job (separate from the unit job, so unit feedback stays fast) runs the Docker-backed integration suite: `go test -tags integration ./internal/...` from `apps/api`.
- [ ] Uses testcontainers; Postgres bound to **127.0.0.1 only** (a prior exposed test DB was crypto-mined — never `-p 5432:5432`).
- [ ] Runs on a runner with Docker available (GitHub Actions `ubuntu-latest` has Docker).
- [ ] Merges to `main` are gated on it.

## Pointers

- Harnesses: `setupConnectorEnv`, `setupPipelineEnv`, `setupDedupEnv` in `apps/api/internal/crm/*_integration_test.go` now apply the full prod migration chain through 0023 (no 0020 — renumbered).
- Go = `/usr/local/go/bin/go`; Docker via `sg docker -c` on the staging host (CI runner differs).

## Out of scope

- The connector-audit fix itself — shipped: migration `0023_connector_audit_actor.sql`, applied to staging DB + deployed 2026-06-02 (commits a8bfc90f, c87f6a8f, 2dda1503).
