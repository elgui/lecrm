# leCRM Stack Selection: Backend, Database, API, Frontend, License

**Status:** Research artefact — 2026-05-10
**Scope:** Selection of the language/runtime, database flavor, ORM/query layer, API surface, frontend stack, OSS license, auth library, observability stack, and build/monorepo tooling for leCRM v1, under the clean-room reimplementation posture set by [ADR-008](../adr/ADR-008-clean-room-reimplementation.md).
**Authors:** Multi-researcher synthesis (5 parallel agents covering backend, database, license, frontend, API+auth+observability+build), 2026-05-10.
**Council validation:** appended as §11 after the four-round council debate run against this dossier.

---

## 0. Frame and What We Inherit From Twenty as Reference

Twenty's source code is read as architectural reference, not copied. The design lessons we **adopt as patterns** (not as code) are:

- **Schema-per-tenant via `workspace_<base36(uuid)>`.** Confirmed in Twenty's `get-workspace-schema-name.util.ts`. This is the locked tenancy primitive (ADR-001) and survives the clean-room reframe unchanged. We re-implement the pattern in our chosen language with our own connection-routing middleware.
- **Custom-object metadata engine.** `object_definitions`, `field_definitions`, `field_values` shape; dynamic schema generation; permission-aware resolvers. We re-implement against our own dynamic-table-creation pattern (Postgres native) without porting Twenty's TypeORM entities.
- **Workspace-scoped service tokens for agent / API access.** Twenty's API key model maps cleanly to our `CrmAdapter` interface in [ADR-005](../adr/ADR-005-ai-agent-tenancy.md).
- **Audit log emission shape.** Per-object create/update/delete events plus auth/export/admin events. Our `audit_log` table (ADR-007 §3) is structurally similar but extended.
- **OIDC strategy module shape.** Twenty's auth module is a useful blueprint for how to compose multiple OIDC issuers (Google + Microsoft) under one auth surface.
- **Migration-management approach under multi-tenant.** Twenty's `WorkspaceManagerService` iterates workspaces during upgrades. We replace this with **Atlas v1.0**'s `parallel + on_error = CONTINUE` staged rollout (see §2.5) — strictly better than re-implementing per-tenant iteration in application code.

What we explicitly **do not inherit**:

- **Language / framework / ORM / GraphQL choice.** None of TypeScript / NestJS / TypeORM / GraphQL is taken on faith. Each is re-evaluated below against weighted criteria.
- **Twenty's package layout, module names, or domain types verbatim.** Inspiration only.
- **AGPL §13 obligations.** Clean-room ⇒ license freedom (see §6).
- **Twenty's UI shape.** v1 is a CRUD CRM with a Cube.dev-iframe dashboard bridge; v2 layers AI-native interfaces on top. Twenty's React+GraphQL UI is not inherited.

---

## 1. Backend Language and Runtime

### Selection criteria (weighted)

| Criterion | Weight |
|---|---|
| Solo-dev velocity with Claude Code 2026 | 25% |
| Multi-tenant primitives + RLS / tenant-scoping ergonomics | 20% |
| Operational sustainability at solo scale (memory, deploy, ops tooling) | 15% |
| AI-native readiness (MCP server libs, streaming, agent orchestration) | 10% |
| Talent / sale-ability to a French CRM consultancy at acquisition | 10% |
| EU-residency-friendliness of hosted dependencies | 8% |
| License compatibility of major dependencies | 7% |
| Greenfield velocity vs comprehension debt at month 12 | 5% |

### Candidate matrix (5-point scale, weighted total at the bottom)

| Criterion | TypeScript / Node | **Go** | Rust | Elixir / Phoenix | Python / FastAPI |
|---|---|---|---|---|---|
| Solo-dev velocity (Claude Code) | 4 | **5** | 2 | 2 | 4 |
| Multi-tenant primitives | 3 | 4 | 3 | 5 | 3 |
| Ops sustainability | 3 | **5** | 4 | 3 | 3 |
| AI-native readiness (MCP) | **5** | 4 | 3 | 2 | **5** |
| Talent / sale-ability | **5** | 4 | 3 | 1 | 4 |
| EU residency | 4 | **5** | **5** | 4 | 4 |
| License compatibility | 4 | **5** | 4 | 4 | 4 |
| Greenfield-vs-debt at m12 | 3 | **5** | 3 | 3 | 4 |
| **Weighted total / 5** | **3.73** | **4.57** | 2.98 | 2.57 | 3.68 |

### Why Go wins

The 4.57 vs 3.73 margin over TypeScript is decisive on the two highest-weighted criteria:

- **Solo-dev velocity (25%):** A 13-language Claude Code benchmark ([InfoQ, April 2026](https://www.infoq.com/news/2026/04/ai-coding-language-benchmark/)) ranked Go best among statically-typed languages for Claude Code: "47 minutes, cleanest output, zero type ambiguity." TypeScript at $0.62/run with documented type-friction; Rust at $0.54/run with the **widest standard deviation** and test failures Claude could not self-correct; Elixir at "6 hours, half-working" in independent reports.
- **Multi-tenant primitives (20%):** Schema-per-tenant via `SET search_path` at the connection level is the canonical Postgres pattern. Go's [Atlas GopherCon 2025 talk](https://atlasgo.io/blog/2025/05/26/gophercon-scalable-multi-tenant-apps-in-go) documents the exact pattern leCRM needs; sqlc cannot parameterize schema names in query files, which is architecturally correct here — `search_path` lives at the connection, not the query.

Go also wins on **operational sustainability** (15%): single 10-30 MB static binary on Phase 1 VPS (vs Node 100-180 MB idle, Python 60-100 MB per worker), zero-runtime Docker image (`FROM scratch`, ~15 MB), trivially `systemd`-deployed.

### Why Go's prior weaknesses no longer disqualify

The historical knock against Go for AI-native work was **MCP SDK gap**. As of 2025, Go has a **Tier 1 official MCP SDK** ([modelcontextprotocol.io/docs/sdk](https://modelcontextprotocol.io/docs/sdk)) with `mark3labs/mcp-go` as the de-facto community implementation already production-grade. Per ADR-005, the agent runtime is a separate microservice; even if a future MCP gap appears, that microservice can be implemented in TypeScript or Python independently of the CRM core language.

### Why TypeScript is the runner-up, not the primary

TypeScript scores 3.73, lower than Go on velocity (-1), ops sustainability (-2), and greenfield-vs-debt (-2). It wins on AI-native readiness (+1) and talent/sale-ability (+1). For a solo dev shipping in 11-13 weeks, the velocity and ops gaps weigh more than the AI-readiness lead — especially because the AI layer is a separate microservice. **If Guillaume has zero Go experience and a 1-week ramp would consume schedule slack, TypeScript on Hono+Node becomes the conservative call.** The council should explicitly resolve this in §11.

### Why Rust, Elixir, Python are ruled out for v1

- **Rust:** Claude Code + borrow checker is a documented schedule risk on greenfield sprints. The benchmark data (test failures, 60% manual rewrite rate) is not survivable on an 11-week solo-dev clock. Rust remains the right answer for a future Phase-3 hot-path service (audit-log ingester, search indexer); not the CRM core in v1.
- **Elixir:** Best-in-class multi-tenant primitives via Ecto's `prefix:` and Triplex (5/5 — uniquely native), but a 1/5 on talent/sale-ability is a knockout for the €170-340k acquisition thesis. France-specific Elixir hiring market is "demand outweighing supply"; a French CRM consultancy acquirer would inherit a maintenance debt cliff.
- **Python:** Capable across all dimensions but no leading score. GIL + 60-100 MB/worker creates Phase-2 memory pressure on shared infra; Python "for a CRM backend" is an unusual choice for an acquirer (more common in data-science / ML contexts).

### Recommendation

**Primary: Go 1.23+ on `net/http` + Chi router + sqlc + Atlas.** No ORM. Background jobs via `river` (Postgres-native, no Redis dependency). OIDC via `zitadel/oidc` (OpenID Foundation certified). Frontend served via `//go:embed dist/*` for single-binary deploy on Phase 1 VPS.

**Runner-up: TypeScript on Node (Hono framework) + Drizzle + Atlas + `openid-client` v6.** Selected if Guillaume's Go ramp would consume schedule.

---

## 2. Database

### Postgres is the answer; the question is which Postgres

Counter-investigation confirmed no credible non-Postgres alternative at leCRM's scale (1-50 workspaces over 24 months):

- **CockroachDB:** Multi-region distributed SQL with Postgres wire compat. Overkill at single-EU-region 50 workspaces; SERIALIZABLE-by-default has OLTP cost; schema-per-tenant semantics differ from Postgres. Revisit at hypothetical Phase 5 (multi-region EU+US).
- **SQLite + LiteFS:** Per-tenant file isolation is interesting but blocks cross-tenant queries (deduplication, global search) that are on leCRM's roadmap.
- **MySQL/Vitess, MariaDB, YugabyteDB, TiDB:** No fit at this scale, no acquirer signal favoring them, TiDB has Chinese ownership (additional EU sovereignty complication).

**Postgres 17 stays.** ADR-001's schema-per-tenant choice survives the clean-room reframe unchanged.

### 2.1 Hosted Postgres: Phase 1 self-host → Phase 2 Ubicloud DE

| Option | EU residency | Schema-per-tenant fit | PgBouncer | Cost (Phase 2) | Verdict |
|---|---|---|---|---|---|
| **Ubicloud on Hetzner DE** | Full EU (Hetzner AX hardware) | Native; full PG control | Built-in :6432 | ~€62/mo Standard | **Phase 2 primary** |
| OVH managed Postgres | Full EU (FR HQ, Strasbourg/Gravelines) | Good | External PgBouncer | ~€40-120/mo | Fallback |
| Raw Postgres on Hetzner VPS | Full EU | Perfect (own everything) | Self-managed | ~€10-40/mo | **Phase 1** |
| Neon | Frankfurt region but Delaware-incorporated | Mismatch — pushes project-per-tenant | Neon Proxy (proprietary) | $19+/tenant/mo | Rule out |
| Supabase | AWS Frankfurt; US-incorporated | Fights schema-per-tenant; RLS-first opinions | Supavisor | $25-599/mo | Rule out |
| Crunchy Bridge | AWS eu-central-1 (US-owned) | Excellent PG support | Included | $50-200/mo | Pass — AWS substrate ambiguity |

**No first-party Hetzner managed Postgres exists** as of May 2026 (Hetzner's docs show MySQL only via Konsoleh). Ubicloud on Hetzner is the genuine EU-resident managed option; OVH is the credible second.

### 2.2 Schema-per-tenant validation (ADR-001 confirmed; one critical pattern change)

**CVE-2025-12819 (PgBouncer 1.25.1, December 2025)** affects the `track_extra_parameters = search_path` pattern that ADR-001 §Operational specifics planned for Phase 3 transaction-mode pooling. The CVE involves a `search_path` injection via `auth_user`; the recommended mitigation reshapes ADR-001's tenant-switching contract:

- **Old plan (ADR-001):** `SET LOCAL search_path = workspace_<id>` per query, with PgBouncer 1.20+ `track_extra_parameters` carrying the value across transaction-mode pooled connections.
- **New plan:** **Set `search_path` at the Postgres role level via `ALTER ROLE workspace_<id> SET search_path = workspace_<id>, public`** at workspace-provisioning time. Each tenant has its own Postgres role; the application acquires a connection AS that role; `search_path` is inherited from the role and is mode-agnostic.

This is operationally cleaner (no per-query SET, no PgBouncer-version dependency) and removes the CVE-2025-12819 attack surface entirely. **ADR-001 will receive a follow-up note** flagging this change before Phase-2 cut-over.

RLS-per-row alternative remains rejected — no Postgres 16/17 development has improved its safety profile relative to schema isolation at our scale. Database-per-tenant adds 50 PgBouncer pools at 50 workspaces with no isolation benefit. Neon's project-per-tenant is a serverless-architecture pattern that does not translate to self-hosted.

### 2.3 Audit logging integration

Recommendation:

1. **Primary:** Application-level emission via Go HTTP middleware that writes `INSERT INTO audit_log` after every privileged mutation. Carries actor context (user, IP, user-agent, request ID) which Postgres triggers cannot see.
2. **Supplemental safety net:** `AFTER INSERT/UPDATE/DELETE` triggers on the three highest-risk tables only (`contacts`, `deals`, `users`). Scoped to bound migration overhead. Removed at Phase 3 if/when CDC becomes viable.
3. **Never:** System-wide trigger matrix on all tenant tables (write-throughput hit, schema-migration coupling).
4. **Phase 4+ (deferred):** Debezium/CDC over WAL logical replication. Over-engineered at solo scale today.

### 2.4 Idempotency-key pattern (Brandur-style, adapted)

Per workspace schema:

```sql
CREATE TABLE idempotency_keys (
    id              BIGSERIAL    PRIMARY KEY,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ  NOT NULL DEFAULT now() + INTERVAL '24 hours',
    idempotency_key TEXT         NOT NULL CHECK (char_length(idempotency_key) <= 100),
    locked_at       TIMESTAMPTZ,
    request_method  TEXT         NOT NULL,
    request_path    TEXT         NOT NULL,
    request_params  JSONB        NOT NULL,
    response_code   INT,
    response_body   JSONB,
    recovery_point  TEXT         NOT NULL DEFAULT 'started',
    actor_id        BIGINT       NOT NULL
);

CREATE UNIQUE INDEX idempotency_keys_actor_key_unique
    ON idempotency_keys (actor_id, idempotency_key)
    WHERE expires_at > now();

CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys (expires_at);
```

Insert with `ON CONFLICT (actor_id, idempotency_key) WHERE expires_at > now() DO NOTHING RETURNING …`. Empty `RETURNING` ⇒ the operation already completed; return cached response. Pair with `pg_advisory_xact_lock(hashtext(key))` under SERIALIZABLE for the critical mutation path. Expiry reaper job via pg_cron or application scheduler.

### 2.5 Migration tooling: Atlas v1.0

[Atlas v1.0 (December 2025)](https://atlasgo.io/blog/2025/12/23/atlas-v1) is the clear winner for leCRM:

- **Schema-per-tenant URL parameterization:** `atlas migrate apply --url "postgres://host/db?search_path=workspace_<id>"`.
- **Staged rollout:** `deploy` block with `on_error = CONTINUE` and `parallel = N` to limit concurrent DDL locks across schemas.
- **Canary tenant pattern** ([Atlas guide](https://atlasgo.io/guides/database-per-tenant/rollout)).
- **Transactional DDL rollback** is Postgres-native; Atlas's revision table updates atomically.
- **`down` migration generation** for rollback plans.
- **Imports Goose / Flyway / golang-migrate** formats — eases brownfield adoption.

Pair with **pgroll** ([pgroll.com](https://pgroll.com)) for individual zero-downtime breaking-column changes (rename, type-change). Don't replace Atlas with pgroll — use Atlas for tenant sweep orchestration, pgroll for the rare hairy column op.

`pg-osc` deferred: leCRM's SMB data volumes (≤millions of rows per workspace) do not justify online schema change at Phase 2. Reconsider at Phase 3.

### 2.6 Backup/DR validation

**pgBackRest archive-status (April 2026):** [pgBackRest was archived](https://thebuild.com/blog/2026/04/30/after-pgbackrest/) — final v2.58.0, no further security patches. **WAL-G remains canonical.** ADR-006's WAL-G + GPG client-side encryption to Hetzner Object Storage stays.

Per-tenant restore under schema-per-tenant: WAL-G restores the full cluster; isolating a single workspace requires `pg_restore -n workspace_<id>` from a temporary instance into production. Documented in ADR-001 §Backup mechanics.

GDPR Art. 17 vs physical backup retention: ADR-007 §4 already documents the 30-day WAL / 90-day base-backup roll-off as the defensible position. Crypto-shredding (per-workspace key wrap via Vault Transit) deferred to v2 — same as ADR-007.

### Recommendation

PostgreSQL 17, schema-per-tenant. Phase 1: self-hosted on Hetzner CX22-CX32. Phase 2: **Ubicloud Managed Postgres on Hetzner Germany Standard tier** (~€62/mo). Phase 3: HA replica via Ubicloud or migrate to self-managed Patroni on Hetzner Dedicated. PgBouncer 1.25.1+, **`ALTER ROLE` per-workspace `search_path`** (not `track_extra_parameters`). Atlas v1.0 for migrations. WAL-G + GPG to Hetzner Object Storage.

---

## 3. ORM / Query Layer

Downstream of §1's Go choice. Recommendation: **`sqlc` for type-safe query generation; no ORM.**

Rationale:
- Multi-tenant correctness wants explicit tenant predicates (where applicable) and explicit connection-level `search_path` switching. Heavy ORMs hide this.
- sqlc's offline build mode generates Go code from SQL files; the schema name is **not parameterized at the query level** — it's set at connection acquisition. This is architecturally correct under our `ALTER ROLE` pattern.
- Ent and GORM are valid alternatives if Guillaume prefers ORM ergonomics. Trade-off: ent's schema-as-code is elegant but adds a code-generation step; GORM hides SQL in a way that complicates audit / log tracing.

If TypeScript is selected at the council step (runner-up), the equivalent is **Drizzle** (closer to SQL than Prisma; better RLS support; simpler tenant-scoping). Prisma's Active Record style fights multi-tenant ergonomics.

---

## 4. API Surface: REST + Thin MCP Adapter (GraphQL Deferred)

### Decision matrix

| Option | Verdict | Reason |
|---|---|---|
| REST-only | Strong | Boring, OpenAPI 3.1 mature, agent-friendly, idempotency + cursor pagination idiomatic |
| GraphQL-only | Reject for v1 | Twenty's choice — correct for them, costly here. Tenant-scoping at every resolver, depth/complexity limits, N+1 mitigation, two schemas (web + agent) |
| REST + GraphQL hybrid | Reject | Two endpoints to authorize, two schemas to evolve — solo-dev tax with no v1 benefit |
| **REST + thin MCP adapter** | **Accept** | REST is the durable contract; MCP is a ~300-LOC translator wrapping REST per ADR-005. Acquirer inherits REST. AI agents get MCP. |
| gRPC internal / REST external | Reject | Over-engineered at solo scale |
| tRPC | Conditional | Only if both backend and frontend land on TS (runner-up scenario). Eliminates OpenAPI generation. |

### MCP SDK status by language (May 2026)

| Language | SDK | Status | Streamable HTTP |
|---|---|---|---|
| TypeScript | `@modelcontextprotocol/sdk` (official) | **Production** | Full |
| Python | `mcp` (official) | **Production** | Full |
| **Go** | `mark3labs/mcp-go` (community, de-facto standard) | **Production** | Yes (over `net/http`) |
| Rust | `rmcp` (official, v0.16.0) | Pre-1.0 but usable | Yes |
| Elixir | `hermes-mcp` (community) | Functional | Partial |

**Streamable HTTP is the mandatory transport** for a multi-tenant network MCP server. stdio is for local single-process tools (Claude Desktop). MCP spec 2025-03-26 formalised this.

### REST design constraints (locked from tasket)

- URL-prefix versioning: `/v1/…`. Boring, predictable, cache-friendly.
- Tenant routing via subdomain: `acme.lecrm.fr/v1/contacts`. Subdomain → workspace lookup happens in one auth middleware; downstream handlers receive a typed `WorkspaceContext` from request context.
- Cursor pagination: opaque base64 cursor, `has_next_page` bool, no offset/limit pagination on collections expected to grow.
- Idempotency: `Idempotency-Key` header, deduplicated against the per-workspace `idempotency_keys` table (§2.4).
- OpenAPI 3.1 spec generated from Go handlers via `ogen` or `oapi-codegen` — published at `/v1/openapi.json` for agent consumption and frontend codegen.

### Recommendation

REST-first with OpenAPI 3.1; thin MCP adapter as a separate service (or, equivalently, a separate package in the same Go monorepo) wrapping REST calls behind the `CrmAdapter` interface from ADR-005. Streamable HTTP transport. Workspace-scoped service tokens carried in `Authorization: Bearer …`. GraphQL deferred to v2 if a design partner explicitly needs it.

---

## 5. Frontend

### Selection (criteria slightly tuned for frontend)

| Criterion | Weight |
|---|---|
| Solo-dev velocity (Claude Code, CRUD UI) | 25% |
| AI-native readiness for v2 (chat / voice / streaming) | 20% |
| Multi-tenant subdomain build/deploy ergonomics | 15% |
| Operational sustainability (build time, bundle size) | 15% |
| Talent / sale-ability | 10% |
| Accessibility / keyboard ergonomics for power users | 10% |
| Component library availability (forms, tables, kanban, calendar) | 5% |

### Candidate scores (out of 10, weighted total)

| Candidate | Weighted total |
|---|---|
| **React 19 + Vite + TanStack Router/Query + shadcn/ui** | **~9.4** |
| Vue + Nuxt + Nuxt UI | ~7.5 |
| Svelte 5 + SvelteKit + shadcn-svelte | ~7.5 |
| Phoenix LiveView (Elixir-only) | ~6.8 |
| Solid + SolidStart | ~6.4 |
| HTMX + server-rendered partials | ~5.8 |
| Inertia.js + React/Svelte | ~6 (varies by backend) |
| Qwik | ~3 |

### Why React + Vite + TanStack wins

- **Claude Code velocity:** TanStack ships official Claude Code agent skills (`@tanstack/intent`); shadcn/ui is the reference React component system; React's training-corpus density is highest. TanStack Start v1 (January 2026) is now production-grade.
- **AI-native readiness:** Vercel AI SDK 6 (December 2025) provides `useChat`, `useCompletion`, SSE streaming. React/Next.js example density is overwhelming. v2 chat/voice features bolt onto a v0 React app without rewrite.
- **Component library:** shadcn/ui (Radix + Tailwind) covers everything leCRM v1 needs: `TanStack Table` (cursor pagination, sorting, filtering), `DnD Kit` (kanban), `react-big-calendar` (calendar view), `cmdk` (command palette), `react-hook-form + zod` (forms with validation).
- **Multi-tenant subdomain:** Single Vite build; tenant detected at runtime from `window.location.hostname` or server-injected config. Caddy/Traefik handles wildcard SSL — frontend never touches certs.
- **Acquirer perspective:** React's 44.7% Stack Overflow share, 1,556+ open Paris React positions ([agency-partners.com 2025](https://agency-partners.com/reports/market-insights/france-paris-frontend-react)). Maximum acquirer legibility.

### v0 → v2 stack continuity (decisive)

Resist the temptation to use HTMX or Inertia for v0 and switch to React for v2 chat features. Migration cost (6-8 weeks for a solo dev) > velocity savings. v2 features (streaming chat, voice dictation, real-time agent push) require fine-grained reactivity, hooks for streaming state, WebSocket subscription management — native to React, bolt-on to HTMX/LiveView. **One stack v0 through v2.**

### Why LiveView is not chosen

LiveView scores 6.8 weighted on its own, with technical excellence (Phoenix PubSub, native real-time, near-zero JS bundle, BEAM concurrency). The decisive deduction is the acquirer-discount factor (1/10) compounded with Elixir-only constraint. We chose Go for the backend (§1), so LiveView is moot.

### Recommendation

**React 19 + Vite + TanStack Router v1 + TanStack Query + shadcn/ui + Radix UI + TanStack Table + DnD Kit + cmdk + react-hook-form + zod.** Single Vite build, runtime tenant detection. Bundle target: ≤350 KB gzip.

Frontend served either:
- **Embedded into the Go binary via `//go:embed dist/*`** — single binary deploy on Phase 1 VPS. Tightest possible coupling. Recommended.
- **Caddy proxying static SPA on `web.acme.lecrm.fr` and Go API on `api.acme.lecrm.fr`.** More services but independent deploys. Reserve for Phase 2 if/when build coupling becomes a constraint.

---

## 6. License Selection

### Decision: Apache 2.0 (with FSL-2.0-Apache-2.0 as a credible upgrade path)

### Recent precedent (2024-2026) shaping the call

- **Cal.com (April 2026)** went closed-source after 5 years of AGPL; community read it as commercial pivot under "AI security" cover. The relicensing damaged trust irrecoverably ([Hacker News thread, 47780456](https://news.ycombinator.com/item?id=47780456)). Strongest argument against starting open and converting later — **pick the right license at launch**.
- **Sentry (BSL → FSL, late 2024):** Sentry abandoned BSL because compliance departments "cannot approve blanket use" — every BSL deployment writes its own Additional Use Grant. FSL standardises the variables (2-year non-compete, then auto-converts to Apache 2.0). Strongest argument for FSL-over-BSL if competitor protection is needed.
- **HashiCorp Terraform (BSL, Aug 2023) → OpenTofu fork:** BSL relicensing of an established tool caused permanent fork and trust rupture. leCRM has no community at launch — this risk is ours to avoid by not relicensing later.
- **Twenty CRM (AGPL + commercial dual, YC-backed):** the model leCRM was originally going to inherit. Open-source pitch + enterprise gate.
- **n8n (Sustainable Use License):** custom "fair-code" — works, but requires a lawyer to navigate.
- **EspoCRM, Bitwarden, Vaultwarden:** AGPL + commercial dual is the conventional self-hostable-CRM pattern.

### Per-candidate compressed evaluation

| License | Trust signal (Marc/Anne/Pierre) | Competitor protection | Asset hardening (€170-340k acquirer) | Verdict |
|---|---|---|---|---|
| MIT | Maximum | Zero | Cleanest | Viable, marginally weaker than Apache 2.0 |
| **Apache 2.0** | Maximum | Zero | **Cleanest + patent grant** | **Primary recommendation** |
| AGPL-3.0 | Strong (technically literate audience) | Strong vs cloud vendors | **Acquirer friction** ([Open Core Ventures](https://www.opencoreventures.com/blog/agpl-license-is-a-non-starter-for-most-companies), MindCTO) | Reject as primary; matches Twenty's posture but narrows buyer pool |
| BSL | Yellow flag in dev community | Strong during BSL window | Variability problem (per-deployment Additional Use Grant); HashiCorp association | Reject — FSL is strictly superior |
| FSL-2.0-Apache-2.0 | Better than BSL (Sentry framing) | Strong for 2 years | Auto-converts to Apache 2.0 in 2028 — likely before acquisition | **Credible upgrade path** if competitor protection becomes a real concern post-launch |
| Elastic v2 / SSPL | Negative in dev community | Anti-cloud-vendor (irrelevant threat for leCRM) | Friction with no upside | Reject |
| Polyform | Unknown to non-lawyers | Comparable to FSL | Zero precedent; legal explanation cost | Reject |
| Proprietary | Undermines "transparent pricing" pitch | Maximum | Cleanest IP but smallest buyer pool | Reject — destroys narrative |
| Dual-license (AGPL + commercial) | Strong | Strong | CLA overhead | Defer — possible v2 monetisation pivot |

### Why Apache 2.0 specifically over MIT

Both are functionally identical from the user's perspective. Apache 2.0's **explicit patent grant** matters at acquisition: a French CRM consultancy's legal team will see Apache 2.0 as a green flag (it's the Kubernetes / TensorFlow / Cassandra license). Marginal hygiene win, zero downside.

### Why not AGPL even though it matches Twenty

AGPL "destroys startup valuation" in M&A ([MindCTO copyleft analysis](https://mindcto.com/insights/copyleft-threat-agpl-risk)). At leCRM's €170-340k acquisition window, a French CRM consultancy is unlikely to pay legal fees to navigate AGPL — they'll walk. The **clean-room reframe gave us license freedom; we should use it**, not voluntarily reinstate AGPL §13.

### ICP reality check

- **Anne** (wine distributor): never looks at the license; trust comes from Leo + product.
- **Marc** (consultancy partner): may ask "is this open source?" — Apache 2.0 says yes cleanly, no §13 explanation.
- **Pierre** (greentech founder, Postgres/Supabase user): most license-aware persona. Will prefer permissive over copyleft for his own modifications. Apache 2.0 removes friction.

### Recommendation

**Apache 2.0.** If, post-launch, a real competitor emerges that appears to be tracking the public codebase, move to FSL-2.0-Apache-2.0 (the 2-year non-compete window converts to Apache 2.0 — predictable, defensible, with Sentry/GitButler/Convex/Liquibase precedent). Do **not** start AGPL or proprietary.

---

## 7. Auth Library and Observability Stack

### 7.1 OIDC client library (locked to Go choice)

**`zitadel/oidc`** (OpenID Foundation certified for both RP and OP profiles). Active. Pairs naturally with Zitadel as the IDP if/when we migrate from self-hosted Authentik. Backup option: `coreos/go-oidc` v3 (also stable, simpler, RP-only).

WebAuthn: `go-webauthn/webauthn`. TOTP: `pquerna/otp`. Session: stateless JWT in cookie (signed with rotated key in Vault).

### 7.2 Auth strategy by phase

**v0 (≤4 clients, weeks 1-11): self-hosted Authentik on Hetzner.**

- Single Compose service (Authentik 2025.10 removed the Redis dependency — caching now Postgres-backed).
- OIDC upstream providers: Google Workspace, Microsoft Entra.
- Built-in TOTP MFA.
- ~0 additional infra cost (shares the Phase 1 VPS).

**v1 (first paying clients, weeks 12+): Zitadel Cloud EU/CH.**

- Removes ops burden, adds SLA, stays sovereign (Switzerland adequacy decision applies).
- Migration from Authentik is one configuration event: change OIDC discovery URL.
- Free tier exists; pay-as-you-go.

**Rule out US-managed (WorkOS, Clerk, Stytch, Auth0):** routing French SMB identity data through US subprocessors creates procurement friction that contradicts leCRM's positioning. WorkOS' EU-hosting "Vault" product is in development as of mid-2026 — re-evaluate when GA.

### 7.3 Observability stack by phase

**v0: LGTM self-hosted on Hetzner Compose.**

- Loki + Grafana + Tempo + Prometheus + OpenTelemetry Collector.
- ~1.1 GB additional RAM; comfortably runs on Hetzner CX22 (4 GB, €4.35/mo) alongside leCRM and Postgres.
- Configuration: ~100 lines of YAML across services.
- Adequate for ≤4 clients.

**v1: Grafana Cloud EU (Frankfurt) free tier first.**

- 10k active series, 50 GB logs, 50 GB traces — likely sufficient through Phase 2.
- Pro tier: $19/mo + usage. EU residency confirmed.
- Migration from self-hosted: re-point OTel Collector exporters. Hours, not days.

**Alternative at v1: SigNoz self-hosted on Hetzner CX31 (8 GB, €9.66/mo)** if log/trace volume blows past Grafana Cloud free tier and self-hosted is preferred. ClickHouse-backed; OTel-native; Datadog-class UI.

### 7.4 OpenTelemetry SDK quality

- Go: `go.opentelemetry.io/otel` — reference-implementation quality. Stable traces + metrics. Logs production-ready.
- (TypeScript: `@opentelemetry/sdk-node` v2.0 (2025) — for runner-up scenario.)

---

## 8. Build Tooling and Monorepo Strategy

### 8.1 Layout (Go primary)

```
lecrm/
├── go.work
├── go.mod                      # leCRM root module
├── apps/
│   ├── api/                    # main HTTP server + MCP adapter
│   │   ├── cmd/lecrm-api/
│   │   ├── internal/
│   │   │   ├── auth/
│   │   │   ├── audit/
│   │   │   ├── tenancy/        # ALTER ROLE search_path provisioning
│   │   │   ├── crm/            # contacts, deals, companies, custom objects
│   │   │   ├── api/            # REST handlers + OpenAPI gen
│   │   │   └── mcp/            # mark3labs/mcp-go server
│   │   └── go.mod              # if split into separate module
│   └── web/                    # React + Vite + TanStack
│       ├── src/
│       ├── package.json
│       └── vite.config.ts
├── packages/
│   ├── db/                     # sqlc-generated Go code + Atlas schema
│   ├── shared-types/           # OpenAPI-generated TS types for web/
│   └── tools/                  # mage tasks, migration runners
├── deploy/
│   ├── compose/
│   │   ├── lecrm-app.yml
│   │   ├── postgres.yml
│   │   ├── authentik.yml
│   │   └── observability.yml   # LGTM stack
│   └── caddy/
└── docs/
```

`apps/api` embeds `apps/web/dist` via `//go:embed dist/*` for single-binary deploy. Go binary serves API on `/v1/*` and frontend SPA on `/*`.

### 8.2 Frontend toolchain (under React runner-up scenario, this becomes pnpm + Turborepo)

For the recommended Go-primary scenario, frontend is its own pnpm + Vite project under `apps/web`. No Turborepo needed at one frontend + zero TS backend services. Add pnpm workspace if a second TS package emerges (e.g., shared MCP client).

### 8.3 Migration tooling

**Atlas v1.0** (also chosen in §2.5). One declarative HCL file per migration. CI pipeline:

```bash
atlas migrate diff <name> --env local
atlas migrate lint --env local
atlas migrate apply --env staging
# canary one workspace
atlas migrate apply --env production --url "...?search_path=workspace_canary"
# parallel sweep
atlas migrate apply --env production --tenants-from sql --parallel 3 --on-error CONTINUE
```

### 8.4 Background jobs

**`river`** (Postgres-native, no Redis, written in Go). Jobs durable in same Postgres cluster as the CRM data. Trade-off: less battle-hardened than Sidekiq-class systems; production-used as of 2025. Acceptable for v1; reconsider at Phase 3 if throughput requires Redis-backed job queue.

### 8.5 Versioning

`semver 0.x` from first commit. `1.0.0` only at public-API stability — likely after the first 5 paying clients have shaped the API surface. CalVer reserved for end-user-facing release notes if useful (Authentik does this well).

### 8.6 CI/CD

GitHub Actions:
1. `go test ./...` (unit + integration via testcontainers-go on Postgres).
2. `pnpm test` for frontend.
3. `atlas migrate lint` to block destructive migrations from being merged.
4. `golangci-lint run`.
5. `gosec` for security linting.
6. Build single Docker image (multi-stage: Vite build → Go build → final scratch image).
7. Deploy to Hetzner via Dokku / Compose pull-and-restart.

---

## 9. Recommendation Summary

| Dimension | Decision | Notes |
|---|---|---|
| Backend language/runtime | **Go 1.23+** | TS+Hono runner-up if Go ramp consumes schedule |
| Web framework | `net/http` + Chi | `gorilla/mux` retired; Chi is the boring stable router |
| Query layer | **`sqlc`** (no ORM) | `ent` or `GORM` only if council prefers ORM ergonomics |
| Database | **PostgreSQL 17** | self-host Phase 1 → **Ubicloud DE** Phase 2 |
| Tenancy primitive | **schema-per-tenant via `ALTER ROLE search_path`** | CVE-2025-12819-clean; supersedes ADR-001's `track_extra_parameters` plan |
| Migrations | **Atlas v1.0** (`parallel + on_error=CONTINUE`); `pgroll` for breaking column ops | |
| Backups | WAL-G + GPG → Hetzner Object Storage | pgBackRest archived 2026-04 |
| Idempotency | Brandur-style partial unique index | per-workspace schema |
| API surface | **REST + thin MCP adapter** | Streamable HTTP transport; OpenAPI 3.1; URL-prefix versioning. GraphQL deferred to v2. |
| MCP SDK | `mark3labs/mcp-go` | community but de-facto standard; Tier 1 official Anthropic Go SDK in development |
| Frontend | **React 19 + Vite + TanStack Router/Query + shadcn/ui + Radix UI** | embedded in Go binary via `//go:embed` |
| AI SDK | Vercel AI SDK 6 | native React support; v2 chat/voice without rewrite |
| Auth (v0) | **Authentik 2025.10 self-hosted** | Redis-free since 2025.10 |
| Auth (v1+) | **Zitadel Cloud EU/CH** | OIDC-native migration; free tier |
| OIDC client | `zitadel/oidc` | OpenID Foundation certified |
| Observability (v0) | **LGTM Compose stack on Hetzner** | ~€4 extra RAM cost |
| Observability (v1+) | **Grafana Cloud EU free tier**, then SigNoz self-host | EU-resident throughout |
| Background jobs | **`river`** (Postgres-native) | no Redis dependency at v1 |
| Build / monorepo | Go workspaces (`go work`) | pnpm + Vite under `apps/web` |
| **License** | **Apache 2.0** | FSL-2.0-Apache-2.0 as upgrade path if competitor protection becomes real |

### Honest 11-13 week feasibility check

Per ADR-008's R4 estimate: 1-2 weeks reading Twenty as textbook + 10-12 weeks scratch implementation. Under the recommended Go stack:

- Week 1-2: Twenty reading; scaffolding (`go.work` + Atlas + Authentik Compose + LGTM stack + Caddy + first OIDC login).
- Week 3-4: tenancy primitive (`ALTER ROLE` provisioning + Phase-1 single-workspace bootstrap), auth flow end-to-end, audit log middleware.
- Week 5-6: contacts + companies + deals CRUD; cursor pagination; idempotency keys; Atlas migration sweep tooling.
- Week 7-8: custom-object metadata engine (definitions + fields + values); REST handlers with OpenAPI gen; React table + form components via shadcn.
- Week 9-10: pipelines + kanban view (DnD Kit); calendar view; OIDC SSO with Google + Microsoft; basic RBAC.
- Week 11-12: email logging integration (Gmail + IMAP + Outlook basic sync); search (typesense or pg full-text); Cube.dev iframe dashboard bridge.
- Week 13: hardening, MCP adapter integration, first Design Partner deploy.

Schedule risk: weeks 7-8 (custom-object metadata engine) and weeks 11-12 (email integration). Both have non-Claude-Code-compressible correctness work. Schedule slack: ~1 week if all goes well; -2 weeks if metadata engine is harder than estimated. **Council should debate whether this slack is realistic.**

---

## 10. Open Questions and Decisions Deferred to ADR-009

1. **Guillaume's Go ramp.** If existing Go exposure is < 2 weeks, the §1 velocity advantage over TS may invert in weeks 1-3. ADR-009 should record the actual decision basis (clean Go selection vs TS-fallback-due-to-ramp). The council validation is the right forum for this.
2. **`ALTER ROLE search_path` provisioning flow.** The CVE-2025-12819 mitigation requires per-workspace Postgres roles created at signup. ADR-001 needs a follow-up note documenting the new pattern; the v0 build sub-tasket re-scoping must include "provision Postgres role per workspace" as an explicit step.
3. **Authentik now or Zitadel Cloud now.** Migrating Authentik → Zitadel at v1 is one config change. But starting on Zitadel Cloud EU free tier eliminates that migration entirely. ADR-009 should resolve.
4. **Background-jobs choice (`river` vs `asynq`).** `river` is Postgres-native (Redis-free) but newer. `asynq` is mature but adds a Redis service to the Phase 1 VPS. Council should weigh in.
5. **Frontend deploy mode.** Embed-in-binary (`//go:embed`) is operationally simplest but couples deploys. Caddy → static SPA + Caddy → API gives independent deploys. At ≤4 clients, simplicity wins; the question is whether to plan the split now.
6. **MCP adapter location.** Same monorepo as Go API (cleaner) vs separate microservice (per ADR-005's Tier-2 service architecture). The two options are reconcilable: same monorepo, separate binary, separate Compose service.
7. **OpenAPI codegen for the React frontend.** Auto-generate TS types from `/v1/openapi.json` (boring, accurate) vs hand-write TS types (faster initially, drifts). Recommend codegen via `@hey-api/openapi-ts`.
8. **Custom-object metadata engine implementation.** Three patterns surface:
   (a) **Dynamic table creation per workspace per object** — Twenty's pattern. DDL at runtime; Postgres handles it; complicates migrations.
   (b) **Single `field_values` EAV table per workspace.** Slow at scale but simple.
   (c) **JSONB `data` column on a generic `objects` table.** Flexible but loses indexability.
   Recommend (a) for v1; ADR-010 will document.
9. **License application timing.** Apache 2.0 LICENSE file goes in at the first commit. NOTICE file with `Copyright (c) 2026 GB Consult SARL`. Decide whether to also include a CLA from day one (deferred — solo dev, no contributors yet).
10. **Council validation of this dossier.** Run before locking ADR-009. See §11 below.

---

## 11. Council Validation

Five-voice council ran in parallel against the dossier and the ADR-009 Proposed draft on 2026-05-10: Architect (Winston), Engineer (Amelia), Researcher (Ava), Pentester, Code Reviewer. Each produced a structured critique. Synthesis below; full critiques retained in agent task outputs.

### 11.1 Verdict consensus

All five voices **concur with the headline stack decisions** (Go + Postgres 17 schema-per-tenant + REST + thin MCP + React 19 + Apache 2.0 + Authentik→Zitadel + LGTM). No voice argued to relitigate language, framework, or license at the headline level. The pre-Accepted refinements are calibration and seam-tightening, not stack-changing.

### 11.2 Evidence corrections (Researcher Ava — highest priority)

Two evidentiary problems were found in the dossier as written:

**11.2.1 InfoQ benchmark misquote.** The "47 minutes, cleanest output, zero type ambiguity" characterization in dossier §1 is not in the source. The actual benchmark (Yusuke Endoh, Ruby committer; simplified Git task across 13 languages, 20 runs each) reports Go at **$0.50/run, 101.6 seconds average, 40/40 tests passed, ranked 4th overall** — behind Ruby, Python, and JavaScript. Headline finding: **dynamic languages outperform statically-typed languages by 1.4-2.6× on cost and time in Claude Code contexts**. Go is the best statically-typed option but is not the best language overall in the benchmark. The benchmark task (CLI tool with file I/O) is non-representative of leCRM's HTTP/multi-tenant CRUD workload — generalizability is suggestive, not proven. **The directional finding (Go beats TypeScript by ~40% on cost; meaningfully better statically-typed Claude Code experience) survives. The narrative around it must be tightened.**

**11.2.2 CVE-2025-12819 framing.** The advisory does **not** recommend `ALTER ROLE SET search_path` as the canonical mitigation. The advisory recommends: (a) remove `search_path` from `track_extra_parameters`; (b) unset `auth_user`; (c) use fully-qualified object names in `auth_query`; (d) upgrade to PgBouncer 1.25.1+. The CVE only triggers under three simultaneous non-default conditions that leCRM's planned configuration doesn't naturally hit. **The `ALTER ROLE` pattern remains the right architectural choice on its own merits** (operational elegance, mode-agnostic, role-defaults inheritance) — but the causal framing "CVE forced this change" is over-claimed. The pattern is recommended for architectural cleanliness and as a CVE-clean side effect, not as the CVE advisory's prescribed fix.

**11.2.3 Other evidence calibration.** Sentry's BSL→FSL transition was November 2023 (not "late 2024"); TanStack Start v1.0 shipped March 2026 (Vite migration June 2025; "production-ready" by January was the migration milestone, not v1); Ubicloud announced a 26% price increase effective May 1, 2026 — Phase-2 Standard-2 tier is now ~€78/mo (was €62); pgBackRest is "unmaintained, Percona/coalition funding ongoing" not formally "archived" (repository archive flag is not set); the "Twenty CLA-ratchet 35-50%" base rate is uncalibrated — comparables (Elastic/HashiCorp/MongoDB) are late-stage/public not seed; reasonable range 15-40%. None of these change the directional decisions; all need correction in the public-facing documentation.

### 11.3 Architectural seams to tighten (Architect Winston)

**11.3.1 Workspace provisioning is a multi-statement transaction without a defined boundary.** ADR-009 §2 names `CREATE ROLE + ALTER ROLE` for tenant provisioning but does not specify the transaction shape. **Refinement R1: a single `SECURITY DEFINER` Postgres function `lecrm_provision_workspace(uuid)` owned by a dedicated `lecrm_provisioner` role**, called by the application via one SQL call, encapsulating CREATE ROLE + CREATE SCHEMA + ALTER ROLE search_path + ALTER DEFAULT PRIVILEGES + grants. Closes the orphan-role / orphan-schema gap. The provisioner credential is a Tier-0 secret (annual rotation) that has no home in ADR-007 today.

**11.3.2 Custom-object DDL needs default privileges.** ADR-010's planned per-tenant DDL pattern (deferred from §10) requires `ALTER DEFAULT PRIVILEGES IN SCHEMA workspace_<id> GRANT ALL ON TABLES, SEQUENCES TO workspace_<id>` set at provisioning. Postgres does **not** automatically grant tenant-role privileges on tables created later in that schema. Easy to forget, silently breaks runtime DDL.

**11.3.3 Workspace role grants must be explicit.** Add `GRANT USAGE, CREATE ON SCHEMA workspace_<id> TO workspace_<id>; REVOKE CREATE ON SCHEMA public FROM workspace_<id>; REVOKE ALL ON ALL TABLES IN SCHEMA public FROM workspace_<id>` to provisioning. Prevents lateral expansion via `public`-schema utility tables/functions and constrains the role to its own schema.

**11.3.4 MCP adapter location decision is being deferred when it shouldn't be.** **Refinement R6: same Go module, separate `cmd/lecrm-mcp/main.go` binary, separate Compose service.** Same module gives shared types, shared sqlc, one `go test ./...`. Separate binary gives crash isolation and independent rebuild/redeploy cadence (per ADR-005's Tier-2 architecture). Promote from §10 to §4 as a binding decision.

**11.3.5 Migrations need a separate binary.** **Refinement R9: `cmd/lecrm-migrate/main.go`** invoked as a Compose pre-deploy job (not from inside the runtime). Migrations need separate credentials (DDL-capable role, not the runtime app role), separate scheduling, clean fail-closed semantics.

**11.3.6 AI SDK ↔ MCP framing is not byte-compatible.** **Refinement R7: a one-paragraph note in §4** that the React-side AI integration (Vercel AI SDK 6 `useChat`) goes through a leCRM REST/SSE endpoint that adapts MCP responses to the AI SDK wire format. The React app does not speak MCP directly. Prevents a wasted v2 week.

**11.3.7 OIDC `sub` claim format differs Authentik vs Zitadel.** **Refinement R5: store `(issuer, sub)` tuple from day 1**, not raw `sub`, as the user identity key. MFA enrolment doesn't migrate either (TOTP secrets are IDP-internal); document as a known v0→v1 ops moment.

**11.3.8 River schema location.** **Refinement R11: dedicated `river_<workspace>` schema per tenant** (not `public`, not the workspace's own schema). Keeps river job state tenant-scoped, makes Phase-1→Phase-2 migration trivial (just dump/restore the river schema alongside the workspace schema).

### 11.4 Implementation feasibility (Engineer Amelia)

**11.4.1 ADR-010 (custom-object metadata engine) must be authored before week 5**, not week 7. Two-connection DDL model spike at week 4. The metadata engine is the single highest-risk implementation block per the dossier and ADR-008 R4.

**11.4.2 Google OAuth app review is an external blocker** that takes 4-6 weeks for production OAuth scopes (Gmail readonly, send, drafts). Must be initiated at week 5-6 or it blocks the week 11-12 email-integration deploy. Microsoft Entra has a similar but faster review.

**11.4.3 Go ramp checkpoint needs concrete week-2 litmus tests**, not "track velocity in week 1." Recommend three tests by end of week 2: (a) implement a minimal HTTP handler with sqlc-typed queries against a local Postgres; (b) write a workspace-scoping middleware with idiomatic Go context propagation; (c) get `golangci-lint` clean on the scaffolding. If any of the three is blocked > 4 hours, switch to TypeScript+Hono fallback.

**11.4.4 Schedule honesty.** P50 achievable; **P80 not achievable at current scope**. Mitigations: bound email scope to Gmail-only at v0 (Microsoft Outlook + IMAP at v1); resolve metadata engine pattern early (week 5 ADR-010); accept that "search" may ship as pg full-text only at v0 (typesense at v1).

### 11.5 Security and isolation gaps (Pentester)

**Top 5 risks ranked HIGH→LOW (severity × probability):**

| # | Risk | Severity × Prob | Refinement |
|---|---|---|---|
| 1 | **Service token design unspecified** (storage / scope / expiry / revocation). Blocks any external API consumer including the MCP adapter. | HIGH × HIGH | Add `§4.1 Service Token Design` to ADR-009: Argon2id-hashed at rest (never plaintext); scopes include read-only / read-write / mcp-enabled flags; 1-year default expiry with extension; synchronous DB-lookup revocation; `actor_type` claim in token metadata (`human_api`, `mcp_agent`, `internal_service`) for audit-log attribution. |
| 2 | **Cookie scope** must be per-subdomain (`Domain=acme.lecrm.fr`) not parent (`Domain=lecrm.fr`). One-line bug = cross-tenant session sharing. | HIGH × MED | Document explicitly in ADR-009 frontend section. |
| 3 | **River job tenant isolation** is undocumented. Job payloads must contain only IDs (no PII directly); river_worker role must `SET ROLE workspace_<id>` before data access. | MED × MED | Specify in ADR-009 §8 alongside Refinement R11 (schema location). |
| 4 | **Ubicloud PgBouncer auth_query mode**, not auth_file with credentials in cleartext. | MED × LOW | Add to TO RESOLVE: verify Ubicloud Standard-2 PgBouncer config supports auth_query. |
| 5 | **Authentik admin credential** must be in SOPS secret manifest (currently absent from ADR-007); admin interface IP-restricted via Caddy. | MED × LOW | Add to ADR-007 follow-up TO RESOLVE. |

**Plus required additions:**
- `security.workspace_id_mismatch` event added to ADR-007's audit-event catalogue. When an inbound request's Bearer-token workspace_id disagrees with the subdomain-derived workspace_id, **the token claim is authoritative**, request is rejected 401, and the mismatch is logged with `actor_ip`, `claimed_workspace_id`, `token_workspace_id`, `subdomain_workspace_id`.
- CSP header for the embedded SPA: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'` (set in Caddy or the Go static handler).
- `govulncheck ./...` and `go mod verify` added to the CI pipeline alongside `gosec` and `golangci-lint`.
- MCP adapter rate-limiting: 60 req/min per `(workspace_id, token_id)` tuple, in-process (`golang.org/x/time/rate`).
- MCP recursive tool-call depth max 5.
- Per-workspace Prometheus labels (`workspace_id`) on all counters/histograms for anomaly detection.
- Audit log writes must be **fail-closed** on the mutation path (a write that cannot be audit-logged must reject the mutation) — must be tested.
- External MCP clients receive **unsanitized** CRM data (the ADR-005 sanitizer lives in agent-runtime, not the MCP adapter); document as a known v1 gap and as a responsibility transfer to the MCP client.

### 11.6 Document quality and governance (Code Reviewer)

**11.6.1 Critical: ADR-001 must be directly edited**, not "follow-up note added later." ADR-001's "Tenant boundary security" section names `SET LOCAL search_path` in TypeORM — both factually wrong under the clean-room ADR-009 reframe. ADR-001's "Operational specifics" plans the phase-3 `track_extra_parameters` upgrade — now superseded. ADR-001's TO RESOLVE item 1 is now obsolete. **Edit ADR-001 with three annotations** marking superseded sections and pointing readers at ADR-009 §2.

**11.6.2 Critical: ADR-005's "substantively survives" claim is misleading.** ADR-005 was authored for TypeScript/NestJS Tier-2 with Redis hot-cache and GraphQL-to-Twenty. ADR-009 eliminates TypeScript (CRM core), GraphQL (API surface), and Redis (background jobs are Postgres-native via `river`). The TypeScript `CrmAdapter` interface in ADR-005, the Redis cache section, and the GraphQL adapter are all now contradicted by ADR-009. **Promote from "Neutral" to "Negative" with new TO RESOLVE-12: confirm agent-runtime language (TS or Go) + Redis-or-Postgres session store + GraphQL-or-REST adapter contract before agent-runtime build (post-v1).**

**11.6.3 Critical: Promote two TO RESOLVE items to Decisions.**
- **§7 Auth — Authentik vs Zitadel from day one.** Bind: **Authentik 2025.10 self-hosted at v0 by default; switch to Zitadel Cloud EU from day 1 if Guillaume estimates Authentik-Zitadel migration cost at >4 hours including `(issuer, sub)` user-record migration.** Decided at scaffolding (week 1).
- **§4 API surface — MCP adapter location.** Bind: **same Go monorepo, separate `cmd/lecrm-mcp/` binary, separate Compose service.** No remaining ambiguity.

**11.6.4 Major fixes.**
- CVE-2025-12819 added to References (most safety-critical change in the ADR; deserves a References entry not just an inline link).
- Caddy description fixed: Caddy terminates TLS and proxies all traffic to a single Go binary; Go internally routes `/v1/*` to REST and `/*` to embedded SPA.
- FSL 2028 auto-conversion timing added to §6: "the 2-year non-compete window from a 2026 launch converts to Apache 2.0 in 2028 — likely before the acquisition window closes, so the FSL upgrade is a temporary instrument that lands on Apache 2.0 regardless."
- `(as of May 2026)` qualifier on the Hetzner-managed-Postgres-doesn't-exist claim.
- Inline URL for the AGPL acquirer-friction claim.
- Frontend score footnote: 5/5 backend scale vs 10/10 frontend scale, called out explicitly.
- ADR-008 TO RESOLVE-7 (Leo pipeline timing) was not carried into ADR-009; close explicitly: "Leo's pipeline absorption tracked in `docs/STRATEGIC-OVERVIEW.md` post-stack-ADR revision."

### 11.7 Final refined decision set

After applying the council's refinements, ADR-009 binds the following (changes from the Proposed draft are **bold**):

- Backend: Go 1.23+; **week-2 ramp checkpoint with three concrete tests** before locking out the TypeScript+Hono fallback.
- Database: PostgreSQL 17 schema-per-tenant via `ALTER ROLE search_path`; **provisioning via a single SECURITY DEFINER function**; **explicit grants and `REVOKE ALL ON SCHEMA public`**; **Phase 2 hosting Ubicloud DE Standard-2 at ~€78/mo (post-May-2026 increase)**.
- ORM: sqlc (no ORM).
- API: REST + thin MCP adapter; **MCP adapter location bound: same monorepo, separate binary**; **service token design specified** (Argon2id, scopes, 1-year expiry, synchronous revocation, `actor_type` claim).
- Frontend: React 19 + Vite + TanStack + shadcn/ui; **per-subdomain cookie scope mandated**; **CSP header specified**; **AI SDK ↔ MCP framing translation note added**.
- License: Apache 2.0 with FSL-2.0-Apache-2.0 upgrade path **(2-year window auto-converts in 2028, likely before acquisition)**.
- Auth: **Authentik 2025.10 self-hosted at v0 by default; Zitadel Cloud EU from day 1 IF migration cost > 4h**; **`(issuer, sub)` tuple stored from day 1**.
- Observability: LGTM Compose v0 → Grafana Cloud EU v1; **per-workspace Prometheus labels mandated**.
- Background jobs: river Postgres-native; **dedicated `river_<workspace>` schema per tenant**; **job payloads contain only IDs (no PII)**; **river_worker `SET ROLE workspace_<id>` before data access**.
- Build: Go workspaces; pnpm + Vite under apps/web; **`cmd/lecrm-migrate/` and `cmd/lecrm-mcp/` separate binaries in same module**; **`govulncheck` and `go mod verify` added to CI**.
- Schedule: **scope gate at week 6 — fall back to JSONB metadata engine if DDL pattern hits complexity ceiling**; **Google OAuth app review initiated at week 5-6**; **Gmail-only at v0, Microsoft Outlook + IMAP at v1**.
- Audit log: **`security.workspace_id_mismatch` event added to ADR-007 catalogue**; **audit writes fail-closed on mutation path**.

### 11.8 ADR-001 amendment (governance follow-through)

ADR-001 is edited directly with three annotations under §11 of this dossier landing:
- Section "Tenant boundary security" — annotate `SET LOCAL search_path = workspace_<id>` in TypeORM as `[SUPERSEDED by ADR-009 §2: ALTER ROLE search_path set at provisioning time]`.
- Section "Operational specifics" — annotate the phase-3 `track_extra_parameters` upgrade plan as `[SUPERSEDED by ADR-009 §2: ALTER ROLE pattern eliminates PgBouncer version dependency]`.
- TO RESOLVE item 1 — annotate as `[CLOSED — obsolete under ADR-009 §2 ALTER ROLE pattern]`.

### 11.9 ADR-005 amendment (governance follow-through)

ADR-005 inherits ADR-009 §Negative and TO RESOLVE-12. No direct amendment to ADR-005 today; `lecrm/research/agent-runtime-stack.md` will be commissioned post-v1 to revisit the TypeScript/NestJS/Redis/GraphQL assumptions before agent-runtime build begins.

---

**End of council validation.**

---

## Sources

### Backend / language

- [13-language Claude Code benchmark — InfoQ, April 2026](https://www.infoq.com/news/2026/04/ai-coding-language-benchmark/)
- [Stack Overflow Developer Survey 2025](https://survey.stackoverflow.co/2025/technology)
- [Atlas GopherCon 2025 — multi-tenant in Go](https://atlasgo.io/blog/2025/05/26/gophercon-scalable-multi-tenant-apps-in-go)
- [MCP SDK official tiers](https://modelcontextprotocol.io/docs/sdk)
- [Claude Code language effectiveness — Stackademic, April 2026](https://blog.stackademic.com/which-programming-language-should-you-use-with-claude-code-b0b7c4598969)
- [zitadel/oidc OIDC library](https://github.com/zitadel/oidc)
- [Elixir hiring market — KORE1 2026](https://www.kore1.com/hire-elixir-developers-2026/)

### Database

- [Ubicloud Managed Postgres on Hetzner](https://www.ubicloud.com/blog/open-and-portable-managed-postgresql-avail-hetzner)
- [PgBouncer 1.25.1 / CVE-2025-12819](https://www.postgresql.org/about/news/pgbouncer-1251-released-fixing-a-bunch-of-bugs-before-christmas-including-cve-2025-12819-3189/)
- [Atlas v1.0 release (December 2025)](https://atlasgo.io/blog/2025/12/23/atlas-v1)
- [Atlas multi-tenant migration guide](https://atlasgo.io/guides/database-per-tenant/rollout)
- [pgroll](https://pgroll.com/)
- [Brandur on idempotency keys](https://brandur.org/idempotency-keys)
- [pgBackRest archived analysis — thebuild.com, April 2026](https://thebuild.com/blog/2026/04/30/after-pgbackrest/)
- [Bytebase top open-source Postgres backup 2026](https://www.bytebase.com/blog/top-open-source-postgres-backup-solution/)
- [Bytebase Postgres RLS limitations](https://www.bytebase.com/blog/postgres-row-level-security-limitations-and-alternatives/)

### License

- [Sentry FSL introduction](https://blog.sentry.io/introducing-the-functional-source-license-freedom-without-free-riding/)
- [Sentry → Fair Source](https://blog.sentry.io/sentry-is-now-fair-source/)
- [FSL.software adopters](https://fsl.software/)
- [TechCrunch — Fair Source startups, Sept 2024](https://techcrunch.com/2024/09/22/some-startups-are-going-fair-source-to-avoid-the-pitfalls-of-open-source-licensing/)
- [OpenTofu fork announcement](https://opentofu.org/blog/opentofu-announces-fork-of-terraform/)
- [Cal.com → closed-source, April 2026](https://cal.com/blog/cal-com-goes-closed-source-why)
- [Cal.com Hacker News thread](https://news.ycombinator.com/item?id=47780456)
- [Open Core Ventures — AGPL is a non-starter](https://www.opencoreventures.com/blog/agpl-license-is-a-non-starter-for-most-companies)
- [MindCTO — AGPL copyleft threat](https://mindcto.com/insights/copyleft-threat-agpl-risk)
- [Heather Meeker on FSL](https://heathermeeker.com/2023/11/18/sentry-launches-functional-source-license-a-new-twist-on-delayed-open-source-release/)

### Frontend

- [TanStack Start v1 production-ready, Vite migration](https://tanstack.com/blog/from-docs-to-agents)
- [Vercel AI SDK 6 — multi-framework parity](https://vercel.com/blog/ai-sdk-6)
- [shadcn/ui](https://ui.shadcn.com/)
- [shadcn-svelte (huntabyte)](https://github.com/huntabyte/shadcn-svelte)
- [Paris React job market — 1,556 positions 2025](https://agency-partners.com/reports/market-insights/france-paris-frontend-react)
- [SvelteKit multi-tenant subdomain demo](https://github.com/miloudi9/sveltekit-multi-tenancy)

### API / Auth / Observability / Build

- [MCP Streamable HTTP transport spec 2025-03-26](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports)
- [mcp-go (mark3labs)](https://github.com/mark3labs/mcp-go)
- [openid-client v6 (panva)](https://github.com/panva/openid-client)
- [coreos/go-oidc v3](https://github.com/coreos/go-oidc)
- [Authentik 2025.10 release](https://goauthentik.io/blog/2025-10-28-authentik-version-2025-10/)
- [Zitadel pricing](https://zitadel.com/pricing)
- [Grafana Cloud EU regional availability](https://grafana.com/docs/grafana-cloud/security-and-account-management/regional-availability/)
- [SigNoz Docker install](https://signoz.io/docs/install/docker/)
- [OpenTelemetry languages](https://opentelemetry.io/docs/languages/)
- [Turborepo vs Nx 2026](https://daily.dev/blog/monorepo-turborepo-vs-nx-vs-bazel-modern-development-teams/)
- [uv workspace docs](https://docs.astral.sh/uv/)
- [Cargo workspace publishing — Tweag, July 2025](https://www.tweag.io/blog/2025-07-10-cargo-package-workspace/)

---

**End of stack-selection research artefact.**
