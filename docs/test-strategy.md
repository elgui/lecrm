# leCRM v0 Test Strategy

**Status:** Accepted
**Date:** 2026-05-14
**Owner:** Guillaume
**Tasket:** `20260514-114210-9b41` (lecrm-v0-sprint-3)
**Related:** [ADR-009 §1, §9](adr/ADR-009-stack-and-license.md) — stack and schedule gates; [ADR-001](adr/ADR-001-tenancy-model.md) — schema-per-tenant; tasket `20260514-114217-3c84` — ADR-010 (metadata-engine pattern); tasket `20260514-114245-d3a8` — G3 metadata-engine scope verify; tasket `20260514-114238-bf09` — G4 Google OAuth submission; tasket `20260510-202450-a5d3` — G1 stack decision (CONTINUE Go, locked 2026-05-14)

> This document operationalises the PRD flags **`solo-builder-capacity-bound`** and **`v0-capability-constraints`** ([prd.md L45/L48, L224/L227](../{output_folder}/planning-artifacts/prd.md)). It commits to what v0 WILL test (minimal but sufficient) and what it WON'T (no full E2E, no chaos, no performance baseline) — and which quality risks are NON-negotiable regardless of capacity.

---

## 1. Why this exists

From PRD step-02 council debates (Murat, 2026-05-13/14):

- **Multi-tenancy is a testability nightmare.** "Your test harness needs tenant provisioning, teardown, and isolation verification baked in from day one, or you spend week 8 discovering cross-tenant data leakage in what you thought was a green suite."
- **Automation must match throughput.** "v0 automation commitment must be proportional to a solo builder's throughput. That means no full Playwright E2E suite at v0 — smoke tests, contract tests on the API layer, manual exploratory on happy paths."
- **Non-negotiable categories.** "Cross-tenant isolation testing. JSONB metadata mutation regression path. Role-based access control test coverage (multi-user with roles is v0; RBAC failures are often silent). Auth token lifecycle testing given the OAuth dependency."

The PRD Executive Summary names data isolation + RBAC failures explicitly as the highest-severity failure modes: **"a single row leak in production ends the Léo channel permanently"** ([prd.md L200](../{output_folder}/planning-artifacts/prd.md)). Everything in §4 below is calibrated to that risk.

---

## 2. Stack-bound assumptions (locked)

G1 closed 2026-05-14 with **CONTINUE Go** (commit `c476737`). This strategy is written against the Go-default branch of `docs/sprint-plan.md`. Test stack:

| Concern | Tool |
|---|---|
| Unit + handler tests | `go test` + `testify` (in-repo, sibling `_test.go` files) |
| Integration (real Postgres) | `go test -tags integration` + `testcontainers-go/modules/postgres` |
| Contract tests (REST OpenAPI) | `schemathesis` against the running binary OR hand-rolled Go HTTP table tests (decide at Sprint 9, default to hand-rolled if Schemathesis schema-gen friction exceeds 1 day) |
| Contract tests (MCP) | Hand-rolled Go table tests against the MCP server; `mark3labs/mcp-go` test harness |
| Linters / static | `golangci-lint`, `go vet`, `gosec`, `govulncheck` (per ADR-009 §8) |
| Mocks (OAuth) | Local Go HTTP server with controllable token state (no third-party mocks) |

**If G1 had failed** and the project had switched to TypeScript + Hono, the substitutions named in `docs/sprint-plan.md` L63 apply (`vitest` + `testcontainers-node` + `nock`). This document does not maintain a parallel TS test-stack catalogue; the four non-negotiable categories in §4 are stack-agnostic and re-express themselves trivially.

---

## 3. Scope: in vs out

### 3.1 In-scope for v0

| Layer | What | Cadence |
|---|---|---|
| **Smoke tests** | Boot the binary, hit `/healthz`, hit one tenant-scoped endpoint per protected resource (contacts, deals, users, OAuth tokens), assert 200 + non-empty workspace context propagated | Every CI run; pre-deploy gate |
| **Contract tests on REST** | OpenAPI schema → property-based requests against the running binary; assert status codes + shape | Every PR touching `internal/http` or any handler |
| **Contract tests on the thin MCP surface** | Each `mcp-go` tool definition gets a positive + a negative test (auth missing, wrong workspace) | Every PR touching the MCP server |
| **Unit / handler tests** | Standard Go table tests for any non-trivial function, especially middleware, query builders, and OAuth state machines | Every PR |
| **Integration tests (real Postgres via testcontainers)** | The four non-negotiable regression categories in §4 | Every PR; nightly full run |
| **Manual exploratory checklist for happy paths** | One human pass per Design-Partner-facing milestone (Sprints 6, 9, 12); checklist in `docs/test-strategy-manual-checklist.md` (created when the first happy-path lands) | Sprint demo gate |
| **Backup restore-test** | Provision tenant → write data → restore from backup to sibling tenant → diff. Lands Sprint 12 ([sprint-plan L212](sprint-plan.md)) | Pre-DP cutover; quarterly |

### 3.2 Out-of-scope for v0 (deferred to v1+)

| Category | Why deferred |
|---|---|
| **Full Playwright / browser E2E suite** | Solo-builder capacity bound. A live browser suite costs ~1 day/week of maintenance at the scale leCRM will reach by Wk 8. The manual exploratory checklist + contract-tested REST/MCP surfaces cover the same failure modes at ~5% of the maintenance cost. |
| **Chaos engineering / fault injection** | No SLA promised at v0 that chaos testing would protect. Defer to v1 when DR/HA matters commercially. |
| **Performance / load baselines** | v0 targets 1-4 design partners on Phase-1 single-VPS-per-tenant ([ADR-001](adr/ADR-001-tenancy-model.md)). Capacity is far above expected load. Performance testing lands in v1 ahead of Phase-2 consolidation. |
| **Mutation testing** | Out-of-budget. Re-evaluate at v1. |
| **Frontend component unit tests beyond smoke** | The embedded React SPA gets a smoke + manual exploratory; component unit tests would consume capacity disproportionate to v0 frontend complexity. |

**Trade-off named honestly.** Deferring full E2E means a class of regression — frontend-to-API contract drift that survives both the REST contract tests AND the manual checklist — will land in production. The mitigation is (i) keeping the manual checklist short and run every demo, (ii) Sprint-12 Design-Partner pre-cutover deep manual pass, (iii) v1 sprint 1 schedules adding Playwright on the top 5 user journeys.

---

## 4. Non-negotiable regression categories

Each of (a)-(d) MUST exist before the first Design Partner migrates production data ([sprint-plan Sprint 11-12](sprint-plan.md)). These are not deferrable on capacity grounds.

### 4.1 (a) Cross-tenant data isolation — **highest severity**

**Risk model.** Two workspaces share a Postgres cluster from Phase 2 onward ([ADR-001](adr/ADR-001-tenancy-model.md), [ADR-009 §2](adr/ADR-009-stack-and-license.md)). Schema-per-tenant is enforced by `ALTER ROLE workspace_<id> SET search_path = workspace_<id>, public` at provisioning. A single row visible across tenants ends the Léo channel permanently (PRD Exec Summary).

**Fixture architecture.**

```
testfixtures/tenantpair
├── Provision(t *testing.T) (*Tenant, *Tenant)
│     uses testcontainers-go Postgres,
│     applies packages/db/migrations,
│     calls core.lecrm_provision_workspace(uuid) twice,
│     returns role-bound *pgxpool.Pool for each tenant
├── Seed(t *Tenant, fixture string) error
│     loads YAML fixtures (contacts, deals, users) into tenant's schema
└── AssertIsolated(t *testing.T, src, other *Tenant)
      runs every registered tenant-filtered endpoint as `other`,
      asserts zero rows from src.* visible
```

The helper lives in `apps/api/internal/testfixtures/tenantpair/` and is shared by every package that introduces a tenant-filtered endpoint. The helper SHOULD be the first piece of test infrastructure to land — sprint-plan Sprint 7 commits to it ([sprint-plan L107](sprint-plan.md)).

**Endpoint registry.** A package-level `var TenantScopedEndpoints []EndpointTest` slice that every handler package contributes to in its `init()`. The fixture iterates this slice; new tenant-scoped endpoints landing without a registry entry cause a CI guard test in `apps/api/internal/http/coverage_test.go` to fail.

**Minimum test count.** **≥15 isolation tests.** One per tenant-filtered endpoint at v0 minimum surface (contacts list/get/create/update/delete, deals list/get/create/update/delete, users list/get, OAuth tokens list/revoke, custom-fields read/write, sequences list/get) — and the count grows automatically through the endpoint registry. A green run requires `len(registry) == len(testedEndpoints)`.

**What "tested" means here.** For each endpoint: (i) authenticate as workspace A, write known row; (ii) authenticate as workspace B, hit the same endpoint with workspace A's resource IDs; (iii) assert 404 (not 403 — 403 leaks the existence of the resource); (iv) assert workspace A's logs do NOT show a query result row count > 0 for the workspace-B caller.

**Postgres-role assertion.** A separate test in `tenantpair` asserts that workspace B's connection cannot `SELECT FROM workspace_a.contacts` even with raw SQL — guarding against the application bug where someone writes a query without `WHERE workspace_id` and accidentally works only because `search_path` happens to be set correctly. The role-grant assertion catches this without depending on the application code being correct.

### 4.2 (b) RBAC regression suite

**Risk model.** Multi-user with roles is v0 ([ADR-009 §7](adr/ADR-009-stack-and-license.md), `core.workspace_members.role IN ('owner','admin','member')`). RBAC failures are silent — a `member` who sees `admin`-only billing data never reports it. The bug surfaces during the next sales call ("your tool leaks our pricing to all our employees"). RBAC matters because the Léo channel signs SMBs who give office-wide access; the difference between admin and member is the difference between a tool people use and a tool people uninstall.

**Fixture architecture.**

```
testfixtures/rbac
├── Provision(t *testing.T, ws *Tenant) RoleSet
│     creates owner, admin, member users
│     returns three *http.Client values each pre-authenticated
└── AssertForbidden(t *testing.T, client *http.Client, req *http.Request)
      asserts 403 with the canonical error body shape
```

**Endpoint-role matrix.** Same registry pattern as isolation. Each protected endpoint declares an `RBACPolicy` (map from role → expected status). The fixture iterates the matrix; missing matrix rows fail CI via `coverage_test.go`.

**Minimum test count.** **≥30 RBAC tests.** Three roles × every protected endpoint (~10 at v0 surface) ≈ 30 cases. The number grows with the surface.

**Two non-obvious cases that MUST be covered.**
1. **Role escalation via PATCH.** A `member` PATCHes their own `workspace_members.role` to `owner`. Must 403.
2. **Role downgrade by non-owner.** An `admin` downgrades the `owner` to `member`. Must 403. (Sounds obvious; failure mode is a frontend that forgets to gate the dropdown, and the backend that trusts the request.)

**Where this lands.** RBAC scaffold begins Sprint 7 ([sprint-plan L119](sprint-plan.md)); the full suite hardens Sprint 9 alongside the feature ([sprint-plan L170](sprint-plan.md)).

### 4.3 (c) JSONB metadata-engine mutation paths — **conditional on ADR-010 outcome**

**Conditionality.** ADR-010 (tasket `20260514-114217-3c84`) commits at Wk 4-5 to either:
- **DDL primary path** (per-tenant `ALTER TABLE` to add custom properties), OR
- **JSONB fallback** (`custom_fields JSONB` column).

If ADR-010 lands on **DDL**, category (c) becomes lightweight (a schema-drift test per tenant, a query-correctness test for the generated SQL) — still required but mechanically simple.

If ADR-010 lands on **JSONB** — which the PRD framing names as the load-bearing-through-v1 outcome ([prd.md L196-197](../{output_folder}/planning-artifacts/prd.md)) — category (c) is non-negotiable and load-bearing for v0 quality.

**Risk model under JSONB.** Three classes of failure:
1. **Concurrent mutation:** two HTTP requests update different keys in the same JSONB row; last-write-wins overwrites a sibling key. Failure: silent data loss.
2. **Schema drift:** a field renamed in the metadata engine leaves stale keys in existing rows. Failure: query-by-new-name returns nothing.
3. **Query correctness:** a `WHERE custom_fields->>'priority' = 'high'` index-scan vs seq-scan plan flips on data shape. Failure: linear-scan latency under load.

**Fixture architecture (JSONB branch).**

```
testfixtures/metadata
├── ConcurrentMutationHarness(t *testing.T, ws *Tenant, n int)
│     spawns n goroutines each writing a different custom_fields key
│     to the same row; asserts all n keys present at end
├── SchemaDriftReplay(t *testing.T, ws *Tenant, oldFixture, newFixture string)
│     loads oldFixture, runs the rename migration, asserts queries by
│     newFixture's key return the expected rows
└── QueryPlanGuard(t *testing.T, ws *Tenant, query string, maxRowsScanned int64)
      runs EXPLAIN ANALYZE; fails if rows_scanned exceeds maxRowsScanned
```

**Minimum test count (JSONB branch).** **≥8 metadata-engine tests:**
- ≥3 concurrent-mutation cases (2 writers / 5 writers / 20 writers).
- ≥3 schema-drift cases (rename, type change, deletion-then-recreation).
- ≥2 query-plan-guard cases (one indexed query, one non-indexed but bounded-cardinality query).

**Minimum test count (DDL branch).** **≥3 tests** — schema-drift per tenant, generated-SQL correctness, locking-under-concurrent-reads (uses Postgres `pg_stat_activity` to detect AccessExclusiveLock contention).

**Where this lands.** Sprint 8 if JSONB is chosen ([sprint-plan L130](sprint-plan.md)).

### 4.4 (d) OAuth token lifecycle

**Risk model.** Gmail OAuth is the single biggest external dependency in v0 ([G4 tasket](.taskets/20260514-114238-bf09)). Token refresh failures, expired refresh tokens, and revocations are the kind of failure that surfaces as silent sync drift — emails stop arriving without a clear error. The user blames the CRM; the CRM blames Google; the truth is the refresh path silently failed two weeks ago.

**Fixture architecture.**

```
testfixtures/oauthmock
├── Start(t *testing.T) *MockServer
│     boots a local HTTP server that mimics the Google OAuth token endpoint
│     with controllable state (valid / expired / revoked / network-error)
├── (*MockServer).SetTokenState(state TokenState)
└── (*MockServer).AssertCalled(t *testing.T, endpoint string, n int)
```

The mock binds to `127.0.0.1:0` and the test config points the OAuth client at `mock.URL` instead of `https://oauth2.googleapis.com`.

**Minimum test count.** **≥10 OAuth lifecycle tests:**
- Refresh success path (token expires → refresh succeeds → next call works).
- Refresh failure: refresh token revoked → user prompted to re-auth (specific error code, not a 500).
- Refresh failure: network error → backoff + retry (assert backoff schedule).
- Refresh failure: clock skew → assert the system uses the `expires_at` from the token response, not local clock arithmetic.
- Concurrent refresh: two goroutines hit an expired token simultaneously; only ONE refresh request to the mock (assert via `AssertCalled` count == 1).
- Token revocation propagation: revoke via mock → next API call returns the canonical "re-auth required" error.
- Initial OAuth flow: state parameter round-trip, PKCE verifier validation.
- Scope mismatch: token returned with fewer scopes than requested → reject + re-prompt.
- Expired-at-issue: token returns `expires_in` ≤ 0 → reject the response (defends against a malicious mock TLS-intercepting proxy).
- Refresh-token rotation: Google rotates refresh tokens on use ([Google OAuth docs](https://developers.google.com/identity/protocols/oauth2)); assert the new refresh token is persisted, not the old one.

**Where this lands.** Sprint 10 ([sprint-plan L198](sprint-plan.md)) alongside the Gmail sync feature, gated by G4 production-review approval.

---

## 5. Coverage and reporting

**No line-coverage target at v0.** A line-coverage gate at the solo-builder scale creates incentive to write thin tests that exercise lines without exercising behaviour. The four non-negotiable categories above are behaviour-coverage gates; they are the budget.

**What CI reports per PR.**
- Pass/fail count per non-negotiable category, with a hard floor (≥15 isolation, ≥30 RBAC, ≥8 or ≥3 metadata depending on ADR-010, ≥10 OAuth).
- Endpoint-registry diff: any new tenant-filtered endpoint without a corresponding isolation + RBAC entry fails CI.

**Nightly run.** Full integration tests against a Postgres testcontainer with seeded data scaled to ~3x design-partner sizing (≈30k contacts, ≈10k deals). Surfaces query-plan regressions before they hit production.

---

## 6. What happens when a non-negotiable test fails in CI

Not a "fix-forward to green CI" event. The branch does not merge. Specifically:

| Category | If a test fails in CI |
|---|---|
| (a) isolation | **HARD STOP.** Branch is reverted, root cause investigated, fix lands as its own PR with a regression test that reproduces the original failure. |
| (b) RBAC | **HARD STOP.** Same as (a). Silent RBAC failures are the second-worst failure mode. |
| (c) metadata mutation | Block merge. Diagnose; if the failure is a real concurrency bug, hard-stop and apply (a)-style discipline. |
| (d) OAuth | Block merge. Lifecycle bugs that survive into prod cause silent sync drift — the worst kind of bug because it's invisible until a DP escalates. |

The CI configuration enforces this via required-status-check on the integration test job ([sprint-plan L63 — Go-default CI](sprint-plan.md)).

---

## 7. Cross-references and TO RESOLVE

**Cross-references.**
- ADR-010 (tasket `20260514-114217-3c84`) commits the metadata-engine pattern at Wk 4-5 — re-read this section when ADR-010 lands; (c) becomes either the lightweight DDL form or the load-bearing JSONB form.
- G3 metadata-engine scope verify (tasket `20260514-114245-d3a8`) at Wk 6 — if G3 forces a fallback from DDL to JSONB, the test count in (c) jumps from ≥3 to ≥8 and that's a binding change to this strategy.
- G4 Google OAuth submission (tasket `20260514-114238-bf09`) Wk 5-6 — (d) fixture should be built parallel to the submission so the lifecycle tests exist when production-review approves.
- Sprint plan ([docs/sprint-plan.md](sprint-plan.md)) names the sprint in which each fixture lands: tenantpair Sprint 7 ([L107](sprint-plan.md)), RBAC scaffold Sprint 7 ([L119](sprint-plan.md)), metadata Sprint 8 if JSONB ([L130](sprint-plan.md)), OAuth Sprint 10 ([L198](sprint-plan.md)), backup restore-test Sprint 12 ([L212](sprint-plan.md)).

**TO RESOLVE.**

| # | Item | Resolve by |
|---|---|---|
| TR-1 | Decide REST contract test runner: Schemathesis or hand-rolled Go HTTP table tests. Time-box Schemathesis schema-gen friction to 1 day. | Sprint 9 entry |
| TR-2 | Confirm `mark3labs/mcp-go` ships a test harness; if not, write one. | Sprint 9 entry |
| TR-3 | Build the endpoint-registry guard test (`apps/api/internal/http/coverage_test.go`). Cannot exist until ≥1 tenant-scoped handler outside of the workspace test handler ships. | Sprint 7 (immediately when the second tenant-scoped handler lands) |
| TR-4 | Manual-exploratory checklist file `docs/test-strategy-manual-checklist.md` — create when first end-to-end happy path ships. | Sprint 6 |
| TR-5 | Decide whether to run the nightly Postgres-3x-DP-size load through CI on every PR (cost: ~3-5 min) or keep nightly-only. | Sprint 8 entry |

---

**End of strategy.** Re-read this document at the start of every sprint that introduces a new tenant-filtered endpoint, OAuth scope, RBAC role, or metadata-engine surface. Strategy revisions land via PR with `docs:` prefix and a one-line `Why:` in the commit body.
