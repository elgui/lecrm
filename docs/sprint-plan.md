# leCRM v0 — Sprint Plan (13 sprints, two explicit branches)

**Author:** Guillaume (GB Consult)
**Date:** 2026-05-14
**Status:** Active — drafted before Sprint 1 starts so the Wk-2 stack-decision gate is structurally absorbed, not chaotically. Re-read on the Monday morning of every sprint.

---

## The Wk-2 fork (read this first)

The locked stack per [ADR-009](adr/ADR-009-stack-and-license.md) §1 is Go 1.23 + PostgreSQL 17 + REST + thin MCP adapter + React 19 + Apache 2.0. The Go choice rests on directional evidence (the InfoQ April-2026 Claude Code benchmark; single-static-binary deploy; less type-friction with Claude Code) that **assumes Guillaume can ramp on Go fast enough that the velocity advantage materialises across the 11-13 week build**. The council's engineer voice explicitly rated this assumption unverified for leCRM's HTTP / multi-tenant CRUD workload.

**ADR-009 §1.1 binds a Wk-2 checkpoint** with three concrete litmus tests (tasket `20260510-202450-a5d3`):

1. An `sqlc`-typed query through an HTTP handler.
2. A workspace-scoping middleware with idiomatic Go `context.WithValue` propagation.
3. `golangci-lint run` and `go vet ./...` clean on the scaffolding.

**Each test is time-boxed to 4 working hours.** If any test blocks past four hours despite Claude Code assistance, the entire backend stack switches to the documented TypeScript + Hono runner-up (Hono + Drizzle + Atlas + `openid-client` v6 + `@modelcontextprotocol/sdk` + pg-boss). **The decision is irrevocable at end of Wk 2**; it is not relitigated at Wk 5. ADR-009 explicitly warns against pushing through linguistic resistance — the runner-up costs ~3-5 days of re-scaffolding and saves potentially weeks of compounded velocity loss.

This sprint plan therefore exists in **two parallel branches** from Sprint 3 onward. Sprints 1-2 are shared (the scaffolding work IS the Go-by-default surface against which G1 is decided). On the Monday of Sprint 3, one branch is taken and the other is closed forever. Both branches end at the same deliverable: **v0 ships at Wk 11-13 with the 8-feature core (per PRD Executive Summary), the four schedule gates passed (or their documented fallbacks exercised cleanly), and the first paying Design Partner onboarded**.

The four schedule gates from ADR-009 §9 — G1 (Wk 2 stack), G2 (Wk 4-5 ADR-010 metadata-engine pattern), G3 (Wk 6 metadata-engine scope verify), G4 (Wk 5-6 Google OAuth submission) — are placed identically in both branches. The branches diverge only in the implementation language, the test/fixture investment, and a handful of architecturally significant tooling substitutions called out at each affected sprint.

---

## The 8 v0 features (PRD Executive Summary)

Both branches schedule these eight features and nothing else (other v1/v2 work is explicitly out of scope; sequences engine deferral per [ADR-004](adr/ADR-004-sequences-architecture.md), AI-native UX deferred to v2):

1. Contacts, Companies, Deals (one work item, three entities).
2. Pipeline Kanban view.
3. Gmail sync (thread → record linking).
4. Notes / activity log per record.
5. Tasks with due dates.
6. Custom properties on Contact + Deal.
7. Multi-user with role-based permissions.
8. Tenant data export (CSV, per-tenant download).

---

## Sprints 1-2 — Shared (scaffolding + G1)

Both branches share the first two sprints. Sprint 1 is the Go-by-default scaffolding from tasket `20260510-202450-b844` Part B. Sprint 2 is the G1 checkpoint from tasket `20260510-202450-a5d3`. If G1 forces a stack switch, the Go-specific work from Sprint 1 (sqlc setup, Chi router, `apps/api` Go module) is dead; the Postgres-side work (provisioning function, schema, Authentik, LGTM Compose, OVH VPS) survives unchanged because it is language-agnostic.

### Sprint 1 (Wk 1) — Scaffolding (stack-agnostic + Go-default)

**Source:** tasket `20260510-202450-b844` Part B.

| Work item | Notes |
|---|---|
| Monorepo layout per ADR-009 §8.1 | `apps/{api,mcp,migrate,web}`, `packages/{db,crm-adapter,shared-types,tools}`, `deploy/{compose,caddy}` |
| `go.work` init + per-package `go mod init` | Go-default; thrown away if G1 fails |
| `pnpm create vite apps/web` + TanStack + shadcn/ui | Stack-agnostic; survives a TS switch identically |
| `LICENSE` (Apache 2.0 verbatim) + `NOTICE` | First-commit obligation per ADR-009 §6 |
| `deploy/compose/{postgres,authentik,lgtm}.yml` | Postgres 17, Authentik 2025.10 (Redis-free), LGTM stack |
| Atlas v1.0 wired into `packages/db/migrations/` | Migration tool is language-agnostic |
| `lecrm_provision_workspace(uuid)` SECURITY DEFINER function | ADR-009 §2.1; pure Postgres — zero stack impact |
| Authentik vs Zitadel decision (TO RESOLVE-4) | Authentik default; Zitadel if migration cost >4h |
| First OIDC login flow end-to-end | RP library differs Go-vs-TS (`zitadel/oidc` vs `openid-client` v6); the IDP setup is shared |
| Session cookies per workspace subdomain | ADR-009 §5.2 — cookie-scope mandate, never wildcard |
| CSP header (`frame-ancestors 'none'`, etc.) | Set in Caddy; stack-agnostic |
| CI scaffold | Go-default checks; TS branch swaps `go test` / `golangci-lint` / `gosec` / `govulncheck` for `vitest` / `eslint` / `biome` / `npm audit signatures` |
| OVH VPS provisioned + first test subdomain serving OIDC login | Stack-agnostic |

**Schedule gates:** none fire in Sprint 1. (G1 fires at end of Sprint 2.)

**Tests:** none yet. (Test-strategy doc lands Sprint 3 post-G1.)

### Sprint 2 (Wk 2) — Go-default scaffold pass + G1 litmus tests

**Source:** tasket `20260510-202450-a5d3`. The work IS the gate.

| Work item | Notes |
|---|---|
| `sqlc` configured against the local Postgres | Pre-requisite for litmus Test 1 |
| Chi router + `WorkspaceContext` middleware skeleton | Pre-requisite for litmus Test 2 |
| `golangci-lint` + `go vet` clean on the scaffolding | Litmus Test 3 |
| **Litmus Test 1 (90 min budget):** sqlc-typed query through HTTP handler | Hard fail at 4h |
| **Litmus Test 2 (90 min budget):** workspace middleware with idiomatic `context.WithValue` | Hard fail at 4h |
| **Litmus Test 3 (30 min budget):** lint + vet clean, no `//nolint` exceptions | Hard fail at 4h |

**Schedule gate — G1 fires end of Sprint 2.**
- All three pass within budget → **CONTINUE Go**, take the Go-path branch from Sprint 3.
- Any one blocks past 4h → **SWITCH to TypeScript+Hono**, take the TS+Hono-path branch from Sprint 3. The first ~3-5 days of Sprint 3 absorb re-scaffolding cost.

The decision is recorded as an Architecture Decision Note appended to ADR-009 (date + per-test elapsed time + qualitative observation on Claude Code's Go fluency). Both branches re-converge at Sprint 12-13 deliverables; nothing in the v0 scope is removed or substituted because of the switch.

---

## Branch A — Go-path (sprints 3-13)

**Selected when G1 passes all three litmus tests within budget.** Stack: Go 1.23 + Chi + `sqlc` + Atlas + `river` (Postgres-native job queue) + `zitadel/oidc` + `mark3labs/mcp-go`. Frontend embedded in the Go binary via `//go:embed dist/*`. Test runner: `go test` + testcontainers-go on Postgres + standard fixtures via Go's stdlib.

### Sprint 3 (Wk 3) — Tenancy + auth foundation

| Work item | Notes |
|---|---|
| `cmd/lecrm-migrate` wraps `lecrm_provision_workspace` | Atlas runner; Compose pre-deploy job |
| Per-workspace Postgres role provisioning live | ADR-009 §2.1 SECURITY DEFINER call from `cmd/lecrm-migrate` |
| OIDC integration via `zitadel/oidc` library | Authentik upstream; certified RP |
| `(issuer, sub)` user-key table | ADR-009 §7.1 — v0→v1 IDP migration is a mapping table, not a destructive rewrite |
| Session cookies scoped to workspace subdomain | `Domain=acme.lecrm.fr; SameSite=Strict; Secure; HttpOnly` — never wildcard |
| `river` worker pattern: acquire workspace-scoped Postgres connection before any data operation | Per-tenant `river_<workspace_base36>` schema |
| **Secrets baseline** — tasket `20260510-162158-1023` | SOPS + age; `lecrm_provisioner` (Tier-0), Authentik admin, Cloudflare DNS API token; per-tenant manifest for OAuth refresh tokens, Brevo API keys, JWT signing keys |
| **Test strategy doc** — tasket `20260514-114210-9b41` | Committed post-G1; in-scope = smoke + contract + manual exploratory; out-of-scope = full Playwright E2E, chaos, perf load; 4 non-negotiable categories named |
| **Cross-tenant isolation test fixture** | 2-tenant provision/write/assert helper; covers every tenant-filtered endpoint added going forward |

**Schedule gates:** none fire. **Sibling taskets land:** `1023` (secrets), `9b41` (test strategy).

### Sprint 4 (Wk 4) — G2 (proactive) + early CRUD + frontend slice

| Work item | Notes |
|---|---|
| **G2 fired proactively at Wk 3** — tasket `20260514-114217-3c84` `done` (2026-05-15). ADR-010 selected JSONB-primary on `objects` table per workspace schema. Load-bearing-through-v1 paragraph in ADR-010 §6. | |
| Metadata-engine implementation begins | Pattern set by ADR-010 |
| Contact / Company / Deal entity work begins (feature 1) | Domain models + sqlc query files |
| React 19 frontend slice live | TanStack Router + TanStack Query + shadcn/ui imports; first route renders against the API |
| RBAC test fixture begins | 3+ role types per test tenant; populates ahead of Sprint 8 RBAC feature |

**Schedule gates:** **G2** (proactive — Winston's "decide before Wk 5, not during" lock). **Sibling taskets land:** `3c84` (ADR-010).

### Sprint 5 (Wk 5) — Metadata engine + Custom properties + G4 prep

| Work item | Notes |
|---|---|
| Metadata engine implementation continues per ADR-010 (JSONB-primary on `objects` + `custom_property_definitions` per workspace schema) | |
| Custom properties on Contact + Deal (feature 6) | First feature consuming the metadata engine |
| **G4 prep** — tasket `20260514-114238-bf09` | Privacy policy + ToS pages live at a stable URL on a verified domain (e.g. `gbconsult.me/lecrm/privacy`); domain verified in Google Search Console; demo video draft script |
| **JSONB regression test coverage** (non-negotiable test category (c), load-bearing per ADR-010) | Concurrent mutation, schema drift against `custom_property_definitions`, GIN-index query correctness — ≥8 tests per `docs/test-strategy.md` §4.3 |

**Schedule gates:** G4 prep work (the gate itself fires Sprint 6 at the latest).

### Sprint 6 (Wk 6) — G3 verify + G4 submission + CRUD continues

| Work item | Notes |
|---|---|
| Contact / Company / Deal CRUD with custom properties live | API + frontend |
| Pipeline Kanban skeleton (feature 2) | Drag-and-drop wired via DnD Kit on TanStack Table |
| **G3 verification fires end of sprint** — tasket `20260514-114245-d3a8` | JSONB-scope sanity check per G3 runbook §5.2.2 (LIVE path): is cumulative JSONB metadata-engine work staying inside the 3.25d projection? Are non-negotiable (c) tests passing? Runbook §5.2.1 (DDL→JSONB switch) is historical. |
| **G4 submission fires by end of sprint** — tasket `20260514-114238-bf09` | Demo video uploaded; OAuth consent screen production-ready; production review submitted via Google Cloud Console; submission ID + date logged; follow-up polling tasket created if no response in 2 wk |

**Schedule gates:** **G3** (verify), **G4** (submit). Sibling taskets land: `d3a8`, `bf09`.

**G3 outcome already determined by ADR-010:** ADR-010 committed JSONB-primary 2026-05-15 (proactive G2). Downstream alignment landed in commit `e875fb8`: tasket `731a` Phase 2 (methodology config) body updated; test-strategy category (c) already load-bearing; G3 runbook §5.2.1 marked historical. Sprint 6 G3 fire is the JSONB-scope sanity check only.

### Sprint 7 (Wk 7) — Standard CRUD complete + audit + service tokens

| Work item | Notes |
|---|---|
| REST handlers for Contact, Company, Deal, Activity, Note, Task complete | URL-prefix versioning `/v1/...` |
| OpenAPI 3.1 generation via `ogen` or `oapi-codegen` | Source for `apps/web` types via `@hey-api/openapi-ts` (ADR-009 TO RESOLVE-6) |
| `Idempotency-Key` header on mutation endpoints | ADR-009 §4 |
| Opaque base64 cursor pagination on list endpoints | ADR-009 §4 |
| Workspace-scoped Bearer service tokens | ADR-009 §4.1: Argon2id-hashed, 1-yr default, synchronous DB lookup, `actor_type` claim (`human_api` / `mcp_agent` / `internal_service`) |
| Audit log infrastructure | `actor_type` from token; ADR-007 §3 catalogue |
| `security.workspace_id_mismatch` event | Subdomain-derived `workspace_id` vs token claim; token authoritative; reject 401 |
| **Audit fail-closed mutation path** | A mutation that cannot be audit-logged MUST be rejected; hard requirement before first paying client |
| Notes / activity log per record (feature 4) | Append-only timeline against the audit log |
| Tasks with due dates (feature 5) | River-scheduled reminders |
| **Contract test suite** against the REST surface | Schemathesis or hand-rolled `go test` HTTP table tests against OpenAPI |

**Schedule gates:** none fire (G4 is in flight at Google; expected approval Wk 9-11).

### Sprint 8 (Wk 8) — RBAC + Pipeline Kanban + integrator handoff Phase 1

| Work item | Notes |
|---|---|
| Multi-user with role-based permissions (feature 7) | Workspace-level roles; per-record ACLs deferred to v1 |
| **RBAC regression suite** | 3+ role types × every protected endpoint; non-negotiable test category (b) |
| Pipeline Kanban view complete (feature 2) | Frontend feature-complete on DnD Kit + TanStack Table |
| **Integrator-handoff Phase 1** — tasket `20260514-114231-8a67` Capability 1 | `lecrm tenant create <slug> --admin <email> [--template <name>]` CLI; ≤30s end-to-end; demoed against test tenant |

**Schedule gates:** none fire.

### Sprint 9 (Wk 9) — Frontend complete + methodology config + CSV export + MCP skeleton

| Work item | Notes |
|---|---|
| React 19 frontend feature-complete for all 8 v0 features | TanStack Table list views, DnD Kit on Kanban, cmdk + react-hook-form + zod |
| Frontend embedded in Go binary via `//go:embed dist/*` | Single-binary deploy story locked |
| Caddy proxy → Go binary; internal routing `/v1/*` → REST, `/*` → embedded SPA | ADR-009 §5.1 |
| **Integrator-handoff Phase 2** — tasket `20260514-114231-8a67` Capability 2 | Methodology config schema (acquisition channels, pipeline stages, stage properties, automations, color coding); one example config (Léo's standard CRM-integrator template) checked into repo; `lecrm config diff/replay` CLI verbs working |
| Tenant data export to CSV (feature 8) | Per-tenant download; sovereignty pitch made concrete |
| Vercel AI SDK 6 wired into frontend (no v0 use yet) | Preserves v2 chat/voice optionality without rewrite |
| `cmd/lecrm-mcp` adapter skeleton | Separate Compose service; `mark3labs/mcp-go`; Streamable HTTP transport; rate-limit per (`workspace_id`, `token_id`) tuple |

**Schedule gates:** none fire. **G4 approval window opens** (4-6 wks after Wk 6 submission).

### Sprint 10 (Wk 10) — Gmail sync via external-system-sync seam + OAuth lifecycle tests

| Work item | Notes |
|---|---|
| **External-system-sync seam** — tasket `20260514-114224-5d12` | Sync direction, identity mapping, conflict policy, rate limiting + retry, webhook vs poll, per-tenant credential storage (consumes secrets baseline), failure modes + observability surface |
| Gmail sync (feature 3) — IMPLEMENTED THROUGH THE SEAM | Thread → record linking via Gmail API readonly + modify scopes; no Gmail-specific bypass paths |
| Per-tenant Gmail OAuth refresh-token storage | SOPS-encrypted per-tenant manifest entries |
| **Paper exercise** — hypothetical Shopify connector through the same seam | Validates the abstraction; if it doesn't fit cleanly, refactor the seam now |
| **OAuth token lifecycle test fixture** | Mock Gmail with controllable token state; refresh, expiration, revocation — non-negotiable test category (d) |

**Schedule gates:** none fire. **G4 must be approved before this sprint ends or Gmail-sync work goes against the 100-user Testing-status cap**; if Google hasn't approved by mid-sprint, the polling tasket from Sprint 6 escalates to chase the verification clarifying round.

### Sprint 11 (Wk 11) — Brevo + backup + observability + integrator audit surface

| Work item | Notes |
|---|---|
| **Brevo transactional integration** — tasket `20260510-162158-499c` | Go HTTP client wrapping `POST /v3/smtp/email`; inbound webhook receivers with HMAC signature verification; `email_suppression` table per workspace; bounce-rate alarm; audit emission |
| **Backup baseline** — tasket `20260510-162158-d1ba` | WAL-G + GPG → OVH Object Storage; per-tenant restore drill on a throwaway tenant |
| LGTM Compose stack operationalized | All metrics labelled with `workspace_id` for per-tenant anomaly detection |
| Caddy DNS-01 wildcard cert config in production | Cloudflare DNS API token from SOPS |
| pg full-text search wired into list endpoints | typesense deferred to v1 |
| **Integrator-handoff Phase 3** — tasket `20260514-114231-8a67` Capability 3 | Per-tenant audit log query surface (`/admin/audit?tenant=X`); Léo-facing view; debug "did the automation fire?" without DB access |
| **Backup restore-test** as part of test-strategy | Smoke test: provision tenant, write data, restore from backup to a sibling tenant, diff |

**Schedule gates:** none fire.

### Sprint 12 (Wk 12) — Metabase reporting bridge + deploy ops + first DP onboarding starts

| Work item | Notes |
|---|---|
| **Metabase reporting bridge** — tasket `20260510-162158-29dc` | iframe-embedded read-only reports; switch to Cube.dev if Metabase tenant-scoping is harder than expected |
| Single-binary deploy via Compose | Pre-deploy `lecrm-migrate` job gates `lecrm-api` startup |
| Lawyer-reviewed DPA + CGV + SLA signed | Parallel non-dev track that ran from Sprint 1; LEGAL-PLAYBOOK.md |
| Customer-facing brand decided + INPI registered | Parallel non-dev track |
| First Design Partner workspace provisioned | Via the `lecrm tenant create` CLI from Sprint 8 |
| **DP onboarding window OPENS** | DNS subdomain, Google Workspace OIDC, Gmail OAuth grant, first data import, first user trained |

**Schedule gates:** none fire.

### Sprint 13 (Wk 13) — DP onboarding closes + slack absorption

| Work item | Notes |
|---|---|
| DP onboarding completes | First paying client live |
| Slack absorption for any G1-G4 consequences | E.g., finalize JSONB regression coverage gaps per category (c) (ADR-010 JSONB-primary already decided) |
| Manual exploratory test pass per test-strategy | Happy paths across all 8 features in production |
| **First paying Design Partner live** | v0 ships |

**Schedule gates:** none fire. v0 ceiling met.

### Go-path test investment summary

| Investment | Sprint | Cost |
|---|---|---|
| `go test` + testcontainers-go fixture for cross-tenant isolation | 3 | ~1 day |
| RBAC role-fixture in Go test helpers | 4-8 | ~1.5 days |
| JSONB regression Go test pack (conditional on G3 RED) | 5-6 | ~0-2 days |
| Contract tests against OpenAPI (schemathesis or table tests) | 7 | ~1.5 days |
| OAuth lifecycle mock Gmail | 10 | ~1 day |
| Backup restore-test | 11 | ~0.5 day |
| **Total Go-path test budget** | | **~5.5-7.5 days across the build** |

---

## Branch B — TS+Hono-path (sprints 3-13)

**Selected when any G1 litmus test blocks past 4 working hours.** Stack: TypeScript (Node 22) + Hono + Drizzle + Atlas + `openid-client` v6 (Foundation-certified) + pg-boss (closest Postgres-native job-queue analogue to `river` — see re-pricing note below) + `@modelcontextprotocol/sdk` (TS-side Tier-1). Frontend deployed **split** (Caddy → static `apps/web/dist`, Caddy → Hono API) — the Go-embed single-binary story does not have a clean TS analogue, and split deploy was already ADR-009's Phase-2 reconsideration anyway. Test runner: Vitest + testcontainers (Node port) + drizzle-seed for fixtures.

Sprints 3-13 schedule the same features and trigger the same gates as Branch A. Below, each sprint lists **only the deltas from Branch A** — the work that is materially different or re-priced. Anything not explicitly re-priced is identical to Branch A.

### Sprint 3 (Wk 3) — Re-scaffold + tenancy + auth foundation

**Re-pricing — additive cost (~+3-5 days re-scaffolding):**
- `apps/api` Go module deleted; replaced with `apps/api` TS package using Hono + Drizzle.
- Per-workspace Postgres connection helper rewritten — Drizzle/Kysely do not natively know about `search_path` switching mid-connection; need a thin pool wrapper that opens connections as the workspace Postgres role. ~1 extra day vs `sqlc`'s clean separation.
- `river` replaced with **pg-boss** as the closest Postgres-native job queue analogue. Risk: pg-boss is less battle-tested than `river` at high concurrency; acceptable at v0 scale (≤4 workspaces). ~1 extra day vs the Go-side river setup. **Flag for v1+:** revisit job-queue choice if Phase 3 throughput demands.
- OIDC via `openid-client` v6 (Foundation-certified) instead of `zitadel/oidc`. Functionally equivalent; idiom different.
- Session cookies: Hono cookie helpers instead of Chi/`net/http`. Same shape per ADR-009 §5.2.
- CI: `vitest run` + `eslint` + `biome` + `npm audit signatures` replace `go test` + `golangci-lint` + `gosec` + `govulncheck` + `go mod verify`.

**Re-pricing — surviving unchanged:**
- `lecrm_provision_workspace` SECURITY DEFINER function (pure Postgres).
- Atlas v1.0 migration tooling.
- All Compose services (Postgres, Authentik, LGTM, Caddy).
- The `(issuer, sub)` user-key table.
- Secrets baseline tasket `1023` — `lecrm_provisioner`, Authentik admin, Cloudflare DNS API token. Per-tenant manifest content the same.
- Test strategy doc tasket `9b41` — committed with test runner = Vitest (not `go test`); fixtures via drizzle-seed (not `testify`); same four non-negotiable categories.

**Schedule gates:** none fire.

### Sprint 4 (Wk 4) — G2 (proactive) + early CRUD + frontend slice

**Re-pricing:**
- G2 (ADR-010) decision is the same: JSONB-primary (per ADR-010, same as Go branch). Drizzle's `jsonb` column type with Zod runtime schemas. Atlas handles the migrations — language-agnostic, same as Go.
- Drizzle's relational query patterns are nicer than sqlc for the Contact↔Company↔Deal JOINs that Sprint 7 needs. Likely ~1 day saved over the build.
- RBAC test fixture begins in Vitest (not `go test`); same shape (3+ role types per test tenant).

**Schedule gates:** **G2** (proactive). Sibling taskets land: `3c84` (ADR-010).

### Sprint 5 (Wk 5) — Metadata engine + custom properties + G4 prep

**Re-pricing:** identical to Go branch. The metadata-engine ADR-010 outcome is stack-agnostic in shape; the implementation diverges in detail (sqlc-generated types vs Drizzle-generated types) but not in scope. G4 prep work is privacy policy / ToS / domain verification / demo video — entirely stack-agnostic.

**Schedule gates:** G4 prep.

### Sprint 6 (Wk 6) — G3 verify + G4 submission

**Re-pricing:** identical to Go branch. G3 measures actual days of effort against the metadata-engine implementation regardless of stack; the 5-day threshold is the same. G4 submission is a Google Cloud Console process — stack-agnostic.

**Schedule gates:** **G3** (verify), **G4** (submit).

### Sprint 7 (Wk 7) — Standard CRUD + audit + service tokens

**Re-pricing — additive savings (~1-2 days):**
- `hono/zod-openapi` (or `@hono/zod-openapi`) generates OpenAPI 3.1 directly from Hono route definitions + Zod schemas. Stronger story than Go's `ogen` / `oapi-codegen` because the contract source lives next to the handler, not in a separate YAML or annotation pass.
- Service tokens: Argon2id via `argon2` Node package (libargon2 binding). Equivalent.
- Audit log emission: TS code differs; schema and `actor_type` semantics identical.
- Contract tests in Vitest against the OpenAPI surface.

**Schedule gates:** none fire.

### Sprint 8 (Wk 8) — RBAC + Pipeline Kanban + integrator handoff Phase 1

**Re-pricing:** identical to Go branch. RBAC suite in Vitest. Integrator-handoff Phase 1 CLI built with `commander` or `clipanion` instead of `cobra` or `urfave/cli`.

**Schedule gates:** none fire.

### Sprint 9 (Wk 9) — Frontend complete + methodology config + CSV + MCP skeleton

**Re-pricing — material divergence:**
- **Frontend deploy is split**, not embedded. Caddy → static `apps/web/dist` + Caddy → Hono API. ADR-009 §5.1 already names this as the Phase-2 reconsideration; the TS branch makes it Phase-1. ~1 day saved (no `go:embed`-equivalent bundling), at the cost of two deploy units instead of one. **Operational consequence:** static SPA and API are versioned and deployed independently; release process gains a "ship the SPA matches the API contract" coordination step. Mitigated by OpenAPI-generated TS types living in `packages/shared-types`.
- **MCP adapter — material saving (~2-3 days).** `@modelcontextprotocol/sdk` is the **Tier-1 reference SDK**; `mark3labs/mcp-go` is community-grade. The TS-side adapter is a smaller, better-supported surface.
- Methodology config schema + CLI verbs: Zod schema authoring tighter than the Go-side equivalents.
- CSV export uses Node streams; semantically identical to Go's encoding/csv.
- Vercel AI SDK 6 integration unchanged (it's a frontend lib).

**Schedule gates:** none fire. G4 approval window opens.

### Sprint 10 (Wk 10) — Gmail sync via external-system-sync seam + OAuth lifecycle

**Re-pricing:** identical to Go branch in shape. Gmail SDK: `@google-cloud/local-auth` + `googleapis` Node packages (vs `google.golang.org/api/gmail/v1`). Webhook ingestion: same. OAuth lifecycle mock: Vitest helper instead of Go test helper. ~0 day delta.

**Schedule gates:** none fire (G4 must approve in this window or escalate).

### Sprint 11 (Wk 11) — Brevo + backup + observability + integrator audit surface

**Re-pricing — additive savings (~1 day):**
- **Brevo:** native TS SDK `@getbrevo/brevo` v5 (vs hand-rolled Go HTTP client). ~1 day saved on the Brevo integration shape (typings, error shapes, retry helpers come from the SDK).
- WAL-G + GPG to OVH Object Storage: Postgres-side, language-agnostic.
- LGTM stack: stack-agnostic.
- Integrator-handoff Phase 3 audit surface: same shape in Hono routes.

**Schedule gates:** none fire.

### Sprint 12 (Wk 12) — Metabase reporting bridge + deploy ops + first DP onboarding starts

**Re-pricing:** identical to Go branch in shape. Deploy is two units (SPA + API) instead of one; Compose definitions adjusted accordingly.

**Schedule gates:** none fire.

### Sprint 13 (Wk 13) — DP onboarding closes + slack absorption

**Re-pricing:** identical to Go branch. First paying Design Partner live; v0 ships.

**Schedule gates:** none fire. v0 ceiling met.

### TS+Hono-path test investment summary

| Investment | Sprint | Cost |
|---|---|---|
| Vitest + testcontainers fixture for cross-tenant isolation | 3 | ~1 day |
| RBAC role-fixture in Vitest helpers | 4-8 | ~1.5 days |
| JSONB regression Vitest pack (conditional on G3 RED) | 5-6 | ~0-2 days |
| Contract tests via Vitest + `hono/zod-openapi` schema | 7 | ~1 day |
| OAuth lifecycle mock Gmail in Vitest | 10 | ~1 day |
| Backup restore-test | 11 | ~0.5 day |
| **Total TS+Hono-path test budget** | | **~5-7 days across the build** |

### Net re-pricing summary (TS+Hono vs Go)

| Sprint | Δ days |
|---|---|
| Sprint 3 — re-scaffold (Go work dead) | **+3 to +5** |
| Sprint 3 — pg-boss vs river | **+1** |
| Sprint 3 — Postgres pool wrapper vs sqlc | **+1** |
| Sprint 4 — Drizzle relational JOINs vs sqlc | **−1** |
| Sprint 7 — `hono/zod-openapi` vs `ogen` | **−1 to −2** |
| Sprint 9 — split deploy vs `go:embed` bundling | **−1** |
| Sprint 9 — `@modelcontextprotocol/sdk` vs `mark3labs/mcp-go` | **−2 to −3** |
| Sprint 11 — `@getbrevo/brevo` v5 vs Go HTTP client | **−1** |
| **Net delta across the build** | **~+1 to +3 days, essentially a wash** |

The honest read: **the TS+Hono branch is not structurally slower than Go on the leCRM surface.** The early re-scaffolding cost is offset by ecosystem maturity in MCP and OpenAPI tooling. The Go choice rests on the run-cost benchmark advantage ($0.50 vs $0.62 per Claude Code task — directional, not workload-proven), the single-static-binary deploy story, and ADR-009's bet on less type-friction in CRUD work. If Go ramp blows up at G1, TS+Hono is a parallel landing pad, not a downgrade.

---

## Both branches — convergence at Sprint 12-13

Both branches deliver the same v0 contents at the same time window:

- All 8 PRD Executive Summary features live.
- Schedule gates G1-G4 all passed or their documented fallbacks executed cleanly.
- The four non-negotiable test categories (cross-tenant isolation, RBAC regression, JSONB regression IF applicable, OAuth token lifecycle) all live before first DP migration.
- Integrator-handoff capabilities 1-3 all live, demoed to Léo (or proxy) end-to-end.
- External-system-sync seam validated by Gmail-sync consuming it + a paper exercise on a hypothetical second connector.
- LGTM observability, WAL-G + GPG backups, fail-closed audit on the mutation path — all production-ready.
- DPA + CGV + SLA signed; customer-facing brand registered.
- First paying Design Partner live at Wk 12-13.

---

## Architect-style review pass (Winston-lens)

Hidden Go-only assumptions that the TS+Hono branch handles differently:

1. **`//go:embed dist/*` frontend embed.** Go-only. The TS branch ships **split deploy from day 1** (Caddy → static SPA + Caddy → Hono API). ADR-009 §5.1 already named split deploy as the Phase-2 reconsideration; the TS branch promotes that to Phase-1. Operational consequence: two deploy units instead of one; release coordination steps documented in Sprint 9 TS-branch notes.

2. **`context.WithValue` workspace propagation.** Go-only idiom. TS uses Hono's `c.var` typed locals (or `AsyncLocalStorage` if cross-cut needed). Functionally equivalent; the litmus-test 2 idiom check from G1 simply doesn't apply on the TS side.

3. **`river` Postgres-native job queue.** Go-only library. TS-side closest analogue is **pg-boss**, which is less battle-tested at high concurrency. Acceptable at v0 scale (≤4 workspaces) but flagged for v1+ revisit if Phase 3 throughput demands. ADR-009's "no Redis at v1" stance still holds with pg-boss.

4. **`sqlc` type-safe generated queries.** Go-only. Drizzle generates types from schema definitions; tighter to ORM patterns than sqlc's SQL-first stance. The architectural intent — types come from the database shape, not the application's runtime guesses — is preserved.

5. **`mark3labs/mcp-go` MCP SDK.** Go-side is community-grade; the TS-side `@modelcontextprotocol/sdk` is **Tier-1**. The TS branch is actually advantaged here.

6. **Schema-per-tenant `ALTER ROLE search_path` pattern.** Postgres-side, language-agnostic. Both branches inherit it identically.

7. **`zitadel/oidc` certified RP.** Go-only library. TS uses `openid-client` v6 — also OpenID Foundation-certified. Equivalent.

8. **`golangci-lint` + `go vet` quality gates.** Go-only. TS branch substitutes `eslint` + `biome` + `tsc --noEmit`. Different rule taxonomy, same gate intent. The G1 litmus-test-3 "clean lint" check would be re-expressed as "Vitest run + biome clean + tsc clean" on a TS scaffold, with the same 30-min budget.

9. **Atlas migration tooling.** Language-agnostic. Both branches inherit it identically.

10. **`river` per-tenant `river_<workspace_base36>` schema convention.** The TS branch preserves the convention (pg-boss tables live in the same per-tenant schemas). Audit-log emission point unchanged.

Hidden assumptions that survive untouched in BOTH branches:

- The `lecrm_provision_workspace` SECURITY DEFINER function (pure Postgres).
- The audit-log catalogue including `security.workspace_id_mismatch`.
- The `(issuer, sub)` user-key migration table.
- Session cookies scoped to per-workspace subdomain.
- Caddy DNS-01 wildcard cert config + CSP headers.
- WAL-G + GPG backup to OVH Object Storage.
- LGTM observability stack with `workspace_id`-labelled metrics.

**Gate-timing honesty check:**

- G1 fires end of Sprint 2 (Wk 2). Correct per ADR-009 §1.1.
- G2 fires Sprint 4 (Wk 4). One sprint *earlier* than ADR-009 §9's "end of Wk 5" wording, per Winston's round-2 lock: decide BEFORE Wk 5, not during. This is intentional. Tasket `3c84` is the source.
- G3 fires end of Sprint 6 (Wk 6). Correct per ADR-009 §9. Tasket `d3a8` is the source.
- G4 fires by end of Sprint 6 (Wk 6) — submission deadline. Correct per ADR-009 §9 (4-6 wk Google review SLA ≥ Wk 11 deploy minus Wk 5-6 submission). Tasket `bf09` is the source. **G4 is the single biggest external-blocker risk and is structurally non-deferrable.**

**v0-build group order vs sprint order — honesty:**

The four v0-build group taskets have group-relative `order` 1-4 (Brevo, Metabase, backup, secrets). The sprint plan re-sequences them by dependency:

| Tasket | Group order | Sprint placement | Reason |
|---|---|---|---|
| `1023` Secrets baseline (SOPS + age) | 4 | **Sprint 3** | OAuth + Brevo + Authentik dev work all need encrypted manifests early |
| `d1ba` Backup baseline (WAL-G + GPG) | 3 | **Sprint 11** | Wait until production deploy is imminent; per-tenant restore drill needs real tenants |
| `499c` Brevo transactional | 1 | **Sprint 11** | Per FEASIBILITY-MEMO §3 Wk 11-12 ("Email + observability + deploy") |
| `29dc` Metabase reporting bridge | 2 | **Sprint 12** | Last in the sequence; nice-to-have for DP demo, not core CRUD |

This re-sequencing is intentional and consistent with the FEASIBILITY-MEMO §3 timeline.

**Open questions the plan flags but does NOT decide:**

- **Léo intake timing.** "First paying DP" is the v0 target as a *result*, not a hard date. Sprint 12-13 is the realistic earliest moment. The plan flags this rather than committing.
- **G4 OAuth approval window vs Sprint 10 Gmail-sync work.** If G4 hasn't approved by mid-Sprint 10, the polling tasket escalates. The build can continue against the 100-user Testing cap, but the DP onboarding (Sprint 12) blocks until production scopes are live. If Google delays force a Wk 13+ deploy, that's the documented failure mode from the PRD Exec Summary — "Léo channel + sovereign codebase outlast schedule slips; the 11-13 week window and tenant trust do not."
- **JSONB-bleed honesty.** ADR-010 committed JSONB-primary 2026-05-15. Test-strategy non-negotiable category (c) is load-bearing through v1. v2 inherits either permanent JSONB acceptance or a dedicated migration epic (not a sprint — live tenant data backfill plus read-path rewrite). G3 runbook §5.2.1 (DDL→JSONB switch) is historical; §5.2.2 (scope sanity check) is the live path. This is documented in ADR-010 §6 verbatim paragraph.

---

## Re-reading cadence

Re-read this plan on the Monday morning of every sprint. Update the post-G1 branch in place once Sprint 2 closes (delete the dead branch outright; do not keep both around as a "just in case"). Update the post-G3 outcome in Sprint 6's row (GREEN continue / RED fallback). Update the G4 approval status in Sprint 9-10's row (approved / pending / clarification round-trip).
