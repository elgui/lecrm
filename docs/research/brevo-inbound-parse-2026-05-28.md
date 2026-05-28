# Brevo Inbound Parse Webhook — Decision Research

**Date:** 2026-05-28
**Scope:** Public information only. Account-specific data (plan, renewal date, invoice) must be verified against your own Brevo dashboard.
**Context:** Outbound transactional email stays on Brevo. This research evaluates the inbound parse path for a Go CRM/sequences product.

---

## Executive Summary

Brevo's inbound parse webhook delivers a well-structured JSON payload that includes all fields critical for a CRM sequences engine: `MessageId`, `InReplyTo`, `Headers` (full block), `RawTextBody`, `RawHtmlBody`, and an AI-extracted `ExtractedMarkdownMessage`. The developer docs impose **no plan-tier gate** on the feature — it appears available across all paid and free tiers, though Brevo's pricing page renders as JavaScript and could not be machine-read to confirm a definitive feature table. Pricing runs ~€7/month (Starter) to €15/month (Business) for typical send volumes, with no documented per-inbound-email overage. The two-MX DNS setup is simple and documented.

The main alternatives — Postmark inbound (now available from $16.50/month on Pro), Mailgun routes (available from $15/month Basic), and AWS SES ($0.10/1000 inbound emails, EU-regional) — all carry inbound-email-in-payload tradeoffs: Postmark requires Pro tier; Mailgun caps inbound routes on Basic; SES requires custom Lambda/S3 parsing work and does not deliver a pre-parsed JSON payload to a webhook. None offers a clearly better payload for the sequences use case.

**Recommended path:** Stay on Brevo for inbound. The payload is rich, the DNS setup is straightforward, and there is no cost to split the inbound path to a second vendor. The single biggest tradeoff is that Brevo's pricing page is not easily inspectable for plan-tier gating — you must confirm in your dashboard that your current plan explicitly enables the inbound MX feature before committing to it in production.

---

## 1. Which Brevo Plans Include Inbound Parse Webhook

The official developer docs at `https://developers.brevo.com/docs/inbound-parse-webhooks` describe the feature without any mention of a plan restriction. The text does not say "available on Business or higher" or list any tier gate. [[Source 1]](#sources)

A third-party Brevo pricing review dated September 2025 lists the plan tiers as Free (€0), Starter (€7/month), Business (€15/month), and Enterprise (custom), with developer features not split by tier. [[Source 8]](#sources)

Brevo's own pricing page (`https://www.brevo.com/pricing/`) rendered as a JavaScript shell without accessible plan feature tables during this research — the page title loaded but no plan details were extractable by public scraping. [[Source 2]](#sources) The same block occurred on the Brevo help article (`https://help.brevo.com/hc/en-us/articles/208589409`), which returned HTTP 403. [[Source 3]](#sources)

**Conclusion:** No public source documents a plan-tier restriction on inbound parse. The feature is documented as generally available. **Verify in your own dashboard** that the "Inbound" MX configuration is unlocked on your current plan before production use.

---

## 2. Current Brevo Pricing in EUR

Prices below are sourced from a third-party review updated September 2025 [[Source 8]](#sources) and a second review updated May 2026 (USD basis, converted). Brevo's own pricing page was not machine-readable during this session.

| Plan | Monthly (EUR) | Annual equiv. (EUR/mo) | Sent emails/month included |
|------|--------------|------------------------|---------------------------|
| Free | €0 | €0 | 300/day (~9,000/month) |
| Starter | ~€7 | ~€6.33 (10% annual saving) | Scales: 5k–100k options |
| Business | ~€15 | ~€13.50 | Scales: 5k–1M options |
| Enterprise | Custom | Custom | Custom |

Source for USD basis (Starter $9/month, Business $18/month): [[Source 6]](#sources), [[Source 9]](#sources). EUR equivalents from [[Source 8]](#sources).

**Per-inbound-email overage:** No Brevo pricing source found that bills for received/parsed email volume. The inbound feature appears to be a capability rather than a metered resource. This is consistent with Brevo's model of metering outbound send volume, not inbound receive events. **(To be confirmed against your account terms.)**

**DNS cost:** Two MX records pointing to `inbound1.sendinblue.com` and `inbound2.sendinblue.com` — no additional domain purchase required. [[Source 1]](#sources)

---

## 3. Brevo Inbound Parse Webhook Payload

Source: `https://developers.brevo.com/docs/inbound-parse-webhooks` [[Source 1]](#sources)

The webhook delivers a JSON object with a top-level `items` array. Each item represents one parsed email. Confirmed fields:

| Field | Type | Notes |
|-------|------|-------|
| `MessageId` | string | The `Message-ID` SMTP header — load-bearing for dedup |
| `InReplyTo` | ?string | The `In-Reply-To` header value — **load-bearing for reply detection** |
| `Headers` | array of string or string[] | Complete raw SMTP headers block |
| `From` | Mailbox object (`Address`, `Name`) | Sender |
| `To` | Mailbox[] | Primary recipients |
| `Cc` | Mailbox[] | CC recipients |
| `Recipients` | Mailbox[] | RCPT TO envelope recipients |
| `ReplyTo` | ?Mailbox | Reply-To header if present |
| `Subject` | string | Subject line |
| `SentAtDate` | string | RFC822 formatted timestamp |
| `RawHtmlBody` | ?string | Full HTML part |
| `RawTextBody` | ?string | Full plain text part |
| `ExtractedMarkdownMessage` | string | AI-extracted message body (signature removed) |
| `ExtractedMarkdownSignature` | ?string | Recognized signature block |
| `SpamScore` | float | rspamd score |
| `Uuid` | string[] | Brevo-internal UUID |
| `Attachments` | Attachment[] | `Name`, `ContentType`, `ContentLength`, `ContentID`, `DownloadToken` |

**Key finding:** `InReplyTo` is a **top-level named field** (not buried in a generic headers array), which is cleaner than any of the alternatives. `MessageId` is similarly first-class. The `Headers` field carries the full raw block for any header not explicitly parsed (e.g. `References`, `X-Mailer`).

**Attachment access:** Attachments are not inlined. Each attachment carries a `DownloadToken` that must be exchanged via a separate Brevo API call. This is a minor integration overhead vs. Postmark or Mailgun, which can base64-encode small attachments inline.

**Fields that are NOT present at the top level (but derivable from `Headers`):** `References` header, `DKIM-Signature`, `Received` chain. These are accessible via the `Headers` array.

---

## 4. Comparable Alternatives — Inbound Path Only

### 4.1 Postmark Inbound

**Plan tier required:** Pro ($16.50/month) or Platform ($18/month) at 10k email volume. Inbound is not available on Basic ($15/month). This changed in August 2025 — previously Pro required a 50k-volume plan at $60.50/month. [[Source 4]](#sources) [[Source 5]](#sources)

**Cost for 1,000–5,000 inbound emails/month:** Each inbound message counts as 1 email against your monthly allowance. At Pro 10k ($16.50/month), 1,000–5,000 inbound emails consume 10–50% of your allowance. Overage: $1.30/1,000.

**Payload parity with Brevo:**
- `MessageID`: present as a top-level field. [[Source 10]](#sources)
- `In-Reply-To`: **not** a top-level field; available only inside the `Headers` array as a `Name`/`Value` pair. Postmark does use `In-Reply-To` to populate `StrippedTextReply` internally, confirming it is parsed, but your Go code must walk the array to extract it. [[Source 10]](#sources)
- `TextBody` and `HtmlBody`: both present as direct fields. [[Source 10]](#sources)
- `Headers`: full raw array (`Name`/`Value` pairs) — includes all SMTP headers. [[Source 10]](#sources)
- Attachments: base64-encoded inline in payload for small attachments. [[Source 10]](#sources)

**DNS setup:** Point your domain MX to `inbound.postmarkapp.com`. Simple, comparable to Brevo.

**Key deal consideration:** Postmark inbound does **not** require a separate account from your transactional account. Both live in the same Postmark workspace. [[Source 11]](#sources) However, since your outbound transactional is already on Brevo, adopting Postmark for inbound means maintaining two ESP relationships.

**Verdict:** Good payload parity (all critical headers reachable), but `In-Reply-To` is not first-class (requires array walk). Costs slightly more per month than Brevo Starter with comparable volume. Adds vendor split complexity.

---

### 4.2 Mailgun Routes (Inbound)

**Plan and cost:** Basic plan ($15/month, 10k emails, 5 inbound routes) or Foundation ($35/month, 50k, full inbound routing). [[Source 7]](#sources)

All plans include inbound routing, but the Basic plan's "5 routes" cap could matter if your sequences product needs per-domain or per-client routing rules at scale.

**Payload fields:** Mailgun POSTs a form-encoded or multipart body (not pure JSON) to your webhook. Fields confirmed in documentation [[Source 12]](#sources):
- `from`, `sender`, `recipient`: top-level fields
- `subject`: top-level
- `body-plain`, `body-html`: present
- `message-headers`: a JSON-string dump of all MIME headers — `In-Reply-To` and `Message-Id` are present here, parseable as key-value pairs
- `stripped-text`, `stripped-signature`: parsed reply content (similar to Brevo's `ExtractedMarkdownMessage`)
- Attachments: base64-encoded inline

**In-Reply-To / Message-Id availability:** Present inside `message-headers` JSON string. Not first-class fields. Requires parsing a JSON string within the form body. [[Source 12]](#sources)

**DNS setup:** Single MX record pointing to `mxa.mailgun.org` and `mxb.mailgun.org`. Comparable simplicity to Brevo.

**Deal-breaker at scale:** The Basic plan's 5-route limit becomes a problem if each client or domain needs its own inbound rule. Foundation at $35/month removes this cap. Pricing jumps significantly.

---

### 4.3 AWS SES + S3 + Lambda

**Cost for 1,000–5,000 inbound emails/month:** $0.10/1,000 inbound email chunks (256 KB/chunk). For typical CRM reply emails under 256 KB, 5,000 emails = $0.50/month for receiving. S3 storage for raw emails is negligible (<$0.10/month). Lambda invocations at 5,000/month are within the always-free tier. Total: effectively **under $1/month** at early CRM volumes. [[Source 13]](#sources)

**EU region availability:** SES inbound email receiving is supported in `eu-west-1` (Ireland), `eu-west-2` (London), `eu-west-3` (Paris), `eu-central-1` (Frankfurt), `eu-north-1` (Stockholm), and `eu-south-1` (Milan) — full list at [[Source 14]](#sources). Earlier reports of Frankfurt being unsupported are outdated; the current AWS endpoints table confirms `inbound-smtp.eu-central-1.amazonaws.com` exists. [[Source 14]](#sources)

**Payload:** SES does **not** deliver a parsed webhook. The raw MIME email is stored in S3 (or forwarded to SNS). Your Lambda must parse the raw `.eml` file using a Go MIME library. `In-Reply-To`, `Message-Id`, and all headers are present in the raw email by definition, but extraction is your responsibility.

**DNS setup:** One MX record pointing to `inbound-smtp.<region>.amazonaws.com`. Simple DNS setup, but requires AWS account, IAM roles, S3 bucket policy, Lambda function, and SES receipt rule configuration — meaningfully more infra work than Brevo or Postmark.

**Deal-breaker consideration:** No webhook push; pull-based (S3 event → Lambda). This is workable but architecturally different from the push model the other providers offer. It also means cold-start latency on Lambda if inbound volume is low.

---

## 5. Recommended Path

**Argument for staying on Brevo:** For a Go CRM/sequences product at 1,000–5,000 inbound parses/month, Brevo is already your outbound ESP, the inbound parse feature appears to be included at no additional plan cost, and the payload is the cleanest of all evaluated options: `InReplyTo` and `MessageId` are **top-level named fields** rather than values you must fish out of a generic headers array. Setup is two MX records. The `ExtractedMarkdownMessage` field (signature-stripped body) saves your engine from having to implement reply-body extraction itself.

**Biggest tradeoff against Brevo:** The plan-tier feature gate is **not publicly documented** in machine-readable form. Brevo's pricing page did not render its feature table during this research session. If Brevo gates inbound to Business tier (€15/month) rather than Starter (€7/month), the math changes. More critically, Brevo is a marketing platform company; the transactional/developer API surface is not their core product, and the inbound parse feature — which still uses legacy MX hostnames (`inbound1.sendinblue.com`) from the pre-rebrand era — has not seen a public changelog entry that could be fetched. That legacy DNS hostname is a minor flag on feature maintenance priority.

**If you switch:** Postmark at Pro tier ($16.50/month) is the cleanest alternative — good payload, simple DNS, no route limits. The only Go-code difference is walking the `Headers` array for `In-Reply-To` instead of reading a top-level field. AWS SES is the right choice only if cost at scale (50k+ parses/month) is the dominant constraint; the per-message cost is near-zero but the engineering overhead is non-trivial.

---

## Sources

1. **Brevo Inbound Parse Webhooks — Developer Docs**
   `https://developers.brevo.com/docs/inbound-parse-webhooks`

2. **Brevo Pricing Page** (JS-rendered, content not extractable)
   `https://www.brevo.com/pricing/`

3. **About Brevo's Pricing Plans — Help Center** (returned HTTP 403)
   `https://help.brevo.com/hc/en-us/articles/208589409-About-Brevo-s-pricing-plans`

4. **Postmark: Pro and Platform Tier Features Now Accessible to Lower-Volume Plans**
   `https://postmarkapp.com/blog/new-we-made-pro-and-platform-tier-features-accessible-to-lower-volume-email-plans`
   (published August 6, 2025)

5. **Postmark Pricing and Free Trial**
   `https://postmarkapp.com/pricing`

6. **Brevo Pricing (Moosend review, updated November 2025)**
   `https://moosend.com/blog/brevo-pricing/`

7. **Mailgun Pricing**
   `https://www.mailgun.com/pricing/`

8. **Brevo Plans and Prices (Open Rate Club, September 2025)**
   `https://openrateclub.com/brevo-pricing/`

9. **Brevo Pricing (Sender.net review, May 2026)**
   `https://www.sender.net/reviews/brevo/pricing/`

10. **Postmark Inbound Webhook Documentation**
    `https://postmarkapp.com/developer/webhooks/inbound-webhook`
    `https://postmarkapp.com/developer/user-guide/inbound/parse-an-email`

11. **Postmark Pricing & Billing FAQ**
    `https://postmarkapp.com/support/article/1285-pricing-billing-faq`

12. **Mailgun — Inbound: Receive, Forward, and Store (HTTP payload)**
    `https://documentation.mailgun.com/docs/mailgun/user-manual/receive-forward-store/receive-http`

13. **Amazon SES Pricing**
    `https://aws.amazon.com/ses/pricing/`

14. **Amazon SES Endpoints and Quotas (Email Receiving Endpoints table)**
    `https://docs.aws.amazon.com/general/latest/gr/ses.html#ses_inbound_endpoints`

15. **AWS SES Regions — Email Receiving section**
    `https://docs.aws.amazon.com/ses/latest/dg/regions.html#region-receive-email`

---

*Research conducted 2026-05-28. All pricing figures are from publicly accessible sources as of that date and should be verified against provider pricing pages before commitment. Brevo EUR pricing sourced from third-party reviews, not directly from brevo.com/pricing (page did not render). AWS SES EU inbound region availability confirmed from the official AWS General Reference endpoints table.*
