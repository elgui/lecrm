---
title: Google OAuth Production Review — Submission Package
status: prep complete; submission blocked until Wk 5-6 DOR satisfied
tasket: 20260514-114238-bf09 (G4 schedule gate)
adr: docs/adr/ADR-009-stack-and-license.md §9 G4
created: 2026-05-14
target-submission-window: Wk 5-6 of v0 build (~2026-06-09 to 2026-06-23, contingent on Wk-2 baseline)
---

# Google OAuth Production Review — Submission Package

This document is the **paste-and-submit kit** for the G4 schedule gate. Everything Google asks for at submission time is drafted below. The actual submission is blocked on the prerequisites in §1; once those are green, the submission itself is a 30-minute Cloud Console form fill.

## 1. DOR gap analysis (state as of 2026-05-14)

| Prerequisite | Current state | Blocker |
|--------------|---------------|---------|
| Google Cloud Console project + OAuth consent screen | ❌ Not created | Create at console.cloud.google.com; brand name "leCRM"; support email contact@gbconsult.me |
| Privacy policy published at stable URL on verified domain | ❌ Not published | Markdown draft ready at `docs/legal/PRIVACY-POLICY.md`. Publish to `gbconsult.me/lecrm/privacy` via VPSDeploy. Fill `_TBD_` placeholders (SIRET, postal address, repo URL) first. |
| Terms of service published | ❌ Not published | Markdown draft ready at `docs/legal/TERMS-OF-SERVICE.md`. Publish to `gbconsult.me/lecrm/terms`. Same `_TBD_` fills as above. |
| Domain ownership verified in Google Search Console | ❌ Not verified | Add TXT record `google-site-verification=<token>` to `gbconsult.me` via Cloudflare DNS skill; confirm at search.google.com/search-console |
| Gmail integration in restricted-test mode (real working flow) | ❌ Not built | Build Wk 3-4: Gmail OAuth handler in `apps/api/internal/integrations/gmail/`, thread-list endpoint, label-modify endpoint, UI wiring in `apps/web/`. See §6 for code-scaffolding plan. |
| Demo video recorded + uploaded to stable URL | ❌ Not recorded | Script ready in §5. Record after Gmail integration is functional. Upload as YouTube unlisted. |
| App branding (logo 120×120 PNG + 32×32 favicon) | ❌ Not created | Use Art skill (Tron-meets-Excalidraw aesthetic) to generate; place in `apps/web/public/`. |

**Critical-path implication:** This work cannot start until Wk 3 at the earliest (Wk-2 Go ramp just completed). The Wk 5-6 submission window is achievable **only if Wk 3-4 prioritises Gmail integration + legal page publication + demo recording**. Any slip in Wk 3-4 cascades to the Wk 11-12 deploy date per ADR-009 §9.

## 2. App basic info (Google Cloud Console → OAuth consent screen)

| Field | Value |
|-------|-------|
| App name | **leCRM** |
| User support email | **contact@gbconsult.me** |
| App logo | _120×120 PNG, place at `apps/web/public/logo-120.png`_ |
| Application homepage | **https://gbconsult.me/lecrm** |
| Privacy policy URL | **https://gbconsult.me/lecrm/privacy** |
| Terms of service URL | **https://gbconsult.me/lecrm/terms** |
| Authorised domains | `gbconsult.me` |
| Developer contact email | **guillaume@gbconsult.me** |
| User type | **External** (production), **In production** once approved |

## 3. Scopes requested

Two **sensitive scopes** (require production review):

### 3.1 `https://www.googleapis.com/auth/gmail.readonly`

**In-app description (paste verbatim into the Cloud Console scope justification field):**

> leCRM surfaces a user's Gmail conversations alongside their CRM records (contacts, deals, companies). The `gmail.readonly` scope is the minimum scope that allows our application to:
>
> 1. List threads in the authenticated user's mailbox to populate the in-app email view next to a CRM record.
> 2. Fetch individual message metadata (From, To, Subject, Date headers, labels) so users can see which conversations relate to which contacts.
> 3. Fetch full message bodies on user demand to display the conversation in context.
>
> Without this scope, the email-thread feature — the core integration described to users at the consent screen and at https://gbconsult.me/lecrm — cannot function. We do not request `gmail.metadata` only because users explicitly need to read the body of the message to update CRM context (next-step, deal status). We do not persist message bodies; metadata persistence is documented in our Privacy Policy §1.3.

### 3.2 `https://www.googleapis.com/auth/gmail.modify`

**In-app description (paste verbatim):**

> The `gmail.modify` scope is requested to support **CRM linkage labels** — when a user links a Gmail thread to a leCRM deal or contact, our application applies a Gmail label (`leCRM/Deal-Linked` or `leCRM/Contact-Linked`) to the thread inside the user's Gmail account. This ensures the relationship between the thread and the CRM record is visible from inside Gmail (not just inside leCRM) and survives if the user later disconnects the integration.
>
> The narrower `gmail.labels` scope is insufficient because applying a label to a specific message requires the `messages.modify` capability covered by `gmail.modify`. We do not request `gmail.compose` or `gmail.send` — leCRM does not send mail on the user's behalf in v0.
>
> The user-facing flow is: user opens a deal → clicks "Link Gmail thread" → selects a thread → leCRM applies the `leCRM/Deal-Linked` label to that thread via the Gmail API. Without this scope the linkage is leCRM-only and breaks the user's expectation that their CRM tags appear in their email client.

### 3.3 Why we do **not** request larger scopes

We deliberately do not request: `gmail.send`, `gmail.compose`, `gmail.insert`, `gmail.settings.basic`, `gmail.settings.sharing`, or `https://mail.google.com/`. The v0 product does not need them. Including this disclosure in the justification narrative helps reviewers see we've practised data minimisation.

## 4. App verification proof

Attach during submission:

1. **Privacy policy URL** — https://gbconsult.me/lecrm/privacy (must resolve at submission time)
2. **Terms of service URL** — https://gbconsult.me/lecrm/terms (must resolve)
3. **Homepage URL** — https://gbconsult.me/lecrm (must clearly describe the product, link to privacy + ToS, and show the app's brand)
4. **Domain ownership** — verified in Google Search Console using a TXT record on `gbconsult.me`
5. **YouTube demo video** (unlisted) — see §5 for the script

## 5. Demo video script (target 2-3 minutes)

Google reviewers want to see:
1. The user begins the OAuth flow from inside the application (not from a test URL).
2. The OAuth consent screen displays the same scopes you're requesting.
3. The application uses those scopes to deliver the user-facing feature it claimed.
4. The branded app logo, name, and URL match the Cloud Console submission.

### Shot list

| # | Duration | Shot | Voice-over (English; Google reviewers are typically EN) |
|---|----------|------|---------------------------------------------------------|
| 1 | 0:00-0:15 | Browser navigates to **https://gbconsult.me/lecrm**, lands on homepage. Show brand, tagline, "Sign in" button. | "This is leCRM, a sovereign customer-relationship-management tool. To start, the user clicks Sign in." |
| 2 | 0:15-0:30 | Authentik login screen. User signs in with their workspace credentials. | "leCRM uses Authentik for primary authentication. The user signs in with their workspace account." |
| 3 | 0:30-0:50 | leCRM dashboard. User clicks Settings → Integrations → Gmail → "Connect Gmail". | "From settings, the user opens the Gmail integration panel and clicks Connect." |
| 4 | 0:50-1:20 | **The Google OAuth consent screen.** Frame this clearly. Show: leCRM app name + logo, the two scopes (read messages, modify messages), continue button. | "Google shows the consent screen. The two scopes we request are gmail.readonly — to display email threads next to CRM records — and gmail.modify — to apply CRM linkage labels back to Gmail. The user reviews and clicks Continue." |
| 5 | 1:20-1:45 | leCRM Inbox view. A list of Gmail threads populates from the user's account. User clicks one. The thread body displays inside leCRM. | "After consent, leCRM uses gmail.readonly to fetch the user's recent threads and display them alongside their CRM contacts. Here the user opens a thread and sees the full conversation." |
| 6 | 1:45-2:15 | User clicks "Link to deal" → selects deal "Acme Corp - Q3 renewal". Confirmation: "Linked. Label applied in Gmail." | "When the user links this thread to a CRM deal, leCRM uses gmail.modify to apply a 'leCRM/Deal-Linked' label to the thread inside the user's Gmail account, so the relationship is visible from either side." |
| 7 | 2:15-2:30 | Switch to actual Gmail (gmail.google.com) in another tab. Show the same thread now carries the `leCRM/Deal-Linked` label. | "Confirming inside Gmail: the thread now carries the leCRM/Deal-Linked label." |
| 8 | 2:30-2:45 | Back in leCRM → Settings → Disconnect Gmail. Confirmation dialog. | "The user can revoke leCRM's Gmail access at any time from settings. Disconnecting immediately invalidates the OAuth tokens and triggers deletion of cached metadata per our privacy policy." |

### Recording checklist

- [ ] Record at 1920×1080 minimum; Google rejects videos with unreadable text.
- [ ] Use a clean browser profile (no other tabs, no extensions in the toolbar).
- [ ] The Cloud Console submission must already have the app brand and scopes configured before recording, so the consent screen shows exactly what reviewers see in the submission.
- [ ] Caption the shots showing the scopes ("This is the gmail.readonly scope being granted").
- [ ] Upload as YouTube **Unlisted** (not Private — reviewers can't access Private videos).
- [ ] Verify the URL works in an incognito window before submitting.

## 6. Wk 3-4 code-scaffolding plan (must complete before Wk 5-6 submission)

Suggested module layout for `apps/api/internal/integrations/gmail/`:

```
apps/api/internal/integrations/gmail/
├── oauth.go          // OAuth flow handlers: /v1/integrations/gmail/connect, /callback
├── client.go         // Authenticated gmail.Service factory, token refresh
├── threads.go        // ListThreads, GetThread (uses gmail.readonly)
├── labels.go         // CreateLabel ("leCRM/Deal-Linked", "leCRM/Contact-Linked"), ApplyLabel (uses gmail.modify)
├── store.go          // Persisted message metadata: msg_id, thread_id, headers, label state, link to crm record
├── tokens.go         // Encrypted token storage (per ADR-007); refresh handling
└── handlers.go       // HTTP handlers: GET /v1/threads, POST /v1/threads/:id/link
```

Required DB additions (next migration `packages/db/migrations/0003_gmail.sql`):
- `gmail_accounts` (workspace_id, user_id, email, encrypted_refresh_token, scopes, connected_at)
- `gmail_threads` (workspace_id, thread_id, last_seen, label_state JSONB, linked_record_type, linked_record_id)
- `gmail_messages` (workspace_id, thread_id, message_id, from_header, to_header, subject, date, body_cached_at NULLABLE — body is fetch-on-demand)

Tests for the submission demo:
- Integration test for OAuth callback exchanging code for token.
- Integration test for label-apply round-trip (apply → verify via API).
- Build-tagged E2E test that drives a real Google OAuth flow (skipped in CI; runs locally with test credentials).

## 7. Submission Day checklist (Wk 5-6, when DOR is met)

1. [ ] All §1 prerequisites green.
2. [ ] Cloud Console → OAuth consent screen → confirm all fields in §2 are populated.
3. [ ] Cloud Console → Scopes → confirm `gmail.readonly` + `gmail.modify` are listed.
4. [ ] Paste §3.1 and §3.2 justifications into the per-scope "How will the scope be used?" field.
5. [ ] Paste demo video URL into the YouTube field.
6. [ ] Confirm privacy + ToS URLs resolve in an incognito window.
7. [ ] Click **Prepare for verification** → **Submit for verification**.
8. [ ] Save the verification request ID Google returns.
9. [ ] Open the tasket file (`.taskets/20260514-114238-bf09-*`) and append a `## Submission Log` section: submission date, verification request ID, expected SLA window.
10. [ ] Create a follow-up tasket via the Tasket skill (NOT direct file write — `Tasket` is required per memory) titled "leCRM v0 — G4 follow-up: Google OAuth review polling" with status `later` and a 14-day deferred review trigger.

## 8. Typical Google round-trip

Google's review process commonly returns clarification requests within 7-14 days. Be prepared for:

- **"Demonstrate exactly what the scope does in the video, at timestamps X-Y."** — re-shoot the relevant clip or annotate the existing video with text overlays.
- **"Why is `gmail.modify` needed instead of `gmail.labels`?"** — paste the §3.2 narrative; reviewers occasionally ask the same question that's already in the justification.
- **"Privacy policy doesn't mention Limited Use of Google API Services User Data."** — our §3 of PRIVACY-POLICY.md addresses this verbatim per [Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy); cite the URL of the published policy.
- **"Logo doesn't match Cloud Console + app."** — re-upload identical 120×120 PNG everywhere.

Reply to each clarification within 24-48 hours. Slow replies extend the timeline more than additional rounds.

## 9. References

- [Google OAuth verification requirements](https://support.google.com/cloud/answer/9110914) — primary checklist.
- [Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy) — Limited Use clause; cite from privacy policy.
- [CASA (Cloud Application Security Assessment)](https://developers.google.com/cloud-security-assessment) — required for some restricted scopes; **not required** for `gmail.readonly` + `gmail.modify` (sensitive, not restricted). Confirm at submission time in case classifications have changed.
- `docs/adr/ADR-009-stack-and-license.md` §9 G4 — the schedule gate.
- `docs/legal/PRIVACY-POLICY.md` — companion draft.
- `docs/legal/TERMS-OF-SERVICE.md` — companion draft.
- `.taskets/20260514-114238-bf09-lecrm-v0-g-4-google-oauth-production-review-sub.md` — the tasket.
