#!/usr/bin/env bash
# Weekly base-backup driver.
#
# Invoked from the wal-g-backup sidecar's cron (Sunday 03:00 UTC). Runs:
#   1. wal-g backup-push $PGDATA   — full base backup, GPG-encrypted
#   2. wal-g delete retain FULL 7  — keep 7 full backups (~49 days)
#   3. wal-g wal-verify timeline   — checks no gaps in the WAL chain
#
# Exits non-zero on any step failure so the cron job logs surface it.
# Output is JSON-ish key=value lines so the structured-slog scrape can
# pick them up.
set -euo pipefail

if [[ -r /etc/postgres/walg.env ]]; then
  # shellcheck disable=SC1091
  set -a; . /etc/postgres/walg.env; set +a
fi

PGDATA="${PGDATA:-/var/lib/postgresql/data/pgdata}"

log() { printf 'walg.backup ts=%s msg=%s\n' "$(date -Iseconds)" "$*"; }

log "stage=start pgdata=${PGDATA} prefix=${WALG_S3_PREFIX:-unset}"

log "stage=backup-push"
/usr/local/bin/wal-g backup-push "${PGDATA}"

log "stage=retention"
/usr/local/bin/wal-g delete retain FULL 7 --confirm

log "stage=verify"
/usr/local/bin/wal-g wal-verify timeline integrity || {
  log "stage=verify status=failed"
  exit 1
}

log "stage=done"
