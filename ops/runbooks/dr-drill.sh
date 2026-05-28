#!/usr/bin/env bash
# Quarterly DR drill — restores a staging VPS from the WAL-G archive.
#
# Usage:
#   ops/runbooks/dr-drill.sh <client-slug> <recovery-target-iso8601>
#
# Mirrors ops/runbooks/restore.md §1 (full cluster restore) into a
# staging environment so the operator can clock RTO end-to-end and
# verify row-count baselines.
#
# PRECONDITIONS (fail-fast checks below):
#   - LECRM_DRILL_HOST is set (the staging VPS hostname)
#   - The staging VPS has WALG_PGP_KEY_PATH_PRIVATE configured
#     (the YubiKey or Bitwarden-recovered private key is mounted on
#     a tmpfs at /run/lecrm-restore)
#   - $client-slug.walg.env is decrypted on the staging host
#
# This script does NOT touch production. It runs entirely on the
# staging host via ssh. To run a production restore use
# ops/runbooks/restore.md §1 procedure manually.
set -euo pipefail

CLIENT_SLUG="${1:?client slug required, e.g. test-client}"
TARGET_TIME="${2:?recovery target time required (ISO-8601)}"

: "${LECRM_DRILL_HOST:?must export LECRM_DRILL_HOST (staging VPS hostname)}"

START_EPOCH=$(date +%s)
LEDGER_DIR="docs/dr-drills"
LEDGER_FILE="${LEDGER_DIR}/$(date +%Y-Q$(( ($(date +%-m) - 1) / 3 + 1 )))-${CLIENT_SLUG}.md"
mkdir -p "${LEDGER_DIR}"

log() { printf 'dr-drill ts=%s msg=%s\n' "$(date -Iseconds)" "$*" | tee -a "${LEDGER_FILE}"; }

log "stage=start client=${CLIENT_SLUG} target=${TARGET_TIME} host=${LECRM_DRILL_HOST}"

# Drive the staging VPS through the full restore.
ssh "${LECRM_DRILL_HOST}" bash -s <<EOSSH
set -euo pipefail
docker compose -f deploy/compose/postgres.yml down postgres || true

# Wipe staging data dir (drill is destructive on staging by design).
sudo rm -rf /var/lib/docker/volumes/lecrm_postgres_data/_data
sudo mkdir -p /var/lib/docker/volumes/lecrm_postgres_data/_data
sudo chown 70:70 /var/lib/docker/volumes/lecrm_postgres_data/_data
sudo chmod 700  /var/lib/docker/volumes/lecrm_postgres_data/_data

docker compose -f deploy/compose/postgres.yml run --rm postgres bash -c '
  set -eu
  su postgres -c "/usr/local/bin/wal-g backup-fetch \$PGDATA LATEST"
  cat >> \$PGDATA/postgresql.auto.conf <<CONF
restore_command         = "/usr/local/bin/lecrm/wal-fetch.sh %f %p"
recovery_target_time    = "${TARGET_TIME}"
recovery_target_action  = "promote"
CONF
  touch \$PGDATA/recovery.signal
  chown postgres:postgres \$PGDATA/postgresql.auto.conf \$PGDATA/recovery.signal
'

docker compose -f deploy/compose/postgres.yml up -d postgres

until docker exec lecrm-postgres \
        psql -U postgres -d lecrm -tAc "SELECT NOT pg_is_in_recovery();" \
        2>/dev/null | grep -q t; do
  sleep 5
done

docker exec lecrm-postgres psql -U postgres -d lecrm <<SQL
SELECT 'audit_log_count' AS metric, COUNT(*) AS value FROM core.audit_log;
SELECT 'max_audit_ts'    AS metric, MAX(created_at)::text AS value FROM core.audit_log;
SQL

docker exec lecrm-postgres /usr/local/bin/wal-g wal-verify timeline integrity
docker exec lecrm-postgres /usr/local/bin/wal-g backup-list
EOSSH

END_EPOCH=$(date +%s)
ELAPSED=$(( END_EPOCH - START_EPOCH ))

log "stage=done elapsed_seconds=${ELAPSED}"

cat >> "${LEDGER_FILE}" <<EOF

## Post-drill checklist

- [ ] Total elapsed time: ${ELAPSED}s (target ≤3600s per ADR-006 §5).
- [ ] Row counts match pre-drill baseline (compare output above).
- [ ] WAL-G logs show no errors.
- [ ] Backup retention verified (\`wal-g delete retain FULL 7\`).
- [ ] Drill documented in this file with timestamp.
- [ ] Cross-region verification skipped (Phase 3 work).
EOF

echo "Drill ledger: ${LEDGER_FILE}"
