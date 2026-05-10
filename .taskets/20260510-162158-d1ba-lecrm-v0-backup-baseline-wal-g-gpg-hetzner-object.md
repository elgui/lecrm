---
id: 20260510-162158-d1ba
title: "leCRM v0 — Backup baseline: WAL-G + GPG + Hetzner Object Storage (Track G)"
status: later
priority: p1
created: 2026-05-10
updated: 2026-05-10
tags: [backup, ops, wal-g, v0]
category: project
group: lecrm-v0-build
order: 3
plan: true
---

# leCRM v0 — Backup baseline (Track G)

## Why this tasket exists
Phase 1 (VPS-per-client) backups must be in place before the first paying client signs. Per ADR-006, the stack is WAL-G with GPG client-side encryption to Hetzner Object Storage, per-client S3 prefix `s3://lecrm-wal/<client-slug>/`, `archive_timeout=60` for max-60s RPO.

Reference: ADR-006 (`docs/adr/ADR-006-backup-dr.md`).

## Done criteria
- [ ] WAL-G container added to per-client docker-compose template, sidecar to Postgres, with the GPG public key for `lecrm-backup@gbconsult.me` baked into the image.
- [ ] Hetzner Object Storage bucket `lecrm-wal` created; per-client IAM role with prefix-restricted access.
- [ ] Postgres `archive_command` and `restore_command` configured.
- [ ] Restore-test playbook documented in `ops/runbooks/restore.md`. Includes the GPG private-key recovery path.
- [ ] First quarterly restore drill executed and signed off. Validates RPO <60s and RTO <30 min.
- [ ] Cross-region cold copy to OVH FR is Phase 3 — explicitly out of scope here (tracked in ADR-006).
