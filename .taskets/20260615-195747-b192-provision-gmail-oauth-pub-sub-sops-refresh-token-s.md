---
id: 20260615-195747-b192
title: Provision Gmail OAuth + Pub/Sub + SOPS refresh token (STAGING) — clear deferred v1 reply-detection gate (5078)
status: next
priority: p2
created: 2026-06-15
updated: 2026-06-15
tags: [sequences, v1, gmail, oauth, pubsub, sops, gate, deferred, human]
category: project
---

## Why

The ord-5 human gate of supervised run `ga-20260615-d100ed` (tasket `20260614-154815-5078`, "Gmail Pub/Sub Watch reply detection") was **consciously deferred** on 2026-06-15 (Guillaume's decision via the supervisor: *unblock order:6 code, defer infra*). The build worker already shipped the setup runbook + SOPS scaffold on branch `auto/lecrm-v1-build-20260615`. This tasket tracks the **real external infra** that must be provisioned **before go-live**. Target is **STAGING** (`demo.lecrm.gbconsult.me`), NOT prod — adjust the runbook's hardcoded prod values (`lecrm-prod` / `api.lecrm.fr`).

## Constraints (human-only — do NOT auto-dispatch)

- Must be done at **Guillaume's workstation**: `gcloud`/`sops`/`age` are NOT on the staging box `51.77.146.49`, and the **operator age key** (to encrypt the refresh token) lives on the workstation/YubiKey, not that box. Needs an interactive **Google sign-in in a browser**.
- Human-only: if ever grouped, flag `gate: human` + `role: epic` so it is never autonomously dispatched (the original gate timed out exactly because a worker cannot do this).

## Steps (staging-adapted from `ops/runbooks/gmail-oauth-pubsub-setup.md`)

1. **Bootstrap the age recipient**: run `ops/secrets/bootstrap.sh` to patch the operator age public key into `ops/secrets/.sops.yaml` — it currently holds the `REPLACE_WITH_AGE_PUBLIC_KEY` placeholder for the `secrets/oauth/gmail/...` rule, so every `sops --encrypt` fails until this is done.
2. **§1 GCP (staging project)**: enable `gmail.googleapis.com` + `pubsub.googleapis.com`; create topic `gmail-inbox-events`; IAM-bind `gmail-api-push@system.gserviceaccount.com` as `roles/pubsub.publisher`; create push subscription `gmail-inbox-push` → `https://demo.lecrm.gbconsult.me/v1/webhooks/gmail/push` with an OIDC push-auth service account (record its email for order:6 config).
3. **§2 OAuth grant**: consent screen + Web OAuth client; minimal scopes `gmail.readonly` + `gmail.send` (add `gmail.modify` ONLY if label/archive needed — bf09 scope-min review); offline auth-code flow (`access_type=offline`, `prompt=consent`) for one staging rep mailbox; capture `refresh_token`; fill `secrets/oauth/gmail/<ws>/<user>.yaml` from `_template`; `sops --encrypt --in-place`; rename to `<user>.enc.yaml`; commit ONLY the `.enc.yaml`.
4. **§4 verify** the 4 gate checks: topic describe; IAM policy shows `gmail-api-push` publisher; subscription `pushEndpoint` + `oidcToken` SA; `.enc.yaml` decrypts (`sops -d`) with a non-empty refresh token + minimal scopes.
5. **§3 `users.watch()`** — run ONLY after order:6 (`5b07`) is deployed; the daily `gmail.watch_renew` job (`0 4 * * *`) keeps the 7-day watch alive.

## Done when

- Staging Gmail OAuth grant encrypted at `secrets/oauth/gmail/<ws>/<user>.enc.yaml` and decrypts cleanly.
- Pub/Sub topic + push subscription live on the staging GCP project; `gmail-api-push` is publisher.
- After order:6 deploys: a reply to a sent step is detected and transitions the enrollment within the reply window.

## References

- `ops/runbooks/gmail-oauth-pubsub-setup.md` (full procedure)
- Origin gate: tasket `20260614-154815-5078`; run `ga-20260615-d100ed` (ord 5, skipped/deferred)
- tasket `1023` (SOPS/age baseline), tasket `bf09` (OAuth review)
- ADR-004 rev2 §4/S3, ADR-007 §2, ADR-009 §9
