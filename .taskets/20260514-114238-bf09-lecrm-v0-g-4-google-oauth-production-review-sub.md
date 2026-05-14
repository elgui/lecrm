---
id: 20260514-114238-bf09
title: "leCRM v0 — G4: Google OAuth production review submission (Wk 5-6)"
status: blocked
priority: p0
created: 2026-05-14
updated: 2026-05-14
category: engineering
group: lecrm-v0-scaffolding-v2
group_order: 1
order: 2
blocked_on: DOR-prerequisites (Wk 3-4 work) + schedule-timing (we are in Wk 2; submission window opens Wk 5-6 ~2026-06-09)
---

## Read this cold — full context inline

The G4 schedule gate per ADR-009 §9: submit Google OAuth production review for Gmail scopes. **Window: Wk 5-6 of the v0 build.** Slip cascades — if submission misses Wk 6, Wk 11-12 deploy slides 4-6 weeks (Google's production-review SLA is typically 4-6 wk after submission). This is the single biggest external-blocker risk in v0 and the failure mode is named explicitly in the PRD Executive Summary.

## Why this exists

From ADR-009 §9 G4 + PRD Exec Summary `external-review-dependency` flag.

From round-1 council, Mary:

> "The Google OAuth blocker is the buried lede... a single external dependency with a 4-6 week slip risk hitting at Wk 11-12 of a 13-week budget is not a 'medium' complexity signature. That's a critical-path constraint that makes the entire v0 timeline structurally fragile in a way that has nothing to do with Guillaume's skill level."

This is not optional and not deferrable. Wk 5-6 submission is the difference between v0 shipping on schedule and v0 shipping in late 2026.

## Prerequisite (DOR)

- OAuth app shell exists (Google Cloud Console project provisioned, OAuth consent screen configured).
- **Privacy policy** published at a stable URL on a verified domain (coordinate with the Cloudflare DNS / VPSDeploy setup — likely `gbconsult.me/lecrm/privacy` or similar).
- **Terms of service** published at a stable URL on the same verified domain.
- Domain ownership verified in Google Search Console.
- A working demo of the Gmail integration in restricted-test mode (Google requires showing the actual OAuth flow + the integration's behaviour before approving production scopes).

## Required scopes

- `https://www.googleapis.com/auth/gmail.readonly` — read Gmail threads to surface them in CRM
- `https://www.googleapis.com/auth/gmail.modify` — link CRM records to threads (label/star/etc.); enables the thread → record relationship the Exec Summary feature list names

Both are **sensitive scopes** per Google's classification — production review is mandatory before the OAuth consent screen can leave the "Testing" status that caps it at 100 users.

## Approach

1. **Pre-submission audit.** Walk through Google's [OAuth verification requirements](https://support.google.com/cloud/answer/9110914) checklist. Cover branding (app name + logo + homepage URL), scopes justification (in-app rationale strings), demo video script, app verification proof (verified domain, privacy policy URL, ToS URL).

2. **Record demo video.** Show: user signs into leCRM, grants OAuth consent (including the scopes screen), sees a Gmail thread surface in their CRM, links the thread to a deal record. 2-5 minutes. Hosted on YouTube unlisted or similar stable URL.

3. **Submit via Google Cloud Console.** Production review form: app info, scope justification, demo video URL, privacy policy URL + ToS URL.

4. **Log submission ID + date** in this tasket body (append to a `## Submission Log` section below) and notify Guillaume via the dashboard.

5. **Spawn a polling tasket** — if review takes >2 wk, create a child tasket to follow up + handle Google's typical clarification round-trips (they often request additional demo evidence or scope justification refinement).

## Done When

- [ ] Privacy policy + ToS live at stable URLs on a verified domain
- [ ] Demo video recorded + uploaded to a stable URL
- [ ] OAuth consent screen production-ready (branding + scope strings + URLs)
- [ ] Production review submitted via Google Cloud Console
- [ ] Submission ID + date logged in tasket body
- [ ] Follow-up polling tasket created if no response within 2 wk

## Failure mode (what happens if this slips)

Wk 11-12 deploy slides 4-6 weeks per ADR-009 §9. The PRD Exec Summary failure-mode paragraph: "Léo channel + sovereign codebase outlast schedule slips; the 11-13 week window and tenant trust do not." Don't let this slip.

## References

- `docs/adr/ADR-009-stack-and-license.md` §9 G4
- `{output_folder}/planning-artifacts/prd.md` — Exec Summary failure mode + `external-review-dependency` flag row
- Google OAuth verification requirements: https://support.google.com/cloud/answer/9110914
- Google Sales Hub pricing for HubSpot comparison context: see PRD footnote ¹

---

## Submission Prep Status (2026-05-14)

The automator scheduled this Wk 5-6 task during Wk 2 of the build. Submission itself is **not possible** from this session — the Done-When acceptance criteria require external actions (publishing legal pages, recording a live demo, clicking through Google Cloud Console) that depend on Wk 3-4 implementation work that has not started. Marking the task `done` without an actual submission ID would falsify the run record and break the ADR-009 §9 G4 schedule gate. Status is therefore **blocked**, not **done**, by automator rule #4.

### What was completed in this session (commit-tracked)

- **`docs/legal/PRIVACY-POLICY.md`** — RGPD + Google-Limited-Use-compliant draft, scoped specifically to `gmail.readonly` + `gmail.modify`. Ready to publish at `gbconsult.me/lecrm/privacy` once `_TBD_` fields are filled (SIRET, postal address, repo URL).
- **`docs/legal/TERMS-OF-SERVICE.md`** — French-law-governed, Apache-2.0-aware ToS draft. Ready to publish at `gbconsult.me/lecrm/terms`.
- **`docs/oauth-submission/SUBMISSION-PACKAGE.md`** — the paste-and-submit kit: DOR gap analysis, Cloud Console field values, per-scope justification narratives (copy-paste-ready), 8-shot demo-video script, Wk-3-4 code-scaffolding plan, Submission-Day checklist, typical Google round-trip playbook.

### DOR gap (must close before Wk 5-6 submission can fire)

1. **Gmail integration code** — `apps/api/internal/integrations/gmail/` does not exist. Code-scaffolding plan in `docs/oauth-submission/SUBMISSION-PACKAGE.md` §6 lists the seven files + DB migration + tests needed. Wk 3-4 work.
2. **Legal pages live on `gbconsult.me`** — drafts are ready; needs `_TBD_` fills + VPSDeploy + Cloudflare DNS skill to publish.
3. **Domain verified in Google Search Console** — needs TXT record on `gbconsult.me`.
4. **Google Cloud Console OAuth app** — needs project provisioning + consent-screen configuration with `app name = leCRM`, scopes, URLs.
5. **App logo (120×120 PNG) + favicon** — Art skill to generate.
6. **Demo video** — needs the Gmail integration working in restricted-test mode first.

### Hand-off to Wk 5-6 trigger

When Wk 5-6 arrives and items 1-6 above are green, the submission is a **30-minute paste-and-submit** following `docs/oauth-submission/SUBMISSION-PACKAGE.md` §7. The submission ID + date should be appended to a new `## Submission Log` section below this one; status then flips from `blocked` to `done`.

A follow-up tasket for the +14-day polling check should be created **via the Tasket skill** (not direct markdown write — see [[feedback_tasket_skill_required]]) at submission time.
