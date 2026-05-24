---
id: 20260515-192005-0f09
title: leCRM v0 — Cross-tenant isolation test fixture (≥15 tests, non-negotiable category (a)) (Sprint 3)
status: done
priority: p1
created: 2026-05-15
updated: 2026-05-18
done: 2026-05-18
tags: [sprint-3, test-infra, cross-tenant-isolation, non-negotiable]
category: engineering
group: lecrm-v0-sprint-3
group_order: 3
order: 5
plan: true
---

## Read this cold — full context inline

Sprint 3 Test infra track. Builds the cross-tenant isolation test fixture that is non-negotiable category (a) per `docs/test-strategy.md` §4.1 — provisions two tenants, writes data in one, asserts reads in the other return zero leakage. Fixture grows alongside endpoint surface across Sprints 4-10.

## Why this exists

A single row of cross-tenant data leakage in production ends the Léo channel permanently. The risk is highest in solo-dev velocity mode where regressions can slip through manual review. The fixture must exist BEFORE the bulk of CRUD endpoints land (Sprints 4-7) so each new endpoint adds its isolation test in the same PR.

`docs/test-strategy.md` §5 hard floor: ≥15 isolation tests. §4.1 fixture spec: `tenantPair(t)` helper that provisions two workspaces, returns scoped clients, and provides isolation assertions. §6 hard-stop discipline: any (a) failure means branch revert + root-cause investigation, NOT fix-forward.

## Prerequisite (DOR)

- Sprint 3 Database/Tenancy track (sibling tasket, order=3 in this group) must produce at least one tenant-filtered endpoint to test. The `/v1/_test/workspaces` handler from commit `f69d24a` is the minimum starter; production-shape endpoints land in Sprint 4 CRUD.
- Provisioning function callable from tests (provided by sibling order=3 tasket).
- ADR-010 JSONB-primary metadata pattern (committed 2026-05-15) — fixture must support both static-table and `objects`-table assertion shapes.

## Approach

### A. `tenantPair` fixture helper
1. `apps/api/internal/testfixtures/tenantpair/` package:
   - `Provision(t *testing.T) (*Tenant, *Tenant)` — calls the provisioning function via `cmd/lecrm-migrate provision-workspace` against the testcontainers Postgres; returns two `Tenant` structs (slug, role, schema, HTTP client scoped to subdomain).
   - `(*Tenant).Client()` — HTTP client preconfigured with the workspace subdomain in Host header + workspace-scoped Bearer token.
   - `(*Tenant).DB()` — pgxpool authenticated as the workspace role.
2. Cleanup: `t.Cleanup` registers schema + role drop on test exit.

### B. Isolation assertion library
1. `apps/api/internal/testfixtures/tenantpair/assertions.go`:
   - `AssertNoCrossRead(t, src, dst *Tenant, endpoint string, srcRecord any)` — writes record to src, hits the endpoint from dst's client, asserts the record is absent.
   - `AssertNoCrossList(t, src, dst *Tenant, listEndpoint string)` — populates src with N records, hits list endpoint from dst, asserts zero results.
   - `AssertNoCrossMutation(t, src, dst *Tenant, mutationEndpoint string, srcRecordID string)` — dst attempts to update/delete src's record by ID; asserts 404 (not 403, which would leak existence).

### C. Endpoint-registry guard test
1. `apps/api/internal/http/coverage_test.go`:
   - Walks the Chi router tree, extracts every route matching `/v1/*`.
   - Reads `docs/test-strategy-endpoint-registry.json` (committed sibling to test-strategy.md).
   - Fails if any route lacks an isolation+RBAC entry in the registry.
   - Cannot exist until ≥2 tenant-scoped handlers ship (per test-strategy TR-3) — wire up infra now, enforce on second handler.

### D. Minimum 15 isolation tests
The 15-floor is met by combining:
- 5 base tests using the `/v1/_test/workspaces` handler from commit `f69d24a` and any Sprint 3 auth-scoped endpoints (`/v1/me`, `/v1/sessions`, etc.).
- 10 tests added incrementally as Sprint 4 CRUD endpoints (Contact, Deal, custom-property surface) land. Each new endpoint comes with ≥1 isolation test in the same PR.

For Sprint 3 close, ≥5 base isolation tests must be green.

## Done When

- [ ] `apps/api/internal/testfixtures/tenantpair/` package implements `Provision`, `Client`, `DB`, cleanup
- [ ] Isolation assertion library implements `AssertNoCrossRead`, `AssertNoCrossList`, `AssertNoCrossMutation`
- [ ] `apps/api/internal/http/coverage_test.go` endpoint-registry guard wired (failing until second handler exists is OK at Sprint 3 close)
- [ ] ≥5 base isolation tests green against existing handlers (Sprint 4+ will grow the count to ≥15)
- [ ] CI required-status-check: integration test job blocks merge on any (a) failure per `docs/test-strategy.md` §6

## References

- `docs/test-strategy.md` §4.1 (fixture spec), §5 (≥15 hard floor), §6 (hard-stop discipline on (a) failures)
- `docs/test-strategy.md` §7 TR-3 (endpoint-registry guard prerequisites)
- ADR-009 §2.1 (provisioning function signature)
- ADR-010 (metadata-engine pattern; fixture supports the JSONB `objects`-table shape)
- PRD `solo-builder-capacity-bound` flag
- Commit `f69d24a` (workspace middleware + first tenant-filtered handler)
- Sibling Sprint 3 taskets: Database/Tenancy track (order=3), Auth track (order=4)
