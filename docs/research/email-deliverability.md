# Email Deliverability Research — leCRM

**Date:** 2026-05-10
**Scope:** Provider selection, sequences architecture, bounce handling, reply detection for leCRM (Twenty CRM fork, NestJS stack, EU data residency mandatory)
**Context:** v0 bridges to Reply.io for sequences; v1 (weeks 5-14) builds native sequences with reply detection.

---

## 1. Provider Deep Comparison: Brevo vs Scaleway TEM vs Mailjet

### 1.1 Pricing Tiers

All three are EU-resident providers. Postmark excluded per constraint (no DPF cert, US-only infra).

#### Brevo (Paris, France)

Source: [Brevo Pricing](https://www.brevo.com/pricing/) | [EmailToolTester analysis](https://www.emailtooltester.com/en/reviews/brevo/pricing/)

| Volume/month | Plan | Approx. Price |
|---|---|---|
| 5k emails | Starter (entry) | ~$9/mo |
| 20k emails | Starter (mid) | ~$18/mo |
| 50k emails | Starter (upper) | ~$25–35/mo (estimate) |
| 100k emails | Business/Standard | ~$65–80/mo (estimate) |
| 150k emails | Professional | starts ~$499/mo |

**Caveat:** Brevo's published tiers jump from Standard (~100k) to Professional (150k+) at a steep price point. The $499 Professional floor for 150k is a known pain point flagged by [EmailVendorSelection](https://www.emailvendorselection.com/brevo-pricing/). For volume in the 50k–150k range, Brevo is competitive; above that, costs escalate sharply unless you qualify for Enterprise negotiation.

Transactional emails and marketing emails share the same monthly allowance. Brevo's model charges by email volume, not contacts (unlimited contacts on most plans).

**Dedicated IP:** $251/year add-on, available only on Professional and Enterprise plans. Minimum recommended volume: Brevo itself says 3 campaigns/week to 3,000 subscribers, i.e., ~36k/month. Industry consensus is 50k–100k/month for dedicated IP to make sense. [Source: Brevo Help](https://help.brevo.com/hc/en-us/articles/208835449-Introduction-to-dedicated-IPs)

#### Scaleway TEM (Paris, France)

Source: [Scaleway TEM page](https://www.scaleway.com/en/transactional-email-tem/) | [Pricing managed services](https://www.scaleway.com/en/pricing/managed-services/)

| Volume/month | Plan | Price |
|---|---|---|
| 300 emails/month | Essential (free tier) | €0 |
| 5k emails | Essential (pay-as-you-go) | ~€1.18 (5k × €0.25/1k — first 300 free) |
| 50k emails | Essential | ~€12.43 |
| 100k emails | Scale plan | €80/mo (100k included, dedicated IP included) |
| 150k emails | Scale plan | €80 + 50k × €0.20/1k = €90/mo |

The Essential plan is pure pay-as-you-go at €0.25/1,000 emails with no monthly fee. The Scale plan at €80/month includes 100k emails + dedicated IP + managed IP warm-up + 99.9% SLA. Overage on Scale is €0.20/1,000.

**Dedicated IP:** Included in Scale plan (€80/mo). Managed warm-up, monitoring, alerting, and corrective actions are included. This is the most cost-effective dedicated IP option in the comparison. [Source: Scaleway TEM docs](https://www.scaleway.com/en/docs/transactional-email/reference-content/tem-dedicated-ip/)

**Note on Essential plan caps:** Scaleway TEM has a capability limit on the Essential plan (max emails/hour, max domains). Check [capabilities and limits doc](https://www.scaleway.com/en/docs/transactional-email/reference-content/tem-capabilities-and-limits/) before relying on Essential at scale.

#### Mailjet (Sinch, originally Paris)

Source: [Mailjet Pricing](https://www.mailjet.com/pricing/) | [Sender.net analysis](https://www.sender.net/reviews/mailjet/pricing/)

| Volume/month | Plan | Approx. Price |
|---|---|---|
| 6k emails | Free | €0 (200 emails/day cap) |
| 15k emails | Essential | ~$17/mo |
| 50k emails | Essential/Premium | ~$35–55/mo (estimate) |
| 100k emails | Premium | dedicated IP included |
| 150k+ emails | Custom (Enterprise) | negotiated |

Mailjet charges by email volume, not contacts (unlimited contacts on all plans). Dedicated IP is included at no extra charge on Premium 100k+ plans. Below 100k, no dedicated IP option. For Enterprise (500k+), dedicated account management and multiple IPs available via Custom plan. [Source: Sender.net Mailjet pricing](https://www.sender.net/reviews/mailjet/pricing/)

**Key gap:** Mailjet's free tier has a hard 200-email/day cap (queued 3 days then deleted). Not suitable for any production transactional use case.

### 1.2 Deliverability Quality

Source: [EmailToolTester transactional email benchmark](https://www.emailtooltester.com/en/blog/best-transactional-email-service/) — GlockApps-based testing across Gmail, Outlook, Yahoo, AOL

| Provider | Avg Deliverability | Notes |
|---|---|---|
| **Mailjet** | **85.0%** | Ranked 3rd overall; strong Outlook performance (92% in rounds 2 and 4) |
| **Brevo** | **79.8%** | "Weaker than average"; notable Hotmail/Outlook issues; round 2 dipped to 72% |
| **Scaleway TEM** | Not tested | Absent from EmailToolTester benchmark; insufficient independent data |

Brevo's Outlook/Hotmail weakness is a meaningful concern for B2B sequences targeting French/EU SMBs, where Outlook is dominant. Mailjet's stronger Outlook performance is a genuine deliverability advantage. [Source: Brevo deliverability analysis](https://www.emailtooltester.com/en/blog/brevo-deliverability/)

Scaleway TEM lacks published independent deliverability benchmarks — this is a significant TO RESOLVE item for any production commitment.

### 1.3 Webhook Quality and Reply Tracking

#### Brevo

Full transactional webhook event set: `sent`, `delivered`, `hardBounce`, `softBounce`, `blocked`, `spam`, `invalid`, `deferred`, `click`, `opened`, `uniqueOpened`, `unsubscribed`. [Source: Brevo transactional webhooks](https://developers.brevo.com/docs/transactional-webhooks)

**Inbound / reply parsing:** Brevo exposes a dedicated inbound parse webhook (`inboundEmailProcessed`). Requires MX delegation of a receiving subdomain to `inbound1.sendinblue.com` (priority 10) and `inbound2.sendinblue.com` (priority 20). Parsed payload includes `InReplyTo` header (links reply to original message ID), `ExtractedMarkdownMessage`, `SpamScore`, full headers. This is exactly what the native v1 sequences engine needs for reply detection. [Source: Brevo inbound parse docs](https://developers.brevo.com/docs/inbound-parse-webhooks)

**Bounce code granularity:** Webhooks carry the raw SMTP response. Hard bounces are auto-suppressed. Soft bounces are distinguished. Complaint events fire on spam reports.

#### Scaleway TEM

Webhook events: `email_queued`, `email_sent`, `email_delivered`, `email_dropped` (hard bounce), `email_mailbox_not_found`, `email_spam`. [Source: Scaleway webhook events](https://www.scaleway.com/en/docs/transactional-email/reference-content/webhook-events-payloads/)

Webhooks are delivered via Scaleway's Topics and Events (SNS-like) infrastructure. Event payloads include `email_response_code` and `email_response_message` (raw SMTP detail).

**Critical gap:** Scaleway TEM has no inbound/reply parsing capability. For native sequence reply detection (v1), Scaleway TEM cannot serve as the inbound gateway — a separate inbound SMTP/IMAP approach would be needed. This is a significant architecture constraint.

#### Mailjet

Events: `sent`, `open`, `click`, `bounce` (with `hard_bounce` boolean + raw SMTP comment), `blocked`, `spam`, `unsub`. [Source: Mailjet Event API docs](https://dev.mailjet.com/email/guides/webhooks/)

**Inbound parse:** Mailjet's Parse API processes inbound email and can route to your webhook. Supports Reply-To detection patterns. [Source: Mailjet Parse API](https://dev.mailjet.com/email/guides/parse-api/)

Bounce webhook includes the raw SMTP error comment for granular diagnosis.

### 1.4 Dedicated IP — Summary

| Provider | Dedicated IP Cost | Minimum Plan Required | Managed Warm-up |
|---|---|---|---|
| Brevo | $251/year | Professional (~$499+/mo) | Manual (dashboard guidance) |
| Scaleway TEM | Included in Scale plan | Scale plan (€80/mo) | Yes — automatic |
| Mailjet | Included on Premium 100k+ | Premium 100k | Manual |

Scaleway's Scale plan is the best price/feature ratio for a dedicated IP: €80/month all-in with automatic warm-up management. Brevo's dedicated IP requires a Professional plan that starts at $499/month — unacceptable at phase 2 volume.

### 1.5 DKIM/SPF/DMARC Management

#### Brevo

REST API endpoints for domain management: `GET /v3/senders/domains/{domainName}` to validate, `PUT /v3/senders/domains/{domainName}/authenticate` to authenticate. [Source: Brevo domain authentication API](https://developers.brevo.com/docs/domain-authentication-and-verification)

Brevo can auto-configure DNS if you grant access to your DNS provider. Per-domain configuration is supported; each sender domain must be individually authenticated. DKIM CNAME records are provided by the dashboard. SPF: add `include:spf.brevo.com` to existing SPF record.

For multi-tenant leCRM, API-driven domain onboarding is feasible. The DNS automation (via Cloudflare/OVH/Gandi API) must be built on the leCRM side, layered on top of Brevo's domain API.

#### Scaleway TEM

Domain authentication via API is supported (Scaleway has a full REST API + Terraform provider). [Source: Scaleway TEM API](https://www.scaleway.com/en/developers/api/transactional-email/)

#### Mailjet

Sender domain management available via API. Similar CNAME-based DKIM setup.

### 1.6 GDPR Posture

| Provider | Headquarters | Data Residency | DPA |
|---|---|---|---|
| **Brevo** | Paris, France | EU default; data never leaves EU for standard accounts | Included automatically with every account [Source](https://help.brevo.com/hc/en-us/articles/15403782599570-Where-can-I-find-the-Data-Processing-Agreement-DPA) |
| **Scaleway TEM** | Paris, France | All data in French data centers; SecNumCloud qualification in progress (Jan 2025); DPA v2024 published | [DPA June 2024](https://www-uploads.scaleway.com/DPA_2024_ENG_b0abb5cc26.pdf) |
| **Mailjet** | Paris (Sinch Group, SE) | EU data centers (Belgium, Germany); first GDPR AFNOR certification globally; DPA via Sinch | [Sinch DPA and sub-processors](https://sinch.com/legal/data-protection-agreement-sub-processors/) |

All three meet the EU data residency requirement. Scaleway has the strongest sovereign posture (pursuing SecNumCloud, purely French/EU infra). Mailjet's parent is Swedish (Sinch), which introduces a Sinch DPA layer but still EU data centers.

### 1.7 SDK/API Maturity for Node.js/NestJS

#### Brevo

Official npm package: `@getbrevo/brevo` ([npm](https://www.npmjs.com/package/@getbrevo/brevo)). Currently on v5 (modern unified client). v3.x receives only security patches. NestJS integration is straightforward — create a provider service injecting the Brevo client. [Source: Brevo Node.js SDK docs](https://developers.brevo.com/docs/api-clients/node-js)

#### Scaleway TEM

Scaleway provides a REST API + CLI. No dedicated Node.js SDK for TEM specifically, but the REST API is standard JSON and usable with any HTTP client (`axios`, `node-fetch`). Terraform provider exists for infrastructure-as-code domain setup. SDK maturity is lower than Brevo.

#### Mailjet

Official Node.js package: `node-mailjet` on npm. REST API with OpenAPI spec. Webhooks and Parse API available. Less actively maintained than Brevo's SDK; the Sinch acquisition has slowed independent Mailjet SDK development.

---

## 2. Per-Client Domain Authentication Onboarding

### DNS Records Required Per Client

Each paying client sends from their own domain (e.g., `crm-mail.clientcorp.fr`). Required records:

```
# SPF (TXT record on sending domain or subdomain)
v=spf1 include:spf.brevo.com ~all

# DKIM (two CNAMEs — Brevo provides the values via API)
mail._domainkey.clientcorp.fr    CNAME    [brevo-dkim-value-1]
mail2._domainkey.clientcorp.fr   CNAME    [brevo-dkim-value-2]

# DMARC (TXT record)
_dmarc.clientcorp.fr   TXT   "v=DMARC1; p=quarantine; rua=mailto:dmarc-rua@clientcorp.fr; ruf=mailto:dmarc-ruf@clientcorp.fr; fo=1"

# For inbound reply parsing (Brevo, if used)
replies.clientcorp.fr   MX 10   inbound1.sendinblue.com.
replies.clientcorp.fr   MX 20   inbound2.sendinblue.com.
```

### DNS Automation via Provider APIs

Cloudflare, OVH, and Gandi all expose DNS management REST APIs. A realistic automation flow:

1. Client completes leCRM onboarding wizard, entering their domain name.
2. leCRM backend calls Brevo API (`POST /v3/senders/domains`) to register the domain and retrieve DKIM CNAME values.
3. If client uses Cloudflare: leCRM calls Cloudflare API (`POST /zones/{id}/dns_records`) to create each record. Same pattern for OVH (`POST /domain/zone/{zoneName}/record`) or Gandi Live DNS API.
4. leCRM polls Brevo's `GET /v3/senders/domains/{domain}` until `dkim_verified: true` (DNS propagation typically 15 min–4 hours; poll every 5 min with BullMQ job).
5. Webhook fires to leCRM when domain is verified; client receives in-app notification.

**Realistic 30-minute onboarding script (manual DNS path):**

- Minutes 0-5: Client enters domain in wizard. leCRM displays 4 DNS records to copy.
- Minutes 5-20: Client adds records in their DNS panel (Cloudflare UI is ~3 minutes; OVH slightly longer).
- Minutes 20-30: leCRM polls verification status, displays progress indicator. Most CNAMEs resolve in 15-30 minutes on Cloudflare. MX records for inbound parsing can take up to 4 hours — set expectation in UI.

For the automated Cloudflare path, the wizard collapses to 5 minutes (OAuth to Cloudflare, leCRM does the rest).

---

## 3. Shared IP vs Dedicated IP

### Volume Threshold Analysis

Industry consensus 2025: dedicated IP makes sense at **50k–100k emails/month sustained**. Below 50k, shared IP reputation benefits outweigh control (shared pools on Brevo/Mailjet have well-established reputations; a cold dedicated IP starts from zero). [Sources: [ReviewMyEmails](https://reviewmyemails.com/emailalmanac/esp-and-infrastructure/shared-vs-dedicated-ips/volume-needed-for-dedicated-ip) | [B2B Deliverability Report 2025](https://thedigitalbloom.com/learn/b2b-email-deliverability-benchmarks-2025/)]

| Phase | Clients | Volume Est. | Recommendation |
|---|---|---|---|
| Phase 1 (5 clients) | ~5k–15k/mo | Shared IP (Scaleway Essential or Brevo Starter) |
| Phase 2 (10 clients) | ~20k–40k/mo | Shared IP still appropriate; watch reputation metrics |
| Phase 3 (20 clients) | ~50k–100k+/mo | Evaluate dedicated IP; Scaleway Scale plan (€80/mo + dedicated) is the trigger |

For sequence-heavy clients (30% running sequences as per plan), phase 3 can spike to 100k+/month — the Scaleway Scale plan's dedicated IP with automatic warm-up is the cleanest phase-3 upgrade.

### Cold IP Warming Protocol

Warming a new dedicated IP takes 4–8 weeks. [Source: [SendGrid IP Warm-Up Guide](https://www.twilio.com/docs/sendgrid/ui/sending-email/warming-up-an-ip-address) | [SparkPost overview](https://support.sparkpost.com/docs/deliverability/ip-warm-up-overview)]

Recommended schedule (engagement-first):

| Period | Daily Volume | Recipient Pool |
|---|---|---|
| Week 1 | 200–500/day | Highest-engagement contacts (opened/clicked last 30 days) |
| Week 2 | 500–2,000/day | Opened/clicked last 60 days |
| Week 3 | 2,000–5,000/day | Opened/clicked last 90 days |
| Week 4 | 5,000–10,000/day | Full list, monitor complaint rate |
| Week 5-8 | Double weekly cap | Target sustained volume |

Rules during warm-up: never send to unengaged lists; keep bounce rate under 2%; complaint rate under 0.08%. Do not send more than 2x the previous week's volume. Maintain daily sending (gaps reset reputation progress).

Scaleway TEM Scale plan automates this via its managed IP service — a significant operational advantage for a solo operator.

---

## 4. Bounce and Complaint Handling Pattern

### Data Model in Twenty CRM

Twenty's base `contact` object should be extended with an `email_suppression` custom object (or field group):

```
email_suppression {
  contact_id        FK → contact
  email             TEXT NOT NULL
  suppression_type  ENUM('hard_bounce','soft_bounce','complaint','unsubscribe','manual')
  suppressed_at     TIMESTAMPTZ
  provider_event_id TEXT          -- idempotency key from webhook
  smtp_code         TEXT          -- raw code e.g. "550 5.1.1"
  smtp_message      TEXT          -- raw SMTP response
  source            TEXT          -- 'brevo_webhook' | 'user_action'
  revocable         BOOLEAN       -- false for hard_bounce and complaint
}
```

**Single source of truth rule:** The suppression table is authoritative. All sequence step sends consult it before dispatch. Provider-side suppression lists (Brevo auto-suppresses hard bounces) are a safety net, not the source of truth.

### Event Handling Pattern (BullMQ)

```
Brevo webhook → POST /api/email/events
  → BullMQ job: handle-email-event
    → switch(event.type)
        'hardBounce' → INSERT email_suppression(type='hard_bounce', revocable=false)
                       UPDATE contact SET email_bounced_at = now()
        'softBounce' → increment soft_bounce_count on contact
                       if count >= 3 → INSERT email_suppression(type='soft_bounce', revocable=true)
        'spam'       → INSERT email_suppression(type='complaint', revocable=false)
                       GDPR note: must honor immediately
        'unsubscribed' → INSERT email_suppression(type='unsubscribe', revocable=true per user request)
```

**Hard bounce:** Permanent. Never retry. SMTP codes 5xx (550, 551, 553). Auto-suppress immediately.
**Soft bounce:** Temporary. SMTP codes 4xx (421, 450, 451). Suppress after 3 consecutive soft bounces.
**Complaint:** Treat identically to hard bounce for suppression. CAN-SPAM/GDPR require immediate stop.
**Unsubscribe:** Log with timestamp; honor for marketing/sequence sends; transactional still allowed per law.

---

## 5. Pre-Flight Content Scoring

### Tools

**GlockApps:** Full-featured inbox placement testing. SpamAssassin score breakdown, content analysis (HTML issues, text-to-image ratio, link reputation, DMARC/DKIM check). API 2.0 available for automation. Pricing: $59–$85/month for commercial use with 30 spam test credits. Free plan: 2 tests. [Source: GlockApps](https://glockapps.com/) | [Review](https://mailflowauthority.com/esp-reviews/glockapps-review)

**Mail Tester (mail-tester.com):** Free, manual, no API. Send an email to a unique test address, get a score. Useful for initial setup validation but not automatable for sequence pre-flight checks.

**Litmus:** Rich email client rendering previews + spam filter checks. More expensive ($99+/month), overkill for SMB CRM sequences.

### Recommendation for leCRM

Integrate GlockApps at sequence creation time (not per-send). When a user finalizes a sequence template, leCRM triggers a GlockApps API test in the background and surfaces the spam score + flagged issues before activation. Cost: $59–85/month is acceptable at phase 2. Budget note: fits within the €60–120/month email layer budget alongside the TEM plan.

Implementation: BullMQ job fires GlockApps API after sequence template save → result stored on `sequence_template.preflight_score` → UI warns if score < 7/10.

---

## 6. Reply Detection Architecture (v1 Native Sequences)

### 6.1 Gmail: Pub/Sub Watch API

Gmail API's push notification model uses Google Cloud Pub/Sub. [Source: Google Developers](https://developers.google.com/workspace/gmail/api/guides/push)

**Flow:**
1. After OAuth consent (scope: `https://www.googleapis.com/auth/gmail.readonly`), call `users.watch()` with a Cloud Pub/Sub topic.
2. Gmail pushes `historyId` notifications to the topic when mailbox changes.
3. Your webhook receives the push; call `users.history.list()` from the stored prior `historyId` to fetch new messages.
4. Filter for `INBOX` + `labelAdded` events; check `InReplyTo` header or `threadId` match against tracked sequences.

**Critical maintenance:** `users.watch()` expires every 7 days. Must renew daily (BullMQ recurring job). [Source: Google Docs](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users/watch)

**OAuth scope:** Per-user delegated consent. Each connected Gmail account requires individual OAuth. Store refresh tokens encrypted in PostgreSQL.

**Pros:** Near-real-time (sub-minute latency). No IMAP connection management.
**Cons:** Cloud Pub/Sub dependency (minor cost, minor infra add); requires daily watch renewal; historyId state management needed.

### 6.2 Microsoft (Outlook/M365): Graph Change Notifications

Microsoft Graph subscriptions for `me/messages` deliver webhook notifications when new messages arrive. [Source: Microsoft Learn](https://learn.microsoft.com/en-us/graph/change-notifications-delivery-webhooks)

**Flow:**
1. OAuth2 (scope: `Mail.Read`) → store tokens per account.
2. Create Graph subscription: `POST /subscriptions` with `changeType: created`, `resource: me/messages`.
3. Graph calls your webhook URL; payload includes `messageId`.
4. Fetch full message (`GET /me/messages/{id}`) to read headers (`InReplyTo`).

**Subscription expiration:** Outlook message subscriptions expire (typically 1 hour to 4230 minutes for `me/messages`). Must renew via `PATCH /subscriptions/{id}` with updated `expirationDateTime`. Lifecycle notification events (`reauthorizationRequired`, `subscriptionRemoved`) signal when renewal is needed. [Source: Microsoft Learn lifecycle events](https://learn.microsoft.com/en-us/graph/change-notifications-lifecycle-events)

**Pros:** Official API, no IMAP needed, full OAuth2.
**Cons:** Very short subscription TTL requires aggressive renewal scheduling; M365 admin consent may be required for app permissions in enterprise tenants.

### 6.3 IMAP IDLE

IMAP IDLE keeps a persistent connection to the mailbox and receives `EXISTS` notifications when new messages arrive. Best Node.js implementation: `imapflow` (maintained by the EmailEngine team). [Source: ImapFlow](https://imapflow.com/) | [npm](https://www.npmjs.com/package/imapflow)

**Multi-mailbox challenge:** Each monitored mailbox requires a persistent TCP connection. At 20 clients × 5-10 users each = 100–200 open IMAP connections. Most ISPs limit simultaneous IMAP sessions (Gmail: 15 concurrent IMAP connections per account). At scale, this becomes a connection management problem.

**Reconnect strategy:** ImapFlow handles reconnect automatically but requires explicit IDLE re-entry after commands. Production pattern: pool connections per account, fall back to polling if IDLE fails.

**Pros:** Works with any IMAP server; no OAuth dependency (app password alternative for Gmail legacy); no subscription renewal.
**Cons:** High connection count at scale; TCP connection state management; NAT/firewall timeout issues on long-lived connections; Gmail's 15-connection limit; not suitable for 100+ concurrent accounts without a connection pool manager like EmailEngine.

### 6.4 Comparison Matrix

| Approach | Implementation Effort | Latency | Reliability at Scale | Auth Complexity |
|---|---|---|---|---|
| Gmail Pub/Sub | Medium | <1 min | High | Per-user OAuth |
| Graph webhooks | Medium-High | <1 min | Medium (short TTL) | Per-user OAuth + tenant admin |
| IMAP IDLE | High | <1 min | Low–Medium (connection mgmt) | Per-user credentials/OAuth |

**Recommendation:** Implement Gmail Pub/Sub + Graph webhooks as the primary path (covers >90% of EU SMB email accounts). IMAP IDLE as fallback for non-standard IMAP accounts (rare in target market). Do not build IMAP IDLE first.

### 6.5 OOO Classifier

For sequence automation, replies must be classified before deciding whether to halt or continue. Key categories: Interested, Not Interested, OOO/Auto-reply, Unsubscribe, Question.

**Options:**

1. **Claude Haiku (LLM):** Best accuracy, zero training data needed. Simple prompt: "Classify this email reply as one of: [OOO, UNSUBSCRIBE, INTERESTED, NOT_INTERESTED, QUESTION, OTHER]. Reply with JSON {category, confidence}." Haiku is fast (<500ms) and cheap (~$0.25/M tokens input). At 10k replies/month, cost is negligible (<$0.50).

2. **Rules-based:** Regex on common OOO phrases ("out of office", "en vacances", "absent du bureau", "je suis absent"). Precision ~95% for OOO; poor for other categories. Build as a pre-filter to avoid LLM call when obvious.

3. **FastText (Apollo's approach):** 99%+ OOO precision, 90% overall accuracy at 1M emails/day scale. Requires labeled training dataset. Overkill for leCRM phase 1-2 volumes.

**leCRM recommendation:** Rules-based OOO pre-filter → Claude Haiku for ambiguous cases. This handles OOO with near-perfect accuracy at minimal cost and zero training data.

**OOO source list for French SMBs:** Include French auto-reply patterns: "absent", "en congé", "en déplacement", "retour le", "je reviens".

### 6.6 Sequence State Machine

```
States: ENROLLED → STEP_PENDING → STEP_SENT → WAITING_REPLY → [REPLIED | BOUNCED | UNSUBSCRIBED | COMPLETED]

Transitions:
  ENROLLED         → STEP_PENDING    (scheduled by BullMQ delayed job)
  STEP_PENDING     → STEP_SENT       (email sent via provider API; log message_id)
  STEP_SENT        → WAITING_REPLY   (if step has reply_detection=true)
  STEP_SENT        → STEP_PENDING    (next step scheduled, no reply detection)
  WAITING_REPLY    → REPLIED         (reply webhook → classify → not OOO → halt sequence)
  WAITING_REPLY    → STEP_PENDING    (reply is OOO → continue; or reply window expires)
  WAITING_REPLY    → BOUNCED         (hard bounce event)
  WAITING_REPLY    → UNSUBSCRIBED    (unsubscribe event)
  any              → UNSUBSCRIBED    (suppression table insert triggers transition)
```

**BullMQ pattern:**
- Each sequence enrollment creates a BullMQ job with `delay` = step.send_at - now.
- Reply detected → cancel pending jobs for that enrollment (`queue.removeJobScheduler(enrollmentId)`).
- OOO reply → reschedule next step with OOO-adjusted delay (e.g., +5 days past OOO return date if parseable).

---

## 7. List Hygiene

### Auto-Suppression Architecture

The `email_suppression` table (defined in section 4) is the single source of truth. Pre-send check pattern:

```sql
-- Before any sequence step send
SELECT 1 FROM email_suppression
WHERE email = $1 AND suppression_type IN ('hard_bounce', 'complaint')
LIMIT 1;
-- If row found: skip send, mark enrollment SUPPRESSED
```

Soft bounces: suppress after 3 consecutive events. Track per-contact with `soft_bounce_count` and `last_soft_bounce_at`. Reset on successful delivery.

**Provider-side suppression (defense-in-depth):** Brevo auto-suppresses hard bounces on their side. This is a second line of defense; leCRM's own suppression table must not depend on it.

**GDPR implications:** Complaint suppression is legally mandatory and must be permanent (revocable=false). Unsubscribes from sequence emails should be respected for all sequence sends; transactional system emails (invoices, password resets) may continue unless the user explicitly requests deletion under Art. 17 GDPR.

---

## 8. Volume Planning

### Email Volume Estimates by Phase

Assumptions:
- 30% of clients run sequences
- Average sequence: 1,000 contacts × 5 steps × 2 campaigns/year = 10,000 sequence emails/client/year = ~833/month/client
- Non-sequence CRM transactional (task notifications, activity summaries, password resets): ~200/user/month = 200 × avg 8 users = 1,600/month/client

| Phase | Clients | Seq clients | Monthly estimate | Provider tier |
|---|---|---|---|---|
| Phase 1 | 5 | 1-2 | ~5k–10k/mo | Brevo Starter ($9–18) or Scaleway Essential (~€1-2) |
| Phase 2 | 10 | 3 | ~15k–25k/mo | Brevo Standard (~$25-35) or Scaleway Essential (~€4-6) |
| Phase 3 | 20 | 6 | ~35k–60k/mo | Scaleway Scale (€80, dedicated IP) or Brevo Business |

At phase 3 upper bound with sequence spikes (campaign blasts), volume can hit 100k+/month during active campaign periods. Scaleway Scale's 100k included + €0.20/1k overage is the cleanest model here.

---

## Recommendations

### Primary Provider Recommendation: Brevo (phases 1-2) → Scaleway TEM Scale (phase 3)

**Rationale:**

- **Phase 1-2:** Brevo Starter/Standard is the pragmatic choice. The `@getbrevo/brevo` official Node.js SDK (v5) is actively maintained and has documented NestJS integration. The inbound parse webhook (for v1 reply detection) is the only provider in this comparison with a production-ready, well-documented reply parsing API. Brevo's GDPR posture (Paris HQ, EU-default data, auto-included DPA) is clean. The API-based domain authentication (`PUT /v3/senders/domains/{domain}/authenticate`) makes multi-tenant onboarding automatable.

  The deliverability caveat (79.8% vs Mailjet's 85%, notably Outlook weakness) is the primary concern. Mitigate with strong DKIM/SPF/DMARC setup, clean list hygiene, and pre-flight content scoring. Monitor complaint rates actively. If Outlook deliverability degrades measurably in phase 2, migrate to Mailjet.

- **Phase 3 / Dedicated IP trigger (~50k+/month):** Scaleway TEM Scale at €80/month all-in with automatic IP warm-up and dedicated IP is the most cost-effective dedicated IP option. Brevo's equivalent requires a $499+/month Professional plan — prohibitive. Scaleway's managed warm-up removes a significant operational burden for a solo operator.

  **Limitation:** Scaleway TEM has no inbound parse API. At phase 3, the native reply detection (v1) must use Gmail Pub/Sub / Graph webhooks for reply ingestion, independent of the outbound provider. This is the correct architecture anyway — reply detection should not depend on the outbound ESP.

**Alternative:** Mailjet (Sinch) is a viable choice if Outlook deliverability becomes a hard requirement. The tradeoff: slightly less developer-friendly SDK, dedicated IP only at 100k+ Premium, Sinch DPA layer adds minor GDPR complexity.

### Sequences Architecture Recommendation

**v0 (current):** Bridge to Reply.io. No custom code for sequences or reply detection. Focus engineering on CRM core.

**v1 (weeks 5-14):**

1. **Outbound:** Send via Brevo API (transactional email endpoint). Store Brevo `messageId` on each sequence step send record.

2. **Reply detection:**
   - Gmail accounts: Gmail Pub/Sub Watch API with per-user OAuth. BullMQ job renews `watch()` daily.
   - Outlook/M365: Microsoft Graph change notifications with short-TTL subscription renewal via BullMQ scheduler.
   - Fallback: Use Brevo inbound parse webhook for replies to the `replies.{client-domain}` MX subdomain (catches replies to generic CRM reply-to addresses).

3. **OOO classification:** Rules-based pre-filter for common OOO phrases (French + English) → Claude Haiku API for ambiguous cases.

4. **State machine:** BullMQ-backed sequence enrollment state machine. Reply detection halts sequence unless classified as OOO (continue) or auto-reply (continue).

5. **Suppression:** Pre-send check against `email_suppression` table on every step. Webhook handlers write to suppression table; state machine reads from it.

### TO RESOLVE

1. **Scaleway TEM deliverability:** No independent inbox placement benchmark exists (GlockApps/EmailToolTester). Before committing Scaleway TEM for phase 3, run a 30-day parallel test against Brevo on a small segment and compare Outlook delivery rates.

2. **Brevo inbound parse plan requirement:** The inbound parse webhook plan tier is not documented publicly. Confirm with Brevo sales whether inbound parsing is available on Starter/Standard plans or requires a higher tier before committing the v1 architecture to it.

3. **Scaleway Scale plan exact pricing confirmation:** The €80/month + 100k emails + dedicated IP was confirmed from Scaleway's pricing page (May 2026), but verify at contract time as Scaleway [has updated pricing in the past](https://www.scaleway.com/en/blog/a-transparent-update-on-scaleway-pricing/).

4. **Microsoft Graph subscription TTL for `me/messages`:** Confirm current maximum `expirationDateTime` for Outlook message subscriptions (was 4230 minutes, but changes periodically). Design renewal to fire at 50% TTL.

5. **Brevo domain API: per-tenant API key isolation:** Verify whether Brevo supports sub-account or multi-domain isolation sufficient to prevent one leCRM tenant's domain issues from affecting others. If not, evaluate whether per-tenant Brevo accounts are needed (adds cost and operational overhead).

---

---

## Addendum 2026-05-10 — Brevo plan-tier verification

**Purpose:** Resolve TO RESOLVE items 2 and 5 from the primary research (inbound parse plan tier; sub-account isolation model). Add updated deliverability data.

---

### A1. Inbound Parse Webhook — Plan Tier & Caveats

**Short answer: Plan tier not publicly documented. Confirmed available as a feature; Enterprise-gating cannot be ruled out.**

Brevo's developer documentation ([developers.brevo.com/docs/inbound-parse-webhooks](https://developers.brevo.com/docs/inbound-parse-webhooks)) describes the inbound parse webhook in full technical detail but contains zero language about which plan tier grants access. The pricing page and the help center "What's on each plan" articles are similarly silent. This is atypical — Brevo's feature matrix is usually explicit about tier restrictions — which may indicate the feature is currently available broadly but the docs have not been updated to reflect any gating added post-GA.

**What is documented:**

- **MX records required:** Delegate a *receiving* subdomain (must differ from your sending domain) to `inbound1.sendinblue.com` (priority 10) and `inbound2.sendinblue.com` (priority 20). Note: despite the servers retaining the legacy `sendinblue.com` hostname, this is the current 2026 config per the live docs.
- **Receiving domain must be pre-verified** in the Brevo dashboard before Brevo will route inbound mail to it.
- **Parsed payload:** Includes `InReplyTo` header, `ExtractedMarkdownMessage`, `SpamScore`, full MIME headers. Exactly what reply-detection requires.
- **No retention period, rate limit, or SLA published.** Brevo explicitly notes: *"impossible to guarantee a 100% success rate on inbound parsing"* — parsing relies on ML signature/quote extraction with inherent accuracy limits. Raw MIME is not always forwarded (raw MIME delivery must be explicitly requested or parsing fallback is raw webhook payload). This is a reliability caveat for production use.
- **Domain count limit:** Not documented. No published cap found.
- **Gradual rollout note (2024 era):** Community discussion referenced inbound webhooks as being "gradually rolled out." As of May 2026, the feature appears GA based on full public documentation, but the rollout language suggests it was not always uniformly available.

**Conclusion for ADR-003:** The inbound parse feature is technically available and well-documented, but the plan tier is a confirmed TO RESOLVE that requires a direct answer from Brevo. The architecture assumption (Starter/Business) is plausible but unverified. **Email Brevo sales before signing up.** If it is Enterprise-only, the v1 architecture must rely exclusively on Gmail Pub/Sub + Graph webhooks for reply detection (the correct long-term architecture anyway), and Brevo inbound parse becomes a secondary/fallback channel.

Sources: [Brevo Inbound Parse Docs](https://developers.brevo.com/docs/inbound-parse-webhooks) | [Brevo Pricing](https://www.brevo.com/pricing/) | [Brevo Plan FAQ](https://help.brevo.com/hc/en-us/articles/208589409-About-Brevo-s-pricing-plans)

---

### A2. Sub-Account / API Key Isolation Per Tenant

**Short answer: Sub-accounts are Enterprise-only. Per-domain DKIM isolation without sub-accounts is possible on all paid plans but shares a single suppression list and stats dashboard.**

**Sub-accounts (sub-organizations) — Enterprise only:**

Brevo's sub-account management feature — called "Sub-Organizations" in the new Admin UI and "Sub-Accounts" in the Classic Admin — is unambiguously documented as exclusive to Enterprise clients. Multiple help center articles confirm: *"Sub-accounts management is available to Brevo's Enterprise clients."* [Source: Brevo Help](https://help.brevo.com/hc/en-us/articles/9003097317138-Classic-Admin-account-What-is-sub-accounts-management) | [Sub-Account Management feature page](https://www.brevo.com/features/sub-account-management/) | [API reference](https://developers.brevo.com/reference/create-a-new-sub-account-under-a-master-account)

What Enterprise sub-accounts provide:
- One Admin (parent) account with N child sub-accounts/sub-organizations
- **Each sub-account has its own isolated API key** (can be created via `POST /v3/corporate/subAccount/{subAccountId}/key`)
- Each sub-account has its own **separate contact list, suppression list, sending stats, and dashboard**
- Each sub-account can have its own **dedicated IP** (dedicated IPs are included in Enterprise at no extra cost per sub-account)
- Sub-account count limit is defined per Enterprise contract; increase requires CSM request
- Admin account can centrally manage billing and allocate monthly email credits across sub-accounts

**Without sub-accounts (Starter / Business plans):**

On non-Enterprise plans, a single Brevo account supports:
- Multiple authenticated **sender domains** (each with its own DKIM CNAME keys + SPF include)
- Multiple **sender addresses** (e.g., `noreply@client1.fr`, `noreply@client2.fr`)
- Multiple **API keys** (Brevo allows creating multiple v3 API keys per account, but they all have the same account-level scope — there is no per-sender or per-domain scoping of API keys)

This means: on Starter/Business, all leCRM tenants share a single suppression list, single sending reputation pool, single unsubscribe management dashboard, and single set of reporting stats. Domain-level DKIM isolation is real (each domain has its own DKIM record), but operational isolation (a suppression on tenant A doesn't leak to tenant B) is **not** achievable without Enterprise sub-accounts.

**Practical implication for leCRM (5-20 tenants, phases 1-3):**

At phase 1-2 volumes (5k–50k/month, 5-10 tenants), the lack of sub-account isolation is a manageable risk if leCRM maintains its own suppression table (Section 4 of the primary doc) as the authoritative source and never relies on Brevo's account-level suppression list as a tenant boundary. The architectural pattern already recommended (leCRM-owned suppression table, Brevo as dumb transport) mitigates most cross-tenant contamination risk.

At phase 3 (20 tenants, 100k+/month), the absence of per-tenant stats and suppression isolation becomes a genuine operational problem for SaaS CRM — a tenant A spam complaint incident can affect the shared IP reputation and impact tenant B deliverability. Enterprise sub-accounts or per-tenant Brevo accounts would be required.

**Alternative: one Brevo account per tenant.** This is operationally heavy (20 accounts, 20 billing lines), but sidesteps the Enterprise pricing. Viable for phase 3 only if the per-account costs are passed through to tenants. Not recommended for phase 1-2.

Sources: [Classic Admin — Sub-accounts overview](https://help.brevo.com/hc/en-us/articles/9003097317138-Classic-Admin-account-What-is-sub-accounts-management) | [Sub-account feature page](https://www.brevo.com/features/sub-account-management/) | [Create API key for sub-account API](https://developers.brevo.com/reference/create-an-api-key-for-a-sub-account) | [Create sub-account API](https://developers.brevo.com/reference/post_corporate-subaccount) | [New Admin — sub-org dedicated IP](https://help.brevo.com/hc/en-us/articles/22135465350290-New-Admin-account-Set-up-a-dedicated-IP-for-your-sub-organizations)

---

### A3. Outbound Deliverability — Updated Benchmarks (2025-2026)

**Short answer: The ~80% average from the primary research is slightly conservative. Independent 2026 data puts Brevo at 78-92% inbox depending on methodology. French ISP data remains unpublished.**

Multiple independent sources tested in 2025-2026:

| Source | Test Period | Inbox % | Spam % | Missing % | Notes |
|---|---|---|---|---|---|
| EmailDeliverabilityReport.com | Through Apr 2026 | 78.71% | 19.39% | 1.90% | 65,820 emails; 25 providers tested; Brevo ranked 1st |
| EmailDeliverabilityReport.com (review) | Through Apr 2026 | 79.29% | 18.85% | 1.86% | Slight variance between report pages; consistent range |
| Encharge.io | Feb 2025 | 89.1% overall | — | — | Gmail: 72.1%, Outlook: 75.3%, Yahoo: 76.1% |
| EmailToolTester | Jan 2024 (latest seed test) | 88.3% | — | — | Methodology shifted in 2025 from seed lists to feature evaluation |
| InboxEagle | Apr 2026 | 18% primary inbox | 17% spam | — | 65% Promotions tab; Grade B; ecommerce/SaaS sender profile |

**Provider-level breakdown (where available):**

The most granular provider data comes from Encharge.io (Feb 2025):
- Gmail: 72.1% inbox / 22.1% spam / 5.9% undelivered
- Outlook: 75.3% inbox / 19.2% spam / 5.5% undelivered
- Yahoo: 76.1% inbox / 18.2% spam / 5.8% undelivered

The EmailDeliverabilityReport data (Apr 2026) shows Brevo ranked first among 25 ESPs, ahead of Sarbacane (78.48%) — a notable result. Best providers in their test: Juno.com (80.52%), Tuta (80.99%). Weakest: iCloud (74.76%), Gmail (75.08%), Outlook (77.10%).

**French ISP data (Free.fr, Orange.fr, Laposte.net, SFR):** No independent published data found as of May 2026. None of the major benchmarking services (EmailToolTester, EmailDeliverabilityReport, GlockApps public reports, Encharge, InboxEagle) include French FAI mailboxes in their seed lists. This gap is a genuine risk for leCRM targeting French SMBs. Brevo's own origin as a French provider (Sendinblue, Paris) and its historical relationship with French ISPs is likely an advantage, but there is no third-party data to confirm it.

**Updated assessment vs primary research:** The 79.8% from the primary doc was EmailToolTester's 2024 average. The 2026 data is broadly consistent (78-80% range in seed-list tests). The Encharge 89% figure uses a different methodology (single-round test). Neither the Mailjet 85% vs Brevo 80% gap nor the Outlook weakness finding has been contradicted by 2026 data — in fact the Outlook weakness (75.3% inbox on Outlook per Encharge) persists.

**InboxEagle's 18% primary inbox / 65% Promotions finding** is the most alarming recent data point. This is likely a methodology artifact (ecommerce sender profile, marketing-type content triggering Google's Promotions classifier), but warrants attention for CRM sequence emails which often have sales-oriented subject lines. For leCRM, using plaintext or near-plaintext sequence emails with minimal HTML reduces Promotions tab classification risk.

Sources: [EmailDeliverabilityReport — Brevo](https://emaildeliverabilityreport.com/en/deliverability/brevo) | [EmailDeliverabilityReport — Brevo review](https://emaildeliverabilityreport.com/en/review/brevo) | [Encharge — Brevo deliverability](https://encharge.io/brevo-deliverability/) | [EmailToolTester — Brevo deliverability](https://www.emailtooltester.com/en/blog/brevo-deliverability/) | [InboxEagle — Brevo insights](https://www.inboxeagle.com/esp-insights/brevo/)

---

### A4. ADR-003 Impact Summary

| Item | Finding | ADR-003 Action |
|---|---|---|
| Inbound parse plan tier | **Undocumented** — could be any paid tier or Enterprise-only | TO RESOLVE: confirm with Brevo sales before architecture lock-in |
| Inbound MX records | `inbound1/2.sendinblue.com` (legacy hostname, current config) | Confirmed — DNS records in Section 2 are correct |
| Inbound parse SLA | No SLA; Brevo explicitly disclaims 100% accuracy | Design reply detection to degrade gracefully; Gmail Pub/Sub is primary |
| Sub-accounts | **Enterprise-only** | Phase 1-2: single account, leCRM manages tenant isolation in its own DB. Phase 3: negotiate Enterprise or move to per-tenant accounts |
| Per-domain DKIM without sub-accounts | Available on all paid plans | Confirmed — multi-tenant domain auth architecture in Section 2 is valid |
| Deliverability (overall) | 78-80% inbox (seed tests), 89% (marketing claim) | Consistent with primary doc; Outlook weakness confirmed for 2025 |
| French ISP deliverability | No published data | TO RESOLVE: run leCRM's own test against Free.fr / Orange.fr after onboarding first clients |

## Sources

- [Brevo Pricing](https://www.brevo.com/pricing/)
- [Brevo Dedicated IP — Introduction](https://help.brevo.com/hc/en-us/articles/208835449-Introduction-to-dedicated-IPs)
- [Brevo Dedicated IP — Setup](https://help.brevo.com/hc/en-us/articles/115000240344-Set-up-your-dedicated-IP-in-Brevo)
- [Brevo Transactional Webhooks](https://developers.brevo.com/docs/transactional-webhooks)
- [Brevo Inbound Parse Webhooks](https://developers.brevo.com/docs/inbound-parse-webhooks)
- [Brevo Domain Authentication API](https://developers.brevo.com/docs/domain-authentication-and-verification)
- [Brevo Node.js SDK](https://developers.brevo.com/docs/api-clients/node-js)
- [@getbrevo/brevo npm](https://www.npmjs.com/package/@getbrevo/brevo)
- [Brevo GDPR](https://www.brevo.com/company/gdpr/)
- [Brevo DPA](https://help.brevo.com/hc/en-us/articles/15403782599570-Where-can-I-find-the-Data-Processing-Agreement-DPA)
- [Scaleway TEM](https://www.scaleway.com/en/transactional-email-tem/)
- [Scaleway TEM Pricing — Managed Services](https://www.scaleway.com/en/pricing/managed-services/)
- [Scaleway TEM Capabilities and Limits](https://www.scaleway.com/en/docs/transactional-email/reference-content/tem-capabilities-and-limits/)
- [Scaleway TEM Dedicated IP Docs](https://www.scaleway.com/en/docs/transactional-email/reference-content/tem-dedicated-ip/)
- [Scaleway TEM Webhook Events](https://www.scaleway.com/en/docs/transactional-email/reference-content/webhook-events-payloads/)
- [Scaleway DPA June 2024](https://www-uploads.scaleway.com/DPA_2024_ENG_b0abb5cc26.pdf)
- [Scaleway SecNumCloud blog](https://www.scaleway.com/en/blog/a-transparent-update-on-scaleway-pricing/)
- [Mailjet Pricing](https://www.mailjet.com/pricing/)
- [Mailjet Event API — Webhooks](https://dev.mailjet.com/email/guides/webhooks/)
- [Mailjet Parse API](https://dev.mailjet.com/email/guides/parse-api/)
- [Mailjet GDPR](https://www.mailjet.com/resources/learn/gdpr/)
- [Sinch DPA and Sub-processors](https://sinch.com/legal/data-protection-agreement-sub-processors/)
- [EmailToolTester — Best Transactional Email Services 2026](https://www.emailtooltester.com/en/blog/best-transactional-email-service/)
- [EmailToolTester — Brevo Deliverability](https://www.emailtooltester.com/en/blog/brevo-deliverability/)
- [EmailToolTester — Brevo Pricing 2026](https://www.emailtooltester.com/en/reviews/brevo/pricing/)
- [EmailVendorSelection — Brevo Pricing](https://www.emailvendorselection.com/brevo-pricing/)
- [GlockApps](https://glockapps.com/)
- [GlockApps Review — Mailflow Authority](https://mailflowauthority.com/esp-reviews/glockapps-review)
- [Apollo Tech Blog — Email Reply Classification](https://www.apollo.io/tech-blog/email-reply-classification-done-right)
- [Gmail API Push Notifications — Google Developers](https://developers.google.com/workspace/gmail/api/guides/push)
- [Gmail API users.watch() reference](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users/watch)
- [Microsoft Graph Change Notifications — Webhooks](https://learn.microsoft.com/en-us/graph/change-notifications-delivery-webhooks)
- [Microsoft Graph Lifecycle Events](https://learn.microsoft.com/en-us/graph/change-notifications-lifecycle-events)
- [Microsoft Graph Subscription resource](https://learn.microsoft.com/en-us/graph/api/resources/subscription?view=graph-rest-1.0)
- [ImapFlow](https://imapflow.com/)
- [imapflow npm](https://www.npmjs.com/package/imapflow)
- [SendGrid IP Warm-Up Guide](https://www.twilio.com/docs/sendgrid/ui/sending-email/warming-up-an-ip-address)
- [SparkPost IP Warm-Up Overview](https://support.sparkpost.com/docs/deliverability/ip-warm-up-overview)
- [ReviewMyEmails — Volume for Dedicated IP](https://reviewmyemails.com/emailalmanac/esp-and-infrastructure/shared-vs-dedicated-ips/volume-needed-for-dedicated-ip)
- [B2B Email Deliverability Report 2025](https://thedigitalbloom.com/learn/b2b-email-deliverability-benchmarks-2025/)
- [Mailjet Dedicated IP help article](https://documentation.mailjet.com/hc/en-us/articles/1260803352789-Dedicated-IPs-What-They-Are-and-How-to-Warm-Them-Up)
- [Sender.net — Mailjet Pricing](https://www.sender.net/reviews/mailjet/pricing/)
