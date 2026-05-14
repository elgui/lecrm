---
id: 20260510-162158-d1ba
title: "leCRM v0 — Backup baseline: WAL-G + GPG + Hetzner Object Storage (Track G)"
status: later
priority: p1
created: 2026-05-10
updated: 2026-05-14
tags: [backup, ops, wal-g, v0]
category: project
group: lecrm-v0-sprint-11
group_order: 11
order: 2
plan: true
---

# leCRM v0 — Backup baseline: WAL-G + GPG → OVH Object Storage

## Why this tasket exists

Phase 1 (VPS-per-client) backups must be in place before the first paying client signs. Per [ADR-006](docs/adr/ADR-006-backup-dr.md), the stack is WAL-G with GPG client-side encryption. Under the revised STRATEGIC-OVERVIEW §2 OVH-first hosting decision, the object-storage target is **OVH Object Storage** (S3-compatible), not Hetzner Object Storage.

[pgBackRest was declared unmaintained 2026-04-27](https://thebuild.com/blog/2026/04/30/after-pgbackrest/); WAL-G remains canonical per ADR-009 §2.5.

**This tasket is downstream of [b844](20260510-202450-b844-lecrm-v0-twenty-fork-tasket-housekeeping-week-1-sc.md) (scaffolding) — start after the scaffold is up.**

## Re-scoped done criteria

- [ ] WAL-G binary added to the per-client OVH VPS deploy (sidecar to Postgres in `deploy/compose/postgres.yml`) with the GPG public key for `lecrm-backup@gbconsult.me` baked in.
- [ ] OVH Object Storage bucket `lecrm-wal` created in EU region; per-client IAM credentials with prefix-restricted access `s3://lecrm-wal/<client-slug>/`.
- [ ] **Verify WAL-G S3 compatibility against OVH's endpoint** — WAL-G's S3 driver assumes AWS-compatible signing; OVH's S3 surface is typically compatible but document any region/endpoint quirks (`AWS_ENDPOINT`, `AWS_S3_FORCE_PATH_STYLE`).
- [ ] Postgres `archive_command` and `restore_command` configured. `archive_timeout=60` for max-60s RPO per ADR-006.
- [ ] Per-tenant restore tooling: `pg_restore -n workspace_<id>` workflow into a temporary instance, then re-load into production cluster (the schema-per-tenant restore pattern from ADR-001 §Backup mechanics).
- [ ] Restore-test playbook in `ops/runbooks/restore.md`, including GPG private-key recovery from YubiKey + Bitwarden.
- [ ] First quarterly restore drill executed and signed off. Validates RPO <60s and RTO <30 min.

## Out of scope

- Cross-region cold copy (Phase 3 work per ADR-006).
- Crypto-shredding for per-tenant key destruction (v2 work per ADR-007 §4).

## References

- [ADR-006](docs/adr/ADR-006-backup-dr.md) (backup/DR posture).
- [ADR-009](docs/adr/ADR-009-stack-and-license.md) §2.5 (WAL-G canonical; pgBackRest archived).
- [STRATEGIC-OVERVIEW §2](docs/STRATEGIC-OVERVIEW.md) (OVH-first hosting decision).
