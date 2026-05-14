# ADR-009 — Stack and License Selection

**Status:** Accepted
**Date:** 2026-05-10 (Proposed); 2026-05-10 (Accepted after five-voice council validation, transcript at `docs/research/stack-selection.md` §11)
**Deciders:** Guillaume
**Related:** [ADR-008](ADR-008-clean-room-reimplementation.md) (created the freedom this ADR exercises). [ADR-001](ADR-001-tenancy-model.md) (amended in §2 — three annotations applied directly to that file). [ADR-005](ADR-005-ai-agent-tenancy.md) (implementation-language and Redis/GraphQL assumptions contradicted by this ADR — see §Consequences/Negative + TO RESOLVE-12). [ADR-006](ADR-006-backup-dr.md), [ADR-007](ADR-007-encryption-secrets-audit.md) (substantively survive).

---

## Context

[ADR-008](ADR-008-clean-room-reimplementation.md) decided that leCRM is a clean-room reimplementation, not a fork of Twenty. Stack and license — previously inherited from Twenty (TypeScript / NestJS / TypeORM / GraphQL / React under AGPL-3.0) — are now greenfield decisions for GB Consult to make.

This ADR records those decisions, derived from the multi-researcher dossier `docs/research/stack-selection.md` (5 parallel researchers covering backend language/runtime, database, license, frontend, and API+auth+observability+build), validated by a five-voice council (Architect Winston, Engineer Amelia, Researcher Ava, Pentester, Code Reviewer) whose synthesised critique is appended as §11 of that dossier.

Selection criteria with weights: solo-dev velocity with Claude Code (25%), multi-tenant primitives (20%), operational sustainability (15%), AI-native readiness (10%), talent/sale-ability (10%), EU residency (8%), license compatibility (7%), 12-month comprehension debt (5%).

Honest delivery target: 11-13 weeks from start to first paying Design Partner (1-2 weeks reading Twenty source + 10-12 weeks scratch implementation), per ADR-008's R4 estimate. The council's engineer voice rates this **P50 achievable, P80 not achievable at current scope**; mitigations in §Schedule.

---

## Decision

### 1. Backend language and runtime: Go

**Go 1.23+** with `net/http` + Chi router + `sqlc` for type-safe query generation (no ORM) + `river` for Postgres-native background jobs + `zitadel/oidc` for OpenID Foundation-certified OIDC.

Weighted score 4.57/5.00 in the dossier matrix vs TypeScript's 3.73. Decisive on the two heaviest criteria:
- **Solo-dev velocity (25%).** The InfoQ April 2026 13-language Claude Code benchmark (Yusuke Endoh's simplified-Git task, 13 languages × 20 runs) reported **Go at $0.50/run vs TypeScript at $0.62/run**, with 40/40 tests passed. Go ranked 4th overall (behind dynamic languages Ruby, Python, JavaScript) but **best of the statically-typed candidates**. The benchmark task is a CLI tool, not HTTP/multi-tenant CRUD — generalisability to leCRM's workload is suggestive, not proven. The directional finding (Go > TypeScript on cost; less type-friction with Claude Code) holds; the case is real but not as overwhelming as it might first appear.
- **Multi-tenant primitives (20%).** Schema-per-tenant via connection-level `search_path` is the canonical Postgres pattern. Go's [Atlas GopherCon Israel 2025 talk](https://atlasgo.io/blog/2025/05/26/gophercon-scalable-multi-tenant-apps-in-go) documents the exact pattern. sqlc cannot parameterize schema names in query files, which is architecturally correct here — `search_path` lives at the connection, not the query.

Go also wins on **operational sustainability (15%):** single 10-30 MB static binary on Phase-1 VPS (vs Node 100-180 MB idle, Python 60-100 MB per worker), zero-runtime Docker image (`FROM scratch`, ~15 MB), trivially `systemd`-deployed.

#### 1.1 Week-2 Go ramp checkpoint (binding)

If Guillaume's recent Go exposure is not deep, the dossier's velocity advantage may invert in weeks 1-3. **By end of week 2, three concrete litmus tests must pass:**

1. Implement a minimal HTTP handler that runs an `sqlc`-typed query against a local Postgres and returns a JSON response.
2. Write a workspace-scoping middleware using idiomatic Go context propagation (`context.WithValue` for `WorkspaceContext`).
3. Pass `golangci-lint run` and `go vet ./...` with zero warnings on the scaffolding.

If any test is blocked > 4 working hours despite Claude Code assistance, **switch to the TypeScript on Hono runner-up** (Hono + Drizzle + Atlas + `openid-client` v6) for the remainder of the build. The decision is irrevocable by end of week 2; do not relitigate at week 5.

#### 1.2 Why Rust, Elixir, Python are ruled out for v1

Per dossier §1: Rust (Claude Code + borrow checker = documented schedule risk), Elixir (1/5 acquirer/talent score knockout), Python (no leading score; GIL + Phase-2 memory pressure; unusual choice for CRM acquirers). All three remain viable for **future microservices** with different constraints (Rust for hot paths, Elixir for real-time agent layer, Python for data/ML adjuncts).

### 2. Database: PostgreSQL 17, schema-per-tenant, Ubicloud Hetzner DE at Phase 2

**PostgreSQL 17.** No credible non-Postgres alternative at 1-50 workspaces over 24 months (counter-investigation in dossier §2 ruled out CockroachDB, SQLite+LiteFS, MySQL+Vitess, MariaDB, YugabyteDB, TiDB at this scale).

**Hosting.**
- Phase 1 (≤4 clients): self-hosted on Hetzner CX22-CX32.
- Phase 2 (5-20 clients): **Ubicloud Managed Postgres on Hetzner Germany Standard-2 tier (~€78/mo post-May-2026 26% price increase; was €62/mo)** — only genuine EU-resident managed Postgres at this scale; first-party Hetzner managed Postgres does not exist as of May 2026. **OVH Managed Postgres** is the documented fallback if Ubicloud economics shift further.
- Phase 3 (20+ clients): HA replica via Ubicloud, or migrate to self-managed Patroni on Hetzner Dedicated.

**Tenancy primitive: schema-per-tenant** (ADR-001), with the workspace-isolation mechanism shifted from `SET LOCAL search_path` (the original ADR-001 plan) to **per-workspace Postgres role with `search_path` set on the role itself**.

#### 2.1 Workspace provisioning: single SECURITY DEFINER function (binding)

Workspace creation is a **single SQL call** to a `SECURITY DEFINER` function owned by the `lecrm_provisioner` role:

```sql
CREATE OR REPLACE FUNCTION lecrm_provision_workspace(p_workspace_id UUID)
  RETURNS TEXT  -- returns the role name
  LANGUAGE plpgsql
  SECURITY DEFINER
AS $$
DECLARE
  v_role_name   TEXT := 'workspace_' || lower(replace(p_workspace_id::text, '-', ''));
  v_password    TEXT := encode(gen_random_bytes(32), 'base64');
BEGIN
  -- 1. Role
  EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L CONNECTION LIMIT 10', v_role_name, v_password);
  EXECUTE format('ALTER ROLE %I SET search_path = %I, public', v_role_name, v_role_name);
  EXECUTE format('ALTER ROLE %I SET statement_timeout = ''30s''', v_role_name);
  EXECUTE format('ALTER ROLE %I SET lock_timeout = ''5s''', v_role_name);
  EXECUTE format('ALTER ROLE %I SET work_mem = ''16MB''', v_role_name);

  -- 2. Schema
  EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_role_name, v_role_name);

  -- 3. Grants on the workspace's own schema
  EXECUTE format('GRANT USAGE, CREATE ON SCHEMA %I TO %I', v_role_name, v_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT ALL ON TABLES TO %I',
    v_role_name, v_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT ALL ON SEQUENCES TO %I',
    v_role_name, v_role_name);

  -- 4. Lateral expansion mitigation: revoke all default access to public
  EXECUTE format('REVOKE CREATE ON SCHEMA public FROM %I', v_role_name);
  EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA public FROM %I', v_role_name);

  -- 5. Riverjob schema for this workspace
  EXECUTE format('CREATE SCHEMA river_%s AUTHORIZATION %I',
    lower(replace(p_workspace_id::text, '-', '')), v_role_name);

  -- Password is captured by the caller via a sibling function that returns it
  -- exactly once and stores it in the secret manifest. Not returned here to
  -- minimise plaintext exposure.
  RETURN v_role_name;
END;
$$;
```

Provisioning is **idempotent** (the application catches `duplicate_object` for the role and `42P06` for the schema and treats them as success on a known workspace_id). Orphan-role / orphan-schema cases cannot occur because the function is one transaction.

The `lecrm_provisioner` role is a **Tier-0 secret** with annual rotation; ADR-007 receives a follow-up TO RESOLVE adding it to the secret manifest.

#### 2.2 Why ALTER ROLE search_path supersedes track_extra_parameters

The original ADR-001 §Operational specifics planned `SET LOCAL search_path` per query with PgBouncer 1.20+ `track_extra_parameters` carrying the value across transaction-mode pooled connections. The new pattern sets `search_path` at the **Postgres role level** at provisioning. The application authenticates as the workspace role; `search_path` is inherited automatically, mode-agnostic, and CVE-2025-12819-clean as a side effect (CVE-2025-12819 affected a non-default PgBouncer configuration that combined `track_extra_parameters = search_path` with `auth_user`; the advisory's primary fix is upgrading to PgBouncer 1.25.1+ and removing `search_path` from `track_extra_parameters`).

The architectural motivation is **operational cleanliness** (no per-query `SET` overhead, no PgBouncer-version dependency, role-defaults inheritance is Postgres-native). The CVE-clean property is a welcome side effect; it is not the primary justification.

**ADR-001 is amended directly** with three annotations under §11.8 of the dossier: the `SET LOCAL search_path in TypeORM's queryRunner` sentence, the phase-3 `track_extra_parameters` upgrade plan, and TO RESOLVE item 1 are all marked superseded.

#### 2.3 Ubicloud Phase-2 PgBouncer mode

Phase 2 onboarding must verify Ubicloud's PgBouncer config uses **`auth_query` mode** (PgBouncer queries Postgres for the role password rather than reading a static `auth_file`). A flat-file `auth_file` exposes every workspace's credentials simultaneously on a single config-file compromise. Tracked in TO RESOLVE-13.

#### 2.4 Migration tooling: Atlas v1.0

[Atlas v1.0 (December 2025)](https://atlasgo.io/blog/2025/12/23/atlas-v1) for schema migrations across all tenant schemas. `parallel + on_error = CONTINUE` for sweeps. Canary-tenant pattern. Transactional DDL rollback via Postgres semantics. `pgroll` for the rare zero-downtime breaking-column op.

#### 2.5 Backup/DR: WAL-G

WAL-G + GPG client-side encryption to Hetzner Object Storage, unchanged from ADR-006. **pgBackRest was declared unmaintained April 27, 2026** (Crunchy Data sponsorship loss); the GitHub repository is not formally archived and Percona/coalition funding is in flight, but WAL-G is the recommended choice for new deployments. Per-tenant restore via temporary instance + `pg_restore -n workspace_<id>` (ADR-001 §Backup mechanics).

### 3. ORM / query layer: sqlc, no ORM

Multi-tenant correctness wants explicit connection-level `search_path` switching. sqlc generates Go code from SQL files with full type safety; the connection-level switching matches our `ALTER ROLE` pattern. Rejected for v1: ent and GORM (abstraction over connection lifecycle complicates audit-log emission and tenant-scoping verification).

### 4. API surface: REST + thin MCP adapter, GraphQL deferred

REST is the durable contract (URL-prefix versioning `/v1/…`; OpenAPI 3.1 generated from Go handlers via `ogen` or `oapi-codegen`; `Idempotency-Key` header; opaque base64 cursor pagination; `Authorization: Bearer …` workspace-scoped service tokens; subdomain routing → `WorkspaceContext` middleware).

#### 4.1 Service token design (binding)

Workspace-scoped Bearer service tokens are **the** authentication primitive for the MCP adapter and external API consumers. Specification:

| Concern | Decision |
|---|---|
| Storage | **Argon2id-hashed** at rest; never plaintext; the raw token is presented exactly once at creation and is unrecoverable thereafter |
| Scope | Encoded in the token row: `read_only` / `read_write` / `mcp_enabled` flags; future scopes (per-object-type) are additive |
| Expiry | 1-year default; explicit extension supported; no permanent tokens |
| Revocation | **Synchronous DB lookup on every authenticated request** (cost: ~1ms; acceptable at our scale) |
| `actor_type` claim | One of `human_api`, `mcp_agent`, `internal_service` — emitted into every audit_log row for correct attribution |
| Subdomain-vs-token authority | **Token claim is authoritative.** When the inbound request's subdomain-derived `workspace_id` disagrees with the Bearer token's `workspace_id`, reject 401 and emit `security.workspace_id_mismatch` (see §7) |

#### 4.2 MCP adapter: same monorepo, separate binary (binding)

Per ADR-005's Tier-2 architecture (rebuild/redeploy/rollback independence; crash isolation): the MCP adapter is **a separate `cmd/lecrm-mcp/main.go` binary in the same Go module**, deployed as a separate Compose service. Same module gives shared types (the `CrmAdapter` interface), shared sqlc-generated code, one `go test ./...`. Separate binary keeps the per-agent-session sticky-connection scaling profile distinct from the CRM core.

Library: **`mark3labs/mcp-go`** as the de-facto standard at this point (production-grade community SDK). The official `modelcontextprotocol/go-sdk` is Tier-1 designated but still stabilising — re-evaluate the package choice at scaffolding time (TO RESOLVE-7-resolved-as-decision below).

Streamable HTTP transport (mandatory for multi-tenant network MCP servers). MCP recursive tool-call depth bounded at 5. **Rate limiting per (`workspace_id`, `token_id`) tuple at 60 req/min**, enforced in-process with `golang.org/x/time/rate`.

**External MCP clients receive unsanitized CRM data.** ADR-005's prompt-injection sanitization layer lives in agent-runtime (Tier 2), not in this thin adapter. Sanitization responsibility is therefore transferred to the MCP client. This is a documented v1 gap, not a defect.

#### 4.3 React-frontend AI integration: AI SDK ↔ MCP framing translation

Vercel AI SDK 6's `useChat` expects an SSE stream in the AI SDK wire format. MCP Streamable HTTP emits JSON-RPC over chunked HTTP. **The two are not byte-compatible.** The React frontend talks AI-SDK protocol to a leCRM REST/SSE endpoint that proxies to the MCP adapter and translates frames. The React app does not speak MCP directly.

#### 4.4 GraphQL deferred to v2

Twenty's choice; correct for them, costly here. Two schemas (web + agent), depth/complexity tooling, tenant-scoping at every resolver — solo-dev tax with no v1 benefit. Re-evaluate if a paying design partner explicitly needs GraphQL.

### 5. Frontend: React 19 + Vite + TanStack + shadcn/ui

**React 19 + Vite + TanStack Router v1 + TanStack Query + shadcn/ui (Radix UI + Tailwind) + TanStack Table + DnD Kit + cmdk + react-hook-form + zod.**

Weighted score ~9.4/10 (10-point scale; not directly comparable to §1's 5-point backend scale) vs runners-up at ~7.5. Decisive: TanStack Start v1.0 (March 2026) + official Claude Code agent skills (`@tanstack/intent`); shadcn/ui covers every CRM v1 component need; Vercel AI SDK 6 (December 2025) makes v0 React extensible to v2 chat/voice without rewrite; 44.7% Stack Overflow share, 1,556+ open Paris React positions = maximum acquirer legibility.

**v0 → v2 stack continuity is binding.** No HTMX-or-Inertia-then-React-rewrite plan despite ~3-4 week build-time advantage of HTMX for v0; migration cost (6-8 solo-dev weeks) exceeds the savings, and v2 features (streaming chat, voice, real-time agent push) are React-native via Vercel AI SDK 6 but bolt-on to HTMX/LiveView.

#### 5.1 Frontend deploy: embedded in Go binary

`apps/api` embeds `apps/web/dist` via `//go:embed dist/*`. **Caddy terminates TLS and proxies all traffic to the single Go binary**, which internally routes `/v1/*` to REST handlers and `/*` to the embedded SPA. Reconsider split deploy (Caddy → static SPA + Caddy → API) at Phase 2 if sub-second hotfix iteration becomes a constraint at 20 tenants.

#### 5.2 Cookie scope and CSP (binding)

- **Session cookies are scoped to the specific workspace subdomain**, not the parent domain. `Set-Cookie: session=...; Domain=acme.lecrm.fr; SameSite=Strict; Secure; HttpOnly`. A wildcard `Domain=lecrm.fr` would leak sessions across workspaces.
- **CSP header on the embedded SPA**: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'`. Set in Caddy or the Go static handler. The `unsafe-inline` for styles is a shadcn/Tailwind necessity; scripts must be `'self'` only.

### 6. License: Apache 2.0 (with FSL-2.0-Apache-2.0 as upgrade path)

**Apache 2.0** at the first commit. `LICENSE` file at repository root. `NOTICE` file with `Copyright (c) 2026 GB Consult SARL`. No CLA at v1 (solo dev, no external contributors yet).

Apache 2.0 over MIT for the patent grant. Apache 2.0 over AGPL because the clean-room reframe gave us the freedom to escape AGPL §13; reinstating it voluntarily would narrow the €170-340k acquirer pool ([Open Core Ventures: AGPL is a non-starter](https://www.opencoreventures.com/blog/agpl-license-is-a-non-starter-for-most-companies); MindCTO; based on general M&A documentation — France-specific sub-€500k CRM acquisition data is not in the public record). Apache 2.0 over BSL/proprietary because the **Cal.com April 2026 closed-source backlash** is the relevant precedent for relicensing established open-source projects, and we are choosing the right license now to avoid the migration ever being needed.

**FSL-2.0-Apache-2.0 is the credible upgrade path** if a real competitor emerges post-launch tracking the public codebase. The 2-year non-compete window from a 2026 launch **converts to Apache 2.0 in 2028 — likely before the acquisition window closes**, so the FSL upgrade is a temporary instrument that lands on Apache 2.0 regardless. Sentry's transition (BSL → FSL, **November 2023**) explicitly addressed BSL's "compliance departments cannot approve blanket use" problem; FSL is strictly superior to BSL.

The ICP does not differentiate between OSI-approved licenses — Marc and Anne won't look; Pierre would prefer permissive over copyleft for his own modifications. The license decision is purely commercial; the commercial logic favors Apache 2.0.

### 7. Auth library and observability stack

#### 7.1 OIDC client and IDP by phase (binding)

**OIDC client: `zitadel/oidc`** (OpenID Foundation certified for both RP and OP). Backup: `coreos/go-oidc` v3.

**Auth IDP:**
- **Default v0 (≤4 clients): Authentik 2025.10 self-hosted on Hetzner.** Single Compose service (Redis dependency removed in 2025.10; Postgres-backed caching). Built-in TOTP MFA. OIDC upstream for Google Workspace + Microsoft Entra. Authentik admin credential **stored in SOPS secret manifest** alongside the per-tenant secrets (ADR-007 follow-up). Admin interface restricted via Caddy to a specific IP allowlist or VPN.
- **Switch v0 → Zitadel Cloud EU/CH from day 1 if Guillaume estimates the Authentik→Zitadel migration cost at >4 hours**, including the `(issuer, sub)` user-record migration and MFA re-enrolment. Decided at scaffolding (week 1).

**Identity-key storage (binding):** users are keyed on **`(issuer, sub)` tuple**, not raw `sub`. Authentik issues UUID-format `sub`; Zitadel issues numeric strings. The tuple makes the v0→v1 migration a mapping table, not a destructive rewrite. MFA enrolment does not migrate (TOTP secrets are IDP-internal); document the user-facing MFA re-enrolment as a known v0→v1 ops moment.

**Rule out US-managed (WorkOS, Clerk, Stytch, Auth0):** US-based subprocessors clash with leCRM's EU-sovereignty positioning. WorkOS' planned EU hosting ("Vault") is in development; re-evaluate when GA.

#### 7.2 Audit log additions (in ADR-007's catalogue)

- `security.workspace_id_mismatch` event with fields `actor_ip`, `actor_user_id` (if present), `claimed_workspace_id` (subdomain-derived), `token_workspace_id` (Bearer-token claim), `subdomain` (raw header). Emitted whenever the two disagree; request rejected 401.
- **Audit writes on the mutation path are fail-closed.** A mutation that cannot be audit-logged must be rejected, not silently allowed. Test as a hard requirement before first paying client.

#### 7.3 Observability by phase

- **v0: LGTM Compose stack on Hetzner** (Loki + Grafana + Tempo + Prometheus + OpenTelemetry Collector). ~1.1 GB additional RAM; runs on Hetzner CX22 (€4.35/mo).
- **v1+: Grafana Cloud EU (Frankfurt) free tier first** → Pro tier ($19/mo + usage) at saturation, or self-hosted SigNoz on Hetzner CX31.
- **All metrics labelled with `workspace_id`** for per-tenant anomaly detection.

### 8. Build tooling and monorepo

#### 8.1 Monorepo layout

```
lecrm/
├── go.work                                 # Go workspaces
├── apps/
│   ├── api/cmd/lecrm-api/                  # main HTTP server (REST + embedded SPA)
│   ├── mcp/cmd/lecrm-mcp/                  # MCP adapter (separate Compose service)
│   ├── migrate/cmd/lecrm-migrate/          # Atlas runner (Compose pre-deploy job)
│   └── web/                                # React + Vite + TanStack
├── packages/
│   ├── db/                                 # sqlc-generated Go + Atlas HCL
│   ├── crm-adapter/                        # CrmAdapter interface (shared by api and mcp)
│   ├── shared-types/                       # OpenAPI-generated TS types for web/
│   └── tools/                              # mage tasks
├── deploy/compose/                         # leCRM-app, postgres, authentik, lgtm.yml
├── deploy/caddy/                           # caddy.json + DNS-01 wildcard cert config
└── docs/
```

`apps/api` embeds `apps/web/dist` via `//go:embed dist/*` for single-binary deploy.

#### 8.2 Three binaries, not one

- `cmd/lecrm-api` — runtime; runs as the application role (no DDL privileges).
- `cmd/lecrm-mcp` — MCP adapter; runs as a constrained role (read-only by default, with explicit write scopes).
- `cmd/lecrm-migrate` — invoked as a Compose pre-deploy job; runs as `lecrm_provisioner` (DDL-capable). Pre-deploy ordering: `migrate exits 0` → `api starts`.

#### 8.3 Background jobs: river

`river` (Postgres-native, no Redis at v1). **Jobs are tenant-scoped:**

- River tables live in **a per-tenant `river_<workspace_base36>` schema**, created at workspace provisioning (§2.1 step 5).
- Job payloads contain **only IDs** (record IDs, workspace_id, operation type) — never PII directly. PII is fetched at job execution time via the workspace-scoped connection.
- River workers acquire a workspace-scoped Postgres connection (via `SET ROLE workspace_<id>` or by connecting as that role) **before** any data operation.

#### 8.4 CI/CD additions

GitHub Actions: `go test ./...` (testcontainers-go on Postgres), `pnpm test` (frontend), `atlas migrate lint` (block destructive migrations), `golangci-lint run`, `gosec`, **`govulncheck ./...`**, **`go mod verify`**, multi-stage Docker build, deploy via Dokku/Compose pull-and-restart.

#### 8.5 Versioning

`semver 0.x` from first commit; `1.0.0` only at public-API stability.

### 9. Schedule and scope gates

Per ADR-008 R4: 1-2 weeks Twenty-as-textbook reading + 10-12 weeks scratch implementation. Council engineer rates this **P50 achievable, P80 not**. Mitigations baked into this ADR:

- **Wk 6 metadata-engine scope gate (binding).** ADR-010 (custom-object metadata engine) authored by **end of week 5**, not week 7. If the per-tenant DDL pattern hits a complexity ceiling by week 6 (signal: cumulative metadata-engine work > 5 days), **fall back to JSONB `data` column on a generic `objects` table per workspace schema**. Faster, less elegant, acceptable for v1 scale (3-15 users × ≤30 custom objects per workspace).
- **Google OAuth app review starts week 5-6.** External process takes 4-6 weeks for production OAuth scopes (Gmail readonly/send/drafts). If not started by end of week 6, week 11-12 deploy is blocked.
- **Email integration scoped to Gmail-only at v0**; Microsoft Outlook + IMAP deferred to v1.
- **Search ships as pg full-text only at v0**; typesense deferred to v1.
- **Microsoft Entra OIDC deferred to v1**; Google Workspace OIDC only at v0.

These scope cuts protect the 13-week ceiling. The council expects week 7-8 (metadata engine) and week 11-12 (email) to consume most of the slack regardless.

---

## Consequences

### Positive

- **Stack chosen for leCRM's actual constraints**, not inherited from Twenty's 2023-era choices. Solo-dev + Claude Code + multi-tenant + EU residency + AI-native v2 are first-class concerns from line 1.
- **Single static binary on Phase-1 VPS**, ~15-30 MB Docker image, embedded React frontend. Trivially `systemd`-managed; per-client provisioning is rsync + `.env`.
- **License freedom exercised in the acquirer's favor.** Apache 2.0 maximises the buyer pool at the €170-340k acquisition window; patent grant adds legal hygiene; open-source narrative intact for the "transparent, honest pricing" pitch.
- **Multi-tenant isolation is database-native, not application-enforced.** The per-workspace Postgres role + `ALTER ROLE search_path` pattern survives ORM bypass, raw SQL, and buggy middleware. **The strongest possible answer** for an acquirer's CTO at TDD.
- **AI-native seams designed in.** REST + MCP adapter (Streamable HTTP, separate binary) is in scope from v1; `actor_type` claim in audit log accepts `agent` from day 1; v2 chat/voice features bolt onto the React frontend without rewrite.
- **No ORM, no GraphQL, no Redis at v1.** Three layers eliminated by `sqlc` + REST + `river`. Each was comprehension debt avoided.
- **Atlas v1.0 + per-workspace Postgres role + WAL-G** is a current, EU-resident, defensible toolchain.

### Negative

- **Go ramp risk for Guillaume.** Mitigated by the §1.1 week-2 checkpoint with three concrete tests.
- **ADR-005's implementation assumptions are contradicted by this ADR.** ADR-005 specified TypeScript/NestJS Tier-2 with Redis hot-cache and GraphQL → Twenty. ADR-009 eliminates TypeScript for the CRM core, GraphQL as the API surface, and Redis at v1. The TypeScript `CrmAdapter` interface signature in ADR-005 must be re-expressed in the agent-runtime's chosen language. **TO RESOLVE-12** tracks the agent-runtime stack confirmation before agent-runtime build begins.
- **`river` is Postgres-native but newer than Sidekiq-class systems.** Phase-3 throughput may demand a Redis-backed queue; deferred decision.
- **Custom-object metadata engine is the highest-risk implementation block.** Mitigated by §9 week-6 scope gate (DDL pattern → JSONB fallback if needed).
- **MCP SDK ecosystem is younger than REST tooling.** `mark3labs/mcp-go` is community (the official Anthropic Go SDK is Tier-1 designated but still stabilising). Mitigated by the thin-adapter pattern (~300 LOC contained surface).
- **No GraphQL means** clients building generic-query UIs cannot deeply introspect leCRM's schema. Acceptable v1 trade-off; revisit at v2.
- **External MCP clients receive unsanitized CRM data.** Documented v1 gap; sanitization is the MCP client's responsibility.
- **The InfoQ Claude Code benchmark task is non-representative** of leCRM's HTTP/multi-tenant CRUD workload. Generalisability is suggestive, not proven. The Go-over-TypeScript decision rests on directional evidence (cost advantage, type-friction observation) rather than task-specific proof.

### Neutral

- **ADR-001 amended directly** with three annotations marking the `SET LOCAL search_path` / TypeORM / `track_extra_parameters` superseded-by-ADR-009 sections.
- **ADR-006 (backup/DR) survives entirely.** WAL-G remains canonical; pgBackRest's April-2026 unmaintained-status reinforces the choice.
- **ADR-007 (encryption/secrets/audit) substantively survives.** Receives follow-ups: `lecrm_provisioner` credential added as Tier-0 secret; Authentik admin credential added; `security.workspace_id_mismatch` event added to catalogue; audit-log fail-closed semantics formalised.
- **ADR-002 was already superseded by ADR-008.** No additional action.
- **ADR-003 (Brevo) survives entirely.** Email-provider relationship, not a stack choice.

---

## Alternatives Considered

### Backend

- **TypeScript on Node + Hono / NestJS / tRPC.** Runner-up at 3.73/5.00. Strongest on AI-native (Tier-1 MCP SDK, dominant `@anthropic-ai/sdk`) and acquirer legibility. Selected by the §1.1 week-2 fallback if Go ramp is schedule-threatening.
- **Rust + Axum + sqlx.** Type-system correctness for tenant-scoping; Claude Code + borrow-checker is documented schedule risk. Reserve for future Phase-3 hot-path services.
- **Elixir + Phoenix + Ecto.** Best multi-tenant primitives in the candidate set (5/5, uniquely native via `prefix:`). Knockout deduction is talent/sale-ability (1/5).
- **Python + FastAPI + SQLAlchemy 2.x.** No leading score; Phase-2 GIL + per-worker memory pressure; unusual choice for CRM acquirers.

### Database

- **CockroachDB / YugabyteDB / TiDB / SQLite+LiteFS / MySQL+Vitess / MariaDB.** Ruled out at this scale per dossier §2.
- **Neon / Supabase / Crunchy Bridge.** Each fights schema-per-tenant or has US-substrate ambiguity.

### API surface

- **GraphQL-only / REST + GraphQL hybrid.** Twenty's choice; tenant-scoping at every resolver, two schemas.
- **gRPC internal / REST external.** Over-engineered at solo scale.
- **tRPC.** Conditional on TS-backend runner-up; compelling for end-to-end TS type safety.

### Frontend

- **Phoenix LiveView.** Technically excellent but Elixir-only; deduction stacks moot the choice.
- **Svelte / Vue / Solid.** ~7.5/10 vs React's 9.4/10. None close the React gap on Claude Code + AI SDK + shadcn ecosystem completeness.
- **HTMX, Inertia.js, Qwik.** Wrong fit for v0→v2 continuity.

### License

- **MIT.** Functionally identical to Apache 2.0; weaker on patent grant.
- **AGPL-3.0** (Twenty's posture). Reinstates §13; documented acquirer friction.
- **BSL with Change Date.** Sentry's BSL → FSL (Nov 2023) move documented BSL's variability problem; FSL is strictly superior.
- **Elastic v2 / SSPL / Polyform.** Wrong threat model or no precedent.
- **Proprietary closed-source.** Undermines "transparent, honest pricing" pitch; Cal.com April 2026 backlash is the cautionary tale.
- **Dual-license (AGPL + commercial).** Defer — possible v2 monetisation pivot; CLA overhead unwarranted at v1.

### Auth IDP

- **WorkOS / Clerk / Stytch / Auth0.** US-based subprocessors clash with EU-sovereignty positioning.
- **Keycloak.** Java; ~512 MB RAM minimum; operationally expensive for solo dev.
- **Ory Kratos + Hydra.** Most flexible; two services + low-level APIs = highest integration cost.
- **Casdoor.** Less battle-tested than Authentik / Zitadel.

### Observability

- **Datadog / New Relic.** Premium pricing; US-based or murky EU residency.
- **Better Stack / Highlight.io / Axiom / Sentry self-hosted.** Each has merits; Grafana Cloud EU's free tier is decisive at this scale.

---

## References

- `docs/research/stack-selection.md` — full multi-researcher dossier (5 dimensions in parallel).
- `docs/research/stack-selection.md` §11 — five-voice council validation transcript (Architect Winston, Engineer Amelia, Researcher Ava, Pentester, Code Reviewer).
- `docs/adr/ADR-008-clean-room-reimplementation.md` — created the freedom this ADR exercises.
- `docs/adr/ADR-001-tenancy-model.md` — schema-per-tenant primitive; **amended directly by this ADR with three superseded-by annotations**.
- `docs/adr/ADR-005-ai-agent-tenancy.md` — agent runtime architecture; implementation-language and Redis/GraphQL assumptions contradicted by this ADR (TO RESOLVE-12).
- `docs/adr/ADR-006-backup-dr.md` — WAL-G + GPG retained.
- `docs/adr/ADR-007-encryption-secrets-audit.md` — receives follow-ups for `lecrm_provisioner` credential, Authentik admin credential, `security.workspace_id_mismatch` event, audit-log fail-closed semantics.
- [PgBouncer 1.25.1 / CVE-2025-12819](https://www.postgresql.org/about/news/pgbouncer-1251-released-fixing-a-bunch-of-bugs-before-christmas-including-cve-2025-12819-3189/) — the rationale for the `ALTER ROLE` pattern is operational cleanliness; CVE-clean is a side effect.
- [Atlas v1.0 (December 2025)](https://atlasgo.io/blog/2025/12/23/atlas-v1) — multi-tenant `parallel + on_error = CONTINUE`.
- [Open Core Ventures — AGPL is a non-starter](https://www.opencoreventures.com/blog/agpl-license-is-a-non-starter-for-most-companies) — acquirer friction documented (general M&A, not French-CRM-specific).
- [Sentry BSL → FSL (November 2023)](https://blog.sentry.io/introducing-the-functional-source-license-freedom-without-free-riding/) — compliance-department blanket-use rationale.
- [Cal.com April 2026 closed-source](https://news.ycombinator.com/item?id=47780456) — community backlash precedent for relicensing established open-source projects.
- [InfoQ April 2026 13-language Claude Code benchmark](https://www.infoq.com/news/2026/04/ai-coding-language-benchmark/) — Go $0.50/run vs TypeScript $0.62/run; benchmark task is simplified Git, not CRUD HTTP services.
- [TanStack Start v1.0 (March 2026)](https://tanstack.com/blog/from-docs-to-agents) — production-ready React full-stack with Claude Code agent skills.
- [Authentik 2025.10 release notes](https://goauthentik.io/blog/2025-10-28-authentik-version-2025-10/) — Redis dependency removed.
- [Ubicloud Managed Postgres on Hetzner](https://www.ubicloud.com/blog/open-and-portable-managed-postgresql-avail-hetzner) — EU-resident hosting (post-May-2026 26% price increase: ~€78/mo Standard-2).

---

## TO RESOLVE

1. **ADR-001 amendment applied.** Three superseded-by annotations under sections "Tenant boundary security", "Operational specifics" (phase-3 plan), and TO RESOLVE item 1. (Done as part of this ADR's acceptance.)
2. **Week-2 Go ramp checkpoint.** Three litmus tests by end of week 2 per §1.1; switch to TypeScript+Hono if blocked > 4 hours on any test.
3. **Wk 5 ADR-010 metadata-engine pattern decision.** Per-tenant DDL recommended; JSONB fallback gate at week 6.
4. **Authentik or Zitadel decision** at scaffolding (week 1) per §7.1: Authentik default; Zitadel Cloud EU if Guillaume estimates migration cost > 4 h.
5. **`(issuer, sub)` user-key migration table** built into the Authentik schema from day 1.
6. **OpenAPI codegen for the React frontend.** Recommend `@hey-api/openapi-ts`; confirm at scaffolding.
7. **MCP adapter location resolved (decision in §4.2).** Same monorepo, separate `cmd/lecrm-mcp/` binary, separate Compose service. Library: `mark3labs/mcp-go` at v0; re-evaluate vs official `modelcontextprotocol/go-sdk` at scaffolding.
8. **`docs/STRATEGIC-OVERVIEW.md` §2 (Technical) revision** — author after this ADR Accepted.
9. **`docs/ARCHITECTURE.md` rewrite** — author after this ADR Accepted.
10. **`docs/FEASIBILITY-MEMO.md` §2-3 revision** — author after this ADR Accepted.
11. **`.taskets/lecrm-v0-build` group re-scoping** — Track A (shallow fork) is permanently dead per ADR-008. Re-scope remaining tracks against the Go + Apache 2.0 stack.
12. **NEW — ADR-005 agent-runtime stack confirmation.** Confirm: (a) agent-runtime language (TS or Go), (b) Redis retain or replace with Postgres-backed session store (aligned with `river`'s no-Redis posture), (c) GraphQL → REST adapter contract update. Author follow-up `docs/research/agent-runtime-stack.md` before agent-runtime build (post-v1).
13. **NEW — Ubicloud PgBouncer auth_query mode verification** at Phase 2 onboarding. If Ubicloud uses static auth_file, single config-file compromise exposes all workspace credentials.
14. **NEW — ADR-007 follow-ups:** add `lecrm_provisioner` credential and Authentik admin credential to secret manifest; add `security.workspace_id_mismatch` event to audit catalogue; formalise audit-log fail-closed semantics on the mutation path.
15. **NEW — Leo pipeline timing.** ADR-008 TO RESOLVE-7 (Leo's pipeline absorbing the wider v0 timeline) closes here: tracking moves to `docs/STRATEGIC-OVERVIEW.md` post-stack-ADR revision (TO RESOLVE-8 above).

---

## Week-2 Go ramp checkpoint outcome (2026-05-14)

The three litmus tests from §1.1 were run on the `apps/api` scaffolding produced in commits 63be520 → 0ba5916. Postgres on the local socket was admin-locked under the session's no-sudo policy, so the live integration test ships as a build-tagged test (`-tags integration`) that can be run when the local compose stack is up; the in-process tests cover the same code paths with a stub `Resolver` and exercise the typed-context + middleware + handler composition end-to-end.

- **Test 1 (sqlc handler):** PASS — ~30 minutes elapsed. `apps/api/sqlc.yaml` generates `internal/sqlcgen/` from `packages/db/queries/workspaces.sql` against the existing `packages/db/migrations` schema; the `/v1/_test/workspaces` handler (`internal/workspace/handlers.go`) calls `sqlcgen.New(pool).ListWorkspacesForTest(ctx)` and marshals the typed rows to JSON. Build-tagged integration test `TestTestListHandler_Integration` exercises the live pgxpool path.
- **Test 2 (workspace middleware):** PASS — ~25 minutes elapsed. `internal/workspace/context.go` uses an unexported `ctxKey struct{}` (per Go idiom — not a string key) with a typed `WithWorkspace` setter and `WorkspaceFromContext(ctx) (*Context, error)` getter. `internal/workspace/middleware.go` resolves the subdomain via a `Resolver` interface (live implementation `PoolResolver` goes through sqlc). Eight unit tests cover round-trip, missing-context error, the string-key shadowing anti-pattern, unknown-slug 404, root-domain 400, multi-label-subdomain 400, and `subdomainOf` parsing edge cases.
- **Test 3 (lint + vet clean):** PASS — ~15 minutes elapsed. `go vet ./...` and `golangci-lint run ./...` both report zero issues. Pre-existing `errcheck`/`bodyclose` findings in `internal/auth/e2e_test.go` were fixed at the source (refactor of `requireComponent` → `requireComponentVia` so the bodyclose linter can see the body lifecycle); no `//nolint:` directives were added. Project-level exclusion is scoped only to `internal/sqlcgen/` (generated code) — a structural policy, not noise suppression.
- **Decision:** CONTINUE Go.
- **Decided by:** Guillaume
- **Decided on:** 2026-05-14
- **Notes:** Claude Code produced idiomatic Go on first prompt across all three deliverables — unexported context-key type, pgxpool acquisition, chi router groups with middleware composition, sqlc query+overrides config. The only Go-adjacent friction was an `sqlc` override duplication that double-imported the `uuid` package; the fix (collapse to two string-form `go_type` overrides) was a config edit, not a language issue. Total elapsed across all three tests was ~70 minutes — well inside the 90 + 90 + 30 = 210 minute combined budget. Per §1.1 the decision is binding for v0; no relitigation at week 5 (the metadata-engine ADR-010 work now proceeds on the Go stack).

