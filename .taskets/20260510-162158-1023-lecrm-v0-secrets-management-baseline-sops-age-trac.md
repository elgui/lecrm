---
id: 20260510-162158-1023
title: "leCRM v0 — Secrets management baseline: sops + age (Track H)"
status: later
priority: p1
created: 2026-05-10
updated: 2026-05-10
tags: [secrets, sops, age, ops, v0]
category: project
group: lecrm-v0-build
order: 4
plan: true
---

# leCRM v0 — Secrets management baseline (Track H)

## Why this tasket exists
The current per-client `.env` files are chmod 0600 plaintext on the VPS. That works for v0 with the operator on a single VPS, but does not survive Phase-2 consolidation (where all workspaces share the cluster) and does not satisfy the backup/restore audit trail. Per ADR-007, sops+age is the v0 secrets baseline; Vault is v1+.

Reference: ADR-007 (`docs/adr/ADR-007-encryption-secrets-audit.md`).

## Done criteria
- [ ] `ops/secrets/` directory with `.sops.yaml` policy. Per-tenant secret namespacing for Anthropic API keys, OAuth client secrets, DKIM private keys.
- [ ] age key generated, stored in 1Password (or equivalent personal vault), and the public key committed.
- [ ] `ops/provision-client.sh` updated to render encrypted-at-rest secrets via sops, decrypt at deploy time only.
- [ ] CI workflow (when CI lands) decrypts via the age key in GitHub Actions secrets.
- [ ] Documented rotation runbook for compromised keys.
