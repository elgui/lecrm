---
id: 20260601-182409-d1c0
title: [Fix] crm integration harness — close 4 residual failures (connector audit_type + French-stage migration)
status: done
priority: p1
created: 2026-06-01
updated: 2026-06-02
tags: [lecrm, integration-tests, migrations, connector, tech-debt]
category: tooling
done: 2026-06-02
---

# [Fix] crm integration harness — close the 4 residual failures (3 connector + 1 pipeline French-stage)

## Why this tasket exists

PR #9 (`auto/lecrm-integrator-gap-closure-20260601`) fixed the CSV-import
runtime breakage and, as a side effect, injected the missing RBAC principal
into the shared `setupPipelineEnv` harness (commit `bad1f5dd`). That took the
`internal/crm` integration suite from **28 failures → 4** — but it also
**unmasked** 4 failures that the universal `401` had been hiding. They are
pre-existing and were deliberately left out of scope for the import fix.

This tasket closes them. **At least one is likely a real production bug, not
just a test-harness gap** (see Root Cause 2) — that is the important part to
verify, not just "make the suite green".

## The 4 failing tests (run under Docker)

```
sg docker -c "/usr/local/go/bin/go -C apps/api test -tags integration -count 1 \
  -run 'TestConnector|TestPipeline_ListStages' ./internal/crm/"
```

- `TestConnector_CandidateEnriched_CreatesContactWithProperties`
- `TestConnector_InvitationClaimed_MovesDealToClosedWonAndCreatesActivity`
- `TestConnector_Idempotency_DuplicateKeyNoDuplicateEntities`
- `TestPipeline_ListStages_ReturnsSeededStagesOrdered`

## Root cause 1 — harness migration set is stale (English stages)

`setupPipelineEnv` (apps/api/internal/crm/pipeline_integration_test.go) seeds
only migrations **0001–0015**, which provision **English** pipeline-stage
labels (`Discovery`, `Qualified`, …). But:

- `connectors.go` hardcodes the **French** canonical name
  (`stageDiscovery = "Découverte"`, apps/api/internal/crm/connectors.go:66) and
  `resolveStage` fails with `pipeline stage not found: Découverte`.
- `TestPipeline_ListStages_ReturnsSeededStagesOrdered` asserts the French
  labels (`Découverte`, `Qualifié`, `Proposition envoyée`, `Négociation`,
  `Gagné / Perdu`).

Both need migration **`0021_french_pipeline_stages.sql`**, which re-defines the
provisioning function to seed French labels. `0021` depends on
`core.lecrm_grant_app_role` (added in `0017`) and the `lecrm_provisioner` role,
so the harness must apply the in-order chain **0016, 0017, 0018, 0019, 0021**
(there is no 0020 — it was renumbered; see the header comment in 0021).

## Root cause 2 — connector audit actor_type rejected by core.audit_log CHECK (LIKELY A PROD BUG)

Once 0021 is applied, the connector tests get past the stage lookup and fail at
the audit write:

```
audit insert connector.contact.enriched:
  new row for relation "audit_log" violates check constraint
  "audit_log_actor_type_check" (SQLSTATE 23514)
```

`core.audit_log.actor_type` CHECK is defined in `0001_init.sql`
(`human_api | mcp_agent | internal_service | system`) and amended by
`0019_integrator_audit_actor.sql` to add `integrator` — but **never `connector`**.
Meanwhile the connector event path writes audit rows with
`actor_type = 'connector'` (the per-workspace activities tables in 0015/0022 DO
allow 'connector', but the central `core.audit_log` does not).

**Investigate first whether production hits this same rejection.** If the
connector audit path writes to `core.audit_log` in prod, connector-driven
events (chatboting candidate enrichment, invitation claims) are silently
failing/rolling back in production too. Resolution is one of:

1. Add a migration extending `core.audit_log_actor_type_check` to include
   `'connector'` (mirror the 0019 DROP IF EXISTS + ADD pattern). Preferred if
   'connector' is a legitimate central-audit actor.
2. OR map connector audit writes to an already-allowed actor_type
   (e.g. `internal_service`) if 'connector' should not appear in core audit.

Pick based on ADR-007 (audit) / ADR-009 §7.2 (fail-closed) intent and what the
per-workspace vs. central audit distinction is meant to encode. Whichever path,
the production migration set — not just the test harness — must carry the fix.

## Done when

- [ ] Root cause 2 is diagnosed as prod-bug vs. test-only, with a one-paragraph
      finding in the commit message / PR (cite where the connector audit row is
      written and which table's CHECK rejects it).
- [ ] The chosen `core.audit_log` actor_type fix lands as a real migration
      under `packages/db/migrations/` (so production is fixed, not just tests).
- [ ] `setupPipelineEnv` applies the full in-order migration chain through 0021
      so its seeded data matches production (French stages).
- [ ] `sg docker -c "/usr/local/go/bin/go -C apps/api test -tags integration -count 1 ./internal/crm/"`
      → **0 failures** (all of TestConnector_*, TestPipeline_*, TestImport_*,
      TestDedup_*, TestANT_*, TestExport_*, TestAuditIdempotency_* green).
- [ ] No regression to the suites already green on this branch (import 7/7,
      ANT 6/6, pipeline transitions, dedup, export, audit).
- [ ] `go build ./...` + `go vet -tags integration ./internal/crm/` clean.

## Pointers

- Harness + RBAC injection precedent: commit `82844ade` (dedup harness) and
  `bad1f5dd` (pipeline harness, this branch).
- Migration ordering note: header of `packages/db/migrations/0021_french_pipeline_stages.sql`.
- actor_type CHECK history: `0001_init.sql:82`, `0019_integrator_audit_actor.sql:39-42`,
  per-workspace activities tables `0015_activities_notes_tasks.sql:302`,
  `0022_dedup_no_merge_rules.sql:295`.
- Connector stage constant: `apps/api/internal/crm/connectors.go:66`.

## Out of scope

- The CSV-import fix itself (already shipped in `bad1f5dd`).
- Any web/frontend change — these are backend integration + migration concerns.
