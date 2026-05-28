# ADR-004 — Sequences Architecture (v1 Native)

**Status:** Superseded by [ADR-004 rev 2](ADR-004-rev2-sequences-architecture.md) (2026-05-28).
**Date:** 2026-05-10
**Deciders:** Guillaume

> **Superseded notice (2026-05-28).** This ADR describes the sequences engine against the NestJS + BullMQ + Redis stack. ADR-009 replaced that runtime with Go + sqlc + river + Postgres-only. The architectural intent (durable Postgres state machine, reply correlation on Message-ID, three-path reply detection, two-stage OOO classifier, suppression as source of truth) all survives — see [ADR-004 rev 2](ADR-004-rev2-sequences-architecture.md) for the Go + river expression of those decisions and the rev-1 → rev-2 delta in the rev-2 Context section. This document is retained as historical context only; do not implement from it.

---

## Context

leCRM v0 bridges to Reply.io for outbound sequences. v1 (weeks 5–14) builds the native sequences engine inside the email service. The native engine has three responsibilities:

1. **Schedule and send** sequence steps for each enrolled contact, respecting suppression and per-contact send windows.
2. **Detect replies** and halt or branch the sequence accordingly. This is the hardest part and the deliverability-quality differentiator vs HubSpot Sales Hub Pro and Reply.io.
3. **Update the suppression list** in real time as bounces, complaints, and unsubscribes flow in.

Constraints:

- Outbound provider is **Brevo** ([ADR-003](ADR-003-email-provider-brevo.md)). The architecture must work within Brevo's webhook event model.
- Reply detection must work for **Gmail** (per-user OAuth + Pub/Sub Watch), **Outlook/M365** (per-user OAuth + Graph change notifications), and a **catch-all inbound** path for clients who want a generic CRM reply-to (Brevo inbound parse).
- **Solo operator.** The state machine must be debuggable from a database query, not from process memory.
- **Per-tenant volume caps** (defined in [ADR-003](ADR-003-email-provider-brevo.md) §Decision) prevent runaway sends.
- The classifier (was-this-a-real-reply-or-an-OOO-bounce) must work for **French and English** auto-replies.

The data and provider choices are governed by `docs/research/email-deliverability.md` §6.

---

## Decision

### 1. Sequence state machine — BullMQ-backed Postgres state model

The state of every sequence enrollment is durable in PostgreSQL. BullMQ (Redis 7) is the scheduler / worker; the database is the source of truth. A worker crash, a Redis flush, or a deploy never loses enrollment state.

**Schema (added via Twenty's metadata extension API so it lives inside each workspace's schema):**

```sql
CREATE TYPE sequence_enrollment_state AS ENUM (
  'ENROLLED',
  'STEP_PENDING',
  'STEP_SENT',
  'WAITING_REPLY',
  'REPLIED',
  'OOO',
  'BOUNCED',
  'UNSUBSCRIBED',
  'COMPLETED',
  'SUPPRESSED'
);

CREATE TABLE sequence_enrollment (
  id                   UUID PRIMARY KEY,
  sequence_id          UUID NOT NULL,
  contact_id           UUID NOT NULL,
  state                sequence_enrollment_state NOT NULL,
  current_step_index   SMALLINT NOT NULL DEFAULT 0,
  enrolled_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  next_action_at       TIMESTAMPTZ,
  last_transition_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  reply_message_id     TEXT,
  ooo_returns_at       TIMESTAMPTZ,
  workspace_id         UUID NOT NULL  -- redundant for query clarity
);

CREATE TABLE sequence_step_send (
  id                   UUID PRIMARY KEY,
  enrollment_id        UUID NOT NULL REFERENCES sequence_enrollment(id),
  step_index           SMALLINT NOT NULL,
  brevo_message_id     TEXT,
  sent_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  delivered_at         TIMESTAMPTZ,
  bounced_at           TIMESTAMPTZ,
  bounce_type          TEXT  -- 'hard' | 'soft' | NULL
);

CREATE INDEX idx_seq_enr_state_next ON sequence_enrollment(state, next_action_at)
  WHERE state IN ('STEP_PENDING','WAITING_REPLY');
```

**State transitions:**

```
ENROLLED        --(scheduled)-->        STEP_PENDING
STEP_PENDING    --(send via Brevo)-->   STEP_SENT
STEP_SENT       --(reply_detection=true)--> WAITING_REPLY
STEP_SENT       --(no reply expected)-->    STEP_PENDING (next step scheduled)
WAITING_REPLY   --(reply, not OOO)-->   REPLIED  (terminal)
WAITING_REPLY   --(reply, OOO)-->       OOO → STEP_PENDING (rescheduled at OOO return + 1 day)
WAITING_REPLY   --(reply window expires)--> STEP_PENDING (next step)
WAITING_REPLY   --(hard bounce)-->      BOUNCED  (terminal)
WAITING_REPLY   --(unsubscribe)-->      UNSUBSCRIBED  (terminal)
*               --(suppression hit pre-send)--> SUPPRESSED  (terminal)
*               --(all steps completed)-->     COMPLETED  (terminal)
```

### 2. BullMQ queues

| Queue | Purpose | Retry policy | Concurrency |
|---|---|---|---|
| `email-send` | Per-step send via Brevo API | exponential backoff, 3 attempts; on final failure → `email-send-dlq` | 5 workers per pod |
| `email-event` | Process Brevo webhook events (delivered, bounce, complaint, etc.) | exponential backoff, 5 attempts; idempotent on `provider_event_id` | 10 workers |
| `sequence-tick` | Fires when a step's `next_action_at` arrives | exponential backoff, 3 attempts | 3 workers |
| `reply-detection` | Process Gmail Pub/Sub / Graph / inbound-parse webhook events | exponential backoff, 3 attempts | 5 workers |
| `gmail-watch-renew` | Daily renewal of Gmail `users.watch()` per connected mailbox | repeat on cron `0 4 * * *`, retries=3 | 1 |
| `graph-sub-renew` | Renewal of Microsoft Graph subscriptions before TTL expiry | repeat every 30 min, lifecycle-event-driven | 2 |

Dead-letter queues (`*-dlq`) for every retry-bounded queue; alerts on DLQ growth.

### 3. Reply detection — three paths, primary + secondary + fallback

**Primary path (per-user OAuth):** Gmail Pub/Sub Watch + Microsoft Graph change notifications. Each connected user mailbox is monitored individually.

- **Gmail:** `users.watch()` with a Cloud Pub/Sub topic. Filter for `INBOX` + `labelAdded` events. Match by `InReplyTo` header or `threadId` against tracked sequences. **Watch expires every 7 days** → daily renewal via `gmail-watch-renew` BullMQ recurring job. Per-user OAuth tokens stored encrypted in Twenty's user table (extension column `gmail_refresh_token_encrypted`).
- **Outlook/M365:** `POST /subscriptions` with `changeType: created`, `resource: me/messages`. **Subscription TTL is short** (typically 1–4230 minutes for `me/messages` — exact value is platform-version-dependent and is a TO RESOLVE). Renewal via `graph-sub-renew`, fires at 50% of remaining TTL. Lifecycle events (`reauthorizationRequired`, `subscriptionRemoved`) trigger immediate renewal logic.

**Secondary path (catch-all):** Brevo inbound parse webhook. Used when:
- The client wants a generic CRM reply-to (`reply@<client-domain>` instead of per-user mailboxes), or
- A user hasn't connected their Gmail/Outlook OAuth, or
- An OAuth grant has been revoked.

DNS setup per client: `replies.<client-domain>` MX 10 → `inbound1.sendinblue.com`, MX 20 → `inbound2.sendinblue.com` ([ADR-003](ADR-003-email-provider-brevo.md)). Sequence step `Reply-To` header is set to `<enrollment-id>@replies.<client-domain>`. The webhook handler matches replies via `InReplyTo` header against `sequence_step_send.brevo_message_id`.

**Fallback path (IMAP IDLE):** for non-Gmail, non-M365 mailboxes (rare in target market — French SMBs are dominantly Gmail Workspace or Microsoft 365). Implementation via `imapflow` npm package. Connection-pool manager required at scale; deferred unless a paying client demands it.

### 4. OOO classification — rules-based + Haiku

**Two-stage classifier:**

**Stage 1 — rules-based pre-filter** (deterministic, near-zero cost). Regex pattern set covering common French and English auto-reply phrases:

```
French:  /\b(absent|en cong[éè]|en d[ée]placement|en vacances|de retour le|je reviens|bureau ferm[ée])\b/i
English: /\b(out of office|on (vacation|leave|holiday)|i (am|will be) (out|away)|automatic reply|currently away|return on)\b/i
```

If matched, classify as OOO immediately. Precision ~95% for OOO; near-zero for other categories. (`docs/research/email-deliverability.md` §6.5.)

**Stage 2 — Claude Haiku for ambiguous cases.** Prompt:

```
Classify this email reply as exactly one of: OOO, UNSUBSCRIBE, INTERESTED, NOT_INTERESTED, QUESTION, OTHER.
Reply with strict JSON: {"category": "...", "confidence": 0.0-1.0}.
---
[full email body, sanitized of HTML]
```

Haiku call: ~$0.001 per classification. At 10k replies/month this is <$0.50. Latency <500 ms. Cached prompts (the classifier's instruction is identical for every call) cut cost ~70% via Anthropic prompt caching ([ADR-005](ADR-005-ai-agent-tenancy.md) §6).

**OOO with parseable return date:** rules-based extraction of "de retour le 15 mai", "back on May 15", etc. → `ooo_returns_at` stored on enrollment → next step rescheduled at `ooo_returns_at + 1 day`. If date isn't parseable, default reschedule is +5 business days.

### 5. Suppression list — single source of truth

Schema (added via metadata extension, lives in each workspace's schema):

```sql
CREATE TABLE email_suppression (
  id                  UUID PRIMARY KEY,
  email               TEXT NOT NULL,
  contact_id          UUID,
  suppression_type    TEXT NOT NULL CHECK (suppression_type IN
                        ('hard_bounce','soft_bounce','complaint','unsubscribe','manual')),
  suppressed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  provider_event_id   TEXT UNIQUE,  -- idempotency key from webhook
  smtp_code           TEXT,
  smtp_message        TEXT,
  source              TEXT NOT NULL,  -- 'brevo_webhook' | 'user_action' | 'sequence_engine'
  revocable           BOOLEAN NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX idx_supp_email_type ON email_suppression(email, suppression_type)
  WHERE suppression_type IN ('hard_bounce','complaint');
```

**Pre-send check** in every `email-send` worker:

```sql
SELECT 1 FROM email_suppression
WHERE email = $1
  AND suppression_type IN ('hard_bounce','complaint','unsubscribe')
LIMIT 1;
```

If row found: skip send, transition enrollment to `SUPPRESSED`. The provider-side suppression (Brevo auto-suppresses hard bounces) is a defense-in-depth, **not** the source of truth.

**Soft bounce policy:** suppress after 3 consecutive soft bounces on the same email. Tracked via `contact.soft_bounce_count` and reset on successful delivery.

### 6. Volume caps and rate limiting

Per-tenant `monthly_send_cap` config in workspace metadata. Pre-send check at the queue boundary: if monthly count >= cap, refuse enrollment with a clear admin notification. Default cap by phase: phase 1 = 5,000/mo; phase 2 = 15,000/mo; phase 3 = 30,000/mo (overridable per tenant).

Per-recipient throttle: never send more than 1 step per 24 h to the same `contact_id` regardless of which sequence. Implemented via `last_sent_at` lookup at worker time.

---

## Consequences

### Positive

- **Durable state machine in Postgres.** A query against `sequence_enrollment` shows the live state of every campaign. Recovery from worker crash or Redis flush is trivial (re-enqueue jobs based on `state IN ('STEP_PENDING','WAITING_REPLY')` and `next_action_at`).
- **Three-path reply detection** covers >95% of EU SMB email accounts (Gmail Workspace + M365) plus the catch-all Brevo inbound for the rest.
- **Two-stage OOO classifier** handles French + English auto-replies with high precision at near-zero cost. The Haiku stage is a small, well-defined LLM dependency with a clear cost ceiling.
- **Suppression list as single source of truth** eliminates the "Reply.io says we suppressed but they sent anyway" failure mode that affects Reply.io users.
- **BullMQ + Postgres** is debuggable. We can inspect a stuck enrollment with `SELECT * FROM sequence_enrollment WHERE id = ?` and follow up with `bullmq` CLI to inspect job state.

### Negative

- **OAuth maintenance burden.** Per-user Gmail and M365 OAuth tokens require refresh-token management, scope-grant tracking, and re-consent workflows when refresh tokens expire. The `gmail-watch-renew` and `graph-sub-renew` recurring jobs are non-trivial reliability surface.
- **Microsoft Graph TTL is volatile.** Subscription expiration depends on platform-version-dependent values that Microsoft has changed in the past. Renewal logic must be conservative (renew at 50% TTL minimum) and lifecycle-event-driven.
- **Pre-flight scoring (GlockApps) is a paid dependency.** $59–85/mo. Sits in the email-layer budget but is a hard cost, not free.
- **OOO date parsing is heuristic.** Some "back on" phrases don't parse; we default to +5 business days, which can frustrate enrollees who return earlier. Acceptable error rate for an SMB sequencer.
- **Brevo inbound parse plan tier is a TO RESOLVE.** If inbound parse requires a higher plan than budgeted, the catch-all path becomes more expensive. Mitigation: most clients will use per-user OAuth (the primary path), so the inbound parse is the secondary surface.

### Neutral

- The state machine is overkill for v0 (transactional only) and just-right for v1 (sequences). v2 adds branching ("if reply contains 'pricing', send template B; else send template A") on top of the same state model with no schema change.
- Telegram-style "respond from chatbot" handover (a v2 feature) attaches to the same `WAITING_REPLY` state — when the agent runtime ([ADR-005](ADR-005-ai-agent-tenancy.md)) classifies a reply and proposes a draft, it writes the draft as a `proposed_response` row keyed to the enrollment.

---

## Alternatives Considered

### Alt 1: IMAP IDLE for all reply detection

Rejected. (`docs/research/email-deliverability.md` §6.3.) IMAP IDLE requires one persistent TCP connection per monitored mailbox. At 20 clients × 5 users = 100+ connections. Gmail caps simultaneous IMAP at 15 per account; firewall/NAT idle timeouts disrupt long-lived connections. Without a connection-pool manager like EmailEngine ($), this is a reliability nightmare. Per-user OAuth (Pub/Sub for Gmail, Graph for M365) is the production-quality path; IMAP IDLE is the fallback only.

### Alt 2: FastText-trained classifier (Apollo's approach)

Rejected for v1. FastText delivers 99%+ OOO precision and 90% overall accuracy, but requires a labeled training dataset of email replies — which we don't have. At leCRM's reply volume (≤10k/month phase 3), the rules-based + Haiku approach achieves ~95% OOO precision and 90%+ overall classification at near-zero cost and zero training data. Reconsider FastText at >100k replies/month, when training data is plentiful and Haiku call cost approaches the FastText hosting cost. (`docs/research/email-deliverability.md` §6.5 option 3.)

### Alt 3: Gmail-only (skip M365)

Rejected. Microsoft 365 is at least as common as Gmail Workspace among French SMBs. Skipping M365 reply detection would force a chunk of clients onto the Brevo inbound parse path (which works but loses the per-user threading fidelity). Worth the implementation cost.

### Alt 4: Bridge to Reply.io permanently (skip native engine)

Rejected. Reply.io's reply detection is opaque and we have repeatedly observed sequence steps continuing to send after a clear reply (the "Reply.io kept sending" failure mode mentioned above). The strategic moat for leCRM is UI freedom and AI-native integration with the CRM; sequencing on Reply.io's terms gives that up. v0 bridge is a tactical shortcut; v1 native is the strategic foundation. (`docs/STRATEGIC-OVERVIEW.md` §4 strategic moat framing.)

### Alt 5: Branchless sequences (no reply-aware branching, just timed sends)

Rejected. Reply detection is the deliverability/sales-rep value-add. A sequencer that doesn't halt on reply is annoying and unprofessional; HubSpot Sales Hub Pro and Reply.io both halt on reply and clients expect this baseline.

---

## References

- `docs/research/email-deliverability.md` (entire document; §6 reply detection architecture, §6.5 OOO classifier, §4 bounce handling, §7 list hygiene).
- [Gmail API push notifications](https://developers.google.com/workspace/gmail/api/guides/push).
- [Gmail `users.watch()` reference](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users/watch).
- [Microsoft Graph change notifications — webhooks](https://learn.microsoft.com/en-us/graph/change-notifications-delivery-webhooks).
- [Microsoft Graph lifecycle events](https://learn.microsoft.com/en-us/graph/change-notifications-lifecycle-events).
- [ImapFlow](https://imapflow.com/) (fallback path).
- [Apollo tech blog — email reply classification](https://www.apollo.io/tech-blog/email-reply-classification-done-right) (FastText alternative reasoning).
- [Brevo inbound parse webhooks](https://developers.brevo.com/docs/inbound-parse-webhooks).
- Related ADRs: [ADR-003](ADR-003-email-provider-brevo.md) (Brevo as the outbound + inbound provider), [ADR-005](ADR-005-ai-agent-tenancy.md) (Haiku for OOO classification reuses the same per-tenant cost ledger and prompt caching), [ADR-007](ADR-007-encryption-secrets-audit.md) (OAuth refresh tokens are per-tenant secrets).

---

## TO RESOLVE

1. **Microsoft Graph subscription max TTL for `me/messages`.** Was previously 4230 minutes; Microsoft has changed this. Confirm current value before launch and design renewal at 50% of confirmed TTL. (`docs/research/email-deliverability.md` §470 item 4.)
2. **Brevo inbound parse plan tier — non-blocking.** Inherited TO RESOLVE from [ADR-003 §TO RESOLVE item 1](ADR-003-email-provider-brevo.md). Affects only the secondary catch-all reply path, not the primary Gmail/Graph OAuth path which covers >95% of EU SMB accounts. Fallback hierarchy if Brevo inbound parse is uneconomic: drop the secondary path in v0/v1, or self-host Postfix → webhook, or use Mailjet inbound parse as a single-feature second vendor. See ADR-003 §TO RESOLVE item 1 for the full decision tree.
3. **OAuth scope minimization.** Validate that `gmail.readonly` (rather than `gmail.modify`) is sufficient for the watch + history.list flow. Same for Graph `Mail.Read` vs `Mail.ReadWrite`. Smaller scopes ease the OAuth consent screen friction.
4. **Reply window expiration policy.** The `WAITING_REPLY → STEP_PENDING` transition fires on reply-window expiry. Decide whether the window is fixed (e.g., 5 days) or per-step-configurable. v1 default: 5 days; v1.1 per-step.
5. **Bounce-with-DSN parsing.** Brevo's webhook carries the raw SMTP code/message but some DSN bounces (especially soft bounces with informative bodies) are richer than what the webhook surfaces. Consider parsing the full DSN for better OOO detection on bounces. Defer to v2.
6. **Per-tenant volume cap defaults.** The phase-based defaults (5k / 15k / 30k) are placeholders. Validate against actual paying-client usage in v0–v1 and adjust.
7. **Suppression sharing across workspaces.** A hard bounce on `john@bigco.fr` in workspace A is not currently visible in workspace B. Decide whether leCRM provides a cross-workspace deny-list (privacy concerns: shared list reveals other clients' contacts) or remains strictly per-tenant. Default v1: per-tenant only.
