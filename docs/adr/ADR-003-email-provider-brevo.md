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

1. **Brevo inbound parse plan tier — non-blocking.** Pricing for `inboundEmailProcessed` webhook access is not publicly documented. Confirm with Brevo sales which plan unlocks it. **This is not a v0/v1 blocker.** Per [ADR-004 §3](ADR-004-sequences-architecture.md), inbound parse is the **secondary** reply-detection surface; the primary path (Gmail Pub/Sub + Microsoft Graph subscriptions on per-user OAuth) covers >95% of the EU SMB target market without it. The Brevo answer determines only how the catch-all "generic CRM reply-to" path is implemented:
   - **Best case** — inbound parse on Standard plan or below: ship as designed in ADR-004 §3.
   - **Bad case** — inbound parse Enterprise-only or volume-capped uneconomically: drop the secondary path entirely in v0/v1. Sequences ship with primary (Gmail/Graph OAuth) + IMAP IDLE fallback for non-mainstream mailboxes (rare in target market). The "generic CRM reply-to" use case is deferred. ~95% of SMB sequences happen from the rep's mailbox anyway, so this is a small UX gap not a feature loss.
   - **Bad case alternative** — if a client genuinely needs the catch-all path, three fallbacks in increasing operational cost: (a) self-hosted Postfix → LMTP → BullMQ webhook (~1 day setup, EU-residency by definition, free); (b) Mailjet inbound parse only as a single-feature second vendor (acceptable DPA cost, two-vendor split); (c) CloudMailin or similar EU-friendly inbound-only service (verify GDPR/DPF posture). All three are viable; Postfix is the default fallback because it has no third-party dependency.
   (`docs/research/email-deliverability.md` §470 item 2; addendum §A1.)
2. **Brevo per-tenant API key isolation.** Verify whether Brevo supports sub-accounts (or equivalent) so that one tenant's domain reputation issues do not affect others. If not, evaluate cost of per-tenant Brevo accounts and whether the operational complexity is worth it. (`docs/research/email-deliverability.md` §470 item 5)
3. **Live Outlook deliverability baseline.** During v0 with the first paying client, run weekly GlockApps inbox-placement tests targeting Outlook/Hotmail. Establish the actual delivery rate at our sender mix, not the headline benchmark. If <70% sustained over 30 days, accelerate Mailjet evaluation.
4. **Scaleway TEM Scale exact pricing.** €80/mo + 100k included was published in May 2026 but Scaleway has updated pricing in the past. Re-confirm at the moment we approach the >50k/mo threshold.
5. **Domain provisioning fallback for non-mainstream DNS providers.** Cloudflare/OVH/Gandi cover most of our French SMB clients but not all. Document a manual DNS-record copy-paste flow with a 4 h propagation window and a clear UI status indicator for clients on other DNS hosts. The 30-min onboarding target ([Scenario A in ARCHITECTURE.md §12](../ARCHITECTURE.md)) does not include DKIM verification — explicitly acknowledge this in the wizard.
6. **DMARC report ingestion (RUA/RUF).** DMARC `rua` and `ruf` aggregate reports are sent to `mailto:` addresses on the sending domain. Decide whether leCRM ingests them (set address to `dmarc-rua@<lecrm-host>` and parse) or delegates to client (default = client-managed). v0 default: client-managed; v2: optional ingestion as a deliverability-monitoring add-on.
