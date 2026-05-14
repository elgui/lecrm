---
id: 20260510-162158-1023
title: "leCRM v0 — Secrets management baseline: sops + age (Track H)"
status: later
priority: p1
created: 2026-05-10
updated: 2026-05-14
tags: [secrets, sops, age, ops, v0]
category: project
group: lecrm-v0-build
group_order: 3
order: 1
plan: true
---

# leCRM v0 — Secrets management baseline: SOPS + age

## Why this tasket exists

Per [ADR-007](docs/adr/ADR-007-encryption-secrets-audit.md), SOPS+age is the v0 secrets baseline; Vault is v1+. The clean-room reframe (ADR-008) and locked Go stack (ADR-009) shift the secret manifest content — TypeScript/NestJS deployment secrets are out, Go-binary deployment secrets are in.

**This tasket is downstream of [b844](20260510-202450-b844-lecrm-v0-twenty-fork-tasket-housekeeping-week-1-sc.md) (scaffolding) — start after the scaffold is up.**

## Re-scoped done criteria (post-ADR-009)

- [ ] `ops/secrets/` directory with `.sops.yaml` policy enumerating recipients (Guillaume's age key; CI deploy key as a future recipient).
- [ ] age key generated, stored in YubiKey (primary) and Bitwarden (backup) per ADR-007 §2.
- [ ] Per-tenant secret manifest under `secrets/clients/<slug>/secrets.enc.yaml` (per ADR-007 §2 canonical form). Encrypted fields:
  - `db_role_password` (workspace-scoped Postgres role password from `lecrm_provision_workspace`)
  - `oauth_gmail_client_secret` (per ADR-009 §9 — Gmail-only at v0; Microsoft Outlook + IMAP deferred)
  - `jwt_signing_key` (workspace-scoped service-token signing key per ADR-009 §4.1)
  - `brevo_api_key`, `brevo_webhook_secret`
- [ ] Operator-level secret manifest under `secrets/operator/` (NEW per ADR-009 council R2):
  - `lecrm_provisioner_password` — Tier-0 secret, annual rotation, used by `cmd/lecrm-migrate` to invoke `lecrm_provision_workspace`
  - `authentik_admin_password` (per pentester council finding — Authentik admin is operationally a high-value target; not in the per-tenant manifest)
  - `cloudflare_dns_api_token` — for Caddy DNS-01 wildcard cert renewal
- [ ] `ops/provision-client.sh` renders the encrypted manifest at deploy time → environment variables on the OVH VPS via Compose `env_file`.
- [ ] Secret rotation runbook in `ops/runbooks/secret-rotation.md` per ADR-007 §2 table.

## Out of scope

- Vault deployment (v1+ trigger, per ADR-007 §2).
- Per-secret automated rotation pipelines (manual rotation runbook is the v0 deliverable).

## References

- [ADR-007](docs/adr/ADR-007-encryption-secrets-audit.md) §2 (secret management) — receives the `lecrm_provisioner` and Authentik admin credential additions per ADR-009 TO RESOLVE-14.
- [ADR-009](docs/adr/ADR-009-stack-and-license.md) §2.1 (provisioning function), §4.1 (service tokens), §7.1 (auth IDP).
