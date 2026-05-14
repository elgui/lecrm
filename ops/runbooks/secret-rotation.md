# Secret rotation runbook (v0)

Operational procedures for rotating every leCRM secret. Scope:
**sops + age era only** (v0; up to ~10 clients). Vault-based rotation
becomes a separate runbook when v1 lands per ADR-007 §2.

The cadences below are the **canonical schedule** referenced by
[ADR-007 §2](../../docs/adr/ADR-007-encryption-secrets-audit.md) and
extended by ADR-009 §2.1 (`lecrm_provisioner`) and §7.1 (Authentik
admin). The "annual" / "quarterly" labels are floors, not ceilings —
always rotate immediately on suspected compromise.

---

## Cadence table

| Secret                          | Cadence              | Tier | Where stored                             | Trigger to rotate now                   |
|---------------------------------|----------------------|------|------------------------------------------|-----------------------------------------|
| `db_role_password`              | Annual               | T1   | `secrets/clients/<slug>/secrets.enc.yaml`| Workspace personnel change; ATO drill   |
| `oauth_gmail_client_secret`     | Annual / personnel   | T1   | `secrets/clients/<slug>/secrets.enc.yaml`| Google flags suspicious activity        |
| `jwt_signing_key`               | Quarterly            | T1   | `secrets/clients/<slug>/secrets.enc.yaml`| Token-leak suspicion                    |
| `brevo_api_key`                 | On demand            | T1   | `secrets/clients/<slug>/secrets.enc.yaml`| Brevo dashboard prompts rotation        |
| `brevo_webhook_secret`          | Quarterly            | T1   | `secrets/clients/<slug>/secrets.enc.yaml`| Webhook payload abuse signal            |
| `lecrm_provisioner_password`    | Annual               | T0   | `secrets/operator/secrets.enc.yaml`      | Any operator-account compromise — P0    |
| `authentik_admin_password`      | Annual               | T0   | `secrets/operator/secrets.enc.yaml`      | Any operator-account compromise — P0    |
| `cloudflare_dns_api_token`      | Annual / personnel   | T0   | `secrets/operator/secrets.enc.yaml`      | Token leaked, scope mis-set, etc.       |
| **age private key (Guillaume)** | Every 3 years OR loss| T0   | YubiKey + Bitwarden backup               | YubiKey lost, suspected theft, P0       |

Tier-0 (T0) means: compromise affects the entire platform; rotate first,
then incident-respond. Tier-1 (T1) means: compromise scoped to one
workspace.

---

## Procedures

### Per-tenant secret rotation (any T1 field)

1. **Open the encrypted manifest in place.**
   ```bash
   sops --config ops/secrets/.sops.yaml \
        secrets/clients/<slug>/secrets.enc.yaml
   ```
   `sops` decrypts to a tempfile, opens `$EDITOR`, re-encrypts on save.
2. **Generate the new secret value.** Mechanics by field:
   - `db_role_password` — call `lecrm_provision_workspace`'s sibling
     rotate function (planned; for v0, run
     `ALTER ROLE workspace_<id> PASSWORD <new>` as `lecrm_provisioner`
     and capture the new value).
   - `oauth_gmail_client_secret` — generate in Google Cloud Console,
     update the manifest, redeploy.
   - `jwt_signing_key` — `openssl rand -base64 48`. Token-validation
     code must accept BOTH old and new for 24h overlap; the
     `cmd/lecrm-api` keyring config holds N=2 active keys.
   - `brevo_api_key`, `brevo_webhook_secret` — rotate in Brevo dashboard
     first, then update the manifest.
3. **Save and exit `$EDITOR`.** Verify the manifest re-encrypted:
   ```bash
   git diff --stat secrets/clients/<slug>/secrets.enc.yaml
   ```
4. **Render the new `.env`:**
   ```bash
   ops/provision-client.sh <slug>
   scp deploy/.env.<slug> deploy-host:/etc/lecrm/.env.<slug>
   ssh deploy-host 'chmod 600 /etc/lecrm/.env.<slug>'
   ```
5. **Rolling-restart Compose services that consume the secret.**
6. **Commit:**
   ```bash
   git add secrets/clients/<slug>/secrets.enc.yaml
   git commit -m "ops(secrets): rotate <field> for <slug>"
   ```
7. **Log** the rotation in `## Bootstrap & rotation log` below.

### Operator (T0) secret rotation

Identical to the T1 procedure but on `secrets/operator/secrets.enc.yaml`
and `--also-operator` on `provision-client.sh`. Order of operations
matters for two of these:

- `lecrm_provisioner_password` — rotate in Postgres FIRST
  (`ALTER ROLE lecrm_provisioner PASSWORD …`), then update the manifest,
  then restart `cmd/lecrm-migrate`'s next scheduled invocation. The
  service is not long-running so no rolling-restart is needed; the next
  workspace provision picks up the new password.
- `authentik_admin_password` — rotate in the Authentik admin UI first
  (or via API with the bootstrap token), then update the manifest.
  Authentik re-reads the bootstrap password only on container restart;
  for an ongoing rotation just change it in the UI and update the
  manifest as the recovery copy.
- `cloudflare_dns_api_token` — create the new token first, deploy, THEN
  revoke the old token. Caddy reloads its config on file change; reload
  is `docker exec lecrm-caddy caddy reload --config /etc/caddy/Caddyfile`.

### age key rotation (every 3 years OR on loss)

Treat as an emergency procedure if triggered by YubiKey loss; otherwise
schedule a 2-hour maintenance window.

1. **Run bootstrap.sh in re-key mode.** (The current bootstrap.sh
   refuses to clobber the existing key — delete the
   `# leCRM v0 — Guillaume` block from `~/.config/sops/age/keys.txt`
   first, and also overwrite or remove
   `ops/secrets/recipients/guillaume.age.pub`.)
2. **Re-key every encrypted file** with the new public key:
   ```bash
   find secrets -name '*.enc.yaml' -print0 |
     xargs -0 -n1 sops --config ops/secrets/.sops.yaml updatekeys
   ```
3. **Verify** by decrypting one file end-to-end.
4. **Commit** the updated public-key file, the `.sops.yaml` diff, and
   every re-keyed manifest in one PR.
5. **Custody** the new private key on the new YubiKey + Bitwarden
   immediately; destroy the old YubiKey (or wipe + re-provision).
6. **Drill** Bitwarden recovery before declaring complete.
7. **Log** below.

---

## Incident rotation (suspected compromise)

P0 if any T0 secret leaks or any administrative account is suspected
compromised. P1 for T1 leaks limited to a single tenant.

P0 sequence (target: < 2 hours):

1. Rotate `authentik_admin_password` in the Authentik UI.
2. Rotate `lecrm_provisioner_password` in Postgres.
3. Rotate `cloudflare_dns_api_token` (create new, swap in Caddy, revoke
   old).
4. Update `secrets/operator/secrets.enc.yaml`, render, deploy.
5. Audit `audit_log` for any unexpected `auth.api_key.create`,
   `auth.permission.change`, `admin.impersonation.start` since the
   suspected breach window.
6. Decide whether T1 keys also need rotation (usually yes for
   `jwt_signing_key`).

P1 sequence (per-tenant) — follow the per-tenant rotation procedure
above and audit that workspace's `audit_log`.

---

## Bootstrap & rotation log

Append-only. One line per event, ISO-8601 date, brief summary.

| Date       | Operator  | Event                                              |
|------------|-----------|----------------------------------------------------|
| 2026-MM-DD | Guillaume | Initial age bootstrap (placeholder; fill on first  |
|            |           | bootstrap.sh run).                                 |

---

## TO RESOLVE

1. **CI deploy age recipient (v0.5).** GitHub Actions needs a
   non-interactive age key to decrypt secrets at release time. Options:
   a tightly-scoped key as an Actions secret (acceptable), or
   Hashicorp's `vault-action` (overkill at v0). Decide before the first
   automated deploy.
2. **`db_role_password` rotate function.** ADR-009 §2.1 specifies a
   sibling function to the provisioner that returns the password
   exactly once; the rotate path is not yet specified. Define before
   the first annual rotation (i.e. before May 2027).
3. **age-plugin-yubikey vs encrypted-keys.txt.** Bootstrap.sh creates a
   software age key; the YubiKey custody is documented as a manual
   step. If we adopt `age-plugin-yubikey` as the default, the script
   should generate directly into the YubiKey identity slot. Decide
   before the first key rotation (3-year horizon).
