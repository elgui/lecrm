# ADR-004 rev 2 — Sequences Architecture (v1 Native, Go + river)

**Status:** Accepted
**Date:** 2026-05-28
**Deciders:** Guillaume
**Supersedes:** [ADR-004 rev 1](ADR-004-sequences-architecture.md) (NestJS + BullMQ runtime, accepted 2026-05-10).
**Related:** [ADR-003](ADR-003-email-provider-brevo.md) (Brevo outbound + inbound parse). [ADR-007](ADR-007-encryption-secrets-audit.md) §3 (audit catalogue, retention classes). [ADR-009](ADR-009-stack-and-license.md) §2 (schema-per-tenant), §8.3 (river tenancy), §9 (Gmail-first scope cut). [ADR-011](ADR-011-external-system-sync.md) (connector seam — sequences emit connector-attributed activities through this boundary).

---

## Context

ADR-004 rev 1 (2026-05-10) described the native sequences engine against the NestJS + BullMQ + Redis stack. ADR-009 (2026-05-10) replaced that runtime with **Go 1.23+ / sqlc / [river](https://riverqueue.com/) / Postgres-only at v1**. The architectural intent of rev 1 survives:

- Durable Postgres state machine — DB is the source of truth, not worker memory.
- Reply correlation on RFC 5322 `Message-ID` / `In-Reply-To` / `References`.
- Three reply-detection paths (per-user OAuth primary, Brevo inbound parse secondary, IMAP fallback).
- Two-stage OOO classifier (rules → Haiku).
- Suppression list as single source of truth.

What changes in rev 2:

- **Job runtime**: BullMQ → river (Postgres-native; no Redis at v1).
- **Job tenancy**: per-workspace `river_<workspace_base36>` schema (ADR-009 §8.3) replaces BullMQ's per-tenant queue-prefix scheme.
- **DB access**: TypeORM entities → sqlc-typed queries against per-workspace schemas; `search_path` is set at the role level (ADR-009 §2.2), not per query.
- **Scope at v1**: Gmail-only per ADR-009 §9; Microsoft Graph reply-detection deferred to v1.1. The catch-all Brevo inbound parse remains the secondary path for clients on a generic `replies.<client-domain>` mailbox.
- **Audit attribution**: state transitions emit `sequences.*` events tagged with `actor_type = internal_service` (the claim shape from ADR-009 §4.1; the audit-log row is per ADR-007 §3).
- **Connector boundary**: sequences-produced inbox events flow through the `sync.Provider` seam (ADR-011) where they generate timeline activities, so the actor / source-system fields match other connectors.

What does **not** change:

- Per-tenant volume caps, throttles, suppression policy, OOO behavior, reply-window expiry semantics — all unchanged from rev 1 §5 / §6 except as noted.
- The Brevo dependency, MX delegation pattern, and webhook event set are unchanged (ADR-003).

---

## Decision

### 1. Schema (per-workspace, provisioned by `core.lecrm_provision_workspace`)

Two tables live in each `workspace_<id>` schema. Atlas migration appends two steps to the provisioning function (after the ADR-011 steps 11–12, so steps 13–14 here):

```sql
CREATE TYPE enrollment_state AS ENUM (
  'enrolled',
  'step_sent',
  'waiting_reply',
  'reply_received',
  'ooo_detected',
  'failed',
  'bounced',
  'unsubscribed',
  'suppressed',
  'completed'
);

CREATE TYPE step_send_state AS ENUM (
  'pending',
  'sent',
  'delivered',
  'bounced',
  'cancelled',
  'superseded'  -- replaced by a retry attempt
);

CREATE TABLE enrollments (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sequence_id         uuid NOT NULL,
  contact_id          uuid NOT NULL,
  state               enrollment_state NOT NULL DEFAULT 'enrolled',
  current_step_index  smallint NOT NULL DEFAULT 0,
  enrolled_at         timestamptz NOT NULL DEFAULT now(),
  next_action_at      timestamptz,
  last_transition_at  timestamptz NOT NULL DEFAULT now(),
  reply_message_id    text,             -- RFC 5322 Message-ID of the reply
  ooo_returns_at      timestamptz,
  created_by_user_id  uuid,             -- nullable: agent-enrolled rows are NULL
  workspace_id        uuid NOT NULL     -- redundant; keeps cross-schema queries safe
);

CREATE INDEX idx_enr_state_next ON enrollments (state, next_action_at)
  WHERE state IN ('enrolled','waiting_reply','ooo_detected');

CREATE TABLE enrollment_steps (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  enrollment_id       uuid NOT NULL REFERENCES enrollments(id) ON DELETE CASCADE,
  step_index          smallint NOT NULL,
  state               step_send_state NOT NULL DEFAULT 'pending',
  brevo_message_id    text,             -- provider message ID; populated after send
  rfc_message_id      text,             -- RFC 5322 Message-ID header we generated
  scheduled_for       timestamptz NOT NULL,
  sent_at             timestamptz,
  delivered_at        timestamptz,
  bounced_at          timestamptz,
  bounce_type         text,             -- 'hard' | 'soft' | NULL
  idempotency_key     text NOT NULL,    -- see §3 below
  created_at          timestamptz NOT NULL DEFAULT now()
);

-- Brandur-style partial unique index: only ONE active (non-superseded, non-cancelled)
-- row per (enrollment_id, step_index). Retries land as new rows; the prior attempt
-- is marked 'superseded' in the same transaction.
CREATE UNIQUE INDEX uniq_enrollment_step_active
  ON enrollment_steps (enrollment_id, step_index)
  WHERE state NOT IN ('cancelled', 'superseded');

CREATE INDEX idx_step_brevo_msgid ON enrollment_steps (brevo_message_id)
  WHERE brevo_message_id IS NOT NULL;

CREATE INDEX idx_step_rfc_msgid ON enrollment_steps (rfc_message_id)
  WHERE rfc_message_id IS NOT NULL;
```

The `email_suppression` table (rev 1 §5) is unchanged in shape but is now `CREATE TABLE IF NOT EXISTS`-idempotent and provisioned in the same Atlas step.

### 2. State machine

```
                                +-- (reply matched) ----------------> reply_received  (terminal*)
                                |
enrolled --(send_step job)--> step_sent --(reply detected) --> waiting_reply -+-- (OOO) ------> ooo_detected
                                |                                             |                    |
                                |                                             +-- (window expires) +-- (resume) --> enrolled (next step)
                                |
                                +-- (provider 5xx, max retries) ---> failed         (terminal)
                                +-- (hard bounce) ----------------> bounced         (terminal)
                                +-- (List-Unsubscribe / +complaint) unsubscribed   (terminal)
                                +-- (suppression hit pre-send) ---> suppressed     (terminal)
                                +-- (all steps complete, no reply) completed       (terminal)
```

`*reply_received` is terminal for the enrollment in v1. v2 branching ("if reply contains 'pricing' → template B") attaches at this state without schema change.

The tasket frames the state set as `ENROLLED → STEP_SENT → WAITING_REPLY → REPLY_RECEIVED | OOO_DETECTED | FAILED`. The richer set above keeps the rev-1 terminal states (`bounced`, `unsubscribed`, `suppressed`, `completed`) because each carries distinct retention and reporting semantics. The simplified vocabulary maps cleanly onto the richer set; no semantic gap.

Transitions are written through a single Go function `sequences.Transition(ctx, tx, enrollmentID, to, reason)`. Every transition:

1. Locks the enrollment row (`SELECT … FOR UPDATE`).
2. Validates `from → to` against an in-code transition table.
3. Updates `enrollments.state`, `last_transition_at`, and any side-effect columns (`reply_message_id`, `ooo_returns_at`, `next_action_at`).
4. Emits an `audit_log` row (see §6) **in the same transaction** (ADR-009 §7.2 fail-closed).
5. Enqueues the next river job if the transition implies one.

Invalid transitions are programming errors; they panic in dev/test and return 500 with `audit_log` `sequences.transition.invalid` in prod.

### 3. river jobs and idempotency

Four job types, all tenant-scoped per ADR-009 §8.3. River table lives in `river_<workspace_base36>`; the worker acquires a workspace-scoped pgxpool by connecting as `workspace_<id>` before executing.

| Job type | Args (IDs only — no PII) | Trigger | Retry policy | river `UniqueOpts` |
|---|---|---|---|---|
| `sequences.enroll` | `{contact_id, sequence_id}` | API call / agent action | exp backoff, 3 attempts | by-args (prevents double-enroll) |
| `sequences.send_step` | `{enrollment_id, step_index, idempotency_key}` | scheduled at `enrollments.next_action_at` | exp backoff, 5 attempts; final failure → `failed` | by-args on `(enrollment_id, step_index)` |
| `sequences.poll_reply` | `{enrollment_id}` | when entering `waiting_reply`; periodic re-check up to window expiry | exp backoff, 3 attempts | by-args |
| `sequences.finalize` | `{enrollment_id, terminal_state, reason}` | from `Transition` when entering a terminal state | exp backoff, 3 attempts | by-args + by-state (one finalize per enrollment) |

**Idempotency key shape.** For `sequences.send_step` the key is:

```
sha256(workspace_id || ':' || enrollment_id || ':' || step_index || ':' || attempt_epoch)
```

`attempt_epoch` is a counter incremented when a previous attempt was explicitly marked `superseded` (e.g., template edited mid-flight, user manually re-queued). It is **not** incremented on river-internal retries — those reuse the same key, and the partial unique index in §1 guarantees at most one active row.

**Brandur-style "at most once" guarantee.** The partial unique index `uniq_enrollment_step_active` is the durable backstop. River's `UniqueOpts{ByArgs: true}` is the in-queue backstop. Either alone is sufficient; the pair gives belt-and-braces protection across the application/queue boundary.

**river worker contract.** Every job handler begins with:

```go
ctx, tx, release, err := workspaceCtx.AcquireTx(ctx, args.WorkspaceID)
// ...
defer release()
// all DB work inside tx; audit_log writes go through the same tx
```

This is the same pattern as ADR-011 sync workers — see `apps/api/internal/jobs/workspace.go`.

### 4. Reply detection (Gmail-first per ADR-009 §9)

**Primary path — Gmail Pub/Sub Watch (per-workspace OAuth).**

- Each connected workspace user (the rep sending from their mailbox) holds a Google OAuth grant with scopes `gmail.readonly` + `gmail.send` + `gmail.modify` (the last only if labelling is required). Refresh tokens encrypted with SOPS-age and stored at `secrets/oauth/gmail/<workspace_id>/<user_id>.yaml` per ADR-007.
- `users.watch()` registers Pub/Sub delivery to `projects/lecrm-prod/topics/gmail-inbox-events`. A single push subscription delivers events to `https://api.lecrm.fr/v1/webhooks/gmail/push`.
- The webhook handler validates the JWT (Google-signed), enqueues a `sequences.poll_reply` job keyed on the `email_address` → workspace+user resolution.
- Renewal: river periodic job `gmail.watch_renew` runs `0 4 * * *` per active connection (Gmail watch expires every 7 days; we renew daily for safety margin).

Reply correlation logic:

1. Worker calls `users.history.list(historyId=last_history_id)` to fetch new messages since the last poll.
2. For each new `INBOX` message, extract `In-Reply-To` and `References` headers.
3. Match against `enrollment_steps.rfc_message_id` (one query, indexed).
4. On match: `Transition(enrollment, waiting_reply → reply_received OR ooo_detected)` per the OOO classifier (§5).

**Secondary path — Brevo inbound parse (catch-all).**

For workspaces using a generic `replies.<workspace-slug>.lecrm.fr` reply-to:

- DNS: `replies.<slug>` MX 10 → `inbound1.sendinblue.com`, MX 20 → `inbound2.sendinblue.com` (ADR-003 §Decision).
- Outbound `Reply-To` set to `<enrollment_id>@replies.<slug>.lecrm.fr`. The local part is the correlation key; the enrollment_id is also opaque enough that scraping it does not leak anything useful.
- Brevo POSTs `inboundEmailProcessed` events to `https://api.lecrm.fr/v1/webhooks/brevo/inbound`.
- Handler lives at **`apps/api/internal/email/brevo/inbound.go`**:

  ```go
  func (h *InboundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
      if err := verifyHMAC(r, h.secret); err != nil {
          http.Error(w, "bad signature", http.StatusUnauthorized)
          return
      }
      var ev brevoInboundEvent
      _ = json.NewDecoder(r.Body).Decode(&ev)

      workspaceID, enrollmentID, ok := parseReplyAddress(ev.To)
      if !ok {
          // unrelated inbound — drop silently
          w.WriteHeader(http.StatusNoContent); return
      }

      _, err := h.river.InsertTx(r.Context(), h.tx, sequences.PollReplyArgs{
          WorkspaceID:  workspaceID,
          EnrollmentID: enrollmentID,
          Hint:         &sequences.InboundHint{
              From:           ev.From,
              InReplyTo:      ev.InReplyTo,
              ExtractedText:  ev.ExtractedMarkdownMessage,
              SpamScore:      ev.SpamScore,
              ProviderMsgID:  ev.MessageID,
          },
      }, nil)
      // ...
  }
  ```

- HMAC signing secret rotated quarterly per ADR-007 §Rotation cadence; secret in SOPS at `secrets/webhooks/brevo.yaml`.

**Fallback path — IMAP IDLE.** Deferred. Not required at v1 per ADR-009 §9 (target market is Gmail Workspace dominated). The `sync.Provider` interface from ADR-011 can later host an `imap.Provider` implementation if a paying client demands a non-Gmail, non-Brevo-inbound mailbox.

**Microsoft Graph.** Deferred to v1.1 per ADR-009 §9. The reply-detection interface (`sequences.replyDetector`) admits a Graph implementation without state-machine changes.

### 5. OOO classifier (unchanged from rev 1 §4, ported to Go)

Two stages, identical logic to rev 1:

1. **Rules pre-filter** — French + English regex set, ~95% OOO precision at zero cost. Implemented in `apps/api/internal/sequences/ooo/rules.go` with a frozen unit-test fixture set (~120 anonymised reply samples from research dataset).
2. **Haiku classifier for ambiguous cases** — `claude-haiku-4-5-20251001` (per ADR-005 model selection), prompt cached. Cost ceiling ~$0.50/month at 10k replies.

Return-date extraction (the "de retour le 15 mai" parsing) lives in `ooo/dateparse.go`. Unparseable dates → reschedule `+5 business days` per rev 1 default.

The OOO classifier is the most-likely-to-be-rewritten module in v2 (FastText, fine-tuned small model, etc.). Its interface is therefore a one-method `Classify(ctx, ReplyBody) (Category, Confidence, OOOReturnDate, error)` — swappable without touching the state machine.

### 6. Audit emission per state transition

Every `Transition` call emits one `audit_log` row in the same transaction. New event types extending ADR-007 §3 catalogue:

| Event | Fields | Retention class |
|---|---|---|
| `sequences.enrolled` | `enrollment_id`, `sequence_id`, `contact_id`, `created_by_user_id` (nullable) | data (3y) |
| `sequences.step_sent` | `enrollment_id`, `step_index`, `brevo_message_id`, `rfc_message_id` | data (3y) |
| `sequences.reply_received` | `enrollment_id`, `step_index`, `reply_message_id`, `classifier_category`, `classifier_confidence`, `detector` (`gmail_push` \| `brevo_inbound`) | data (3y) |
| `sequences.ooo_detected` | as above + `ooo_returns_at` (nullable) | data (3y) |
| `sequences.failed` | `enrollment_id`, `step_index`, `error`, `attempts` | data (3y) |
| `sequences.bounced` | `enrollment_id`, `email`, `bounce_type`, `smtp_code` | data (3y) |
| `sequences.unsubscribed` | `enrollment_id`, `email`, `source` (`list_unsubscribe` \| `complaint` \| `manual`) | data (3y) |
| `sequences.transition.invalid` | `enrollment_id`, `from`, `to_attempted`, `caller` | auth (1y) — programming-error trace |

**Actor attribution.** Every row carries:

- `actor_user_id` — the human who initiated the enrollment, NULL for agent-initiated.
- `actor_type` (ADR-009 §4.1 claim) — `human_api` for UI-driven enrolment, `mcp_agent` for agent-driven, `internal_service` for system-emitted transitions (e.g., a bounce webhook firing `sequences.bounced`). The audit_log table does not currently have an `actor_type` column; **TO RESOLVE-S1** below: add it via the ADR-007 follow-up migration before v1 sequences ship.

### 7. Connector-boundary alignment (ADR-011)

Reply events surfaced in the UI timeline are written as `activities` rows through the same `sync.Provider`-aligned path used by Gmail-import. Specifically:

- `sequences.reply_received` and `sequences.ooo_detected` insert one row into `activities` with:
  - `source_system = 'sequences'` (a new ProviderID alongside `gmail`, `brevo`, `shopify`);
  - `external_id = <reply_message_id>`;
  - `actor_type = 'internal_service'`;
  - `kind = 'email_reply'` or `'email_ooo'`.
- The `external_entity_mappings` table (ADR-011 §4) keys the reply back to the originating `enrollment_steps.id`.

This means the **same timeline-rendering code** displays sequences-detected replies and Gmail-sync-imported emails. No special-case UI path.

`sync.Provider` is not implemented for the sequences engine — sequences are an internal producer, not an external connector — but the **emit shape** (ProviderID, source_system, actor_type) matches so the consumer side is uniform.

### 8. Volume caps, throttles, suppression — unchanged from rev 1

Carry-over from rev 1 §5 and §6 with no semantic change:

- Per-tenant `monthly_send_cap` enforced at the `sequences.send_step` worker entry.
- Per-recipient throttle: no more than 1 step per 24h to the same `contact_id`.
- Suppression pre-send check before every Brevo call; row in `email_suppression` → transition to `suppressed`.
- Soft-bounce policy: suppress after 3 consecutive soft bounces.
- Hard bounce / complaint → immediate suppression row.

The implementation moves from BullMQ pre-handler middleware to a Go function `sequences.preflight(ctx, tx, enrollmentID, stepIndex)` called inside the `send_step` job before any Brevo API call. All checks share the same workspace-scoped transaction.

---

## Open questions

### Q1 — OOO classifier baseline (rules vs ML)

Rev 1 chose rules + Haiku because no labelled dataset existed. At ~10k replies/month phase-3 volume, the Haiku spend is ~$0.50/mo and FastText hosting cost would exceed it. Reconsideration trigger: >100k replies/month sustained **or** measured OOO false-positive rate >5% on a labelled sample. v1 ships with rules + Haiku.

### Q2 — GlockApps preflight integration point

Rev 1 left this as a "$59–85/mo dependency, manual cadence." For v1, the integration question is: do we trigger GlockApps inbox-placement tests automatically from the engine (e.g., before a campaign step deploys to >500 contacts) or remain manual / monthly? **Open.** Default for v1: manual, run via `ops/scripts/glockapps-preflight.sh`. Automated integration requires GlockApps' API tier, which we have not yet costed.

### Q3 — Suppression-list propagation across workspaces

Rev 1 §TO RESOLVE-7: a hard bounce on `john@bigco.fr` in workspace A is not visible in workspace B. Cross-workspace deny-list raises a privacy concern (the list reveals other clients' contact universes). Default for v1: **per-workspace only.** A v2 mechanism could share *only* hashed-email + bounce-type rows in a separate sovereign deny-list, but the privacy review is non-trivial and not v1-critical.

### Q4 — Brevo inbound parse plan tier (inherited from ADR-003 TO RESOLVE-1)

Non-blocking. If inbound parse turns out to be Enterprise-only, the secondary catch-all path is deferred and sequences ship with Gmail-only reply detection at v1. ~95% of SMB sequences are sent from the rep's own mailbox, so the Gmail-only path covers the bulk of the use case. Tracked separately in `.taskets/20260528-142628-2702-*`.

### Q5 — Reply window expiration policy

Rev 1 chose 5-day fixed window. v1 ships the same. Per-step-configurable windows deferred to v1.1.

### Q6 — Audit `actor_type` column migration

The `audit_log` schema in ADR-007 §3 does not include an `actor_type` column today; the ADR-009 §4.1 claim is captured only at the request edge. **Add the column** (`actor_type text NOT NULL DEFAULT 'human_api'`) in the same Atlas migration that lands the sequences tables — sequenced-engine emissions are the first hard requirement for it. ADR-007 follow-up TO RESOLVE-14 covers this; verify it lands before merging the sequences package.

---

## Consequences

### Positive

- **Single-runtime simplification.** No Redis at v1 (ADR-009 §8.3 + this ADR). One less daemon, one less backup target, one less failure mode.
- **Per-workspace river schemas** mean a worker that mis-routes a job cannot touch another tenant's data — the Postgres role barrier catches it.
- **Brandur-style partial unique index + river UniqueOpts** gives belt-and-braces at-most-once on `send_step`. Either alone would do; both is cheap and reassuring before a paying client.
- **Audit on the same transaction as the state change.** No "step sent but audit lost" failure mode. Aligns with ADR-009 §7.2 fail-closed mutation policy.
- **Connector-aligned activity emission.** Sequences-detected replies render through the same timeline path as Gmail-imported emails. No bespoke UI plumbing.
- **State machine is debuggable from a SQL prompt.** `SELECT * FROM enrollments WHERE state IN ('enrolled','waiting_reply')` is the live operational view.

### Negative

- **river is younger than Sidekiq-class systems** (rev 1 had BullMQ which is itself young). Phase-3 throughput may demand a different queue. Not v1-blocking; the four job types are small enough to port.
- **`actor_type` column gap** (Q6) is a pre-req migration that must land before sequences code merges.
- **No Microsoft Graph at v1.** Workspaces with M365-mailbox reps must use the Brevo inbound parse catch-all reply-to. UX gap, not a feature loss for the v1 Gmail-first ICP.
- **Idempotency-key derivation** is application-level — a buggy key generator could cause double-sends. Mitigated by the partial unique index (DB-level final word) and a unit-test fixture (`sequences/idempotency_test.go`) that asserts the key formula across edge cases.
- **`sequences.transition.invalid` is a panic in dev.** Intentional — invalid transitions are bugs, not runtime conditions — but it does mean a single rogue caller can crash a worker. Acceptable: river will restart the worker, the bad call will be in the trace.

### Neutral

- The state machine vocabulary (`enrolled / step_sent / waiting_reply / reply_received / ooo_detected / failed / bounced / unsubscribed / suppressed / completed`) is richer than the tasket's six-state framing. The two map cleanly; the richer set keeps rev-1 reporting semantics.
- Suppression sharing across workspaces, per-step reply window, and GlockApps API integration are all deferred to v1.1 or v2 — explicit decisions, not omissions.
- The `imap.Provider` fallback can be added later via ADR-011's `sync.Provider` seam without ADR-004 changes.

---

## References

- [ADR-003](ADR-003-email-provider-brevo.md) — Brevo as outbound + inbound provider; webhook signing.
- [ADR-007](ADR-007-encryption-secrets-audit.md) §3 — audit_log schema, retention classes, fail-closed semantics.
- [ADR-009](ADR-009-stack-and-license.md) §2 (schema-per-tenant), §4.1 (`actor_type` claim), §7.2 (audit fail-closed), §8.3 (river tenancy), §9 (Gmail-first scope).
- [ADR-011](ADR-011-external-system-sync.md) — `sync.Provider` seam; activity emission shape (ProviderID, source_system, actor_type).
- [ADR-004 rev 1](ADR-004-sequences-architecture.md) — superseded by this ADR; retained as historical context for the NestJS+BullMQ design.
- `.taskets/20260510-162158-aa6f-lecrm-v1-native-sequences-engine-track-f-post-firs.md` — the v1 build tasket that consumes this ADR.
- [river — Postgres-native job queue for Go](https://riverqueue.com/).
- [Brandur Leach — Idempotency keys](https://brandur.org/idempotency-keys) — partial-unique-index pattern.
- [Gmail API push notifications](https://developers.google.com/workspace/gmail/api/guides/push).
- [Brevo inbound parse webhooks](https://developers.brevo.com/docs/inbound-parse-webhooks).

---

## TO RESOLVE

- **S1.** Add `actor_type text NOT NULL DEFAULT 'human_api'` to `audit_log` in the same Atlas migration that lands `enrollments` / `enrollment_steps`. Cross-reference ADR-007 follow-up TO RESOLVE-14.
- **S2.** Confirm Brevo plan-tier for `inboundEmailProcessed` (covered by tasket `20260528-142628-2702`). If Enterprise-only, document the secondary-path deferral in `docs/STRATEGIC-OVERVIEW.md`.
- **S3.** OAuth scope minimisation — verify that `gmail.readonly` + `gmail.send` (no `gmail.modify`) is sufficient for the watch+history+send flow. Smaller scope eases the Google OAuth review (tasket `20260514-114238-bf09`).
- **S4.** Add the `sequences.*` event set to the audit-log fixture covered by ADR-007 §pen-test cadence. Pentester voice required these be exercised before v1 cut.
- **S5.** Decide GlockApps integration tier (manual vs API) once first paying client volume is known.
