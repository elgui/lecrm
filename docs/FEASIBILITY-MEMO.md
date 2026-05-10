# leCRM — Feasibility Memo: Forking Twenty as a HubSpot Alternative

**Author:** Guillaume (GB Consult)
**Date:** 2026-05-07 (v2 — updated after license + moat reassessment)
**Status:** Internal decision document — not for client distribution
**Subject:** Can we fork Twenty's AGPL-3.0 portion only, build the missing enterprise features and connectors ourselves using AI-augmented dev velocity, and offer a managed CRM service through Leo's sales channel that builds serious MRR?

---

## TL;DR — Verdict

**GO on the technical path. The remaining decision is commercial, not legal or technical.**

The strategic shift — "we build connectors and missing features ourselves with AI-augmented dev velocity" — collapses two of the three biggest concerns from v1:

- **License:** The Enterprise-licensed surface is much narrower than initially feared. Only ~5-7 confirmed files (concentrated in the SSO module and the enterprise gate itself). Everything else (custom objects, workflows, audit log infrastructure, field-level permissions, 2FA, REST/GraphQL API, multi-tenant workspaces) is **AGPL-3.0 today**. We can stay 100% on AGPL by stubbing out the enterprise module and building our own SSO + row-level permissions in **3-6 weeks of dev time**. ([1])
- **HubSpot moat with connectors removed:** Of HubSpot's seven structural moats, two are real engineering work (sequences with reply-detection: 6-9 weeks; reporting depth via Cube.dev: 3-4 weeks), one is positioning (brand trust → GDPR DPA + SLA + price delta), and the rest are buyable or negligible for our ICP. ([2])

**Realistic timeline with parallel AI-coding agents: ~4-6 weeks to first paying Design Partner live; ~12-18 weeks to 5 clients with full v1 stack.** See Section 3 for the parallelized roadmap.

The remaining hard questions are commercial:
1. **ICP discipline** — first paying clients must NOT be Marketing-Hub-Pro users or Shopify-heavy e-commerce shops. See `ICP-ARCHETYPE.md` for the full archetype.
2. **Solo-operator capacity** — even with parallel agents, build + client ops + onboarding is multi-quarter execution. Cap clients at 5 in Phase 1-2.
3. **Leo's positioning** — resolved: Leo introduces discreetly, his name does not appear on bills or contracts (GB Consult ↔ client direct, Leo paid via separate referral agreement). His HubSpot brand stays clean.

**Recommended path:** Phase 0 commercial gate (~5 hours, this week), then 4-track parallel build + 1 paid Design Partner targeted for week 4-6, layering v1 features (native sequences, Cube.dev dashboards, RLS) over weeks 5-14 in parallel with the first 3 clients live.

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

## 2. License Posture — Resolved (mostly)

### What's gated behind `@license Enterprise`

Direct source inspection ([4]) confirmed only ~7 files carry the Enterprise header, all concentrated in two modules:

| File | Feature | If Missing |
|---|---|---|
| `core-modules/sso/sso.module.ts` + `sso.resolver.ts` | SAML + OIDC SSO module + GraphQL API | No SSO login; settings UI inert |
| `core-modules/auth/strategies/saml.auth.strategy.ts` | Passport SAML strategy | No SAML auth flow |
| `core-modules/auth/strategies/oidc.auth.strategy.ts` | Passport OIDC strategy | No OIDC auth flow |
| `core-modules/enterprise/enterprise.module.ts` + `enterprise.resolver.ts` + `enterprise-plan.service.ts` | License key system, subscription gating | The gate itself — stub it out |

Probable additions (directory confirmed but headers not directly inspected): row-level permission predicates, advanced encryption module, audit log gating UI (the audit infrastructure module itself is **AGPL**).

### What's AGPL today (i.e. free to use, modify, ship)

Custom objects, custom fields, multi-tenant workspaces (subdomain-based), custom views, **workflow engine** (with filter conditions), 2FA, **field-level permissions** (shipped v1.4 as AGPL), **audit log infrastructure**, REST + GraphQL API, webhooks, MCP server, AI chat, Gmail/IMAP/Outlook email sync, calendar sync, multi-step workflows, custom subdomain support, the entire `twenty-sdk` extension package system.

### Build effort to reproduce Enterprise features in AGPL code

| Feature | Effort (AI-augmented) | Notes |
|---|---|---|
| OIDC SSO (Google Workspace, Microsoft Entra) | 2-3 days | `openid-client` already a Twenty dep; substitute the strategy |
| SAML SSO | ~1 week | `passport-saml` MIT; same lib Twenty uses internally |
| Row-level permissions | 2-4 weeks | PostgreSQL RLS + workspace-aware policies; touches the metadata schema layer |
| Audit log UI surface | 3-5 days | Infrastructure already AGPL; just add the view + ensure all writes emit |
| Stub enterprise gate | 1 day | Replace `EnterprisePlanService` with always-valid stub |
| Custom AI models config | 1 week | env-var driven; not really a code feature |
| **Total** | **~3-6 weeks** | One senior dev + Claude Code |

### Fork architecture

This is a **shallow fork**. Twenty's `twenty-sdk` extension model handles 80% of what we want to add (custom objects, workflows, UI panels, AI agents) **without touching core**. The remaining 20% — auth pipeline substitution and row-level permissions — requires modifying ~3-5 core files (`auth.module.ts`, the strategy files, the permission middleware). Not "fork the world," not "vendor a maintained patchset" — just shallow source modifications to a small number of well-isolated files.

Estimated merge/rebase cost against Twenty's 2-week upstream release cadence: **2-4 hours/month**. Manageable.

### AGPL distribution obligation

If we run this as managed SaaS, AGPL §13 obligates us to publish our fork's source on request. **We publish to GitHub by default** — costs nothing, signals trust, complies cleanly. No proprietary value lost (the value is in operations, integrations as separate services, and managed support, not in the CRM source).

### Trademark + CLA risks

- **Trademark:** Must rebrand entirely — no "Powered by Twenty" without permission. "leCRM" is internal-only; needs a customer-facing brand.
- **CLA risk (medium-term):** Twenty's CLA gives them rights to relicense contributions. If they pull a HashiCorp-style ratchet to BSL/SSPL, our fork freezes at the last AGPL release. Mitigation: pin to a stable release tag, plan a once-yearly fork health review. Not an immediate concern.
- **Twenty's stance toward forks:** README explicitly says "Contribute, self-host, fork." No adversarial signal. ([5])

### Verdict

The AGPL-only path is **real and bounded**. Single 30-min courtesy email to Twenty's partner team to introduce ourselves is good practice but not a blocker — we don't need their permission to fork an AGPL project.

---

## 3. Build Roadmap — Aggressive AI-Coding Parallelization

The previous serial estimate (16-22 weeks) compresses substantially if multiple Claude Code agents work in parallel on independent workstreams, AND if v0 leans on hosted services / API bridges where native build can come later.

### v0 — First Paying Client Live (target: 4-6 weeks)

Run 4 parallel tracks. Each track is a distinct agent context with its own scope.

| Track | Scope | Effort | Notes |
|---|---|---|---|
| **A. Shallow fork + multi-tenant baseline** | Stub `EnterprisePlanService`; OIDC strategy (Google + Microsoft Entra covers ~95% of SMB SSO; skip SAML for v0); custom subdomain + workspace provisioning script | 1-2 weeks | One agent, focused |
| **B. Email layer** | Postmark integration: per-client DKIM template, bounce/complaint webhooks → Twenty contact suppression, sending domain template | 3-5 days | Independent of A |
| **C. Ops baseline** | 1-VPS-per-client Docker compose template, automated backups (postgres + S3), UptimeRobot, basic runbook | 1 week | Independent of A and B |
| **D. Embedded reporting (Metabase, not Cube.dev for v0)** | Self-hosted Metabase pointed at Twenty's PostgreSQL with workspace-scoped SQL queries; embed via iframe in Twenty's UI as an extension package; live with the "Powered by Metabase" logo for v0 (free Embedding License) | 1 week | Cube.dev custom dashboards is a v1 swap-in |
| **E. Legal/trust track (parallel non-dev)** | GDPR DPA template (1 lawyer engagement), uptime SLA doc, terms of service, brand identity, customer-facing brand decision | 2 weeks calendar (mostly waiting) | Runs alongside A-D |

**v0 deferred to bridges, not built:**
- **Sequences:** Reply.io API bridge for clients 1-3. Native build kicks off after first client live. ($89/seat passed through to client or absorbed in MRR.)
- **Custom connectors:** Built only per signed-client requirement, 3-5 days each with AI agent.
- **PWA polish:** Responsive web works for v0; PWA upgrade in v1.
- **Row-level permissions:** Workspace-level isolation suffices for v0; per-record ACLs added in v1.
- **Audit log UI:** Infrastructure already AGPL; ship the existing surface as-is; custom view in v1.
- **SAML SSO:** OIDC covers Google Workspace + Microsoft Entra; SAML if a specific client needs it.

**v0 critical path:** ~4 weeks calendar with 3-4 parallel agents + 1 day/week founder oversight, plus ~2 weeks for the legal track (mostly external waiting). First paying Design Partner can sign in **week 4-6**.

### v1 — Productized Stack (target: weeks 5-12, parallel with first 3 clients live)

Run while clients 1-3 are in production. The first clients' needs drive the v1 backlog.

| Track | Scope | Effort |
|---|---|---|
| **F. Native sequences w/ reply-detection** | Gmail Pub/Sub + Microsoft Graph subscription + IMAP IDLE + state machine + OOO classifier (Haiku) | 4-6 weeks (compressed from 6-9 with agent parallelization on each protocol) |
| **G. Cube.dev semantic layer + custom dashboards** | Replace Metabase iframe with embedded React dashboards consuming Cube REST API | 2-3 weeks |
| **H. Row-level permissions** | PostgreSQL RLS + workspace-aware policies in metadata schema | 2 weeks |
| **I. SAML SSO + audit UI surface** | If client demand emerges | 1 week each |
| **J. Standard connector library** | Shopify, Klaviyo, Stripe, HelloHarel, Brevo — built reactively as clients sign | 3-5 days per connector |
| **K. PWA polish** | Service worker, offline read, Web Push, install prompt | 1-2 weeks |
| **L. Multi-tenant ops tooling v2** | Provisioning automation, central monitoring dashboard, scheduled rebases | 1-2 weeks |

**v1 critical path:** ~8 weeks calendar with 2-3 parallel agents + ongoing client ops capacity.

### Rolled-up timeline

| Milestone | Target |
|---|---|
| Phase 0 commercial gate cleared | Week 0 (this week, 5 hours) |
| First Design Partner signed | Week 4-6 |
| 3 clients live with v0 stack | Week 8-12 |
| Native sequences shipped | Week 10-14 |
| 5 clients live, full v1 stack, Phase-3 trigger met | Week 14-18 |

This is materially faster than the v1 memo's 16-22 weeks. The compression comes from: (a) parallel agents on independent tracks, (b) hosted-service bridges in v0 instead of native builds, (c) deferring features to actual client demand instead of speculative build, (d) accepting "good-enough" v0 quality where v1 polish doesn't gate first revenue.

**Real ceiling on this timeline isn't dev velocity — it's:**
- Client decision cycles (4-8 weeks from intro to signing — externally bounded)
- Legal review and DNS propagation lead times
- Twenty release rebases (low-frequency but distracting)
- Founder context-switching between build, sell, and ops

### What we're explicitly NOT building

- Marketing automation engine — n8n + Brevo handles 80% of SMB needs; Mautic as optional managed add-on for clients who genuinely need lead scoring
- Native iOS/Android apps — PWA suffices for 12-18 months
- HubSpot Academy equivalent — Loom videos + Mintlify docs
- Custom landing page builder — Webflow/Carrd as recommended pairing
- E-signature — PandaDoc API for clients who need quotes

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
