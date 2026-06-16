# Gmail reply detection — OAuth + Pub/Sub setup runbook (v1)

**Human checkpoint runbook.** This is the external setup that gates the
Gmail reply-detection code (tasket `20260614-154815-5078`, `gate: human`).
None of the steps below can be performed by the automated build — they
require an interactive Google sign-in, access to the `lecrm-prod` GCP
project, and the operator age private key (for SOPS). When all steps are
done and verified, **continue the gate** from the automation dashboard
(or `POST /api/automation/{run_id}/continue-gate`); the dependent
handler-code tasket `20260614-154815-5b07` (order:6) then runs.

The architecture this serves is ADR-004 rev 2 §4 (Gmail-first per
ADR-009 §9). The rep sends from their own mailbox, so a per-user Gmail
`users.watch()` → Pub/Sub push covers ~95% of SMB sequences; there is no
Brevo `inbound.go` at v1 (ADR-003 Addendum A2026-06-14, inbound deferred).

> **Ordering note.** The push subscription targets
> `https://api.lecrm.fr/v1/webhooks/gmail/push`, which is implemented by
> order:6. Pub/Sub push redelivers on any non-2xx for the message
> retention window (default 7 days), so it is safe to create the
> subscription *before* the handler ships — events buffer and replay once
> the handler is live. But do NOT call `users.watch()` until the handler
> exists, because the Gmail watch itself expires after 7 days and the
> first `historyId` would be stale by the time replies arrive. Practical
> order: do §1–§2 (GCP + OAuth) now; do §3 `users.watch()` once order:6 is
> deployed (its `gmail.watch_renew` periodic job, `0 4 * * *`, then keeps
> it alive).

---

## Prerequisites

- `gcloud` CLI authenticated as a principal with Editor (or
  Pub/Sub Admin + Service Usage Admin) on the `lecrm-prod` project.
- `sops` + the operator **age private key** available locally (YubiKey or
  Bitwarden backup per ADR-007 §2) — needed to encrypt the refresh token.
  Confirm `ops/secrets/.sops.yaml` has the real recipient patched in (not
  the `REPLACE_WITH_AGE_PUBLIC_KEY` placeholder); if not, run
  `ops/secrets/bootstrap.sh` first.
- A Google Workspace user (the rep) who will grant the OAuth scopes.

Set once for the shell session:

```bash
export PROJECT=lecrm-prod
export TOPIC=gmail-inbox-events
export SUBSCRIPTION=gmail-inbox-push
export PUSH_ENDPOINT="https://api.lecrm.fr/v1/webhooks/gmail/push"
```

---

## 1. One-time GCP project setup

### 1.1 Enable the APIs

```bash
gcloud services enable gmail.googleapis.com pubsub.googleapis.com \
  --project="$PROJECT"
```

### 1.2 Create the Pub/Sub topic

```bash
gcloud pubsub topics create "$TOPIC" --project="$PROJECT"
```

### 1.3 Let Gmail publish to the topic (REQUIRED)

Gmail publishes watch notifications as the fixed system service account
`gmail-api-push@system.gserviceaccount.com`. Without this binding,
`users.watch()` returns `PERMISSION_DENIED` and no events ever flow.

```bash
gcloud pubsub topics add-iam-policy-binding "$TOPIC" \
  --project="$PROJECT" \
  --member="serviceAccount:gmail-api-push@system.gserviceaccount.com" \
  --role="roles/pubsub.publisher"
```

### 1.4 Create the push subscription with OIDC auth

The handler validates a Google-signed OIDC JWT on each push (ADR-004 rev2
§4: "validates the JWT (Google-signed)"). Pub/Sub mints that token for a
service account you nominate here; order:6's handler verifies its
`aud`/`iss`. Create (or reuse) a dedicated push-auth service account:

```bash
gcloud iam service-accounts create gmail-push-invoker \
  --project="$PROJECT" \
  --display-name="Gmail Pub/Sub push OIDC identity"

export PUSH_SA="gmail-push-invoker@${PROJECT}.iam.gserviceaccount.com"

gcloud pubsub subscriptions create "$SUBSCRIPTION" \
  --project="$PROJECT" \
  --topic="$TOPIC" \
  --push-endpoint="$PUSH_ENDPOINT" \
  --push-auth-service-account="$PUSH_SA" \
  --ack-deadline=10 \
  --message-retention-duration=7d
```

> Record the push-auth SA email — order:6's handler config needs it as the
> expected OIDC audience/subject. Do NOT download a JSON key for this SA;
> Pub/Sub mints the token internally, so the SA needs no exported key.

---

## 2. Per-workspace-user OAuth grant

Each rep grants leCRM access to *their own* mailbox. Repeat §2 for every
connected user.

### 2.1 OAuth client + consent screen (one-time per project)

In the GCP console → **APIs & Services**:

- **OAuth consent screen**: app type Internal (Workspace) or External as
  appropriate; add the rep(s) as test users until the app is verified.
- **Credentials → Create OAuth client ID → Web application**. Record the
  **client ID** (non-secret → per-workspace config table) and **client
  secret** (→ per-tenant manifest field `oauth_gmail_client_secret` in
  `secrets/clients/<slug>/secrets.enc.yaml`, NOT the oauth manifest).

### 2.2 Scopes — minimise (ADR-004 rev2 S3, tasket bf09)

Request the **minimal** set. Default:

```
https://www.googleapis.com/auth/gmail.readonly
https://www.googleapis.com/auth/gmail.send
```

`gmail.readonly` covers `users.watch` + `users.history.list` + message
reads; `gmail.send` covers sequence sends. Add
`https://www.googleapis.com/auth/gmail.modify` **only** if the workspace
needs leCRM to label/archive — it is opt-in, and a smaller scope set eases
Google's OAuth review. Confirm the minimal set is sufficient before
shipping (open question S3 / tasket bf09).

### 2.3 Obtain the refresh token

Run the authorization-code flow for the rep with `access_type=offline`
and `prompt=consent` (so Google returns a refresh token, not just an
access token). For the bootstrap checkpoint either use the OAuth 2.0
Playground configured with *your own* client ID/secret (gear icon → "Use
your own OAuth credentials") or a short local CLI helper. The in-product
"Connect Gmail" flow that does this for non-operator users is part of
order:6 — for this gate a manual grant for one connected user is enough.

Capture the `refresh_token` from the token response.

### 2.4 Encrypt and store the refresh token

```bash
export WS=<workspace_id>            # the workspace UUID/slug
export U=<user_id>                  # the user UUID

mkdir -p "secrets/oauth/gmail/$WS"
cp secrets/oauth/gmail/_template/secrets.yaml.template \
   "secrets/oauth/gmail/$WS/$U.yaml"

# Fill email_address, oauth_refresh_token, oauth_scopes:
$EDITOR "secrets/oauth/gmail/$WS/$U.yaml"

# Encrypt in place to the operator age key, then rename to .enc.yaml:
( cd "$(git rev-parse --show-toplevel)" && \
  sops --config ops/secrets/.sops.yaml --encrypt --in-place \
       "secrets/oauth/gmail/$WS/$U.yaml" )
mv "secrets/oauth/gmail/$WS/$U.yaml" \
   "secrets/oauth/gmail/$WS/$U.enc.yaml"

git add "secrets/oauth/gmail/$WS/$U.enc.yaml"
git commit -m "secrets(oauth): add Gmail grant for $WS/$U"
```

The plaintext `$U.yaml` is gitignored (see `.gitignore` →
`secrets/oauth/`); never commit it. Only `$U.enc.yaml` is committed.

---

## 3. Register the watch (run once order:6 is deployed)

With the handler live and the grant stored, register the watch for the
rep's mailbox so Gmail starts publishing to the topic:

- Call `users.watch()` with `topicName =
  projects/lecrm-prod/topics/gmail-inbox-events` and
  `labelIds = ["INBOX"]`.
- Persist the returned `historyId` as the connection's baseline for
  `users.history.list` (DB connection row, not the secrets manifest).
- The watch expires after 7 days; order:6's river periodic job
  `gmail.watch_renew` (`0 4 * * *`) renews it daily for safety margin.

---

## 4. Verify before continuing the gate

- [ ] `gcloud pubsub topics describe gmail-inbox-events --project=lecrm-prod` succeeds.
- [ ] `gcloud pubsub topics get-iam-policy gmail-inbox-events --project=lecrm-prod`
      shows `gmail-api-push@system.gserviceaccount.com` as `roles/pubsub.publisher`.
- [ ] `gcloud pubsub subscriptions describe gmail-inbox-push --project=lecrm-prod`
      shows `pushConfig.pushEndpoint = https://api.lecrm.fr/v1/webhooks/gmail/push`
      and an `oidcToken.serviceAccountEmail`.
- [ ] At least one `secrets/oauth/gmail/<workspace_id>/<user_id>.enc.yaml`
      exists, decrypts with `sops -d`, and contains a non-empty
      `oauth_refresh_token` with the minimal scope set.

When all four hold: **continue the gate** so order:6
(`20260614-154815-5b07`) runs.

---

## Staging (demo.lecrm.gbconsult.me)

> **Status: LIVE since 2026-06-16.** The engine wiring + this rollout are deployed
> on the **Netcup** box (`root@152.53.143.175`, `/opt/lecrm`, arm64 — see
> `docs/INFRASTRUCTURE.md`). `POST /v1/webhooks/gmail/push` returns **401** (not 404)
> and the per-workspace river runtime is up (`clients_started=3`). What remains is
> the human OAuth/GCP setup (§1–§2) + `users.watch()` (§3); the route's
> `LECRM_GMAIL_*` config currently uses placeholders for topic / push-SA / OAuth
> client, so end-to-end reply detection is not active until those are real.

The §1–§4 above are written for production (`lecrm-prod` / `api.lecrm.fr`). On
staging the runtime is the same code; only the externals and secret-routing
differ. These are the staging-specific deltas.

**Externals.** Use a staging GCP project (or a staging-suffixed subscription on
`lecrm-prod`) and the staging push endpoint:

```bash
export PUSH_ENDPOINT="https://demo.lecrm.gbconsult.me/v1/webhooks/gmail/push"
```

**Secret routing (operator key stays OFF this box).** Staging Gmail manifests are
encrypted to the **disposable staging age key** (the one already used for
`deploy/.env.staging.enc`), not the operator key — `ops/secrets/.sops.yaml` has a
dedicated rule keyed on the `staging/` path segment:

```
secrets/oauth/gmail/staging/<workspace_id>/<user_id>.enc.yaml
```

**Decrypt-at-deploy.** The API reads rendered *plaintext* manifests (no runtime
SOPS); render them into the gitignored mount source before/at deploy:

```bash
sops -d secrets/oauth/gmail/staging/$WS/$U.enc.yaml \
  > deploy/gmail-creds/$WS/$U.yaml          # mounted ro at /run/secrets/gmail
```

**Enable the feature** in `deploy/.env.staging` (compose passes these through;
empty by default = route unmounted, runtime not started):

```bash
LECRM_GMAIL_PUBSUB_TOPIC=projects/<staging-project>/topics/gmail-inbox-events
LECRM_GMAIL_PUSH_AUDIENCE=https://demo.lecrm.gbconsult.me/v1/webhooks/gmail/push
LECRM_GMAIL_PUSH_SA=gmail-push-invoker@<staging-project>.iam.gserviceaccount.com
LECRM_GMAIL_OAUTH_CLIENT_ID=<oauth client id>
LECRM_GMAIL_OAUTH_CLIENT_SECRET=<oauth client secret>
LECRM_GMAIL_CREDS_DIR=/run/secrets/gmail
```

**River tables + grants (once per workspace).** The river client cannot start
until River's tables exist in each `river_<hex>` schema and `lecrm_api` is granted
on them — idempotent backfill:

```bash
# Prereq on Netcup: the cutover's per-DB pg_dump restore dropped database-level
# grants, so lecrm_provisioner lacked CREATE on the db (provision fn / 0025 →
# "permission denied for database lecrm"). Restore it once (postgres superuser):
docker exec lecrm-postgres psql -U postgres -d lecrm \
  -c "GRANT CREATE ON DATABASE lecrm TO lecrm_provisioner"

lecrm-migrate apply          # applies 0025/0026/0027 (incl. core.gmail_mailbox_index)
lecrm-migrate river-setup --all
```

**Register the connection** (stands in for the in-product connect flow):

```bash
psql "$SUPERUSER_DSN" -v ws='<workspace_uuid>' -v usr='<user_uuid>' \
  -v email='<rep mailbox>' -f deploy/seed/gmail-demo-connection.sql
```

**Verify the route is live** after rebuilding the api:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' \
  -X POST https://demo.lecrm.gbconsult.me/v1/webhooks/gmail/push   # 401, not 404
```

Then do §3 `users.watch()` and **continue the gate** as above.

---

## References

- `docs/adr/ADR-004-rev2-sequences-architecture.md` §4 (Gmail path), S3 (scope minimisation).
- `docs/adr/ADR-009-stack-and-license.md` §9 (Gmail-first scope cut).
- `docs/adr/ADR-003-email-provider-brevo.md` Addendum A2026-06-14 (Brevo inbound deferred).
- `docs/adr/ADR-007-encryption-secrets-audit.md` §2 (sops + age baseline).
- `secrets/oauth/README.md`; `secrets/oauth/gmail/_template/secrets.yaml.template`.
- `ops/secrets/.sops.yaml` (creation rule); `ops/runbooks/secret-rotation.md` (rotation).
- [Gmail API push notifications](https://developers.google.com/workspace/gmail/api/guides/push).
- Handler code (order:6): tasket `20260614-154815-5b07` (push handler + `poll_reply` worker + `gmail.watch_renew`).
