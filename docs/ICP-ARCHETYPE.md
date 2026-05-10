# leCRM — Ideal Client Archetype (Beta Phase)

**Author:** Guillaume (GB Consult)
**Date:** 2026-05-07
**Status:** Internal qualification document — Phase 0 / Phase 1 use
**Purpose:** Define the perfect first 3-5 beta clients so Leo can scout/refer carefully without exposing his HubSpot positioning, and so we don't burn the early prototype on a mismatched client.

---

## TL;DR — One-Paragraph Profile

> A French or EU-based small business (3-15 active CRM users) where the founder or commercial lead is the decision-maker, currently running their pipeline in spreadsheets, Notion, Airtable, or a CRM they're actively unhappy with. They have a real sales process to manage (deals, follow-ups, multi-stage pipeline) but DO NOT depend on marketing automation, transactional e-commerce sync, or sequences-with-reply-detection at scale. They're price-sensitive enough that HubSpot Sales Hub Pro at €100/seat is a non-starter, OR they have explicit data-sovereignty concerns (legal, regulated industry, anti-Big-Tech founder). They are technically comfortable enough to engage with a beta product, willing to provide regular feedback, and able to wait for features that aren't shipped yet. Budget: €250-500/month MRR + €2,000-4,000 setup, paid willingly because the alternative was either nothing or a tool they hated.

---

## Hard Filters (Must Have All)

A prospect must satisfy **every** item below to qualify as a beta candidate. If any fails, refuse politely — even if they want to pay.

| # | Filter | Why |
|---|---|---|
| 1 | **3-15 active CRM users** | Below 3, value of multi-user CRM is thin (notebook works); above 15, ops complexity exceeds beta capacity |
| 2 | **EU-based entity** (FR/BE/CH/Lux preferred) | Data sovereignty narrative requires EU; supports our French invoicing flow |
| 3 | **Decision-maker is founder or direct commercial lead** | Avoid procurement / IT committee buying cycles — too slow, too feature-list-driven |
| 4 | **Currently using a worse tool OR no CRM** (spreadsheets, Notion, Airtable, Pipedrive Free, abandoned HubSpot Free, custom Google Sheets) | Migration from real HubSpot deployment too risky for beta — they have feature expectations we won't meet yet |
| 5 | **Sales process exists** (multi-stage pipeline, follow-up cadence, deal management) | Without a real process, CRM has no anchor; client can't articulate need; beta feedback is useless |
| 6 | **NOT dependent on Marketing Hub Pro features** (no marketing email automation, ad attribution, lead scoring, multi-touch attribution, behavioural nurture) | These are P3+ in our roadmap; if they need them now, refuse |
| 7 | **NOT dependent on real-time Shopify/WooCommerce/Stripe order sync** for their sales pipeline | E-commerce integration depth is the killer for beta — too many edge cases, too easy to break trust |
| 8 | **Tolerance for v0 rough edges** | Tech-comfortable founder, beta-tester mindset, asks good questions, doesn't expect feature-list parity |
| 9 | **Pays willingly** (€250-500 MRR + €2-4k setup acceptable) | Free clients become the worst clients; beta discount yes, free no |
| 10 | **Available for monthly feedback call** (30-45 min) | Co-design partnership, not just a customer relationship |

---

## Strong Positive Signals (Most Should Be Present)

| Signal | Why it's positive |
|---|---|
| Explicit data-sovereignty concern (recently spooked by US-based SaaS, GDPR sensitive industry) | Anchors the differentiation; not just price-driven |
| Recently rejected HubSpot on price | Lost-deal pipeline is the literal definition of our ICP |
| Founder has technical background or working with a technical advisor/freelancer | Lower friction during onboarding, fewer "why doesn't it do X?" surprises |
| Existing relationship with Leo (warm intro, not cold) | Trust transfer; faster to close |
| Ops are tracked in PostgreSQL-friendly tools already (Notion DB, Airtable, Google Sheets) | Migration is straightforward |
| Has a Google Workspace or Microsoft 365 account (not random Gmail) | OIDC SSO works on day 1; cleaner email sync |
| 1-3 sales pipelines (not 10+) | Configuration scope manageable in beta |
| Less than 5,000 contacts in current system | Migration complexity bounded |
| Industry is NOT food retail / fashion e-commerce / marketing agency | These verticals lean heavily on the features we don't ship yet |
| Customer-facing meetings happen via Google Meet / Ringover / Microsoft Teams (recordable) | Future AI-agent integration story is natural |
| Founder is bilingual (FR + EN) | Documentation and support trade-offs become flexible |

---

## Anti-Patterns — Refuse Even If They'd Pay

| Anti-pattern | Why it kills beta |
|---|---|
| **Heavy Shopify/WooCommerce dependency** (DTC brand, daily order sync expected) | Critical fragility; one breaking change in the connector damages trust |
| **Marketing-led organization** (CMO is the buyer, marketing email is the use case) | Wrong tool category; we're a sales CRM, not Marketing Hub |
| **Existing HubSpot Pro+ user wanting to "save money"** | They'll feel every gap; reference value is negative if they churn back |
| **Compliance-driven client requiring SOC 2 / ISO 27001 certification at signing** | We won't have these for 12+ months; honest no |
| **Enterprise-style RFP with feature-checklist evaluation** | Slow, feature-driven, will surface every gap; not beta-suitable |
| **Multi-stakeholder decision (committee, board sign-off needed)** | Decision cycle exceeds beta velocity; we won't iterate fast enough |
| **Industry with regulatory data-handling requirements we haven't researched** (healthcare, banking, defense) | Risk profile too high for solo-operator beta |
| **More than 30 active users** | Multi-tenant ops at that scale is post-Phase-2 |
| **Demands SLA stronger than 99% uptime** | Our infrastructure can do 99% comfortably; 99.9% is Phase-3 territory |
| **Wants white-label resell rights** | Different product entirely; refer to a separate conversation |
| **Wants to self-host on their own infrastructure** | Different product (consulting + license, not managed service); valid future product but not beta |

---

## Two Beta Tiers — Design Partner vs Paying Beta

The first 3-5 clients should be carefully tiered. Treat them differently in pricing, expectations, and SLA.

### Tier 1 — Design Partner (1-2 clients max)

- **Profile:** Highest engagement, lowest revenue. Founder is excited about the product itself, not just the price.
- **Pricing:** €0 setup + €99-149/month MRR for 6 months, then renegotiate at production rates. Locked in for 6 months minimum.
- **Expectations:** Monthly co-design call. Right to influence roadmap. Public testimonial expected after 90 days. Case study expected after 6 months.
- **What we get:** Reference logo. Product-market signal. Real-data testbed. Bug surface area we'd never hit alone.
- **Who they are:** Tech-comfortable, network-connected, vocal on LinkedIn or in their industry community.

### Tier 2 — Paying Beta (3-4 clients)

- **Profile:** Real customers who happen to be early. Want the value, accept the rough edges, pay normal prices for them.
- **Pricing:** €1,500-3,000 setup + €250-450/month MRR. Standard 30-day cancellation, no minimum term.
- **Expectations:** Quarterly feedback call. Monthly health-check email. Testimonial appreciated, not contractual.
- **What we get:** Real revenue. Production usage patterns. Validation that pricing holds. Willing referrals to similar clients.
- **Who they are:** Pragmatic founders who needed a CRM yesterday, found us through Leo, and chose value over polish.

---

## Sourcing — Where Beta Clients Come From (Through Leo)

Leo is the introducer, not the seller. His name does NOT appear on the contract or invoice. The contractual relationship is GB Consult ↔ client. Leo is paid via a separate referral agreement (per the existing Vernayo–GB Consult partnership structure: 15-20% of MRR, scaled to 30% for this product to reflect his higher-touch role).

### Sourcing channels Leo can work without exposure

| Channel | How Leo introduces | Exposure risk |
|---|---|---|
| **Lost-HubSpot-deal pipeline** (prospects who balked on price/sovereignty in the past 6 months) | Cold revival: "I came across an alternative that might fit your earlier concern. Want me to introduce you to the team behind it?" | Low — he's helping a prospect, not selling against HubSpot |
| **His personal/founder network** outside HubSpot client base | Direct: "I know someone whose situation sounds like yours, want to talk to my partner?" | None — no HubSpot context |
| **CRM-curious referrals from existing happy clients** (e.g., Mariana at ChefCheffe knows other restaurateurs/producers) | Indirect: existing client refers a peer, Leo brokers the introduction | Low — Leo isn't selling, just connecting |
| **French SMB founder communities** (Founders.fr, Bpifrance Hub, La French Tech locales) | Lurking + introducing peers when relevant conversations come up | None |
| **Past consulting prospects who never closed** (companies he scoped but didn't sign) | "Reaching out because we now offer a more affordable alternative — different from what we discussed before." | Low — clearly a different product |

### Sourcing channels Leo should NOT use

- His active HubSpot client roster (avoid cannibalization, avoid HubSpot partner conflicts)
- HubSpot Solutions Partner directory or HubSpot-branded forums (high visibility, partner agreement risk)
- Any forum/community where he is positioned as a HubSpot specialist (mixed message, brand dilution)

---

## Discovery Call Playbook (30 minutes)

Use this on every initial qualification call. Goal: bucket prospect into Yes / Maybe / No within 30 minutes.

### Opening (3 min)
- "Leo mentioned you might be looking at CRM options — happy to walk through what we've built and see if it fits."
- No HubSpot mention unless they bring it up.

### Discovery (15 min) — qualify against hard filters
1. "Tell me about your sales process today — how do you track deals, follow up, hand off between people?"
2. "How many people log into your current system regularly?"
3. "What's the biggest pain with what you have now?"
4. "Are you using marketing email at scale today, or is sales the main job-to-be-done?"
5. "Where does your data live now? Sheets, another CRM, custom?"
6. "What does 'success' look like in 6 months for how your team manages the pipeline?"
7. "Where do you sit on cost vs. control? Some people want a polished SaaS, some want their own data on infrastructure they control."
8. "What's your timing — looking to switch in weeks, months?"

### Beta framing (5 min) — set expectations explicitly
- "We're early. The product is real and works, but we're shipping new features every couple of weeks. Some things HubSpot has — for example native sales sequences with reply detection — we don't have yet, we're building them now."
- "What we offer in exchange: heavy customization for your specific use case, EU hosting, your data on infrastructure we operate end-to-end, no vendor lock-in, and a price that doesn't punish you per-seat."
- "If you'd be a great fit but need feature X today, I'll be honest and tell you to wait or stay where you are."

### Quick close decision (5 min)
- If clearly Yes: "Want me to send a 1-pager and a 30-day pilot proposal?"
- If clearly No: "Sounds like HubSpot or Pipedrive is still the right answer for you today — happy to recap why."
- If Maybe: "Let me think about whether we're the right fit and I'll come back to you in 48h."

### Bonus (2 min)
- "Who else in your network might be in a similar situation?"

---

## Three Illustrative Personas

### Persona A — "Marc, French Boutique Consultancy"
- 7-person strategy/M&A advisory firm in Paris
- 4 partners, 3 associates, all in the CRM
- Currently using Notion + Airtable + Google Sheets for deal flow
- Rejected HubSpot 8 months ago: €700+/month for the team felt absurd for a CRM
- Loves the data-sovereignty angle (their clients are sensitive about confidentiality)
- Tech-comfortable: one of the partners codes for fun
- Need: clean pipeline, multi-pipeline support (corporate finance / strategy / talent), email logging, basic reporting
- **Beta tier:** Design Partner. Will test, give feedback, refer peers.

### Persona B — "Anne, Régional Wine Estate Distributor"
- 12-person team distributing wines from 30+ small estates to French + Belgian on-trade and retail
- B2B sales, deal cycles 3-9 months, follow-up heavy
- Currently using a 2018-vintage Pipedrive deployment they're frustrated with
- Has a Shopify B2B site but it's separate from sales (transactions don't drive CRM logic)
- Cares about French infrastructure (sustainability narrative is part of brand)
- Need: pipeline + follow-up sequences + custom properties (estate, region, vintage, grape) + Cal.com-style meeting links
- Sequences are important but Reply.io API bridge is acceptable for v0
- **Beta tier:** Paying Beta. Pays normal price, expects it to work, will refer peers in the wine industry.

### Persona C — "Pierre, Greentech B2B Sales Lead"
- Series-A startup, 9 people, 4 in commercial team
- Sells decarbonization consulting to mid-market industrial clients (long deal cycles, 6-18 months)
- Currently has HubSpot Free, growing pains: 10-property limit hit, 2-pipeline limit hit, no automation
- Quoted €1,400/month for HubSpot Sales Pro + Marketing Starter; chokes on it
- Tech-savvy founder, runs internal tooling on Postgres/Supabase already
- Need: custom objects (clients, projects, milestones, deliverables), multi-pipeline, OIDC via Google Workspace, simple reporting
- Doesn't need marketing automation; uses Brevo separately for newsletters
- **Beta tier:** Paying Beta. Strong fit, vocal in his ecosystem if happy.

---

## Pricing Structure for Beta Phase

| Tier | Setup | MRR | Term | What's included |
|---|---|---|---|---|
| **Design Partner** | €0 | €99-149/mo | 6-month locked | Branded instance, OIDC SSO, basic ops, monthly co-design call, public testimonial expected at 90 days |
| **Paying Beta — Solo** | €1,500-2,000 | €250/mo | Standard 30-day | Same as Design Partner without the co-design intensity; 1 standard connector included |
| **Paying Beta — Team** | €2,500-3,500 | €350-450/mo | Standard 30-day | + 1 custom connector + advanced workflow setup + Cube.dev dashboards |

Custom development beyond beta scope (specific Shopify/Stripe/HelloHarel/Klaviyo connector, bespoke automation, white-label theming) is quoted separately at €600-800/day.

---

## Selection Criteria — When to Say Yes

After a discovery call, run this checklist. Yes only if at least 8/10:

- [ ] Passes all 10 hard filters
- [ ] At least 4 strong positive signals present
- [ ] No anti-patterns triggered
- [ ] Decision-maker engaged in the call (not delegated)
- [ ] Clear sales process described in their own words (not handwaved)
- [ ] Realistic timeline (not "we want it next week" nor "maybe next year")
- [ ] Willing to engage in monthly/quarterly feedback rhythm
- [ ] Pricing didn't trigger sticker shock
- [ ] Gut sense: would I enjoy a 6-month working relationship with this person?
- [ ] Strategic value beyond revenue (reference, network, learning)

If any of the bottom 3 fail (gut, strategic value, working relationship), refuse even if the top 7 pass. Beta is intimate work; the wrong client poisons the well for the next three.

---

## Phase Targets

- **Phase 1 goal:** 1 Design Partner signed within 6 weeks of Phase 0 completion
- **Phase 2 goal:** 3 paying clients (1 Design Partner + 2 Paying Beta) live within 12 weeks
- **Phase 3 trigger:** 5 paying clients sustaining €1,500+ MRR before considering a non-Leo sales channel

---

## Open Questions

1. Should "EU-based" be relaxed to "primarily Francophone" to include North African or West African Francophone SMBs in scope? (Reasonable expansion, but adds tax/billing complexity.) — Decision needed.
2. Do we accept clients who currently use HubSpot Free (not Pro) as beta candidates? Persona C is exactly this. Risk: they'll compare feature-by-feature with HubSpot. — Suggest yes, with explicit framing in discovery.
3. What's the floor on monthly MRR we'd accept for a Design Partner? €99 feels low; €149 feels more sustainable. — Decision needed.
4. Should we cap the total number of clients at 5 during Phase 1-2 to protect dev velocity? — Recommend yes; raise to 10 after Phase 3 trigger.

---

**End of archetype.**
