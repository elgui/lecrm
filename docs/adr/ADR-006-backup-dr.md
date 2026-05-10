# ADR-006 — Backup, Disaster Recovery, RPO/RTO

**Status:** Accepted
**Date:** 2026-05-10
**Deciders:** Guillaume

---

## Context

leCRM's clients trust GB Consult with their CRM data. Loss-of-data is the most damaging incident class — worse than downtime, worse than a security disclosure, because it's irreversible. The DR architecture must:

- meet a defensible **RPO ≤15 min** (production target; v0 ceiling 1 h);
- meet **RTO ≤4 h** (v0) and **≤1 h** (production);
- support **per-client surgical restore** — recover one client without touching others, in both phase 1 (VPS-per-client) and phase 2+ (shared cluster);
- keep all backup data in the EU;
- **encrypt before upload** to off-site storage so key sovereignty is preserved against subpoena risk and provider compromise (`docs/research/dr-security.md` §2 critical caveat: Hetzner Object Storage does not encrypt at rest by default);
- be operable by a solo operator with quarterly drill discipline.

Tool comparison from `docs/research/dr-security.md` §1:

| Tool | RPO | Operational weight | Status (May 2026) |
|---|---|---|---|
| `pg_dump` daily | ≤24 h | Minimal | Stable, logical only |
| WAL-G (continuous WAL + base backup) | ≤60 s | Low–Medium | Active, recommended |
| pgBackRest | ≤60 s | Medium | **Archived Apr 2026** — funding lost; no new features expected |
| Hetzner VM snapshots | ≤1 h | Minimal | Coarse; not PITR-grade |

pgBackRest's archival changes the calculus: WAL-G is the active tool with healthier governance. Snapshots are a coarse safety net, not the primary mechanism.

---

## Decision

### 1. Backup tool: WAL-G with continuous WAL archiving

WAL-G is a single Go binary that integrates with PostgreSQL's `archive_command`. EU-friendly, S3-compatible, supports GPG encryption native to the upload pipeline.

**PostgreSQL configuration:**

```ini
# postgresql.conf
wal_level = replica
archive_mode = on
archive_command = 'wal-g wal-push %p'
archive_timeout = 60          # forces WAL rotation every 60s → RPO ceiling ~60s
restore_command = 'wal-g wal-fetch %f %p'
```

**WAL-G environment (injected via systemd unit + sops/Vault):**

```bash
WALG_S3_PREFIX=s3://lecrm-wal/<client-slug-or-shared>
AWS_ENDPOINT_URL=https://nbg1.your-objectstorage.com  # Hetzner NBG1
AWS_ACCESS_KEY_ID=<key>
AWS_SECRET_ACCESS_KEY=<secret>
AWS_REGION=eu-central-1
WALG_COMPRESSION_METHOD=brotli
WALG_UPLOAD_CONCURRENCY=4
WALG_PGP_KEY=<armored-public-key>     # client-side encryption
```

**Base backup cadence:** weekly full base backup (`wal-g backup-push`); daily compressed delta where supported. Retention: 7 full backups (≈49 days), then `wal-g delete retain FULL 7`.

### 2. Backup destination: Hetzner Object Storage (NBG1) + cross-region copy to OVH FR

Primary destination: Hetzner Object Storage NBG1 (Nuremberg). Best price/TB for our scale (~€4.99 flat for 1 TB) (`docs/research/dr-security.md` §2). Pre-upload GPG encryption preserves key sovereignty (Hetzner does not encrypt at rest by default).

Secondary cross-region copy: weekly rclone sync from Hetzner Object Storage to OVH Object Storage GRA (Gravelines, FR). Provides cross-region DR capability for the rare scenario of a full Hetzner DE outage. Cost: ~€7–12/mo for 1 TB.

GPG key management: single leCRM-controlled GPG keypair (v0); per-tenant keypairs (v1+ via Vault). Public key on every server with archive permissions; private key stored in:
- v0: YubiKey (primary) + Bitwarden vault (backup)
- v1+: Vault Transit secret engine

**Why client-side encryption:** Hetzner Object Storage offers SSE-C (customer-supplied keys per request) but it is awkward to integrate with WAL-G's `archive_command` (each PUT/GET would need the key). GPG client-side encryption via WAL-G's `WALG_PGP_KEY` happens transparently in the WAL-push pipeline and gives us provider-independent key sovereignty.

### 3. Per-client restore granularity

#### Phase 1 (VPS-per-client) — trivial

Each client has its own VPS, its own WAL archive (`s3://lecrm-wal/<client-slug>/`), its own GPG-encrypted backups.

Restore procedure:

```bash
# 1. Provision fresh VPS (or stop the existing one).
systemctl stop postgresql

# 2. Move broken data dir aside; create fresh empty.
mv $PGDATA ${PGDATA}.bak-$(date +%s)
mkdir -p $PGDATA && chown postgres:postgres $PGDATA && chmod 700 $PGDATA

# 3. Fetch base backup.
sudo -u postgres wal-g backup-fetch $PGDATA LATEST

# 4. Configure recovery.
cat > /etc/postgresql/16/main/postgresql.conf.d/recovery.conf <<EOF
restore_command = 'wal-g wal-fetch %f %p'
recovery_target_time = '<target>'
recovery_target_action = 'promote'
EOF
touch $PGDATA/recovery.signal

# 5. Start PostgreSQL — WAL replay begins.
systemctl start postgresql
```

Total operation: 30–60 min for a typical SMB DB <20 GB. Within v0 RTO.

**Hetzner Volume detach/attach optimisation.** PostgreSQL data volume on a Hetzner Volume can be detached from a failed VPS and re-attached to a replacement in ~2 min, skipping WAL replay entirely. RTO drops to 20–30 min if the volume is intact.

#### Phase 2 (shared cluster, schema-per-tenant) — surgical

Per-tenant restore is supported natively by PostgreSQL via `pg_dump -n` and `pg_restore -n`. (`docs/research/multi-tenant-postgres-patterns.md` §7.)

**Daily per-workspace `pg_dump` supplement.** WAL-G handles cluster-wide PITR; for surgical per-tenant restore we add a daily logical dump per workspace to a separate prefix:

```bash
# Per-workspace daily dump (cron, 02:00 local)
WS_ID=<base36-id>; CLIENT=<slug>
pg_dump -h localhost -U twenty -d twenty_db \
  -n "workspace_${WS_ID}" -Fc \
  | gpg --encrypt --recipient lecrm-backup \
  | aws s3 cp - "s3://lecrm-pgdump/${CLIENT}/$(date +%Y%m%d).dump.gpg" \
    --endpoint-url "$AWS_ENDPOINT_URL"
```

Retention: 30 days of daily dumps + monthly full of `core` and `metadata` schemas.

**Per-tenant restore in shared cluster:**

```bash
# 1. Decrypt the dump.
aws s3 cp s3://lecrm-pgdump/<client>/<date>.dump.gpg - --endpoint-url $AWS_ENDPOINT_URL \
  | gpg --decrypt > /tmp/<client>.dump

# 2. Drop the damaged schema (be sure!).
psql -c 'DROP SCHEMA "workspace_<id>" CASCADE;'

# 3. Restore.
pg_restore -h localhost -U twenty -d twenty_db -n "workspace_<id>" /tmp/<client>.dump

# 4. Verify row counts and a few canonical queries.
psql -c "SELECT COUNT(*) FROM workspace_<id>.opportunity;"
```

This is fully native PostgreSQL — no custom ETL. Time: 5–15 min for a typical SMB workspace.

For PITR within the day (between daily dumps), restore is more involved: spin up a temporary recovery cluster from WAL-G, replay to the target time, `pg_dump -n workspace_<id>` from there, restore into production. Documented as the rarer "intra-day surgical PITR" runbook. Estimate: 60–90 min.

### 4. RPO/RTO targets by phase

| Phase | RPO target | RPO mechanism | RTO target | RTO mechanism |
|---|---|---|---|---|
| v0 (phase 1) | ≤15 min (achieve 1–3 min) | WAL-G + `archive_timeout=60` | ≤4 h (achieve 30–60 min) | Cold WAL-G restore + Hetzner Volume re-attach |
| v1+ (phase 2) | ≤15 min | WAL-G + per-workspace pg_dump | ≤1 h | Cold restore + Hetzner Floating IP |
| v2+ (phase 3) | ≤30 s | Streaming replication via Patroni | ≤5 min | Patroni auto-promote standby |

The `archive_timeout = 60` setting is the active knob for archive-based RPO; combined with WAL-G's brotli compression and 4-way upload concurrency, real-world RPO on an SMB instance is 1–3 min.

For sub-minute RPO (v2+), streaming replication to a hot standby (FSN1 secondary) is required. Patroni with etcd-backed leader election handles automatic failover. This is a phase 3 investment, not a v0/v1 commitment.

### 5. Restore drill — quarterly, scripted, documented

Drill cadence: quarterly. Target: 90 min wall-time per drill. Run against a **test client** with dummy data on a staging VPS, never production. The drill script is committed to `infra/dr-drill.sh` (`docs/research/dr-security.md` §5 has the canonical version):

```bash
#!/usr/bin/env bash
set -euo pipefail
RESTORE_TARGET="<last-known-good-timestamp>"
PGDATA=/var/lib/postgresql/16/main
BACKUP_PREFIX=s3://lecrm-wal/test-client

systemctl stop postgresql
mv $PGDATA ${PGDATA}.bak-$(date +%s)
mkdir -p $PGDATA && chown postgres:postgres $PGDATA && chmod 700 $PGDATA
sudo -u postgres wal-g backup-fetch $PGDATA LATEST
cat > /etc/postgresql/16/main/postgresql.conf.d/recovery.conf <<EOF
restore_command = 'wal-g wal-fetch %f %p'
recovery_target_time = '${RESTORE_TARGET}'
recovery_target_action = 'promote'
EOF
touch $PGDATA/recovery.signal
chown postgres:postgres $PGDATA/recovery.signal
systemctl start postgresql

until sudo -u postgres psql -c "SELECT pg_is_in_recovery();" | grep -q 'f'; do
  echo "Replaying WAL..."; sleep 10
done

sudo -u postgres psql -d lecrm -c "SELECT COUNT(*) FROM crm_contacts;"
sudo -u postgres psql -d lecrm -c "SELECT MAX(created_at) FROM crm_contacts;"
```

**Post-drill checklist (mandatory, archived in `docs/dr-drills/`):**

- [ ] Total elapsed time documented (target ≤1 h).
- [ ] Row counts match expected baseline.
- [ ] WAL-G logs show no errors.
- [ ] Backup retention policy verified (`wal-g delete retain FULL 7`).
- [ ] Test client data verified at recovery target time.
- [ ] Drill documented in incident log with timestamp.
- [ ] Phase-2-specific: `pg_dump -n` per-workspace dumps decrypt-and-restore tested.
- [ ] Cross-region: rclone sync to OVH FR verified successful.

**Drill failure = blocking issue.** If quarterly drill fails any check, treat as a P0 incident: stop feature work, fix backup mechanics, re-run drill until passing.

### 6. Provider failure runbook

| Scenario | Immediate action | Customer comms template |
|---|---|---|
| **Hetzner outage (full region NBG1)** | (1) Trigger cross-region restore from OVH FR mirror. (2) Provision new VPS in OVH FR. (3) Cold-restore from cross-region WAL archive. ETA 45–90 min. | "We are aware of a hosting provider incident. Your data is safe. We are restoring service to an alternate data center. ETA: [X]. We will update every 30 minutes." |
| **Hetzner Object Storage outage (NBG1 only)** | WAL archiving pauses; PostgreSQL queues WAL files locally bounded by `wal_keep_size`. Production continues. Restore is blocked until storage recovers. | Not customer-impacting unless DB also fails during outage; no proactive comms. |
| **Brevo outage** | CRM sends queue and retry on Brevo recovery. Sequence engine pauses but resumes cleanly. | "Email send features are temporarily delayed. CRM data access is unaffected." (Auto-status-page; no direct email if outage <2 h.) |
| **Anthropic API outage** | Agent runtime degrades gracefully (LiteLLM circuit breaker on 5xx); CRM core unaffected. | No proactive comms unless outage exceeds 4 h. |
| **GB Consult primary operator unavailable** | Documented secondary-operator runbook in `infra/runbook-secondary.md` covering: Vault unlock procedure, restore drill, customer comms templates, billing pass-through. (Out of architectural scope but flagged.) | n/a |

### 7. Backup integrity verification

Beyond the quarterly drill, monthly automated checks:

- `wal-g wal-verify` on the primary's archive — checksum integrity of WAL segments.
- `wal-g backup-list` parsed by a small script that asserts: (a) at least one full backup in the last 8 days, (b) no gaps in the WAL chain.
- Synthetic restore test: spin up a tiny VM weekly, restore the latest base backup + ≤5 min of WAL, run a smoke query, destroy the VM. Cost negligible (~€0.01/run on a CPX11).

---

## Consequences

### Positive

- **WAL-G with `archive_timeout=60` hits 1–3 min RPO** for typical SMB traffic — well inside the 15-min target.
- **Per-tenant surgical restore via `pg_dump -n`** is native PostgreSQL; no custom tooling. Phase 2 architecture inherits this for free.
- **Client-side GPG encryption preserves key sovereignty.** Subpoena of Hetzner buckets returns ciphertext.
- **Quarterly drill discipline** catches backup-mechanic regressions before they hit a real incident.
- **Cross-region copy to OVH FR** gives a defensible answer to "what if Hetzner has a full DC failure" — a question regulated prospects ask.
- **Phase 3 path to RPO 30s / RTO 5 min** is well-understood (Patroni + streaming replication); not built but architecturally provisioned.

### Negative

- **Daily per-workspace `pg_dump` is duplicative** with WAL-G but the duplication is intentional: surgical per-tenant restore needs the logical dump format. Storage cost is small (typical SMB workspace dump is <100 MB).
- **GPG key management is single-point-of-failure-shaped.** Loss of the GPG private key = backups are unrecoverable. Mitigation: YubiKey + Bitwarden + Vault Transit (v1+) gives three independent key custodians; a documented quarterly key-recovery drill verifies all three paths.
- **WAL-G's brotli compression is CPU-heavy** during base backup. Phase 1 CX21 is fine; phase 2 CCX33 has plenty of CPU; phase 3 must monitor.
- **Cross-region restore latency** to OVH FR is unmeasured. The TO RESOLVE flags this — without a measured baseline we can't confidently quote a cross-region RTO.
- **pgBackRest's archival** removes a fallback option. If WAL-G has a critical regression, we have less optionality. Mitigation: monitor pgBackRest's coalition-funding status (`docs/research/dr-security.md` §1 status); if revived, evaluate as a fallback.

### Neutral

- The drill script is short enough to be re-checked manually each quarter. Automating it further (CI job) would be possible but adds infrastructure for a 4-times-a-year operation.
- WAL-G's compression ratio (~70–80% on CRM data) means a 20 GB cluster generates ~5 GB/month of WAL plus weekly ~5 GB base backups. ~25 GB/month/client is the rough scale; well within the Hetzner Object Storage flat-rate envelope.

---

## Alternatives Considered

### Alt 1: pgBackRest

Deprioritised due to **archival in April 2026** (`docs/research/dr-security.md` §1). pgBackRest would otherwise be a strong choice (better incremental backup support, more mature parallel-restore). If the coalition-funding model materialises and Percona backs it, revisit. For now, WAL-G has healthier governance.

### Alt 2: `pg_dump` daily only

Rejected on RPO grounds. Daily dumps give RPO ≤24 h, far short of the 15-min target. Acceptable only as a supplementary surgical-restore mechanism, not the primary.

### Alt 3: Hetzner VM snapshots only

Rejected. Snapshots are coarse (whole-VM granularity), not PITR-grade, and Hetzner's snapshot scheduler is hourly minimum. Useful as a safety net before risky operations (pre-rebase, pre-major-migration); not as primary DR.

### Alt 4: Streaming replication from day one

Rejected for v0/v1. Streaming replication requires a second VPS (~€8/mo doubled), Patroni + etcd setup, automated promotion logic, and a runbook for split-brain detection. RPO 30 s / RTO 5 min is overkill for ≤4 paying clients. Cold restore + Hetzner Volume re-attach hits the v0/v1 SLA targets.

### Alt 5: AWS S3 + cross-region replication

Rejected. AWS in EU triggers DPF/SCC complexity that's avoidable with Hetzner + OVH (both EU-headquartered). Cost is also ~3× Hetzner Object Storage at our scale.

### Alt 6: Provider-managed PostgreSQL (Scaleway DB / Hetzner managed PG)

Rejected. Provider-managed PostgreSQL takes per-tenant restore granularity out of our hands (most managed offerings restore the whole DB or whole instance). The schema-per-tenant model in [ADR-001](ADR-001-tenancy-model.md) requires native PostgreSQL access for `pg_dump -n` workflows. Self-managed PostgreSQL on a VPS preserves that capability.

---

## References

- `docs/research/dr-security.md` (entire document; §1 backup tools, §2 destination, §3 per-client restore, §4 RPO/RTO mechanics, §5 drill runbook, §6 failover, §13 phase recommendations).
- `docs/research/multi-tenant-postgres-patterns.md` §7 (per-tenant `pg_dump -n` mechanics).
- [WAL-G PostgreSQL docs](https://wal-g.readthedocs.io/PostgreSQL/).
- [WAL-G storages](https://wal-g.readthedocs.io/STORAGES/).
- [Hetzner Object Storage](https://www.hetzner.com/storage/object-storage/).
- [Hetzner SSE-C docs](https://docs.hetzner.com/storage/object-storage/howto-protect-objects/encrypt-with-sse-c/).
- [pgBackRest archival announcement (Apr 2026)](https://percona.community/blog/2026/04/28/pgbackrest-is-archived-what-now/).
- [PostgreSQL continuous archiving](https://www.postgresql.org/docs/current/continuous-archiving.html).
- [`archive_timeout` GUC](https://postgresqlco.nf/doc/en/param/archive_timeout/).
- [Patroni docs](https://patroni.readthedocs.io/) (phase 3 reference).
- Related ADRs: [ADR-001](ADR-001-tenancy-model.md) (per-tenant restore inherits from schema-per-tenant), [ADR-007](ADR-007-encryption-secrets-audit.md) (GPG key management lives in the secrets architecture).

---

## TO RESOLVE

1. **Cross-region restore latency baseline.** Measure: full WAL restore of a 20 GB database from Hetzner Object Storage NBG1 to a fresh OVH FR VPS. The result determines whether the cross-region 4 h RTO target is realistic and feeds the customer DPA wording. (`docs/research/dr-security.md` §455 item 5.)
2. **WAL-G GPG vs SSE-C interaction.** Confirm WAL-G's GPG encryption is mutually exclusive with Hetzner SSE-C, and that GPG-only is sufficient for our threat model (no double-encryption needed). (`docs/research/dr-security.md` §455 item 4.)
3. **pgBackRest coalition funding status.** Quarterly check on whether pgBackRest gets revived governance. If yes, re-evaluate as a backup-tool option. (`docs/research/dr-security.md` §455 item 2.)
4. **Per-tenant pg_dump dump-size growth.** Monitor monthly; if any single workspace's daily dump exceeds 1 GB, reconsider the daily cadence (move to weekly + WAL for that workspace).
5. **GPG key recovery drill.** Define the drill: simulate loss of YubiKey, recover from Bitwarden vault, verify decryption of recent WAL segments. Run annually.
6. **Patroni etcd quorum sizing for phase 3.** A 3-node etcd cluster on small VPSes is the standard but adds €15–20/mo. Validate the cost vs alternative (single-leader with manual failover) at phase 3 trigger.
7. **Backup encryption-key rotation.** GPG keypair rotation is operationally heavy (re-encrypt all archive history). Decide policy: never rotate routinely, only on suspected compromise.
