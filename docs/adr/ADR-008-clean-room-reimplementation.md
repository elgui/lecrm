# ADR-008 — Clean-Room Reimplementation (Path D)

**Status:** Accepted
**Date:** 2026-05-10
**Deciders:** Guillaume
**Supersedes:** ADR-002 (Twenty Fork Management)
**Related:** ADR-001 (tenancy), ADR-005 (AI agent tenancy), ADR-007 (encryption/secrets/audit). All to be revisited under the scratch-build foundation.

---

## Context

Earlier ADRs assumed leCRM would be a shallow AGPL fork of [twentyhq/twenty](https://github.com/twentyhq/twenty). That assumption was challenged in a four-round multi-agent council debate (Architect, Engineer, Researcher, Pentester, Code Reviewer, plus a fresh Engineer voice in Round 4). The debate resolved that **the shallow-fork claim was structurally wrong** — honest fork modification touches 30-50 files in security-critical zones, the 2-4 h/month rebase budget is unrealistic once multi-tenant + audit + OIDC work lands, and Twenty's CLA-ratchet probability over a 24-month horizon is non-trivial (35-50% per the researcher voice's base-rate analysis against Elastic / HashiCorp / MongoDB precedents at equivalent funding stage).

The strategic posture also shifted during this conversation:

- **Moat (revised):** ownership + Leo's distribution + tailorization + transparent pricing — NOT AI-native UX. AI-native interfaces remain a near-future upside, not the v1 bet.
- **Pricing:** explicit acceptance of price-tag competition (HubSpot Sales Hub Pro at €100/seat is the obvious frame).
- **Distribution:** Leo's HubSpot-rejected lost-deal pipeline + integration experience is the GTM accelerant.
- **One-sentence pitch:** "transparent, honest pricing with any kind of tailorization."
- **Project trigger:** Leo (Vernayo, HubSpot integrator partner) has been "making HubSpot richer with no MRR." This project gives him something he co-owns equity in.

Under this revised posture, Guillaume's operating premise became:

> "Consider the fork as static and immediately consider it's ours and not relying on anything happening online from third parties to solve anything. ... I'm so blown away by how much can be done nowadays by a solo dev, using a solid, agentic, coding team — that I feel we can tackle pretty much anything as long as the specs are precise and super solid."

The council's Round 4 vote split A=3 / B=2 / D=1, but the synthesis identified Path D as the option that satisfies every requirement in Guillaume's revealed premise — ownership, no third-party dependency, no upstream comprehension debt, no AGPL §13 / CLA exposure — without inheriting Twenty's architectural decisions about a future Guillaume does not share.

The four paths considered:

| Path | Description | Why rejected (or accepted) |
|---|---|---|
| A | Shallow fork (ADR-002 original) | Fork "shallow" claim unrealistic at 30-50 files; rebase tax compounds; ratchet risk inherited; AGPL §13 exposes tenant-scoping logic to §13 filers |
| B | Blind scratch (no Twenty reference) | Forgoes 3 years of CRM domain learning embedded in Twenty's design; reinvents the metadata-engine wheel without benefit of prior art |
| C | Fork v1, migrate customers to scratch v2 | No precedent at solo-operator scale; "where SaaS companies die" (Architect); customer migration across two architectures is an 18-month distraction with churn |
| **D** | **Clean-room reimplementation informed by reading Twenty as a reference** | **Accepted.** Captures Twenty's design lessons; zero derivative-work exposure; full license freedom for leCRM; no upstream dependency; no rebase tax; no ratchet risk |

---

## Decision

### 1. Clean-room reimplementation, not a fork

leCRM is **not a fork of Twenty**. No Twenty source code is copied, ported, or transformed into leCRM's repository. The leCRM repository is greenfield, originated by GB Consult, under a license of GB Consult's choosing (see §2).

Twenty's source code may be **read as architectural reference** — the way a developer reads a textbook. Specifically, the design lessons worth studying are:

- The custom-object metadata engine shape (`object_definitions`, `field_definitions`, `field_values` patterns; dynamic GraphQL schema generation; permission-aware resolvers).
- The workspace isolation pattern (boundaries, scoping, tenant-FK-on-every-row, RLS predicates).
- The audit log infrastructure (event emission, queryable schema).
- The extension/SDK architecture (how to extend per-customer without core modification).
- The auth strategy module shape (OIDC, SAML, password, 2FA flows).
- The migration management approach (NestJS + TypeORM patterns under multi-tenant).

What we explicitly do **not** import:

- Twenty's codebase, file structure, or migrations.
- Twenty's choice of language, framework, ORM, database driver, frontend library, or build system. These choices are revisited from scratch — see ADR-009 (forthcoming, to be authored from the stack research; see §3).
- Twenty's package names, identifiers, or domain types verbatim. Inspiration ≠ copying.

### 2. License freedom

Because leCRM does not contain or derive from any Twenty AGPL-3.0 code, leCRM is **not bound by AGPL §13**. The leCRM codebase may be licensed under any license GB Consult selects — including:

- **MIT / Apache 2.0** (most permissive; signals trust to clients; commodity infrastructure).
- **AGPL-3.0** (matches Twenty's posture; reinforces "open-source-first" pitch even though not legally required).
- **BSL with Change Date** (defensive against competitor lift-and-shift; converts to permissive later).
- **Proprietary closed-source** (zero distribution obligation; reduces reputational and operational surface; closes the open-source narrative).

The license selection is **deferred** to ADR-009 alongside the stack decision. The point of this ADR is that leCRM has **regained** that choice.

### 3. Stack research is the next critical-path decision

Prior ADRs assumed Twenty's stack: TypeScript + NestJS + GraphQL + TypeORM + React. Under Path D, **none of those choices are inherited**. Stack selection becomes a primary research question with weight on:

- **Solo-dev velocity with Claude Code** in 2026 (which stacks does Claude Code understand most fluently and produce correct code for, especially around distributed-systems primitives).
- **Multi-tenant primitives** (RLS support, schema-per-tenant ergonomics, connection-routing patterns native to the framework).
- **AI-native readiness** (which stacks have the cleanest seams for v2 chatbot/voice/agent layers — MCP server libraries, streaming primitives, prompt caching integration).
- **Operational sustainability** at solo scale (memory footprint per tenant, cold-start time, ops tooling maturity, observability ergonomics).
- **Sale-ability of the asset** (would a French CRM consultancy or strategic acquirer at €170-340k 2-4× ARR want to inherit this stack? "Boring TypeScript or Go" sells; exotic-stack discount applies).
- **License compatibility** of major dependencies (we want the license freedom we just gained — no GPL-bombed ecosystems).

The candidate space includes — but is not limited to:

- TypeScript + Node (Bun, Deno, Node) with NestJS / Hono / tRPC / Elysia
- Go with stdlib net/http or Echo, sqlc, ent
- Rust with Axum, Tokio, sqlx, SeaORM (Guillaume explicitly named Rust as a candidate to investigate)
- Elixir + Phoenix (LiveView for the React-killer angle)
- Python + FastAPI
- Hybrid (e.g., Go API + React frontend; Rust API + HTMX frontend; ...)

This research is being primed as a tasket (`#TBD-stack-research-priming`) — see §4.

### 4. Operational consequences

- **`docs/STRATEGIC-OVERVIEW.md` §2 "Technical"** is now superseded — it describes the Twenty fork posture. A revised section will be authored after the stack ADR lands.
- **`docs/ARCHITECTURE.md`** is now an artefact of the previous architectural assumption. It will be substantially rewritten after stack selection. A header banner has been added pointing at this ADR.
- **`docs/FEASIBILITY-MEMO.md` §2-3** (license posture, build roadmap) needs revision. The 4-6 week v0 timeline becomes 11-13 weeks (1-2 weeks Twenty-source reading + 10-12 weeks scratch implementation per the council's R4 honest estimate, accounting for AI-velocity compression on greenfield CRUD work and no compression on RLS / IDOR / metadata schema lifecycle).
- **`docs/adr/ADR-002`** is marked Superseded. Header updated to point at this ADR.
- **`docs/adr/ADR-001`** (tenancy) substantively survives — VPS-per-client → schema-per-tenant migration path is stack-agnostic. To be reconfirmed after stack ADR.
- **`docs/adr/ADR-005`** (AI agent tenancy) substantively survives — agent runtime is a separate microservice in any case; stack-independent.
- **`docs/adr/ADR-007`** (encryption / secrets / audit) substantively survives — SOPS+age secrets-at-rest, audit emission patterns are stack-agnostic.
- **`docs/adr/ADR-003`** (Brevo) survives entirely — email provider is a vendor relationship, not a stack choice.
- **`docs/adr/ADR-006`** (backup/DR) survives entirely — RPO/RTO targets are stack-agnostic.
- **`.taskets/002-v0-build-kickoff.md`** group: Track A (shallow fork) is dead and replaced by the new stack-research tasket and a downstream Track A' (scratch foundation implementation) which will be queued after stack ADR lands. Tracks B-F remain valid in spirit; they will be re-scoped against the chosen stack.

---

## Consequences

### Positive

- **Total ownership of the codebase from line 1.** No comprehension debt, no upstream landlord, no rebase tax, no §13 obligation, no CLA-ratchet risk.
- **Stack freedom.** Choices made for leCRM's actual constraints (solo dev + Claude Code + AI-native v2 + multi-tenant + EU residency), not inherited from Twenty's 2023-era choices.
- **License freedom.** leCRM can be MIT/Apache for trust signaling, BSL for competitor protection, or proprietary for asset hardening. Decision can be made on commercial grounds, not legal obligation.
- **Asset value.** A clean-room scratch CRM under permissive license at 20 clients (€84k ARR) is a more attractive acquisition target than a Twenty-fork — no AGPL contagion concern for the acquirer's broader product line.
- **AI-native seams designed in from line 1.** The MCP server / agent integration / streaming / prompt-caching layers are first-class concerns in the architecture rather than retrofits over an inherited UI shape.
- **Pitch sharpens.** "We built it ourselves, on EU infrastructure, under [chosen license], so you can audit every line that touches your data" is materially stronger than "we forked an AGPL project from Paris and operate it."

### Negative

- **Time-to-first-paying-client widens** from the previous 4-6 week v0 estimate to 11-13 weeks (1-2 weeks Twenty-as-textbook reading + 10-12 weeks scratch implementation per Engineer Amelia's revised Round 4 estimate). Leo's pipeline cycles must absorb this — qualifying conversations should start during the build window, not after.
- **No precedent base** for solo-dev shipping multi-tenant CRM with custom-object metadata in <12 weeks even post-Claude-Code (Researcher Ava's Round 4 finding). This is the first attempt at scale; we have no comparable to anchor schedule confidence.
- **No community-audited security baseline.** Twenty's 10k production users have burned in bugs we will rediscover. Compensating controls: the stack-research dimension explicitly weights distributed-systems-correctness ergonomics (e.g., Rust's type system for tenant scoping, Postgres RLS as the source of truth for isolation, contract-test-first development per ADR-007).
- **More upfront design surface** — every architectural decision Twenty made for free is now ours to make explicitly. The 1-2 week "read Twenty as textbook" phase is non-optional for this reason.
- **No SDK / extension ecosystem inherited.** Twenty's `twenty-sdk` extension package architecture would have been free leverage. We will need to design a similar extension surface ourselves; this is a separate ADR after v1 stabilises.

### Neutral

- **The legal subtlety** in Guillaume's "play with the license" comment is dissolved: there is no Twenty license to play with under Path D. leCRM operates entirely under a license GB Consult chooses, applied to code GB Consult wrote. This sidesteps any AGPL §13 / contract-breach question entirely and is the recommended legal posture for the path.
- **The "ownership" semantic** Mary flagged in the strategic-debate is fully satisfied. Path D is what "ownership" means in the strongest sense.

---

## Alternatives Considered

See the four-path table in the Context section. Paths A, B, C were debated across four council rounds; Path D emerged as the synthesis in Round 4. The council vote tally was A=3 / B=2 / D=1, but the synthesis identified Path D as the option that satisfies every named requirement while neutralising both camps' strongest objections.

The orchestrator-level recommendation, after Round 4, was Path D, on the basis that:

1. It dissolves the time-delta debate (fork 10-12w vs scratch 10-12w; D adds 1-2w reading on top, all within tolerance).
2. It eliminates the comprehension-debt argument (everything is code we wrote; no 2am debugging of someone else's mental model).
3. It captures Twenty's design lessons (the strongest A-camp argument).
4. It satisfies every component of Guillaume's revealed operating premise.

---

## References

- This ADR's decision captured in conversation transcript 2026-05-10 (council mode, four rounds).
- `docs/STRATEGIC-OVERVIEW.md` §1, §4 (strategic moat — to be revised post-stack).
- `docs/FEASIBILITY-MEMO.md` §2 (license posture, fork sizing — superseded for Path D).
- `docs/adr/ADR-002-twenty-fork-management.md` (superseded by this ADR).
- `docs/research/fork-management.md` (research dossier for the fork path; retained for reference; the §6 AGPL §13 discussion no longer applies).
- Council debate full transcript: 2026-05-10 conversation, rounds 1-4.

---

## TO RESOLVE

1. **Stack research and ADR-009.** Tasket `#TBD-stack-research-priming` primes this. Deliverables: research dossier in `docs/research/stack-selection.md` + ADR-009 recording the language / framework / database / API / frontend / license selections with selection criteria and council validation.
2. **License selection.** Bundled into ADR-009 once stack is chosen. Decision criteria: trust signaling vs competitor protection vs asset hardening trade-off.
3. **`docs/ARCHITECTURE.md` rewrite** post-stack ADR. The current document describes the Twenty-fork architecture and is reference-only until rewritten.
4. **`docs/STRATEGIC-OVERVIEW.md` §2 (Technical) revision** post-stack ADR.
5. **`docs/FEASIBILITY-MEMO.md` §2-3 revision** post-stack ADR (license posture, build roadmap, week-targets).
6. **`.taskets/002-v0-build-kickoff.md` group rework.** Track A (shallow fork) is dead and replaced by the stack-research tasket. Other tracks (Brevo wiring, ops baseline, secrets, OIDC, AGPL §13 footer, license guard, Metabase reporting, v2 prototype) need re-scoping against the chosen stack. Re-scoping happens after ADR-009.
7. **Team-of-one scaling check.** The council debate flagged that the v0 timeline widens by ~5 weeks under Path D vs the original fork plan. Confirm Leo's pipeline can absorb this — qualifying conversations should start during the build window, not after.
8. **ADR-005 (AI agent tenancy) reconfirmation.** Substantively stack-agnostic but worth re-reading once the stack is chosen — agent runtime communication patterns may simplify if the chosen stack has first-class MCP / streaming primitives.
