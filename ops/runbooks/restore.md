# Restore runbook — WAL-G + GPG + OVH Object Storage (Phase 1)

Canonical procedures for restoring a leCRM client cluster from the
WAL-G archive on OVH Object Storage. The drill version at the bottom
of this document is the **quarterly** procedure; the procedures above
it are the **incident-response** versions.

References:
- [ADR-006](../../docs/adr/ADR-006-backup-dr.md) — backup/DR posture, RPO/RTO targets.
- [ADR-001](../../docs/adr/ADR-001-tenancy-model.md) — schema-per-tenant model that the per-tenant restore inherits.
- [ADR-007](../../docs/adr/ADR-007-encryption-secrets-audit.md) §2 — GPG key custody (YubiKey + Bitwarden).
- [STRATEGIC-OVERVIEW.md](../../docs/STRATEGIC-OVERVIEW.md) §2 — OVH-first hosting.

---

## 0. Pre-flight (always)

```bash
# Confirm the bucket prefix is reachable and the GPG key matches.
docker exec lecrm-postgres /usr/local/bin/wal-g backup-list
docker exec lecrm-postgres gpg --with-fingerprint --with-colons \
  /etc/postgres/gpg/lecrm-backup.pub.asc | awk -F: '/^fpr:/ {print $10; exit}'
# Compare against deploy/postgres/gpg/lecrm-backup.fingerprint.
```

If `backup-list` returns nothing, the bucket is empty (no base backup
yet) or the IAM credential lost its prefix permission. Check OVH IAM
before any restore attempt.

The **private** GPG key is needed only for restore; the production VPS
MUST NOT carry it. Pull it from YubiKey or Bitwarden onto the recovery
host before running step 2 below.

---

## 1. Full cluster restore (Phase 1, per-client VPS — incident)

Apply when: the client's VPS is lost, the Postgres data directory is
corrupt, or a destructive operator action (DROP DATABASE, rm -rf
pgdata) needs to be reversed.

```bash
# 1.1 Stop the live cluster. If the VPS is already gone, provision a
#     replacement OVH VPS and skip to 1.2 on the fresh host.
docker compose -f deploy/compose/postgres.yml down postgres

# 1.2 Move the broken data dir aside (or remove if disk is full).
sudo mv /var/lib/docker/volumes/lecrm_postgres_data/_data \
        /var/lib/docker/volumes/lecrm_postgres_data/_data.broken-$(date +%s)

# 1.3 Recreate the volume mount point.
sudo mkdir -p /var/lib/docker/volumes/lecrm_postgres_data/_data
sudo chown 70:70 /var/lib/docker/volumes/lecrm_postgres_data/_data   # postgres uid in alpine
sudo chmod 700  /var/lib/docker/volumes/lecrm_postgres_data/_data

# 1.4 Decrypt the GPG private key onto the host (RAMFS / tmpfs — never
#     to a persistent disk). Path goes into walg.env as
#     WALG_PGP_KEY_PATH_PRIVATE.
sudo mkdir -p /run/lecrm-restore
sudo mount -t tmpfs -o size=10M,mode=0700 tmpfs /run/lecrm-restore
# YubiKey path:
gpg --export-secret-keys --armor lecrm-backup@gbconsult.me \
  > /run/lecrm-restore/lecrm-backup.priv.asc
sudo chown 70:70 /run/lecrm-restore/lecrm-backup.priv.asc
sudo chmod 0400 /run/lecrm-restore/lecrm-backup.priv.asc

# 1.5 Edit deploy/postgres/walg.env to add:
#       WALG_PGP_KEY_PATH_PRIVATE=/run/lecrm-restore/lecrm-backup.priv.asc
#     and mount /run/lecrm-restore into the postgres container.

# 1.6 Fetch the latest base backup.
docker compose -f deploy/compose/postgres.yml run --rm \
  -v /run/lecrm-restore:/run/lecrm-restore:ro \
  postgres bash -c '
    su postgres -c "/usr/local/bin/wal-g backup-fetch \$PGDATA LATEST"
  '

# 1.7 Configure point-in-time recovery target (omit for "latest").
docker compose -f deploy/compose/postgres.yml run --rm \
  postgres bash -c '
    cat >> $PGDATA/postgresql.auto.conf <<EOF
restore_command         = "/usr/local/bin/lecrm/wal-fetch.sh %f %p"
recovery_target_time    = "2026-05-27 14:32:00 UTC"     # OR omit for latest WAL
recovery_target_action  = "promote"
EOF
    touch $PGDATA/recovery.signal
    chown postgres:postgres $PGDATA/postgresql.auto.conf $PGDATA/recovery.signal
  '

# 1.8 Boot postgres; it replays WAL until the target is reached and
#     promotes.
docker compose -f deploy/compose/postgres.yml up -d postgres

# 1.9 Wait for promotion, then verify.
until docker exec lecrm-postgres \
        psql -U postgres -d lecrm -tAc "SELECT NOT pg_is_in_recovery();" \
        2>/dev/null | grep -q t; do
  echo "Replaying WAL..."; sleep 5
done
docker exec lecrm-postgres \
  psql -U postgres -d lecrm -c "SELECT MAX(created_at) FROM core.audit_log;"

# 1.10 SCRUB the tmpfs (private key out of memory).
sudo umount /run/lecrm-restore
sudo rmdir /run/lecrm-restore
# Restore deploy/postgres/walg.env to the production version
# (no WALG_PGP_KEY_PATH_PRIVATE). Re-encrypt with SOPS.
```

Target RTO: **30–60 minutes** for a typical SMB cluster (<20 GB).
Target RPO: **<60 seconds** thanks to `archive_timeout=60`.

---

## 2. Per-tenant surgical restore (schema-per-tenant)

Apply when: one workspace's data is corrupted (bad migration applied to
a single schema, accidental DROP TABLE inside workspace_<id>, customer
asked for a point-in-time rewind of their workspace only).

This is the Phase 2 procedure in ADR-001 §Backup mechanics; in Phase 1
it still applies because each VPS hosts one cluster with one workspace
schema today.

```bash
# Drives wal-g backup-fetch into a side cluster, replays WAL to the
# target time, pg_dumps the workspace schema, and prints the
# pg_restore command to apply against the live cluster.
docker exec -it lecrm-postgres \
  /usr/local/bin/lecrm/restore-tenant.sh \
    <workspace-id> \
    '2026-05-27 14:32:00 UTC'

# The script ends by printing the next steps. Verify the dump file,
# DROP SCHEMA, pg_restore, row-count check. NEVER pipe directly into
# the live cluster — the manual gates are intentional.
```

Target RTO: **5–15 minutes** for SMB workspace dumps.

---

## 3. Provider-failure restore (OVH region outage)

Apply when: OVH Object Storage GRA is unreachable AND the local VPS
has lost its Postgres data dir (the lethal combination).

Phase 1 has no cross-region copy in production today (that's
[ADR-006] §6 work, Phase 3). If both the primary OVH region and the
local data are gone simultaneously, the v0 SLA does not cover this.
Customer comms template in ADR-006 §6. Future Phase 3 procedure goes
here once the OVH-cross-region rclone sync is shipped.

---

## 4. Quarterly drill — scripted

The drill mirrors §1 above against a **staging** VPS with a **test
client's** archive. It MUST pass before a production restore is
attempted. Failure = P0 backlog item until green.

The drill script is committed at `ops/runbooks/dr-drill.sh`. Invoke:

```bash
ops/runbooks/dr-drill.sh test-client '2026-05-25 10:00:00 UTC'
```

Post-drill checklist (file under `docs/dr-drills/YYYYQX-<client>.md`):

- [ ] Total elapsed time ≤60 min (ADR-006 §5 target).
- [ ] Row counts on `core.audit_log`, `workspace_<id>.contact`,
      `workspace_<id>.deal` match pre-drill baseline.
- [ ] `wal-g backup-list` shows no gaps in the WAL chain.
- [ ] `wal-g delete retain FULL 7 --confirm` ran successfully (retention
      not silently broken).
- [ ] Drill ledger entry committed to `docs/dr-drills/`.
- [ ] If quarter's drill failed: P0 ticket open, drill re-run within 7
      days.

---

## 5. GPG key recovery drill (annual)

Simulates loss of the YubiKey. Recovery path:

1. Pull the encrypted private-key blob from the Bitwarden vault entry
   "leCRM — backup GPG private key (encrypted)".
2. Decrypt with the symmetric passphrase from the same Bitwarden
   entry's note field.
3. Import into a fresh YubiKey **or** a dedicated air-gapped restore
   laptop.
4. Re-run §1 above end-to-end on a staging VPS. Verify decryption of
   a recent WAL segment.
5. Record success/failure in `docs/dr-drills/YYYY-key-recovery.md`.

Failure modes to watch for: Bitwarden 2FA lockout (test the recovery
codes annually), YubiKey firmware mismatch, age key rotation breaking
SOPS-encrypted walg.env.

---

## 6. Out of scope

- Cross-region cold copy to a second OVH region — Phase 3 (ADR-006 §6).
- Per-tenant crypto-shredding (drop one tenant's data via key
  destruction) — v2 work (ADR-007 §4).
- Streaming replication / Patroni failover — Phase 3.
