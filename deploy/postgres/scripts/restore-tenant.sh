#!/usr/bin/env bash
# Per-tenant restore workflow (Phase 2 surgical restore from ADR-001
# §Backup mechanics, applied to Phase 1 VPS-per-client where we still
# have a single workspace per cluster).
#
# Spins up a sidecar Postgres pointed at a recovered base backup,
# pg_dumps the requested workspace_<id> schema, then pg_restore-s it
# into the live cluster after dropping the damaged schema.
#
# Usage:
#   restore-tenant.sh <workspace-id> <recovery-target-time-iso8601>
#
# Example:
#   restore-tenant.sh 7k3q4m9p '2026-05-27 14:32:00 UTC'
#
# Requires:
#   - /etc/postgres/walg.env present (S3 creds + GPG key)
#   - WALG_PGP_KEY_PATH pointing at the private key (mounted at runtime,
#     never baked into the image)
#   - a side cluster directory writable by the postgres user
set -euo pipefail

WORKSPACE_ID="${1:?workspace id required, e.g. 7k3q4m9p}"
TARGET_TIME="${2:?recovery target time required (ISO-8601)}"

SIDE_DATA="${LECRM_RESTORE_SIDE_DATA:-/var/lib/postgresql/restore-side}"
DUMP_PATH="${LECRM_RESTORE_DUMP_PATH:-/tmp/workspace_${WORKSPACE_ID}.dump}"
DB_NAME="${LECRM_DB_NAME:-lecrm}"
DB_USER="${LECRM_DB_USER:-postgres}"

if [[ -r /etc/postgres/walg.env ]]; then
  # shellcheck disable=SC1091
  set -a; . /etc/postgres/walg.env; set +a
fi

log() { printf 'walg.restore ts=%s msg=%s\n' "$(date -Iseconds)" "$*"; }

[[ -d "${SIDE_DATA}" ]] && {
  log "stage=preflight error=side-data-exists path=${SIDE_DATA}"
  echo "Refusing to clobber existing side cluster at ${SIDE_DATA}" >&2
  exit 1
}

log "stage=fetch-base prefix=${WALG_S3_PREFIX:-unset}"
mkdir -p "${SIDE_DATA}"
chown postgres:postgres "${SIDE_DATA}"
chmod 700 "${SIDE_DATA}"
su postgres -c "/usr/local/bin/wal-g backup-fetch '${SIDE_DATA}' LATEST"

log "stage=write-recovery-signal target=${TARGET_TIME}"
cat > "${SIDE_DATA}/postgresql.auto.conf" <<EOF
restore_command          = '/usr/local/bin/lecrm/wal-fetch.sh %f %p'
recovery_target_time     = '${TARGET_TIME}'
recovery_target_action   = 'promote'
EOF
touch "${SIDE_DATA}/recovery.signal"
chown postgres:postgres "${SIDE_DATA}/postgresql.auto.conf" "${SIDE_DATA}/recovery.signal"

log "stage=start-side-cluster port=5433"
su postgres -c "pg_ctl -D '${SIDE_DATA}' -o '-p 5433' -l '${SIDE_DATA}/restore.log' start"

# Wait for recovery to promote.
until su postgres -c "psql -h 127.0.0.1 -p 5433 -tAc 'SELECT NOT pg_is_in_recovery();'" 2>/dev/null | grep -q t; do
  log "stage=wait-recovery"
  sleep 5
done
log "stage=side-cluster-ready"

log "stage=pg_dump schema=workspace_${WORKSPACE_ID}"
su postgres -c "pg_dump -h 127.0.0.1 -p 5433 -d '${DB_NAME}' -n 'workspace_${WORKSPACE_ID}' -Fc -f '${DUMP_PATH}'"

log "stage=stop-side-cluster"
su postgres -c "pg_ctl -D '${SIDE_DATA}' stop -m fast"

cat <<EOF

Per-tenant dump captured at:
  ${DUMP_PATH}

Next steps (manual, irreversible — review before running):

  # 1. Verify the dump non-empty.
  pg_restore -l '${DUMP_PATH}' | head -20

  # 2. Drop the damaged schema in the live cluster.
  psql -U ${DB_USER} -d ${DB_NAME} -c 'DROP SCHEMA "workspace_${WORKSPACE_ID}" CASCADE;'

  # 3. Restore into the live cluster.
  pg_restore -U ${DB_USER} -d ${DB_NAME} -n 'workspace_${WORKSPACE_ID}' '${DUMP_PATH}'

  # 4. Verify row counts against the pre-incident baseline.
  psql -U ${DB_USER} -d ${DB_NAME} -c "SELECT COUNT(*) FROM workspace_${WORKSPACE_ID}.contact;"

  # 5. Tear down the side cluster directory.
  rm -rf '${SIDE_DATA}'

EOF
