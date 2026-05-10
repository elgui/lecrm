---
id: 20260510-184630-0c44
title: leCRM stack research — language, framework, DB, API, frontend, license (priming for ADR-009)
status: done
priority: p0
created: 2026-05-10
updated: 2026-05-10
category: tooling
group: lecrm-stack-decision
order: 1
done: 2026-05-10
---

# Stack Research Session — leCRM Foundation (Path D)

## Context — what just changed

leCRM has pivoted from a Twenty AGPL fork to **clean-room reimplementation** (Path D, see `docs/adr/ADR-008-clean-room-reimplementation.md`). The pivot was made via a four-round multi-agent council debate on 2026-05-10. None of Twenty's stack choices (TypeScript / NestJS / TypeORM / GraphQL / React) are inherited. Twenty's source code may be **read as architectural reference** (a textbook), but no code is copied or ported. leCRM is greenfield, originated by GB Consult, under a license of GB Consult's choosing (license selection is bundled into this research — see §6 below).

## Strategic posture (locked, do not relitigate in this session)

- **Moat:** ownership + Leo's distribution + tailorization + transparent pricing. NOT AI-native UX. AI-native interfaces remain a near-future upside, not the v1 bet.
- **Pricing:** explicitly OK competing on the price tag.
- **Distribution:** Leo (Vernayo, HubSpot integrator partner) introduces from his lost-deal pipeline + integration network.
- **One-sentence pitch:** *"transparent, honest pricing with any kind of tailorization."*
- **ICP:** French/EU SMBs, 3-15 users, who rejected HubSpot on price/sovereignty/customization (see `docs/ICP-ARCHETYPE.md`).
- **Solo dev** (Guillaume) + Claude Code parallel agents. No FTE engineers under contract.

## v1 scope (locked for sizing — boring CRM, no LLM features in user-facing product)

Multi-tenant workspaces (subdomain-based) · contacts · companies · deals · pipelines · custom objects + custom fields with metadata · REST + GraphQL APIs (and possibly MCP server hooks) · multi-stage pipeline + kanban + table + simple list views · OIDC SSO (Google Workspace + Microsoft Entra at v0; SAML deferred) · workspace-scoped data isolation (per-VPS in Phase 1; RLS schema-per-tenant in Phase 2 per ADR-001) · email logging integration (Gmail + IMAP + Outlook — basic sync, NOT reply-detection state machines) · audit log infrastructure (every privileged action emits an event) · 2FA · basic RBAC (workspace member roles, no per-record ACL in v1) · standard search (typesense or pg full-text) · idempotency-key writes · cursor pagination · tenant-scoped URL routing · basic React-or-equivalent UI (forms / lists / kanban / calendar). Workflow engine, native sequences with reply detection, custom dashboards (Cube.dev iframe is the v0 bridge), LLM/AI features, mobile native, PWA polish are all DEFERRED.

## Honest delivery target

11-13 weeks from start to first paying Design Partner (1-2 weeks reading Twenty's source as architectural reference + 10-12 weeks of scratch implementation). This is the council's R4 honest number, accounting for AI-velocity compression on greenfield CRUD work and the lack of compression on RLS / IDOR / metadata-schema-lifecycle correctness work.

## Research dimensions (cover each)

### 1. Backend language & runtime

Candidates and what to evaluate:

- **TypeScript on Bun, Deno, or Node** (NestJS / Hono / tRPC / Elysia / Fastify). Twenty's choice. Maximally familiar to Claude Code. Mature ORM ecosystem.
- **Go** (stdlib net/http or Echo or Chi or Huma; sqlc for type-safe queries; ent for ORM). Boring, fast, deploys as single binary. Excellent ops profile at solo scale. Claude Code is fluent.
- **Rust** (Axum + Tokio + sqlx or SeaORM). Guillaume named this explicitly as a candidate to investigate. Type system buys real correctness for tenant-scoping. Compile times and learning curve are the costs. Claude Code is competent in Rust 2026 but slower than TS/Go for greenfield velocity.
- **Elixir + Phoenix + LiveView**. Multi-tenant story is excellent (per-process isolation, BEAM supervision trees). LiveView is a credible "replace React" play. Niche talent pool — sale-ability question.
- **Python + FastAPI + SQLAlchemy 2.x + Alembic**. Common, mature, async-friendly. AI ecosystem alignment is strong. Performance ceiling at scale is the question.

For each candidate, answer:
- Solo-dev velocity with Claude Code in 2026 (1-5 score with rationale)
- Multi-tenant primitives the language/framework provides natively (none / weak / strong)
- Memory footprint per tenant under expected v1 load (~5-15 users per workspace, ~50 simultaneous workspaces at Phase 2)
- AI-native readiness — MCP server libraries, streaming primitives, prompt caching ergonomics, agent orchestration framework availability
- Talent / sale-ability profile (is this stack a discount or premium at acquisition?)
- Deployment story (single binary? container with runtime? matters for VPS-per-client Phase 1)

### 2. Database

PostgreSQL is almost certainly the answer. Validate by examining:
- Multi-tenant patterns at our scale (RLS-per-row vs schema-per-tenant vs database-per-tenant)
- Hosted variants (Hetzner managed, Neon, Supabase, Crunchy Bridge, raw Postgres on VPS) — each has trade-offs around EU residency, backup ergonomics, and cost-per-tenant
- Audit logging integration (logical replication, triggers, dedicated audit table)
- Idempotency-key patterns native to Postgres (unique partial indexes, advisory locks)
- Migration tooling under multi-tenant (per-tenant schema migrations, lock contention, online schema change)

Counter-investigation: is there a credible non-Postgres answer? (CockroachDB for multi-region; SQLite + LiteFS for per-tenant isolation extreme; Postgres always wins but verify.)

### 3. ORM / query layer

Downstream of stack choice. For each backend candidate, name the recommended ORM and why. Bias against heavy ORMs that hide SQL — multi-tenant correctness wants explicit tenant predicates in every query, which is harder when the ORM abstracts the SQL layer.

### 4. API surface

Evaluate:
- **REST-only** (boring, OpenAPI tooling mature, agent-friendly via standard tool-use)
- **GraphQL-only** (Twenty's choice; nice for evolving schemas; cursor pagination native; harder to enforce tenant scoping at gateway)
- **REST + GraphQL hybrid** (Twenty serves both; meaningful complexity tax)
- **REST + MCP server first-class** (REST is the durable contract, MCP is a thin adapter that wraps it — Winston's architectural recommendation)
- **gRPC for internal, REST for external** (over-engineered at solo scale; rejected)

Recommend: REST + thin MCP adapter as the v1 surface, deferring GraphQL until v2 if a design partner specifically needs it. Validate this against the council's R3 architectural reads.

### 5. Frontend

Candidates:
- **React + Vite + TanStack Query/Router** (Twenty's choice; Claude Code fluent; ecosystem mature)
- **Solid + Vite** (faster runtime, smaller bundle, Claude Code competent but less idiomatic)
- **Svelte + SvelteKit** (good DX, smaller bundle, Claude Code competent)
- **HTMX + server-rendered partials** (radical reduction; works only if backend stack is server-side-rendering-friendly — Phoenix LiveView, Go templ, Rust Maud, ...)
- **Inertia.js + React/Svelte** (server-driven SPA-feel without SPA tax)

Criteria: AI-native readiness for v2 (chat UIs, voice UIs), velocity for boring CRUD forms in v1, build/deploy ergonomics under multi-tenant subdomains, accessibility / keyboard-driven CRM ergonomics for power users.

### 6. License selection (deferred from ADR-008)

Since leCRM is no longer a derivative of Twenty's AGPL, GB Consult chooses leCRM's license. Candidates:

- **MIT / Apache 2.0** — most permissive; signals trust to clients; commodity infrastructure; zero distribution obligation. Risk: a competitor lifts and shifts.
- **AGPL-3.0** — matches Twenty's posture; reinforces "open-source-first" pitch even though not legally required; requires §13 disclosure (ironic given we just escaped that).
- **BSL with Change Date** (e.g., 4-year BSL → Apache 2.0). Defensive against competitor lift-and-shift; converts to permissive later. Adopted by Sentry, MariaDB, HashiCorp.
- **Source Available** (e.g., Elastic License 2.0, Functional Source License). No competitor SaaS hosting allowed; otherwise free use.
- **Proprietary closed-source** — zero distribution obligation; reduces reputational and operational surface; closes the open-source narrative. Not really an OSS pitch then.

Decision criteria to apply: trust signaling vs competitor protection vs asset hardening (acquirer-friendliness) trade-off. Reference the ICP — would Marc, Anne, or Pierre care which license? If they don't, the decision becomes purely commercial.

### 7. Build tooling, monorepo strategy, package layout

Downstream of stack choice. Capture briefly: workspaces (Yarn / pnpm / Cargo / Go modules), what gets versioned together, how the frontend builds and ships alongside the backend.

### 8. Auth library / service

For each backend candidate, name the OIDC client lib (`openid-client` for Node; `oidc-rp` for Go; `openidconnect` crate for Rust; `oidcc` for Elixir; `authlib` for Python) and the session library. Evaluate WorkOS, Clerk, Stytch as fully-managed alternatives — convenience vs sovereignty trade-off (these are US-based). Hetzner-EU-hosted Authentik or Zitadel as self-hosted sovereign options.

### 9. Observability

Downstream of stack choice. Note OpenTelemetry support per language. Hetzner-EU-hosted options (Grafana Cloud EU region, Better Stack, Sentry self-hosted EU).

## Constraints (from earlier ADRs that survive Path D)

- **EU data residency.** All hosting on Hetzner DE/FR or OVH FR. No US sub-processors for primary data path. Brevo (FR) for email per ADR-003.
- **Tenancy migration path** per ADR-001: VPS-per-client (Phase 1) → schema-per-tenant on shared cluster (Phase 2) → cluster + read replica + Patroni (Phase 3). Stack choice must not box us out of any phase transition.
- **Audit log infrastructure** per ADR-007: every privileged action emits an event; events are queryable and tenant-scoped; actor-type field accepts `user`, `agent`, `system` from day 1 even though v1 only writes `user`.
- **Backup/DR** per ADR-006: nightly Postgres dumps + WAL archiving; cross-region replication is Phase 3. Stack must not block this.
- **Encryption + secrets** per ADR-007: SOPS+age secrets at rest, encryption in transit (TLS everywhere), audit logging on every privileged action.
- **Idempotency, cursor pagination, tenant-scoped URL routing** are first-class API design constraints not nice-to-haves.
- **AI-native readiness** seams designed in from line 1: actor-type field already accepts `agent`, MCP server adapter is in scope for v1 even if no agent client exists yet.

## Selection criteria (apply with weights)

Weights are illustrative; tune in the deliverable.

| Criterion | Weight |
|---|---|
| Solo-dev velocity with Claude Code (correctness on greenfield CRUD) | 25% |
| Multi-tenant primitives + RLS / tenant-scoping ergonomics | 20% |
| Operational sustainability at solo scale (memory, deploy, ops tooling) | 15% |
| AI-native readiness (MCP server libs, streaming, agent orchestration) | 10% |
| Talent / sale-ability of the asset to a French CRM consultancy at acquisition | 10% |
| EU-residency-friendliness of hosted dependencies | 8% |
| License compatibility of major dependencies with the leCRM license choice | 7% |
| Greenfield velocity vs comprehension debt at month 12 | 5% |

## Out of scope for this tasket

- Actual implementation. This tasket produces decisions and a research dossier, not code.
- Dependency pinning beyond top-level choices.
- Frontend component library selection (Radix vs Headless UI vs custom) — that's a v0 implementation tasket downstream of this.
- AI agent layer architecture beyond "v1 must not box out v2" — that's ADR-005 territory.

## Reading list (start here)

1. `docs/STRATEGIC-OVERVIEW.md` (note the 2026-05-10 banner re partial supersession of §2 / §4 / §8).
2. `docs/FEASIBILITY-MEMO.md` (note that §2-3 are partially superseded; the strategic frame and risk register still hold).
3. `docs/ICP-ARCHETYPE.md` (lock — Marc, Anne, Pierre constraints feed the velocity/sale-ability criteria).
4. `docs/ARCHITECTURE.md` (note the PENDING REWRITE banner — read for tenancy migration path and service boundary intent, ignore the Twenty-fork specifics).
5. `docs/adr/ADR-001-tenancy-model.md` (locked — feeds DB tenancy primitive evaluation).
6. `docs/adr/ADR-005-ai-agent-tenancy.md` (locked — feeds AI-native readiness criterion).
7. `docs/adr/ADR-006-backup-dr.md` (locked — feeds DB choice).
8. `docs/adr/ADR-007-encryption-secrets-audit.md` (locked — feeds auth library and audit infra evaluation).
9. `docs/adr/ADR-008-clean-room-reimplementation.md` (this tasket exists because of this ADR).
10. **Twenty source as textbook**: clone https://github.com/twentyhq/twenty into a scratch directory OUTSIDE the leCRM repo. Read for: metadata-engine shape, workspace-scoping pattern, audit log emission, OIDC strategy module, GraphQL resolver structure. Take notes; do NOT copy code into leCRM.

## Deliverables

1. **`docs/research/stack-selection.md`** — research dossier in the same shape as `docs/research/fork-management.md`. Sections:
   - §1 Backend language/runtime evaluation matrix
   - §2 Database choice (Postgres flavor + multi-tenant primitives)
   - §3 ORM / query layer choice
   - §4 API surface (REST / GraphQL / MCP) recommendation
   - §5 Frontend choice
   - §6 License selection rationale
   - §7 Auth library + observability stack
   - §8 Build tooling and monorepo strategy
   - §9 Recommendation summary
   - §10 Open questions and decisions deferred to ADR-009

2. **`docs/adr/ADR-009-stack-and-license.md`** — the decision record. Crisp Status / Context / Decision / Consequences / Alternatives Considered / References sections. The research dossier is the evidence base; the ADR is the conclusion.

3. **Optional: a 4-round council validation** of the stack recommendation before locking ADR-009. Pattern: launch the same architect/engineer/researcher/pentester/code-reviewer council (council mode in the Thinking skill) with the dossier as input, produce a debate transcript, refine the ADR if the council surfaces concerns. This is a 30-90 second / ~15 agent calls investment that the previous build-vs-fork decision proved high-leverage.

## Acceptance criteria

- [ ] `docs/research/stack-selection.md` committed with all 10 sections populated.
- [ ] `docs/adr/ADR-009-stack-and-license.md` committed with status=Accepted.
- [ ] License selection explicit and justified.
- [ ] Stack recommendation traces back to the weighted selection criteria; weights are documented and applied transparently.
- [ ] Twenty's design lessons documented in §1 of the research dossier (what we're inheriting as inspiration vs what we're discarding).
- [ ] No Twenty code is copied into the leCRM repo. The repo is greenfield from line 1.
- [ ] (Optional) Council validation transcript appended to the research dossier as §11 if the validation round was run.

## What this tasket explicitly does NOT cover

- Setting up the leCRM repo, scaffolding, dependency installs.
- Writing a single line of v1 implementation code.
- Selecting individual UI components, icon libraries, or build-flags.
- Re-scoping the v0 build sub-taskets in `.taskets/` group `lecrm-v0-build`. That re-scoping happens after ADR-009 lands, in a separate tasket.

## Notes for the executor

This is a research + decision session, not an implementation session. Expected duration: 1-2 working days for the dossier + ADR; +30-90 seconds if the council validation round is run. Use Plan mode if you want to think through the matrix before writing. Use Research / WebSearch to validate base rates and benchmark claims (e.g., Bun production benchmarks for v1 expected load; Phoenix LiveView solo-CRM precedents; Rust + Axum + sqlx multi-tenant CRM precedents).

The outcome of this tasket unblocks the v0 build re-scoping. Until ADR-009 lands, the v0 build sub-tasket group `lecrm-v0-build` Track A (shallow fork) is dead and the other tracks are pending re-scoping against the chosen stack.
