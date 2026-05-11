# leCRM — Feasibility Memo: Clean-Room CRM as a HubSpot Alternative

**Author:** Guillaume (GB Consult)
**Date:** 2026-05-07 (v2); **§2-3 rewritten 2026-05-11 (v3)** for Path D + ADR-009 alignment
**Status:** Internal decision document — not for client distribution. §2 (License) and §3 (Build Roadmap) reflect the locked Apache 2.0 clean-room posture per [ADR-008](adr/ADR-008-clean-room-reimplementation.md) and the locked Go + PostgreSQL + Apache 2.0 stack per [ADR-009](adr/ADR-009-stack-and-license.md). §1 (Strategic Frame), §4 (HubSpot Moat sizing), §5 (GTM), §6 (Operational Reality), §7 (Risks), §8 (Decision Path), §9 (Open Questions), §10 (v1 → v2 diff) still describe the earlier fork frame in places and will be reissued as a light pass after the v0 build re-scope completes. Where the doc is internally inconsistent between §2-3 and the rest, **§2-3 are canonical**.
**Subject (v3):** Can we build a clean-room CRM under Apache 2.0 (Go + PostgreSQL, single-binary deploy, schema-per-tenant), ship it to a first paying Design Partner inside 11-13 weeks, and operate it as a managed service through Leo's sales channel — building serious MRR?

> **v3 banner (2026-05-11).** The fork-of-Twenty premise was retired by [ADR-008](adr/ADR-008-clean-room-reimplementation.md) (clean-room reimplementation) and the stack + license inheritance was retired by [ADR-009](adr/ADR-009-stack-and-license.md) (Go 1.23 + PostgreSQL 17 schema-per-tenant + React 19 + Apache 2.0). §2 and §3 are rewritten to that frame. The strategic moat is also reframed in `STRATEGIC-OVERVIEW.md` §4 (revised 2026-05-11): ownership + Leo's distribution + tailorization speed + transparent pricing; AI-native UX is v2 strategic optionality, not the v1 bet.

---

## TL;DR — Verdict

**GO on the technical path. The remaining decisions are commercial and execution-discipline, not legal or technical.**

Two pivots from v2 collapsed the earlier license and stack debate:

- **License posture (Path D, §2).** leCRM is a clean-room reimplementation under Apache 2.0 ([ADR-008](adr/ADR-008-clean-room-reimplementation.md), [ADR-009](adr/ADR-009-stack-and-license.md)). No AGPL §13 obligation, no CLA-ratchet risk, no fork rebase tax. Twenty's source is read as architectural reference only. The license question is resolved; the technical sizing becomes the load-bearing question.
- **HubSpot moat sized post-tailorization (§4, still based on v2 frame; light reissue pending).** HubSpot's structural moats reduce, for our ICP, to two genuine engineering items (sequences with reply-detection; reporting depth) and one positioning item (brand trust). All three are buildable/closeable at v1 against the locked Go stack.

**Honest timeline (Path D, §3): 11-13 weeks to first paying Design Partner — council rated P50 achievable, P80 not at current scope.** The earlier 4-6 week estimate was based on the AGPL-fork posture (ADR-002, superseded). Four binding schedule gates carry the schedule risk: Week 2 Go ramp litmus test; Week 5 ADR-010 metadata-engine pattern; Week 6 metadata-engine scope gate; Week 5-6 Google OAuth app review kickoff (4-6 week external blocker for production Gmail scopes). See §3 for the week-by-week roadmap.

The remaining hard questions are commercial:
1. **ICP discipline** — first paying clients must match the Marc / Anne / Pierre archetypes (see [ICP-ARCHETYPE.md](ICP-ARCHETYPE.md)), NOT Marketing-Hub-Pro users or Shopify-heavy e-commerce shops.
2. **Solo-operator capacity** — 11-13 weeks of focused build + client ops + onboarding is multi-quarter execution. Cap clients at 5 in Phase 1-2.
3. **Leo's positioning** — resolved: Leo introduces discreetly, his name does not appear on bills or contracts (GB Consult ↔ client direct, Leo paid via separate referral agreement). His HubSpot brand stays clean.

**Recommended path:** Phase 0 commercial gate (~5 hours, this week), then the 11-13-week v0 build per §3, with the four binding schedule gates enforced. First paying Design Partner targeted by end of week 12-13. v1 productisation and clients 2-5 follow over months 4-9.

---

## 1. Strategic Frame — What We're Actually Building

A **managed CRM service** in the Plausible/Mautic-managed-hosting tradition: open code (we'll publish our AGPL fork on GitHub), premium operations, custom integration on demand. ([3])

- The product is the **service**, not the software.
- Margin comes from ops + setup + custom dev, not from licensing software.
- Twenty AGPL is a 18-24 month head start on the CRM data model + UI + workflow engine. We do not build that. We build what's missing on top.
- Connectors and "enterprise features" are commodity work for us thanks to AI-augmented dev velocity — they are no longer moats for HubSpot, just line items in a build plan.

**Pitch frame for Leo's sales conversations:**

> "When HubSpot wins, sell HubSpot. When HubSpot loses on price, EU sovereignty, customization, or vendor lock-in concerns, you have an alternative we host and operate end-to-end. Same data model, fully customizable, your client owns their data on EU infrastructure."

---

## 2. License Posture — Apache 2.0 Clean-Room (Path D)

*Rewritten 2026-05-11 (v3). The earlier section sized the AGPL-fork posture: Enterprise-licensed file inventory, build effort to reproduce gated features, AGPL §13 obligations, Twenty CLA-ratchet risk. None of that applies under Path D. The original content is preserved in git history.*

### Posture

leCRM is a **clean-room reimplementation** under Apache 2.0, per [ADR-008](adr/ADR-008-clean-room-reimplementation.md) and [ADR-009 §6](adr/ADR-009-stack-and-license.md). No Twenty source code is copied, ported, or transformed into the leCRM repository. Twenty's source is read as architectural reference — the way a developer reads a textbook — for the metadata-engine pattern, workspace-isolation patterns, audit-log infrastructure, OIDC strategy shape, and migration-management approach. Specifically NOT inherited: Twenty's codebase, file structure, migrations, package names, domain types, language, framework, ORM, database driver, frontend library, or build system. Every line in the leCRM repository is greenfield code originated by GB Consult.

### Consequences of Path D for the license question

- **No AGPL §13 distribution obligation.** The earlier draft of this memo sized that carefully; it does not apply.
- **No Twenty CLA exposure.** Twenty's right to relicense contributions (HashiCorp-style ratchet risk over a 24-month horizon, base-rated by the Researcher voice at 35-50% in the four-round council; see [ADR-008](adr/ADR-008-clean-room-reimplementation.md) Context) does not bind leCRM.
- **No fork rebase tax.** The "2-4 hours/month" estimate from the v2 memo was acknowledged unrealistic at honest accounting (30-50 files in security-critical zones; see [ADR-008](adr/ADR-008-clean-room-reimplementation.md) Context). Under Path D it is irrelevant.
- **Trademark cleanliness is straightforward.** No "Powered by Twenty" question to negotiate; leCRM ships under GB Consult's own brand (the customer-facing brand decision still pending — see §9) and INPI registration (one class).

### License selected: Apache 2.0 (with FSL-2.0-Apache-2.0 as upgrade path)

Per [ADR-009 §6](adr/ADR-009-stack-and-license.md):

- **Apache 2.0** at the first commit. `LICENSE` file at repository root. `NOTICE` file with `Copyright (c) 2026 GB Consult SARL`. No CLA at v1 (solo dev, no external contributors yet).
- Apache 2.0 over MIT for the **patent grant**. Apache 2.0 over AGPL because the clean-room reframe gave us the freedom to escape AGPL §13; reinstating it voluntarily would narrow the €170-340k acquirer pool. Apache 2.0 over BSL/proprietary because the **Cal.com April 2026 closed-source backlash** is the relevant precedent for relicensing established open-source projects; we are choosing the right license now to avoid the migration ever being needed.
- **FSL-2.0-Apache-2.0 is the credible upgrade path** if a real competitor emerges post-launch tracking the public codebase. Sentry's BSL → FSL move (November 2023) is the relevant precedent. The 2-year non-compete window from a 2026 launch converts to Apache 2.0 in 2028 — likely before the acquisition window closes, so the FSL upgrade is a temporary instrument that lands on Apache 2.0 regardless.
- **No CLA at v1.** A CLA may be revisited at v2 if dual-licensing (Apache + commercial) becomes a monetisation lever. Decision deferred.

The ICP (Marc / Anne / Pierre archetypes — see [ICP-ARCHETYPE.md](ICP-ARCHETYPE.md)) does not differentiate between OSI-approved licenses. The license decision is purely commercial; the commercial logic favors Apache 2.0.

### What this enables on the sales call

The pitch sentence becomes:

> "We wrote every line of code that touches your data. It is open-source under Apache 2.0; you can audit it, fork it, run it yourself if we ever disappear. Your data lives on French infrastructure operated by GB Consult, no US sub-processors on the primary data path."

This is materially stronger than the prior "we forked an AGPL project and operate it" pitch — and aligns with the moat reframe in `STRATEGIC-OVERVIEW.md` §4 (revised 2026-05-11): sovereign codebase + tailorization + transparent pricing + Leo's distribution. The license-posture conversation moves from "can we" (a legal-risk frame) to "we did" (an ownership claim).

### Acquirer story

A clean-room scratch CRM under permissive license at 20 clients (~€84k ARR) is a more attractive acquisition target than a Twenty fork. No AGPL contagion concern for the acquirer's broader product line, no CLA inheritance, no upstream landlord. Apache 2.0's patent grant adds legal hygiene. The €170-340k 2-4× ARR window is maximised, not narrowed, by this license choice.

### Verdict

**GO on the technical path.** The license posture is resolved. The technical sizing (§3) becomes the load-bearing question. The remaining hard questions are commercial — ICP discipline, solo-operator capacity, Leo's positioning — and execution-discipline at the four binding schedule gates documented in §3.

---

## 3. Build Roadmap — 11-13 Weeks to First Paying Design Partner (Path D)

*Rewritten 2026-05-11 (v3). The earlier section described a 4-6 week, 4-track parallel-agents build with Reply.io / Metabase iframe / Postmark bridges in v0 and a v1 productisation track over weeks 5-14. That plan was downstream of the AGPL-fork posture and the TypeScript / NestJS / GraphQL stack inherited from Twenty. Both are gone. This section describes the v0 plan against the locked Go + PostgreSQL + Apache 2.0 stack per [ADR-009](adr/ADR-009-stack-and-license.md), with the four binding schedule gates from §Schedule of that ADR.*

### Honest top-line numbers

- **11-13 weeks** total: 1-2 weeks Twenty-as-textbook reading + 10-12 weeks scratch implementation.
- **Council rating: P50 achievable, P80 not at current scope.** (Five-voice council validation: Architect Winston, Engineer Amelia, Researcher Ava, Pentester, Code Reviewer; transcript at `docs/research/stack-selection.md` §11.)
- **Four binding schedule gates** carry the schedule risk; details below.
- **Scope cuts baked in** to protect the 13-week ceiling: Gmail-only at v0 (Microsoft Outlook + IMAP at v1); pg full-text search at v0 (typesense at v1); Google Workspace OIDC at v0 (Microsoft Entra at v1); native sequences with reply detection deferred to v1; row-level permissions at workspace level only (per-record ACLs at v1); no SAML, no Cube.dev dashboards at v0.
- **Engineering team: solo (Guillaume) + Claude Code.** Aaron held as optional collaborator if Phase-3-class infrastructure is demanded earlier than the staged plan anticipates (see `STRATEGIC-OVERVIEW.md` §2).

### Pre-flight: 1-2 weeks Twenty-as-textbook reading

Read Twenty's source as architectural reference, no copying. Specific lessons to capture:

- Custom-object metadata-engine shape: `object_definitions`, `field_definitions`, `field_values` patterns; dynamic schema generation; permission-aware query patterns.
- Workspace isolation patterns: boundaries, scoping, tenant-FK-on-every-row.
- Audit log infrastructure: event emission, queryable schema.
- Auth strategy module shape: OIDC, password, 2FA flows.
- Migration management approach.

Notes captured in `docs/research/twenty-as-textbook-notes.md` (to be authored during this phase).

### Weeks 1-2 — Scaffolding + Go ramp checkpoint (binding gate)

Scope:
- Monorepo layout per [ADR-009 §8.1](adr/ADR-009-stack-and-license.md) (`apps/api`, `apps/web`, `apps/mcp`, `apps/migrate`, `packages/db`, `packages/crm-adapter`, `packages/shared-types`, `deploy/compose`, `deploy/caddy`).
- Authentik 2025.10 vs Zitadel Cloud EU decision at scaffolding (week 1 per [ADR-009 §7.1](adr/ADR-009-stack-and-license.md)) — Authentik default; Zitadel if Guillaume estimates the v0→v1 migration cost at >4 h.
- Bare HTTP server (net/http + Chi) with one sqlc-typed query against local PostgreSQL.
- `WorkspaceContext` middleware via idiomatic Go context propagation.
- `golangci-lint run` + `go vet ./...` clean.
- Atlas v1.0 migration tooling wired in.
- CI/CD scaffolding: `go test ./...` (testcontainers-go), `pnpm test`, `atlas migrate lint`, `gosec`, `govulncheck`, `go mod verify`.

**Gate 1 (Week 2 end) — Go ramp litmus test (binding, irrevocable).** Three concrete tests per [ADR-009 §1.1](adr/ADR-009-stack-and-license.md):
1. Minimal HTTP handler running an `sqlc`-typed query against local Postgres, returning JSON.
2. Workspace-scoping middleware using `context.WithValue` for `WorkspaceContext`.
3. `golangci-lint run` and `go vet ./...` with zero warnings.

If any test is blocked > 4 working hours despite Claude Code assistance, **switch to TypeScript on Hono runner-up** (Hono + Drizzle + Atlas + `openid-client` v6) for the remainder of the build. Decision irrevocable by end of week 2; do not relitigate at week 5. Tasket: `20260510-202450-a5d3`.

### Weeks 3-4 — Tenancy + auth foundation

- `lecrm_provision_workspace` `SECURITY DEFINER` function per [ADR-009 §2.1](adr/ADR-009-stack-and-license.md): single SQL call creating per-workspace Postgres role, schema, river schema, default privileges, and lateral-expansion-mitigated public-schema revocations. Idempotent.
- `lecrm_provisioner` role added to SOPS secret manifest as Tier-0 ([ADR-007](adr/ADR-007-encryption-secrets-audit.md) follow-up).
- Per-workspace Postgres role + `ALTER ROLE search_path` pattern (supersedes the original ADR-001 `SET LOCAL search_path` plan; see [ADR-009 §2.2](adr/ADR-009-stack-and-license.md)).
- OIDC integration with the selected IDP (Authentik or Zitadel) via `zitadel/oidc` library.
- `(issuer, sub)` user-key migration table built in from day 1 for the v0→v1 IDP-migration path.
- Session cookies scoped to specific workspace subdomain ([ADR-009 §5.2](adr/ADR-009-stack-and-license.md)).
- CSP header on the embedded SPA per ADR-009 §5.2.
- River workers acquiring workspace-scoped Postgres connections before any data operation.

### Weeks 5-6 — Metadata engine + Google OAuth review kickoff (binding gates)

**Gate 2 (Week 5 end) — ADR-010 authored (binding).** Custom-object metadata-engine pattern recorded with per-tenant DDL as primary and JSONB fallback documented. Per [ADR-009 §9](adr/ADR-009-stack-and-license.md): "ADR-010 authored by end of week 5, not week 7."

- Metadata engine implementation: `object_definitions`, `field_definitions`, dynamic table creation per workspace schema.
- Permission-aware query layer (workspace + role-aware).
- Custom-object CRUD surface.

**Gate 3 (Week 6 end) — metadata-engine scope (binding).** If cumulative metadata-engine work > 5 days by week 6, **fall back to JSONB `data` column on a generic `objects` table per workspace schema**. Faster, less elegant, acceptable for v1 scale (3-15 users × ≤30 custom objects per workspace).

**Gate 4 (Week 5-6 start) — Google OAuth app review initiated (binding).** External process takes 4-6 weeks for production OAuth scopes (Gmail readonly / send / drafts). **If not started by end of week 6, week 11-12 deploy slips by 4-6 weeks.** This is the critical-path external blocker — start it before the metadata engine if scheduling conflict emerges.

### Weeks 7-8 — Standard CRUD surface + audit log + service tokens

- REST handlers for standard objects (Contact, Company, Deal, Activity, Note, Task) with URL-prefix versioning (`/v1/…`).
- OpenAPI 3.1 generated via `ogen` or `oapi-codegen`; `@hey-api/openapi-ts` for the React frontend types ([ADR-009 TO RESOLVE-6](adr/ADR-009-stack-and-license.md)).
- `Idempotency-Key` header support.
- Opaque base64 cursor pagination.
- Workspace-scoped Bearer service tokens per [ADR-009 §4.1](adr/ADR-009-stack-and-license.md): Argon2id-hashed at rest, 1-year default expiry, synchronous DB lookup on every authenticated request, `actor_type` claim (`human_api` / `mcp_agent` / `internal_service`).
- Audit log infrastructure with `actor_type` accepting `agent` from day 1.
- `security.workspace_id_mismatch` event emitted when subdomain-derived `workspace_id` disagrees with Bearer-token claim (token authoritative; request rejected 401).
- **Audit writes on the mutation path are fail-closed.** A mutation that cannot be audit-logged must be rejected. Hard requirement before first paying client.

### Weeks 9-10 — Frontend (React 19 + shadcn/ui + TanStack)

- React 19 + Vite + TanStack Router v1 + TanStack Query.
- shadcn/ui (Radix UI + Tailwind) component library.
- TanStack Table for list views; DnD Kit; cmdk; react-hook-form + zod.
- Workspace subdomain → `WorkspaceContext` routing.
- Frontend embedded into Go binary via `//go:embed dist/*` for single-binary deploy.
- Caddy terminates TLS and proxies all traffic to the single Go binary; internal routing `/v1/*` → REST handlers, `/*` → embedded SPA.
- Vercel AI SDK 6 wired but not exercised at v1 (preserves v2 chat/voice optionality without rewrite).
- MCP adapter (`cmd/lecrm-mcp/`) skeleton implemented as a separate Compose service per [ADR-009 §4.2](adr/ADR-009-stack-and-license.md); React-to-MCP framing translation deferred (the React app does not speak MCP directly).

### Weeks 11-12 — Email + observability + deploy

- Gmail integration via the production OAuth scopes (gated on Gate 4 completing).
- LGTM Compose stack on Hetzner: Loki + Grafana + Tempo + Prometheus + OpenTelemetry Collector (~1.1 GB additional RAM on CX22 €4.35/mo per [ADR-009 §7.3](adr/ADR-009-stack-and-license.md)).
- All metrics labelled with `workspace_id` for per-tenant anomaly detection.
- WAL-G + GPG client-side encryption to Hetzner Object Storage per [ADR-006](adr/ADR-006-backup-dr.md).
- pg full-text search wired into the CRUD surface (typesense deferred to v1).
- Caddy DNS-01 wildcard cert configuration.
- Brevo as transactional email provider per [ADR-003](adr/ADR-003-brevo-transactional.md) (unchanged from earlier ADR).
- Single-binary deploy via Compose; pre-deploy `lecrm-migrate` job (Atlas runner) gates `lecrm-api` startup.

### Week 12-13 — First Design Partner onboarding

- Lawyer-reviewed DPA + CGV + SLA signed (parallel non-dev track, started in week 0-2 alongside scaffolding — see [LEGAL-PLAYBOOK.md](LEGAL-PLAYBOOK.md)).
- Customer-facing brand decided and INPI registered (parallel non-dev track).
- Workspace provisioned for the Design Partner via the `SECURITY DEFINER` function.
- Onboarding checklist executed: DNS subdomain, Google Workspace OIDC, Gmail OAuth grant, first data import, first user trained.
- **First paying Design Partner live.**

### Four binding schedule gates (summary)

| Gate | Week | Decision | If failed |
|---|---|---|---|
| **G1 — Go ramp litmus** | End of week 2 | Three concrete tests per [ADR-009 §1.1](adr/ADR-009-stack-and-license.md) pass within 4 h each | Switch stack to TypeScript+Hono for remainder of build (irrevocable) |
| **G2 — ADR-010 authored** | End of week 5 | Metadata-engine pattern recorded (per-tenant DDL primary; JSONB fallback) | Decision deferred to G3 — adds week-6 risk |
| **G3 — Metadata-engine scope** | End of week 6 | Cumulative metadata work ≤ 5 days | Fall back to JSONB `data` column on generic `objects` table per workspace schema |
| **G4 — Google OAuth review** | End of week 6 at latest | Application submitted to Google for production Gmail scopes | Week 11-12 deploy slips by 4-6 weeks |

### Rolled-up timeline

| Milestone | Target |
|---|---|
| Phase 0 commercial gate cleared | Week 0 |
| Twenty-as-textbook reading complete; scaffolding repo initialised | Week 1-2 |
| **Gate 1 — Go ramp litmus test passed** | End of week 2 |
| Tenancy + auth foundation in place | End of week 4 |
| **Gate 2 — ADR-010 authored** | End of week 5 |
| **Gate 3 — Metadata-engine scope decision** | End of week 6 |
| **Gate 4 — Google OAuth app review initiated** | End of week 6 |
| CRUD surface + audit log + service tokens | End of week 8 |
| Frontend embedded in single Go binary | End of week 10 |
| Email + observability + deploy ready | End of week 12 |
| **First paying Design Partner live** | End of week 12-13 |

### Real ceiling on this timeline

It isn't dev velocity. It's:

- **Google OAuth app review** — 4-6 week external blocker, the single hardest constraint on the schedule.
- **Client decision cycles** — 4-8 weeks from intro to signing. Phase-0 qualifying conversations must start during the build window, not after (per [ADR-008](adr/ADR-008-clean-room-reimplementation.md) TO RESOLVE-7 → moved to `STRATEGIC-OVERVIEW.md`).
- **Legal review and DNS propagation lead times.**
- **Founder context-switching** between build, sell, and ops. Solo-operator capacity is the §6 / §7 risk; do not run more than one active discovery conversation while the build is in weeks 3-10.

### v1 — Productized stack (post-Design-Partner, months 4-9)

Run while clients 1-3 are in production. The first clients' needs drive the v1 backlog. Items deferred from v0:

- Native sequences with reply-detection (Gmail Pub/Sub + Microsoft Graph subscription + IMAP IDLE + state machine + OOO classifier on Haiku) — see tasket `20260510-202450-aa6f` for v1 native sequences scope.
- Microsoft Outlook + IMAP email sync.
- Microsoft Entra OIDC.
- SAML SSO (if a specific client requires it).
- Row-level permissions (per-record ACLs in addition to workspace-level isolation).
- typesense full-text search.
- Metabase reporting bridge as the interim dashboard solution (tasket `20260510-202450-29dc`); custom React dashboards on a Postgres semantic layer later.
- Standard connector library: built reactively per signed-client requirement, 3-5 days each.
- PWA polish (service worker, offline read, Web Push, install prompt).
- HA replica + Patroni only at Phase 3 (20+ clients) per [ADR-001](adr/ADR-001-tenancy-model.md).

### v2 — Strategic optionality (post-Phase-3, months 9+)

Per `STRATEGIC-OVERVIEW.md` §4 (revised 2026-05-11), v2 features are premium add-ons opportunistically priced after first 5 clients are stable:

- Telegram/WhatsApp chatbot CRM as a per-client opt-in (tasket `20260510-202450-11e5` is the v2 prototype).
- Voice-call-to-CRM logging.
- Autonomous pipeline-watching agents.
- LLM-driven dashboards ("ask your CRM").

The architecture in [ADR-009](adr/ADR-009-stack-and-license.md) preserves the optionality at near-zero v1 cost: MCP adapter ships in v1 as a separate binary; `actor_type=agent` accepted from day 1; React 19 + Vercel AI SDK 6 frontend is layer-on-able without rewrite.

### What we're explicitly NOT building at v0

- Marketing automation engine — n8n + Brevo handles 80% of SMB needs; Mautic as optional managed add-on if a client genuinely needs lead scoring.
- Native iOS/Android apps — responsive web at v0; PWA at v1; native only if a client requirement justifies it.
- HubSpot Academy equivalent — Loom videos + Mintlify docs.
- Custom landing page builder — Webflow/Carrd as recommended pairing.
- E-signature — PandaDoc API as a per-client integration for those who need quote sign-off.
- GraphQL — Twenty's choice; REST + thin MCP adapter is durable for solo-dev maintenance; re-evaluate at v2 only if a paying Design Partner explicitly demands it.
- Redis at v1 — `river` is Postgres-native; Phase-3 throughput may demand a Redis-backed queue, deferred decision.

---

## 4. HubSpot Moat — Sized Post-Connectors

Reassessment with connectors removed (we build them as needed):

| Moat | Severity (post-connectors) | Strategy | Cost / Time |
|---|---|---|---|
| Integration ecosystem (1,500+ apps) | **Eliminated** | Build connectors on demand | 1-2 weeks per connector, AI-augmented |
| Email deliverability | Medium | Buy: Postmark ($200-870/mo all-clients) | 1 week to wire bounce webhooks |
| Sequences w/ reply-detection | High | Build natively | 6-9 weeks |
| Reporting depth | Medium | Cube.dev (Apache 2.0, free) + custom dashboards | 3-4 weeks |
| Marketing automation depth | Low for our ICP | n8n + Brevo; Mautic optional | $14-150/mo, no build sprint |
| Brand recognition / "nobody fired for HubSpot" | Medium | GDPR DPA + SLA + price delta + case studies | $1-2k legal; weeks of positioning |
| Mobile native apps | Low | PWA with Web Push + offline cache | 2-3 weeks |
| Training content | Low | Loom + Mintlify | 3-4 weeks writing/recording |

**Bottom line:** With connectors removed, HubSpot's remaining moat reduces to two genuine engineering moats (sequences, reporting depth) and one positioning moat (brand). All three are buildable/closeable inside 16-22 weeks of work + parallel non-tech tracks.

---

## 5. GTM Positioning — Leo as Discreet Introducer

Leo introduces. He does not sell, and his name does not appear on the contract or invoice. The contractual relationship is **GB Consult ↔ client direct**. Leo is compensated via a separate referral agreement, paid by GB Consult.

This structure resolves the v1 positioning concerns:
- His HubSpot specialist brand stays clean — clients of leCRM have no awareness Leo is involved unless he chooses to disclose
- No HubSpot Solutions Partner agreement conflict — he's not selling a competing product, he's making warm introductions
- He scouts carefully, only when relevant and safe — his active HubSpot prospects are off-limits, his lost-deal pipeline and broader network are fair game

### Sourcing channels (see `ICP-ARCHETYPE.md` for full detail)

- Lost-HubSpot-deal pipeline (prospects who balked on price/sovereignty)
- Personal/founder network outside HubSpot client base
- Peer referrals from existing happy clients
- French SMB founder communities

### Leo's current book is the WRONG ICP — restated

ChefCheffe, L'Expressionist, Chauvé 79, Château Orquevaux all skew toward Shopify-heavy e-commerce or marketing-automation-heavy use — exactly where the build cost is highest. **First 3 clients come from his lost-deal pipeline or external network, not from his active HubSpot book.**

### Pricing structure (beta phase)

| Tier | Setup | MRR | Term | Margin split (Guillaume / Leo referral) |
|---|---|---|---|---|
| Design Partner (1-2 max) | €0 | €99-149/mo | 6-month locked | 70 / 30 |
| Paying Beta — Solo | €1,500-2,000 | €250/mo | 30-day | 70 / 30 |
| Paying Beta — Team | €2,500-3,500 | €350-450/mo | 30-day | 70 / 30 |
| Custom dev (separate devis) | €600-800/day | — | — | 100 / 0 |

### Billing structure migration plan

Per `HUBSPOT-PARTNER-BILLING-RESEARCH.md`: HubSpot's Solutions Partner Program Agreement is **explicitly non-exclusive (Section 2)**. Three viable billing structures, ordered by SPPA risk:

- **Structure A — Apporteur d'affaires (current plan, lowest risk):** GB Consult bills client direct; Leo paid as referrer. Use for v0.
- **Structure C — Hybrid (recommended migration):** Vernayo invoices a one-time integration consulting fee; GB Consult invoices MRR. Captures most of the tax efficiency without putting Leo on the recurring CRM invoice. Migrate to this once v0 stabilizes.
- **Structure B — Vernayo prime (full service):** Only viable if Leo's HubSpot status is **individual certifications only** (not a formally enrolled Solutions Partner entity, which requires verification). If confirmed, Structure B becomes legally trivial and the deductibility trap disappears.

**Open questions for Leo this week (not blocking Phase 0):** (1) Is Vernayo formally enrolled as a Solutions Partner entity, or only individual certs? (2) Does he have a Partner Development Manager assigned? (3) Is Vernayo listed in the public Solutions Marketplace directory? Answers determine whether Structure B is on the table.

### MRR math at realistic scale

- 5 clients @ €350/mo = €21k/year
- 10 clients @ €350/mo = €42k/year
- 20 clients @ €350/mo = €84k/year

Below 10 active clients = supplementary income. 20+ = real business needing 0.5 FTE ops support.

---

## 6. Operational Reality (sharpened)

| Responsibility | Effort | Notes |
|---|---|---|
| VPS hosting + monitoring | Low | Hetzner/OVH + UptimeRobot |
| Backups + DR | Medium | No native Twenty tooling — must build (~1 week, in workstream H) |
| Security patches | Medium | Monthly cadence, AGPL fork rebase included |
| GDPR posture (DPA per client, audit trail, right-to-erasure) | High | DPA template + tooling — workstream G |
| Email deliverability | Low (with Postmark) | Per-client domain DKIM setup ~30 min/client |
| Sequences/reply detection ops | Medium | IMAP IDLE connections + Graph webhook subscription renewal |
| Per-client custom integration | Variable | €2-8k per integration as separate devis |
| Twenty fork rebase | Low | 2-4 hours/month |

**Realistic ops baseline at 10 clients:** ~1.5-2 days/week of pure ops. Marcela-time-budget aware. Must not sell past capacity — raise prices first.

---

## 7. Top Risks (re-ranked)

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| 1 | First-client ICP mismatch (Shopify-heavy or Marketing-Hub-Pro user) loses the deal early | **Critical** | Apply `ICP-ARCHETYPE.md` hard filters; first 3 clients from Leo's lost-deal pipeline or external network |
| 2 | Solo-operator capacity ceiling — parallel build + ongoing ops + onboarding | **Critical** | Cap at 5 clients during Phase 1-2; raise to 10 only after Phase 3 trigger; contract 0.5 FTE ops at that point |
| 3 | Build effort overruns (12-18 weeks → 25+ weeks) eats runway | High | Strict v0 scope discipline; reject feature creep; defer to bridges where possible |
| 4 | Twenty CLA → future closed-source ratchet (HashiCorp/Elastic precedent) | High | Pin fork on stable AGPL release; annual fork health review |
| 5 | Leo accidentally exposed (client mentions him publicly, HubSpot partner contact discovers the link) | Low (downgraded) | SPPA Section 2 explicitly non-exclusive — partners may recommend competing third-party services. Brand separation suffices: no HubSpot Solutions Partner badge, co-marketing, or visual identity in any leCRM material. |
| 6 | Micro-entrepreneur deductibility trap erodes margin (~46% net under Structure A vs ~70% under Structure C) | High | Migrate from Structure A to Structure C once first 3 clients onboard; plan SASU incorporation around €3-4k MRR (~8-12 clients) |
| 6 | Email deliverability fails despite Postmark (per-client domain misconfig, complaint spikes) | Medium | Per-client onboarding checklist; complaint rate alerts |
| 7 | Custom dev work eats setup margin (under-quoted integrations) | Medium | Tiered setup pricing (basic/standard/complex); separate devis for non-standard work |
| 8 | Reporting depth gap loses sales-VP-driven prospects | Medium | Cube.dev + custom dashboards in P1; honest in sales: "sales rep tool first, sales VP dashboard second" |
| 9 | Twenty 2-week release cadence creates merge fatigue on shallow fork | Medium | Don't chase every release; track only security + features we want |
| 10 | HubSpot drops prices or releases free-tier features that close the value gap | Low-Medium | Reposition on data sovereignty + customization, not just price |

---

## 8. Recommended Decision Path (updated)

### Phase 0 — Commercial Gate (this week, ~5 hours of work)
- [ ] Brief Leo with this v2 memo + `ICP-ARCHETYPE.md`. Align on the discreet-introducer model (no name on bills, separate referral agreement).
- [ ] Decide customer-facing brand name ("leCRM" is internal-only).
- [ ] Identify 3 candidate prospects matching the archetype from Leo's lost-deal pipeline or external network.
- [ ] **Decision point:** if no qualifying prospects exist within Leo's reachable network, stop here.

### Phase 1 — v0 Parallel Build + First Client (weeks 1-6)
Run 4 dev tracks + 1 legal track in parallel (see Section 3 for detail):

**Track A:** Shallow fork — stub enterprise gate, OIDC SSO, multi-tenant baseline (1-2 weeks)
**Track B:** Postmark integration + bounce webhooks (3-5 days)
**Track C:** Per-client ops baseline (Docker compose, backups, monitoring) (1 week)
**Track D:** Embedded Metabase reporting via iframe extension (1 week)
**Track E (parallel non-dev):** GDPR DPA + SLA + terms + brand identity (2 weeks calendar, mostly external waiting)

**Sales track:** Discovery call(s) on Phase-0 candidates → sign first Design Partner.

**Decision point at week 6:** First Design Partner signed and v0 stack live? If yes, proceed. If no, root-cause and decide.

### Phase 2 — v1 Productization in Parallel with Clients 2-3 (weeks 5-14)
- Track F: Native sequences with reply-detection (4-6 weeks, parallel agents per protocol)
- Track G: Cube.dev semantic layer + custom React dashboards (2-3 weeks)
- Track H: Row-level permissions + SAML SSO + audit UI (3-4 weeks combined)
- Track J: Standard connector library, built reactively per signed client (3-5 days each)
- Track K: PWA polish (1-2 weeks)
- Track L: Multi-tenant ops automation (1-2 weeks)

**Sales track:** Sign 2-3 additional clients (mix of Design Partner + Paying Beta).

**Decision point at week 14:** 5 paying clients sustaining €1,500+ MRR? If yes, trigger Phase 3. If no, hold scope, focus on retention.

### Phase 3 — Scale (Phase 3 trigger met)
- Contract 0.5 FTE ops support (likely from existing GB Consult network)
- Formalize per-client DPA, SLA, runbooks
- Raise client cap from 5 to 10
- Consider non-Leo sales channel only after model is proven through Leo

### What NOT to do
- Do not start building before Phase 0 commercial gate clears.
- Do not build P1+P2 features before a paying prototype validates demand.
- Do not market this publicly. Sales is 100% via Leo until Phase 3.
- Do not promise feature parity with HubSpot Marketing Hub. Ever.
- Do not take on a Phase 1 client whose use case requires P1 features we haven't built yet.

---

## 9. Open Questions Requiring Decisions

1. Does Leo's HubSpot partner status restrict him from offering competing CRM platforms? — **Ask Leo directly this week.**
2. Customer-facing brand name? "leCRM" works internally; needs a real product name. Decision needed before any client conversation.
3. Joint product (Vernayo + GB Consult) or GB Consult product Leo refers? — Affects DPA, contracts, IP. Decision needed with Leo.
4. ICP positioning: "lost HubSpot deals" or "EU-sovereignty-driven prospects"? Will shape pitch script. Validate with Leo against his actual lost-deal pipeline.
5. Should the public AGPL fork live under "github.com/gbconsult/lecrm" or under a dedicated brand org? Decision needed before publishing first commit.

---

## 10. What's Different from v1

- **License question dramatically narrowed:** ~7 confirmed Enterprise files vs. fear of broad gating. AGPL-only is achievable in 3-6 weeks of dev, not blocked by economics.
- **HubSpot moat sized down:** Connectors moot, deliverability solved by Postmark, sequences buildable in 6-9 weeks, reporting via Cube.dev in 3-4 weeks. The "structural moat" reduces to brand trust + 16-22 weeks of focused build work.
- **Risk profile shifted:** v1 listed license economics + technical gaps as Critical. v2 lists Leo's positioning, solo-operator capacity, and ICP discipline as Critical. The gates moved from legal/technical to commercial.
- **Decision path now 3 phases (vs 4 in v1)** with faster initial validation: Phase 0 is 5 hours, Phase 1 is 6 weeks of build + paid prototype, Phase 2 is the productization sprint. End-to-end to "real business" is a quarter and change, not a year.

---

## Sources

1. Twenty Enterprise file inventory + build sizing: source-grounded inspection of `core-modules/sso/`, `core-modules/auth/strategies/`, `core-modules/enterprise/`. Twenty pricing tiers Pro vs Organization: https://twenty.com/pricing
2. HubSpot moat reassessment 2026 — sources: Postmark pricing (https://postmarkapp.com/pricing), Brevo (https://www.brevo.com/pricing/), Cube.dev semantic layer (Apache 2.0), HubSpot OOO reply-detection community release notes, Reply.io / Apollo.io / Lemlist 2026 pricing
3. Plausible Analytics open-source managed-hosting model: https://plausible.io/, Mautic managed hosting: https://elest.io/open-source/mautic
4. Twenty source files: github.com/twentyhq/twenty/blob/main/packages/twenty-server/src/engine/core-modules/{sso,auth/strategies,enterprise}/
5. Twenty CLA: https://github.com/twentyhq/twenty/blob/main/.github/CLA.md, README + repo activity confirms permissive stance toward forks
6. Leo client portfolio (vault): `${PAI_DATA_DIR}/35_CLIENTS/Leo/README.md`
7. AGPL §13 SaaS obligations: https://fossa.com/blog/open-source-software-licenses-101-agpl-license/

Full research transcripts (license inventory + post-connector moat analysis) retained in session log for deeper drill-down if needed.

---

**End of memo v2.**
