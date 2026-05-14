---
id: 20260514-114210-9b41
title: "leCRM v0 — Test strategy and non-negotiable quality regression"
status: later
priority: p0
created: 2026-05-14
updated: 2026-05-14
category: engineering
group: lecrm-v0-sprint-3
group_order: 3
order: 2
---

## Read this cold — full context inline

Document v0 test scope explicitly so the PRD's `solo-builder-capacity-bound` flag is operationalized rather than aspirational. A solo builder cannot maintain a full Playwright E2E suite AND ship features in 11-13 weeks. This doc commits to what v0 WILL test (minimal but sufficient) and what it WON'T (no full E2E, no chaos, no performance baseline) — and which quality risks are NON-negotiable regardless of capacity.

## Why this exists

From PRD step-02 round-1 + round-2 council debates (Murat, 2026-05-13/14):

- "Multi-tenancy (schema-per-tenant) is a testability nightmare — your test harness needs tenant provisioning, teardown, and isolation verification baked in from day one, or you spend week 8 discovering cross-tenant data leakage in what you thought was a green suite."
- "v0 automation commitment must be proportional to a solo builder's throughput. That means no full Playwright E2E suite at v0 — smoke tests, contract tests on the API layer, manual exploratory on happy paths. The document should say this explicitly rather than leaving it implicit."
- "Cross-tenant isolation testing. JSONB metadata mutation regression path. Role-based access control test coverage (multi-user with roles is v0; RBAC failures are often silent). Auth token lifecycle testing given the OAuth dependency."

## Prerequisite (DOR)

- Stack decision locked (post-G1 / tasket `20260510-202450-a5d3`). The test runner / fixture library depends on Go-vs-TS choice (`go test` + `testify` vs Vitest / Bun test + sibling fixtures).
- Tenant model committed (schema-per-tenant migrations exist in scaffolding output).

## Approach

1. **Write `docs/test-strategy.md`.** Structure:
   - **In-scope (v0):** smoke tests, contract tests on REST + thin MCP API surfaces, manual exploratory checklist for happy paths.
   - **Out-of-scope (deferred):** full Playwright E2E, chaos engineering, performance / load baselines (these land in v1+).
   - **Non-negotiable regression coverage** — must exist before first DP migration:
     - **(a) Cross-tenant data isolation.** Test fixture: provision two tenants, write data in one, assert reads in the other return zero leakage. Cover every endpoint that filters by tenant. A single row leak in production ends the Léo channel permanently — this is the highest-severity test.
     - **(b) RBAC regression suite.** Multi-user mode with roles is v0. RBAC failures are often silent (user sees data they shouldn't, never reports it). Test fixture: 3+ role types per tenant, every protected endpoint tested for each role.
     - **(c) JSONB metadata-engine mutation paths** — IF ADR-010 (tasket `20260514-114217-3c84`) lands on JSONB. Concurrent mutation, schema drift detection, query correctness.
     - **(d) OAuth token lifecycle.** Token refresh, expiration, revocation. Test fixture: mock Gmail OAuth with controllable token state.
2. **Sketch the fixture architecture** (Postgres test-tenant provisioning helper, OAuth mock, role provisioning).
3. **Enumerate the minimum acceptance test list** — count of tests per non-negotiable category (e.g. "≥15 isolation tests covering all tenant-filtered endpoints").

## Done When

- [ ] `docs/test-strategy.md` committed
- [ ] In-scope vs out-of-scope clearly named
- [ ] All 4 non-negotiable regression categories specified with fixture architecture
- [ ] Minimum test count per category called out
- [ ] Cross-references: links to ADR-010 (when committed) and to G3 / G4 schedule-gate taskets

## References

- `{output_folder}/planning-artifacts/prd.md` — `solo-builder-capacity-bound` + `v0-capability-constraints` flags; Exec Summary failure mode (data isolation + RBAC paragraph)
- Tasket `20260514-114217-3c84` — ADR-010 metadata-engine commitment (this strategy adjusts to ADR-010's outcome)
- Tasket `20260510-202450-a5d3` — Wk-2 stack decision determines test runner choice
- Tasket `20260514-114238-bf09` — G4 OAuth submission (OAuth token lifecycle category depends on Gmail integration shape)
