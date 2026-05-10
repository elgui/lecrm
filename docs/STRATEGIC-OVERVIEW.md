# leCRM — Strategic Overview

**Author:** Guillaume (GB Consult)
**Date:** 2026-05-07
**Status:** Synthesis document — decision-ready
**Audience:** Guillaume primary; Leo on a need-to-know basis (with the agreed discreet-introducer framing)
**Length:** ~10-15 minute read

This document synthesizes the situation across `FEASIBILITY-MEMO.md`, `ICP-ARCHETYPE.md`, `LEGAL-PLAYBOOK.md`, and `HUBSPOT-PARTNER-BILLING-RESEARCH.md` into a single decision-ready overview. Cross-references those for full detail.

---

## 1. One-Page Executive Summary

**What:** Managed CRM SaaS for French/EU SMBs, built on a forked AGPL-3.0 Twenty CRM (Paris-based, YC-backed, $5M raised, v2.2 active). GB Consult hosts, customizes, and operates end-to-end. Leo introduces.

**Why now:**
- HubSpot is structurally exposed on price (Sales Hub Pro €100/seat) and on EU data sovereignty
- AI-augmented dev velocity collapses the two historical moats (integration count, custom feature breadth) — connectors and "enterprise" features (SSO, RLS, audit) become weeks-of-work, not blocking
- An open-source AGPL core gives us complete UI freedom — we can build interfaces HubSpot's API rate-limits make impossible (chatbot-native, voice-first, AI-agent-driven CRM)
- Leo has a steady stream of HubSpot lost-deals and an under-monetized warm network of price-sensitive French SMBs
- Guillaume's existing CaaS/AI-agent infrastructure (Tele-Claude, OpenClawing) is a head start on the AI-native UI angle

**Verdict:** **GO**. The technical path is bounded and resolved. Legal posture is workable (apporteur d'affaires now, hybrid billing later, SPPA non-exclusive). The unit economics work at 10+ clients under SASU. The forward-looking strategic value (AI-native interface freedom) is the actual product moat, not the price differential.

**What it costs to find out:** ~5 hours commercial gating + €1,500-2,400 one-time legal + 4-6 weeks parallel build = a paid Design Partner live in 4-6 weeks, decision point clean.

**What it could become:** 20 paying clients at €350+ MRR + setup + custom dev = ~€100k/year revenue, ~€50k/year personal income under SASU, plus a sellable asset (open-source SaaS valuations 2-4× ARR = €200-400k optionality), AND a deployable platform for Guillaume's broader AI-agent work.

---

## 2. The Situation Today (What's Validated, What's Decided)

### Technical
- **Twenty AGPL-3.0** is the right foundation. ~7 confirmed `@license Enterprise` files (SSO module + license-gating system). Everything else (custom objects, workflows, audit infrastructure, field-level permissions, 2FA, REST/GraphQL, multi-tenant workspaces, Gmail/Outlook sync) is AGPL today. ([1])
- **Building enterprise features in AGPL code** = 3-6 weeks AI-augmented dev (OIDC + SAML + row-level permissions + audit UI). Shallow fork (~3-5 core files modified). 2-4h/month upstream rebase cost.
- **Compressed v0 timeline with parallel agents**: 4-6 weeks to first paying Design Partner live. Use bridges in v0 (Reply.io for sequences, embedded Metabase for reporting, OIDC-only SSO) and replace with native builds in v1 weeks 5-14 while clients are live.

### Legal
- **AGPL §13 distribution obligation**: publish fork on GitHub by default — no proprietary value lost (value is in operations, not source). ([2])
- **HubSpot SPPA Section 2 explicitly non-exclusive**: partners may recommend and work with competing third-party products. The Leo positioning risk is **far lower than v1 feared**. Brand separation is the primary mitigation. ([3])
- **Apporteur d'affaires structure** is the right legal frame for Leo. Avoids agent commercial statute (L134-1) which would trigger mandatory termination indemnity.
- **Phase-0 lawyer engagement** scoped at €1,500-2,400 fixed budget: DPA, CGV, SLA, beta agreement, apporteur d'affaires contract, AGPL compliance review. ([4])

### Commercial
- **ICP archetype defined**: French/EU SMBs (3-15 users), price-sensitive or sovereignty-driven, NOT marketing-Hub-Pro-dependent, NOT Shopify-heavy. First clients from Leo's lost-deal pipeline, NOT his active book. ([5])
- **Beta tiers structured**: 1-2 Design Partners (€99-149/mo, 6-month locked, public testimonial expected) + 3-4 Paying Beta (€250-450/mo, 30-day cancel).
- **Sub-processor stack**: swap Postmark (US, no DPF cert) for Brevo or Scaleway TEM (FR) before first client signs. Same cost, full EU residency.
- **Postmark TIA risk eliminated** by the swap.

### Financial reality (per legal playbook)
- **Micro-entrepreneur deductibility trap**: under régime BNC micro, Leo's 30% commission is NOT deductible from URSSAF base. Effective per-client margin drops from ~70% (naive) to ~46% (real). ([4])
- **SASU incorporation trigger**: ~€3-4k MRR (8-12 clients), not at the €77,700 micro ceiling. SASU restores commission deductibility and gets net margin back to ~70%.
- **Structure C migration** (Vernayo invoices integration consulting; GB Consult invoices MRR) captures most tax efficiency at low SPPA risk. ([6])

---

## 3. The Opportunity

### Market frame
The French SMB CRM market is bifurcated:
- HubSpot owns the upper SMB and mid-market segment via brand recognition + free-tier funnel
- Pipedrive, Monday.com, Zoho, Sellsy, noCRM.io fight in the affordable mid-tier
- Below them, a long tail of SMBs use spreadsheets, Notion, Airtable, abandoned HubSpot Free, or nothing

**Our wedge is not "cheaper HubSpot."** That fight is crowded. Our wedge is:
- **For prospects who rejected HubSpot already** (price, sovereignty, customization) — they are pre-qualified for an alternative
- **For prospects with non-standard data models** (vintages, projects with milestones, regulated industries) — Twenty's unlimited custom objects beat HubSpot's tier-gated Custom Objects
- **For prospects who want AI-native workflows now**, not when HubSpot's Breeze pricing tier reaches them — and this is where the strategic moat lives (Section 4)

### Sourcing through Leo
Leo's reachable network at-rest produces an estimated ~30-50 qualifying prospects/year:
- Lost-HubSpot-deals (price, sovereignty, customization rejections): historically 40-60% of his pipeline
- Founder peer network (consultancies, distributors, B2B services): warm intros possible
- Existing happy clients refer their peers (ChefCheffe → other restaurateurs/producers)

We need ~5-10 of these to convert to paying clients in year 1 — a ~15-25% close rate from a pre-qualified pool — to validate the model.

### Three illustrative ICP fits (per archetype doc)
- **Persona A** — Boutique consultancy (7 partners + associates, Paris, rejected HubSpot 8 mo ago on price, data-sovereignty conscious, technically curious)
- **Persona B** — Regional wine distributor (12 people, B2B, frustrated Pipedrive, custom data model around estates/vintages, French infrastructure narrative fits brand)
- **Persona C** — Greentech B2B sales lead (Series-A startup, hit HubSpot Free limits, can't justify Pro, needs custom objects, technical founder)

These are not theoretical — Leo can name 3-5 actual prospects matching each persona within his lost-deal pipeline.

---

## 4. The Strategic Moat — Complete UI Freedom and AI-Native Interfaces

This is the section that matters most for the long-term value of the project. It is also the angle that no incumbent CRM can match.

### Why HubSpot's API is structurally limited
HubSpot's API is a *gatekeeping interface*, not a freedom interface:
- **Rate limits**: Sales Hub Pro at ~100 requests/10s, ~1,500/100s burst, daily caps. This is fine for periodic data sync. It is **fundamentally incompatible** with interactive UIs that read/write CRM data on every user action — multi-user chatbots, voice interfaces, real-time AI agents all hit the ceiling fast.
- **Read-only access to certain primitives**: workflows, sequences, and proprietary AI features cannot be replicated externally — partners build *around* HubSpot, not on top of it.
- **No source access**: integrators cannot modify the UI, the data model behind the UI, or the relationship between data and UI. The product is what HubSpot ships.
- **AI features locked behind tier upgrades**: Breeze Assistant requires Sales Hub Pro+; Breeze Agents are consumption-priced credits *on top of* base subscription. Custom AI agents that the *operator* controls are simply not on offer.

### What Twenty AGPL gives us
- **Full source access** to the data model, the API server, the workflow engine, the UI. We can change anything.
- **Unlimited internal API** — when our UI talks to our backend, there is no rate limit. We can build interactive interfaces that issue 50 calls/second per user without any throttling.
- **Custom UI per client if we want**. Wine distributor sees a wine-shaped interface. Consultancy sees deal-flow-shaped. SaaS company sees pipeline-shaped. Same backend.
- **Headless mode**: the CRM data model becomes a service that *anything* can be a UI for — Telegram bot, WhatsApp bot, voice assistant, native mobile shell, embedded widget in another product.

### What we can build that HubSpot cannot
**Conversational CRM as the primary interface.** Instead of users clicking through HubSpot's UI to update a deal stage, log a call, send a follow-up, schedule a meeting — they message a chatbot in Telegram/WhatsApp/Slack. The chatbot:
- Logs a call after the rep finishes ("just had a call with Marc, he's interested but wants to think for two weeks")
- Updates deal stage on its own decision based on conversation context
- Drafts and queues a follow-up email for the rep to approve
- Surfaces upcoming actions ("Marc said two weeks — that's tomorrow, do you want me to send the follow-up?")
- Pulls reports on demand ("how many deals did we win last quarter from referrals?")

This is not a HubSpot integration. It is a different product category. **HubSpot's API rate limits make this UX impossible at multi-user scale.** Twenty AGPL with our UI freedom makes it trivial.

**Voice-first CRM.** A rep finishes a client call, hits a button on their phone, dictates a 30-second summary. The CRM extracts: contact, deal, stage, next action, sentiment, follow-up date. Logs everything. Sends notifications. Updates the pipeline. Voice → action, no clicking. The infrastructure for this (Whisper transcription, Claude classification, Twenty CRUD via internal API) costs €0.01/call.

**AI agents as autonomous CRM users.** An agent watches the CRM in real time. When a deal sits in "Qualified" for >14 days without activity, it drafts a re-engagement email referencing the prospect's stated pain points (logged in custom fields). When a meeting transcript lands, it updates the deal stage, identifies objections, schedules follow-ups. The rep approves; the agent executes. This is what HubSpot's Breeze Agents *aspire to* but are constrained by — they run inside HubSpot, on HubSpot's data, on HubSpot's terms, paying HubSpot's per-credit rates. Ours run on our infrastructure, with our prompts, on our data, at our cost (~€0.01-0.10 per agent action with Haiku/Sonnet).

**Embedded LLM-driven dashboards.** Instead of static charts, the dashboard is a chat interface. "Show me deals likely to close this quarter" becomes a Claude-driven query that the agent answers from the actual PostgreSQL data, with reasoning, and you can drill in by following up. This is a Cube.dev semantic layer plus an LLM agent — buildable in 2-3 weeks.

### Why we are uniquely positioned to build this
- Guillaume already operates **CaaS infrastructure** (Tele-Claude, OpenClawing, the existing Telegram-bot-as-AI-interface pattern is in production for Leo himself today)
- Anthropic API integration is a daily-use skill, not a learning curve
- AI-augmented dev velocity makes "weeks of focused build" a real schedule, not a fantasy
- The Twenty AGPL data model is well-documented and accessible from outside via GraphQL — a chatbot or voice agent can read/write it cleanly
- French SMBs are increasingly comfortable with conversational interfaces (WhatsApp Business, Telegram, voice) — but no CRM has shipped this UX

### Sequencing
- **v1 stack** focuses on parity (sales pipeline, sequences, reporting) — the must-haves for any CRM
- **v2 stack** layers AI-native interfaces on top:
  - Telegram/WhatsApp chatbot CRM as a per-client opt-in (4-6 weeks build, leverages existing Tele-Claude infrastructure)
  - Voice-call-to-CRM logging (3-4 weeks, Whisper + Claude + Twenty API)
  - Autonomous agent that watches the pipeline and drafts actions (2-3 weeks per agent type)
  - LLM-driven dashboard ("ask your CRM") (2-3 weeks)
- **v2 timing**: opportunistic, after first 5 clients live. Sell as a premium add-on at €100-200/mo extra per client.

### The defensive moat this creates
Once we ship v2 features, the gap with HubSpot is no longer "open-source alternative" — it's "fundamentally different product category." HubSpot has no answer because their API is the bottleneck. Salesforce has the same constraint. Pipedrive doesn't even try. Twenty itself has not built this — and our AGPL fork can ship it without negotiating with anyone.

This is the actual product moat. The cost-savings narrative gets us to first clients. The AI-native interface narrative gets us to a defensible long-term position.

---

## 5. Cost of Running

All figures in EUR, monthly unless noted, mid-2026 pricing.

### Phase 1 — 1-3 paying clients (months 1-6)

| Item | Monthly | Annual | Notes |
|---|---|---|---|
| Hetzner CX22 per client (1-VPS-per-client v0) | €5/client | €60/client | Dedicated VM, 4GB RAM, sufficient for 5-15 user workspace |
| S3-compatible backups (Hetzner Storage Box) | €3/client | €36/client | Daily Postgres dump + S3 object storage |
| Email layer: Brevo Lite (shared) | €25 base | €300 | All clients on shared Brevo account; per-client domain auth |
| Monitoring: Better Stack starter | €5 | €60 | Uptime + log aggregation |
| AI APIs (Anthropic, in-app features at v0) | €50 | €600 | Embedded AI chat, light usage |
| Claude Code subscription | €150 | €1,800 | Dev productivity (mandatory infra) |
| RC Pro insurance (Hiscox-class) | €60 | €720 | €1M coverage estimate |
| Cyber insurance | €40 | €480 | Data breach response |
| Accounting (Tiime/Pennylane micro tier) | €25 | €300 | |
| Domain + SSL + misc | €5 | €60 | |
| INPI trademark amortized | €15 | €190 (one-time) | One class registration |
| Lawyer engagement amortized | €100/mo over 18 mo | €1,800 (one-time) | DPA + contracts + apporteur |
| **Phase 1 total at 3 clients** | **~€520/mo** | **~€6,200/yr** | Plus one-time legal+brand ~€2,500 |

### Phase 2 — 10 clients (months 6-14)

| Item | Monthly | Annual | Notes |
|---|---|---|---|
| Shared Hetzner CX42 (8GB) for 10 clients | €18 | €216 | Or 10 × CX22 = €50; shared cheaper if multi-tenant works |
| Backup storage S3 | €15 | €180 | |
| Brevo Pro (~50k emails/mo) | €60 | €720 | |
| Monitoring (Better Stack Pro) | €30 | €360 | |
| AI APIs (more usage as v2 features ship) | €200 | €2,400 | |
| Claude Code | €150 | €1,800 | |
| RC Pro | €60 | €720 | |
| Cyber insurance | €60 | €720 | |
| Accounting (more transactions) | €40 | €480 | |
| Domain/SSL/misc | €10 | €120 | |
| Trademark amortized | €15 | €190 | Continuing |
| **Phase 2 total at 10 clients** | **~€660/mo** | **~€7,900/yr** | Infra cost is small |

### Phase 3 — 20 clients (months 14-24, requires SASU)

| Item | Monthly | Annual | Notes |
|---|---|---|---|
| Shared Hetzner CCX23 (8 vCPU dedicated, 32GB) | €30 | €360 | Production-grade |
| Backup VPS for HA | €20 | €240 | Standby for failover |
| Postgres replica VPS | €20 | €240 | |
| S3 backups (longer retention) | €40 | €480 | |
| Brevo Premium (~150k emails/mo) | €120 | €1,440 | |
| Monitoring | €50 | €600 | |
| AI APIs (v2 features at scale) | €400 | €4,800 | Chatbot interfaces, agents, LLM dashboards |
| Claude Code | €150 | €1,800 | |
| RC Pro (higher coverage) | €100 | €1,200 | €2M coverage |
| Cyber insurance (higher) | €100 | €1,200 | |
| Accounting (SASU complexity) | €60 | €720 | |
| Expert-comptable (mandatory under SASU) | €150 | €1,800 | |
| Domain/SSL/misc | €15 | €180 | |
| **Phase 3 total at 20 clients** | **~€1,260/mo** | **~€15,000/yr** | Still <20% of revenue |

### One-time costs (year 1)

| Item | Cost |
|---|---|
| Lawyer (Phase-0 templates + apporteur contract) | €1,500-2,400 |
| INPI trademark filing (1 class) | €190 |
| Brand identity (logo, basic visual) | €0-1,500 (DIY-with-AI possible) |
| Initial dev time (4-6 weeks Guillaume's time) | Opportunity cost €15-25k @ €600-800/day |
| SASU incorporation (when triggered) | €500-1,500 + €300/year minimum activity |
| **Total cash one-time** | **~€2,200-5,500** (excluding opportunity cost of dev time) |

### What infrastructure cost is NOT
A meaningful percentage of revenue. At 20 clients, **infra is ~17% of revenue**, well below SaaS industry norms. The real cost of this business is Guillaume's time, not infrastructure.

---

## 6. Revenue Model and Unit Economics

### Revenue streams per client

| Stream | Phase 1 | Phase 2 | Phase 3 |
|---|---|---|---|
| Setup (one-time) | €0-2,000 | €1,500-3,500 | €2,500-4,000 |
| MRR | €99-450 | €250-450 | €350-500 |
| Custom dev (separate devis) | Rare | 5-15 days/year | 15-30 days/year |

### Total annual revenue at each phase (mid-case)

**Phase 1 (3 clients steady, mix of Design Partner + Paying Beta):**
- MRR: 3 × €250 avg = €750/mo = €9,000/yr
- Setup: 3 × €1,500 avg = €4,500 (year 1 only)
- Custom dev: minimal
- **Year 1 total: ~€13,500**

**Phase 2 (10 clients steady, 5 newly onboarded that year):**
- MRR: 10 × €350 avg = €3,500/mo = €42,000/yr
- Setup: 5 new × €2,500 = €12,500
- Custom dev: 8 days × €700 = €5,600
- **Year 2 total: ~€60,000**

**Phase 3 (20 clients steady, 10 newly onboarded that year):**
- MRR: 20 × €375 avg = €7,500/mo = €90,000/yr
- Setup: 10 new × €3,000 = €30,000
- Custom dev: 25 days × €750 = €18,750
- **Year 3 total: ~€140,000**

### Net to Guillaume after Leo + infra + tax/social

The structure matters a lot. Three scenarios:

**Scenario X — Stay micro-BNC, Structure A (apporteur):**
- Year 2 (10 clients) revenue: €60,000
- Less infra: -€7,900 = €52,100
- Less Leo commission (30% of MRR + setup, paid by GB Consult): -€16,350
- Less URSSAF (21.2% on gross €60k, *not* on net): -€12,720
- Less IR (versement libératoire 2.2% on gross): -€1,320
- **Net to Guillaume: ~€21,700/year**

**Scenario Y — SASU + Structure C (Vernayo bills setup, GB Consult bills MRR):**
- Year 2 (10 clients) revenue: €60,000 split as €42k MRR via GB Consult-SASU + €18k setup/integration via Vernayo
- Vernayo's cut on its own invoices (€18k less Vernayo's costs/tax): not Guillaume's concern
- GB Consult-SASU revenue: €42k MRR + share of custom dev = ~€47k
- Less infra (full): -€7,900 = €39,100
- Less commission paid to Vernayo on MRR (30% × €42k, fully deductible under SASU): -€12,600
- Pre-tax SASU profit: €26,500
- Corporate tax (15% on first €42k): -€3,975
- Post-tax SASU profit: €22,525
- Distributed as dividend (PFU 30%): €15,800 to Guillaume personally
- OR distributed as salary (~50% charges total): roughly €11,000 net
- Choose dividend: **Net to Guillaume: ~€15,800/year**, but with the SASU retained earnings building reserves

**Scenario Z — SASU + Structure B (Vernayo prime, GB Consult is subcontractor) — only if Leo confirms not formally Solutions Partner:**
- Vernayo bills client €60k; Vernayo pays GB Consult-SASU as subcontractor (~70% of revenue = €42k)
- Effectively reverses the apporteur structure
- Tax burden distributed differently between Vernayo and GB Consult
- Optimal under correct accounting; ~€18-22k net to Guillaume estimated

### The honest unit economics summary

- **Below 5 clients**: side bet. Net €5-10k/year personal income. Strategic value (learning, IP, optionality) > financial return.
- **5-10 clients**: substantial side income. Net €15-25k/year. Time investment ~1.5 days/week.
- **10-20 clients**: real second income. Net €25-50k/year. Time investment ~2-3 days/week. Triggers SASU incorporation.
- **20+ clients**: primary income tier. Net €50-80k/year. Time investment ~3-4 days/week. Needs second hire on ops.

---

## 7. Valuation — Is It Worth It?

### Three things you're buying for the build investment

1. **Direct income** at the unit-economics tiers above. ROI on the ~5-week build is positive at >5 paying clients within 12 months — a low bar given Leo's pipeline.

2. **An asset.** Open-source SaaS businesses sell at 2-4× ARR on EBITDA-positive deals. At 20 clients × €350 MRR = €84k ARR, the asset is worth €170-340k to a strategic acquirer (a French CRM consultancy, a bigger HubSpot partner, a vertical SaaS player). This optionality has real value even if you never sell.

3. **A platform** for the broader CaaS / AI-agent work. Every CaaS opportunity (Tele-Claude, OpenClawing, future personal-agent products) benefits from having a real CRM-shaped data model already in production. The leCRM platform is reusable infrastructure, not just a product.

### The strategic upside that's harder to quantify
- **AI-native interface category creation.** If chatbot-driven CRM and voice-first CRM become a real category, leCRM is among the first credible products with that DNA. First-mover positioning in a French/EU SMB segment HubSpot cannot serve.
- **Reference clients for Guillaume's broader consulting.** Each leCRM client becomes a case study + proof of execution + warm referral source for the rest of GB Consult's offerings.
- **A vehicle for Leo to expand his own footprint** without diluting his HubSpot brand — strengthens the partnership long-term.

### Compared to alternatives
- **Pure consulting.** Bills time-for-money with no asset accumulation. leCRM builds an asset.
- **Building a different SaaS from scratch.** Twenty saves 18-24 months of foundational engineering. Greenfield SaaS attempts at this scale typically fail on time-to-market.
- **Doing nothing.** Leo's lost-deal pipeline currently produces €0 for both parties. Even at modest 5-client conversion this is incremental revenue with no cannibalization.

### What kills the deal
- Leo's lost-deal pipeline produces fewer than 3 qualifying prospects in 6 months → market signal is wrong, stop.
- First Design Partner refuses to pay €150+/mo after 90 days → pricing model is wrong, stop or pivot.
- Twenty pulls a HashiCorp-style license ratchet to closed-source within 12 months → fork freezes, maintenance cost rises sharply, reassess whether to continue or rebuild from scratch.
- Solo-operator capacity ceiling hits before 5 clients (build + ops + sales overwhelms one person) → sequence the work harder, defer features, do not take new clients past capacity.

---

## 8. Top Risks (final ranked list, post-research)

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| 1 | First-client ICP mismatch (Shopify-heavy, Marketing Hub Pro-dependent) → bad reference | Critical | Apply ICP archetype hard filters; first 3 from Leo's lost-deal pipeline only |
| 2 | Solo-operator capacity ceiling | Critical | Cap clients at 5 in Phase 1-2; raise to 10 only after Phase 3 trigger; contract 0.5 FTE ops at 15+ |
| 3 | Micro-BNC deductibility trap erodes margin | High | Migrate to Structure C after first 3 clients; SASU incorporation around €3-4k MRR |
| 4 | Build effort overruns (12-18 wk → 25+ wk) | High | v0 leans on bridges (Reply.io, Metabase iframe); strict scope discipline; defer features to client demand |
| 5 | Twenty CLA → future closed-source ratchet (HashiCorp/Elastic precedent) | High | Pin fork on stable AGPL release; annual fork health review |
| 6 | Postmark TIA risk under GDPR | Medium → eliminated | Swap to Brevo (FR) or Scaleway TEM (FR) before first client signs |
| 7 | Leo's HubSpot partner exposure | Low (downgraded) | SPPA Section 2 explicitly non-exclusive; brand separation is sufficient mitigation |
| 8 | HubSpot drops prices or releases free-tier features that close the gap | Low-Medium | Reposition on AI-native interface freedom + data sovereignty + customization, not price |

---

## 9. Decision and Next Steps

### Decision: GO

The technical path is bounded. The legal posture is workable. The ICP exists and is reachable through Leo. The unit economics work at 10+ clients under SASU. The strategic upside (AI-native interface category) is uniquely available because of the AGPL-fork-of-Twenty foundation and Guillaume's existing CaaS infrastructure.

The cost of finding out is small (5h commercial gating + €2-4k legal/brand + 4-6 weeks parallel build), and the decision tree at week 6 is clean.

### Phase 0 — This week (~5 hours of work)

- [ ] Brief Leo on the strategic overview + ask: (a) Vernayo formally enrolled as Solutions Partner? (b) PDM assigned? (c) Listed in Solutions Marketplace directory? — answers determine whether Structure B is on the table.
- [ ] Decide customer-facing brand name (not "leCRM" — pick the public-facing brand)
- [ ] Identify 3 candidate prospects matching the ICP archetype from Leo's lost-deal pipeline or external network
- [ ] Engage the lawyer (€1,500-2,400 fixed scope) — this triggers a 2-3 week external waiting window so kick off early
- [ ] **Decision point:** if no qualifying prospects exist OR Leo can't carry the discreet-introducer model, stop here.

### Phase 1 — Weeks 1-6 (parallel build + first paying client)

Run 4 dev tracks + 1 legal track in parallel (per `FEASIBILITY-MEMO.md` Section 3):
- Track A: shallow fork (OIDC, multi-tenant baseline, stub enterprise gate)
- Track B: email layer (Brevo or Scaleway TEM, NOT Postmark)
- Track C: ops baseline (per-client Docker, backups, monitoring)
- Track D: embedded Metabase reporting via iframe extension
- Track E: legal (DPA, SLA, CGV, beta agreement, apporteur contract)

Sales track: discovery calls on Phase-0 candidates → sign first Design Partner.

### Phase 2 — Weeks 5-14 (productize + clients 2-5)

v1 stack (native sequences, Cube.dev custom dashboards, RLS, SAML if needed) in parallel with onboarding. Switch billing to Structure C as second client onboards.

### Phase 2.5 — V2 strategic features (weeks 14+)

Begin layering AI-native interfaces *after* first 5 clients are live and stable:
- Telegram/WhatsApp chatbot CRM (per-client opt-in, premium add-on at +€100-200/mo)
- Voice-call-to-CRM logging
- Autonomous pipeline-watching agents
- LLM-driven dashboards ("ask your CRM")

These are the *strategic* moat features. Not blocking for first revenue. Strongly differentiating once shipped.

### Phase 3 — Scale trigger met (5+ paying clients, €1,500+ MRR)

- Incorporate SASU (with expert-comptable engagement)
- Migrate billing to Structure C uniformly
- Contract 0.5 FTE ops support
- Raise client cap from 5 to 10
- Consider non-Leo sales channel only after Leo-channel saturation

---

## 10. Documents in This Project (at 2026-05-07)

| Doc | Purpose |
|---|---|
| `FEASIBILITY-MEMO.md` | Core decision document with risk register, build roadmap, GTM positioning |
| `ICP-ARCHETYPE.md` | Beta client qualification framework, anti-patterns, 30-min discovery playbook, 3 personas |
| `LEGAL-PLAYBOOK.md` | Phase-0 lawyer brief, DPA structure, CGV, SLA, beta agreement, apporteur d'affaires, AGPL compliance |
| `HUBSPOT-PARTNER-BILLING-RESEARCH.md` | SPPA non-exclusivity verified; three billing structures analyzed; recommended Structure A→C migration |
| `STRATEGIC-OVERVIEW.md` (this doc) | Synthesis, opportunity, cost of running, valuation, AI-native interface moat, decision |

---

## Sources (cross-references)

1. Twenty AGPL inventory + build sizing: `FEASIBILITY-MEMO.md` Section 2 + Section 3
2. AGPL §13 SaaS implementation: `LEGAL-PLAYBOOK.md` Section 7
3. HubSpot SPPA Section 2 non-exclusivity: `HUBSPOT-PARTNER-BILLING-RESEARCH.md`
4. Phase-0 lawyer brief and unit economics under micro vs SASU: `LEGAL-PLAYBOOK.md` Sections 11 + 14 + 15
5. ICP archetype: `ICP-ARCHETYPE.md`
6. Structure C migration logic: `HUBSPOT-PARTNER-BILLING-RESEARCH.md` Section 6

External sources for technical and legal claims are cited inline in the source documents above.

---

**End of strategic overview.**
