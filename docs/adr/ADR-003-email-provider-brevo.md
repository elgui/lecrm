# ADR-003 — Email Provider: Brevo (FR), Single Vendor Across Phases

**Status:** Accepted
**Date:** 2026-05-10
**Deciders:** Guillaume

---

## Context

leCRM sends three categories of email on behalf of clients:

1. **Transactional CRM notifications** (task assignments, mention notifications, password resets, sequence step sends, system alerts). Volume: low per client, but every client uses it.
2. **Outbound sequences** (v1+ native sequences engine — see [ADR-004](ADR-004-sequences-architecture.md)). Volume: spiky, dominated by sequence campaigns.
3. **Inbound parse for reply detection** (when clients use a generic `replies.<client-domain>` mailbox rather than per-user OAuth). The inbound webhook routes parsed emails into the sequences state machine.

Constraints:

- **EU data residency mandatory.** Postmark (US, no DPF cert) is excluded per `docs/LEGAL-PLAYBOOK.md`.
- **Solo operator simplicity.** A single vendor with one DPA, one set of keys, one webhook surface, one set of monitoring is materially less ops than a split-vendor architecture.
- **Per-client domain authentication** (DKIM / SPF / DMARC) must be automatable. Clients send from their own sending domain (e.g., `crm-mail.clientcorp.fr`).
- **Cost-per-client scaling.** Email layer ≤€80–120/mo across the phase 3 portfolio is the budget envelope.
- **Must support the v1 sequences architecture** which depends on a production-quality inbound parse webhook.

The candidate providers (research artefact `docs/research/email-deliverability.md` §1):

| Provider | EU residency | Inbound parse | Independent deliverability score |
|---|---|---|---|
| Brevo (Paris) | Yes (default) | Yes — `inboundEmailProcessed` webhook | 79.8% (notable Outlook/Hotmail weakness) |
| Mailjet (Sinch, EU DCs) | Yes | Yes — Parse API | 85.0% (strong Outlook) |
| Scaleway TEM (Paris) | Yes (SecNumCloud track) | **No** | Not benchmarked |

Each option has a real trade-off:
- Brevo has the cleanest single-vendor story for outbound + inbound + domain API but the weakest deliverability number, especially against Outlook/Hotmail (a major B2B mailbox among French SMBs).
- Mailjet wins on deliverability but its inbound parse API is less actively documented and the Sinch acquisition has slowed its Node.js SDK maintenance.
- Scaleway TEM has the strongest sovereign posture and the cheapest dedicated IP, but no inbound parse capability — meaning we'd need a second provider or a self-hosted IMAP gateway for replies, which doubles the operational surface.

---

## Decision

### Brevo is leCRM's single email provider through phases 1, 2, and 3.

Specifics:

- **Outbound transactional:** Brevo Transactional Email API (`POST /v3/smtp/email`). Node SDK: `@getbrevo/brevo` (v5).
- **Outbound sequences:** same API. Sequence-step sends carry the Brevo `messageId` for downstream reply correlation.
- **Inbound parse:** Brevo's `inboundEmailProcessed` webhook. MX delegation of `replies.<client-domain>` to `inbound1.sendinblue.com` (priority 10) and `inbound2.sendinblue.com` (priority 20). Parsed payload includes `InReplyTo`, `ExtractedMarkdownMessage`, `SpamScore`, full headers — exactly what the sequences state machine ([ADR-004](ADR-004-sequences-architecture.md)) needs for reply correlation.
- **Domain authentication:** Brevo's domain API (`PUT /v3/senders/domains/{domain}/authenticate`). leCRM's onboarding wizard automates the flow against Cloudflare / OVH / Gandi DNS APIs (or falls back to a copy-paste DNS record list for clients on other providers).
- **Webhook security:** HMAC signature verification on inbound webhooks; webhook signing secret rotated quarterly via Vault (v1+) or sops (v0).
- **Webhook event handling:** BullMQ queue `email-event`, retry policy exponential backoff with 3 attempts and a `email-event-dlq` dead-letter queue. Events: `delivered`, `hardBounce`, `softBounce`, `blocked`, `spam`, `unsubscribed`, `inboundEmailProcessed`.

### Plan tier and phase progression

| Phase | Clients | Volume estimate | Brevo plan | Approx €/mo |
|---|---|---|---|---|
| 1 | ≤4 | ≤10k/mo | Starter | ~€9–18 |
| 2 | ≤10 | 15–25k/mo | Standard | ~€25–35 |
| 3 | ≤20 | 35–60k/mo (peaks 100k+ during campaigns) | Business | ~€65–80 |

Numbers from `docs/research/email-deliverability.md` §8.

### Mitigations for the Brevo deliverability gap

The 79.8% vs Mailjet's 85.0% (5-percentage-point gap, mostly on Outlook/Hotmail) is the primary risk. Mitigations layered on top:

1. **DKIM/SPF/DMARC discipline per client.** No client sends without all three records validated. Brevo's domain API + leCRM's poll-and-verify loop blocks sequence enrollment until DKIM is verified. DMARC policy `p=quarantine` minimum (not `p=none`).
2. **Pre-flight content scoring.** GlockApps API integration on every sequence-template save. Score below 7/10 blocks activation (warning, not hard block — admin can override with explicit confirmation). Cost: $59–85/mo; sits in the email-layer budget. (`docs/research/email-deliverability.md` §5.)
3. **List hygiene as gating.** Pre-send check against the `email_suppression` table on every step. Hard bounces, complaints, unsubscribes never re-send. (`docs/research/email-deliverability.md` §7.) Bounce rate target: <2%. Complaint rate target: <0.08%.
4. **Per-client deliverability dashboard.** Brevo's webhook events feed a per-workspace dashboard showing delivery rate, bounce rate, complaint rate. If a client's complaint rate exceeds 0.1% over a rolling 7 days, sequences are auto-paused and admin is notified.
5. **Active monitoring against Outlook.** Monthly GlockApps inbox-placement test specifically targeting Outlook/Hotmail. If Outlook delivery rate degrades >10 points sustained over 30 days, escalate to migration plan (see Consequences below).

### Dedicated IP — deferred

Brevo's dedicated IP is **$251/year** but requires the **Professional tier (~$499+/mo)**, which is prohibitive for our volume. Per industry consensus, dedicated IP doesn't pay back below 50–100k/mo sustained. Phase 1–3 volumes don't justify it on Brevo. (`docs/research/email-deliverability.md` §3.)

If sustained volume crosses 50k/mo and shared-IP reputation issues materialize, the most cost-effective dedicated-IP path is **Scaleway TEM Scale plan** at €80/mo (all-in: 100k emails included + dedicated IP + automated warm-up). At that point the architecture splits: Scaleway for outbound sequences (the spiky/high-volume traffic), Brevo retained for transactional + inbound parse. This is a **deferred decision**, not a v1 commitment.

### Per-tenant Brevo isolation

Brevo's account model: a single Brevo account, multiple sender domains (one per client). Each leCRM tenant has its own sender domain configured in Brevo, but the API key and account-level reputation are shared. **TO RESOLVE:** verify whether one client's domain reputation issues can damage other clients' deliverability through shared-account reputation effects, and whether Brevo offers sub-accounts to mitigate this.

---

## Consequences

### Positive

- **One vendor, one DPA, one webhook surface.** Brevo's DPA is auto-included with every account (`docs/research/email-deliverability.md` §1.6). Sub-processor list is one entry.
- **Inbound parse is production-quality.** Brevo is the only candidate with a documented `inboundEmailProcessed` webhook including `InReplyTo` and `SpamScore`. This unblocks the v1 sequences architecture without a second vendor.
- **Domain authentication API.** `PUT /v3/senders/domains/{domain}/authenticate` enables the automated onboarding wizard. Provider is the right level of automation for leCRM's solo-operator constraint.
- **Mature Node SDK.** `@getbrevo/brevo` v5 is actively maintained and integrates cleanly with NestJS providers.
- **GDPR posture is clean.** Paris HQ, EU-default data, included DPA, no DPF certification debate.

### Negative

- **5-point deliverability gap vs Mailjet,** especially against Outlook/Hotmail, which is a significant share of B2B mailboxes among French SMBs. Mitigated by DKIM/SPF/DMARC discipline and pre-flight scoring (§Mitigations) but is the primary residual risk.
- **No dedicated IP at our price point.** Shared-IP reputation is at the mercy of other Brevo customers. If neighbour reputation degrades, we feel it. Mitigation is the migration plan to Scaleway Scale at >50k/mo.
- **Sequence campaigns can spike volume past plan tier.** A Business-tier client mid-month running a 100k-contact campaign overflows the included allowance and incurs overages. Pricing flow handles this transparently but is a budget surprise risk. Add per-tenant volume caps in the sequences engine to prevent runaway sends.
- **Account-level reputation coupling.** All clients share the Brevo account reputation. One client's bad list hygiene can affect others' delivery. Defended by the auto-pause-on-complaint-spike rule (§Mitigations item 4) but TO RESOLVE flagged.

### Neutral

- Mailjet remains a viable fallback if Brevo Outlook deliverability degrades materially in production. The migration cost is bounded: rewrite the outbound API client (a small NestJS service) and update DNS records. Keep `EmailProvider` as an interface in the email service so the migration is a swap, not a rewrite.
- Scaleway TEM remains a deferred upgrade path for the high-volume tier (>50k/mo).

---

## Alternatives Considered

### Alt 1: Mailjet primary

Tempting on deliverability (5-point lead). Rejected because:
- The Sinch acquisition introduces a Sinch-DPA layer (extra GDPR moving part).
- Inbound parse exists but is less actively documented than Brevo's; production patterns in the SMB SaaS space lean on Brevo for this.
- Mailjet's free tier hard cap (200 emails/day) is unhelpful for our staging/dev environments.
- Node.js SDK is less actively maintained post-Sinch.

If real-world Brevo deliverability fails to hit 75%+ at our sender mix, this becomes the migration target.

### Alt 2: Scaleway TEM primary

Strongest sovereign posture (Paris-only, SecNumCloud in progress) and the cheapest dedicated IP at €80/mo. Rejected because:
- **No inbound parse API.** Reply detection would require a second vendor (Brevo for inbound) or a self-hosted IMAP gateway. Doubles the ops surface and contradicts the single-vendor goal.
- **No independent deliverability benchmark.** EmailToolTester / GlockApps lack data. Committing to Scaleway as primary is committing to a deliverability unknown.
- **SDK maturity** is lower than Brevo's; we'd own more glue code.

Scaleway re-enters at >50k/mo as the dedicated-IP outbound provider, retaining Brevo for inbound parse. The right shape for that future split is in §Decision.

### Alt 3: Postmark

Excluded per binding constraint (`docs/LEGAL-PLAYBOOK.md` §2 — US sub-processor, no DPF cert). Even if DPF certification arrives later, the trust posture vs an EU-only provider is harder to defend in a French SMB DPA conversation.

### Alt 4: AWS SES + custom inbound (S3 + Lambda + IMAP gateway)

Rejected. AWS introduces US-jurisdiction sub-processor questions (DPF can be argued, but is harder than a French provider). The DIY operational cost (managing inbound on S3+SES, building reply-correlation glue) consumes the time we'd otherwise spend on product. Solo operator constraint dominates.

### Alt 5: Self-hosted (Postal, Listmonk + their own SMTP)

Rejected. IP reputation from a fresh self-hosted sender is a 4–8 week warm-up project with no managed assistance. Unsuitable for a CRM where deliverability is a customer expectation, not a stretch goal.

---

## References

- `docs/research/email-deliverability.md` (entire document; §1 provider comparison, §2 onboarding, §3 dedicated IP, §5 pre-flight scoring, §7 list hygiene, §8 volume planning).
- `docs/LEGAL-PLAYBOOK.md` §2 (Postmark exclusion rationale, US sub-processor flag).
- [Brevo transactional webhooks](https://developers.brevo.com/docs/transactional-webhooks).
- [Brevo inbound parse webhooks](https://developers.brevo.com/docs/inbound-parse-webhooks).
- [Brevo domain authentication API](https://developers.brevo.com/docs/domain-authentication-and-verification).
- [`@getbrevo/brevo` npm](https://www.npmjs.com/package/@getbrevo/brevo).
- [Brevo DPA](https://help.brevo.com/hc/en-us/articles/15403782599570).
- [Scaleway TEM](https://www.scaleway.com/en/transactional-email-tem/) (deferred-upgrade reference).
- [EmailToolTester transactional benchmark](https://www.emailtooltester.com/en/blog/best-transactional-email-service/).
- [GlockApps](https://glockapps.com/) (pre-flight scoring tool).
- Related ADRs: [ADR-004](ADR-004-sequences-architecture.md) (sequences engine consumes Brevo outbound + inbound), [ADR-007](ADR-007-encryption-secrets-audit.md) (Brevo API key + DKIM private key per tenant in secrets management).

---

## TO RESOLVE

1. **Brevo inbound parse plan tier — non-blocking. `[RESOLVED 2026-06-14 — see Addendum A2026-06-14 below. Brevo inbound parse IS Professional-tier-gated (~€500/mo, confirmed by account owner) — uneconomic for a secondary path. v1 ships Gmail-only; generic reply@ catch-all deferred, provider undecided.]`** Pricing for `inboundEmailProcessed` webhook access is not publicly documented. Confirm with Brevo sales which plan unlocks it. **This is not a v0/v1 blocker.** Per [ADR-004 §3](ADR-004-sequences-architecture.md), inbound parse is the **secondary** reply-detection surface; the primary path (Gmail Pub/Sub + Microsoft Graph subscriptions on per-user OAuth) covers >95% of the EU SMB target market without it. The Brevo answer determines only how the catch-all "generic CRM reply-to" path is implemented:
   - **Best case** — inbound parse on Standard plan or below: ship as designed in ADR-004 §3.
   - **Bad case** — inbound parse Enterprise-only or volume-capped uneconomically: drop the secondary path entirely in v0/v1. Sequences ship with primary (Gmail/Graph OAuth) + IMAP IDLE fallback for non-mainstream mailboxes (rare in target market). The "generic CRM reply-to" use case is deferred. ~95% of SMB sequences happen from the rep's mailbox anyway, so this is a small UX gap not a feature loss.
   - **Bad case alternative** — if a client genuinely needs the catch-all path, three fallbacks in increasing operational cost: (a) self-hosted Postfix → LMTP → BullMQ webhook (~1 day setup, EU-residency by definition, free); (b) Mailjet inbound parse only as a single-feature second vendor (acceptable DPA cost, two-vendor split); (c) CloudMailin or similar EU-friendly inbound-only service (verify GDPR/DPF posture). All three are viable; Postfix is the default fallback because it has no third-party dependency.
   (`docs/research/email-deliverability.md` §470 item 2; addendum §A1.)
2. **Brevo per-tenant API key isolation.** Verify whether Brevo supports sub-accounts (or equivalent) so that one tenant's domain reputation issues do not affect others. If not, evaluate cost of per-tenant Brevo accounts and whether the operational complexity is worth it. (`docs/research/email-deliverability.md` §470 item 5)
3. **Live Outlook deliverability baseline.** During v0 with the first paying client, run weekly GlockApps inbox-placement tests targeting Outlook/Hotmail. Establish the actual delivery rate at our sender mix, not the headline benchmark. If <70% sustained over 30 days, accelerate Mailjet evaluation.
4. **Scaleway TEM Scale exact pricing.** €80/mo + 100k included was published in May 2026 but Scaleway has updated pricing in the past. Re-confirm at the moment we approach the >50k/mo threshold.
5. **Domain provisioning fallback for non-mainstream DNS providers.** Cloudflare/OVH/Gandi cover most of our French SMB clients but not all. Document a manual DNS-record copy-paste flow with a 4 h propagation window and a clear UI status indicator for clients on other DNS hosts. The 30-min onboarding target ([Scenario A in ARCHITECTURE.md §12](../ARCHITECTURE.md)) does not include DKIM verification — explicitly acknowledge this in the wizard.
6. **DMARC report ingestion (RUA/RUF).** DMARC `rua` and `ruf` aggregate reports are sent to `mailto:` addresses on the sending domain. Decide whether leCRM ingests them (set address to `dmarc-rua@<lecrm-host>` and parse) or delegates to client (default = client-managed). v0 default: client-managed; v2: optional ingestion as a deliverability-monitoring add-on.

---

## Addendum A2026-06-14 — Inbound parse plan tier resolved (closes TO RESOLVE item 1)

**Author:** Guia (automated research pass, tasket `20260528-142628-2702`); **corrected 2026-06-14**
after account-owner verification (see Correction note).
**Status:** Accepted — **Option D: Gmail-only reply detection at v1; generic `reply@` catch-all
deferred, provider undecided** (Guillaume, 2026-06-14).
**Why now:** v1 sequences dev (`apps/api/internal/email/brevo/inbound.go`, not yet written) needs a
confirmed inbound-reply input before code lands. The first-paying-client gate was dropped on
2026-06-14, so this ran as standalone self-research (no external-collaborator wait).

### Headline & correction

**Brevo inbound parse is Professional-tier-gated (~€500/mo)** — confirmed by Guillaume from the live
account (2026-06-14). This is the ADR-004 rev 2 §Q4 / TO RESOLVE-1 **"bad case."** €500/mo is ~4–6×
the *entire* email-layer budget envelope (≤€80–120/mo across the phase-3 portfolio, §Context) and
would buy only the **secondary** catch-all reply path. **Decision: don't pay for it.** v1 ships
**Gmail-only** reply detection (covers ~95% of SMB sequences, sent from the rep's own mailbox —
ADR-004 rev 2 §4). The generic `reply@<client-domain>` catch-all is **deferred**, provider
undecided; a costed comparison is recorded in Finding 3 for when that decision is taken.

> **Correction note.** An earlier revision of this addendum concluded "no tier gate, €0, Option B,"
> inferred from the *absence* of a gating note in Brevo's developer docs. That was wrong — dev-doc
> silence is not "free." The two sources that would have caught it (Brevo's help-center inbound
> article; the pricing page) were not machine-readable this pass (403 / JS-rendered), which was
> flagged as the residual caveat — and it resolved against the inference. The account owner's
> dashboard visibility is authoritative and supersedes it.

### Evidence

```
Current plan:        Free (Guillaume, 2026-06-14)
Renewal date:        N/A — Free plan is free-forever; no paid renewal date.
Required plan:       Professional (~€500/mo) to use inbound parse / inbound webhooks.
                     CONFIRMED by Guillaume from the live Brevo account, 2026-06-14 — this
                     SUPERSEDES the earlier doc-inference of "no gate". Corroborating list
                     price: Brevo Professional ≈ $499/mo (2026 review sources).
Monthly cost delta:  ~€500/mo to enable inbound on Brevo → REJECTED (≈4–6× the whole email-layer
                     budget of ≤€80–120/mo, §Context, spent on a secondary path).
Upgrade trigger:     N/A for inbound — not taken. v1 reply detection = Gmail-only.
Catch-all decision:  DEFERRED — build a generic reply@ path only when a paying client needs it;
                     provider undecided (costed comparison in Finding 3).
Inbound payload doc: https://developers.brevo.com/docs/inbound-parse-webhooks
                     (Brevo; reference only — Brevo inbound is NOT on the v1 path)
Guillaume decision:  Option D (Gmail-only at v1; catch-all deferred, provider undecided) —
                     ACCEPTED 2026-06-14. Also requested: cost the paid alternatives now (done).
```

### Finding 1 — Brevo inbound parse is Professional-gated (~€500/mo), so it's not used

- **Confirmed by the account owner** (Guillaume, dashboard, 2026-06-14): inbound webhooks / inbound
  parse require a **Professional** subscription (~€500/mo). Corroborating list price: Brevo
  Professional ≈ **$499/mo** ([Costbench — Brevo pricing 2026](https://costbench.com/software/marketing-automation/brevo/)).
- Brevo's [inbound parse docs](https://developers.brevo.com/docs/inbound-parse-webhooks) document the
  feature, MX delegation and payload but **do not state the tier requirement** — which is exactly why
  the gate was invisible from docs alone (the original mistake).
- **Why we don't pay it:** inbound parse is the **secondary** reply path; the primary Gmail path
  (per-user OAuth) already covers ~95% of SMB sequences (ADR-004 rev 2 §4 / §Q4). €500/mo for the
  remaining-~5% catch-all is ~4–6× the entire email-layer budget (≤€80–120/mo, §Context).
  Economically a non-starter.

### Finding 2 — v1 decision: Gmail-only, catch-all deferred (Option D)

v1 sequences ship with **Gmail Pub/Sub reply detection only** (ADR-004 rev 2 §4; Gmail-first per
ADR-009 §9). No `inbound.go` against Brevo at v1. The generic `reply@<client-domain>` catch-all is
**deferred** until a paying client genuinely needs it, and the provider for it is **left undecided**
(Guillaume's explicit choice — don't pre-commit, not even to Postfix). The costed comparison in
Finding 3 is the ready input for that future decision.

### Finding 3 — Costed comparison of catch-all options (for the deferred decision)

When the generic `reply@` catch-all is eventually needed, these are the realistic providers. Brevo
Professional is shown only as the rejected reference. Pricing triangulated from 2026 sources;
re-confirm at adoption time (§Source hygiene).

| Option | Recurring cost | EU / GDPR posture | Effort & ops | New sub-processor? |
|---|---|---|---|---|
| **Postfix self-host → river webhook** | **€0** (runs on existing EU VPS, e.g. Netcup DE) | Strongest — you pick server location (EU by construction); no third party, no extra DPA | ~1 day build, then you own the MTA (spam filtering, MIME parse) | No |
| **Mailjet Parse API** | **~$17/mo** (Essential, cheapest paid; "Crystal and above" is legacy naming — free tier excluded) | EU data centres (Sinch); adds a Sinch DPA sub-processor | Low — managed API, basic-auth + HTTPS webhook | Yes (Mailjet/Sinch) |
| **CloudMailin** | **$0** ≤10k/mo → $25 Starter (10k) → $45 Pro (20k) → $85 Premium (40k) | Weaker by default — shared clusters are "US **and/or** Europe"; guaranteed EU needs a Dedicated server (from ~$1,499/mo) or a confirmed EU cluster + DPA. UK firm (UK has GDPR adequacy). | Low — managed API | Yes (CloudMailin) |
| Brevo Professional *(rejected ref)* | **~€500/mo** | EU-default (Paris); no new vendor | Low — already integrated | No |

Sources: [Mailjet Parse API guide](https://dev.mailjet.com/email/guides/parse-api/) (gating: paid
plans, "Crystal and above"), [Mailjet pricing](https://www.emailsoftwareinsights.com/reviews/mailjet/pricing/)
(Free $0 / Essential from $17 / Premium from $27), [CloudMailin pricing](https://www.cloudmailin.com/plans),
[CloudMailin inbound](https://www.cloudmailin.com/inbound) ("default clusters operate in the US
and/or Europe; dedicated servers … any region").

**Non-binding read (decision is deferred):**
- **Mailjet Parse API (~$17/mo)** — cheapest *managed* EU-acceptable option, ~30× cheaper than Brevo
  Pro; leCRM already keeps Mailjet as the deliverability fallback (Alt 1), so the vendor/DPA may
  exist anyway. Strong default if a managed path is wanted.
- **Postfix self-host (€0)** — best sovereignty/cost if avoiding any new sub-processor outweighs
  running an MTA.
- **CloudMailin** — most generous free tier (10k/mo) but its default "US and/or Europe" shared
  clusters are the weakest fit for the EU-residency-mandatory constraint (§Context) unless an
  EU-only cluster + DPA is contractually confirmed; discount it otherwise.

**Reference — Brevo inbound payload shape (if Brevo inbound is ever reconsidered).** The Brevo
payload *does* satisfy the ADR-004 rev 2 `brevoInboundEvent` struct (`From`, `To`, `InReplyTo`,
`MessageId`, `ExtractedMarkdownMessage`, `SpamScore`, full `Headers` all present), with three
implementation gotchas: the envelope is an `items[]` array (not one object — contrast the outbound
webhook at `apps/api/internal/email/brevo/webhook.go:38`); `From`/`To` are `Mailbox{Address,Name}`
objects, not strings; and `SpamScore` may be nested as `Spam.Score`. Source:
[inbound parse webhooks](https://developers.brevo.com/docs/inbound-parse-webhooks). Not on the v1 path.

### Decision matrix (final)

| Option | Cost delta | Status |
|---|---|---|
| Use Brevo inbound (requires Professional) | ~€500/mo | **Rejected** — uneconomic for a secondary path |
| **D — Gmail-only at v1; catch-all deferred, provider undecided** | **€0** | **ACCEPTED** (Guillaume, 2026-06-14) |
| Build a non-Brevo catch-all (Postfix / Mailjet / CloudMailin) | €0–~$17/mo (Finding 3) | Deferred — pick from Finding 3 when a client needs it |

### Impact on this ADR and others

- **ADR-003 §Decision is PARTIALLY SUPERSEDED for the inbound role.** The "Inbound parse: Brevo's
  `inboundEmailProcessed` webhook … MX delegation to `inbound*.sendinblue.com`" decision bullet and
  the "inbound parse is production-quality / unblocks v1" Consequence no longer hold at the chosen
  budget — Brevo inbound is Pro-gated and **not used**. Brevo remains the **outbound transactional +
  sequences** provider (unchanged); only its **inbound** role is dropped. Fold this into an ADR-003
  rev 2 if the catch-all is ever rebuilt on a provider.
- **ADR-004 rev 2 §Q4 / TO RESOLVE-S2:** resolved — inbound parse is Pro-only/uneconomic ⇒ catch-all
  deferred, v1 Gmail-only. The `inbound.go` sketch in ADR-004 rev 2 §4 is **not built at v1**; if a
  catch-all is added later, its parse layer follows the chosen provider (Finding 3), not Brevo.
  Record the deferral in `docs/STRATEGIC-OVERVIEW.md` per S2.
- **ADR-003 §Decision (plan progression):** still volume-driven and otherwise unchanged for outbound.
