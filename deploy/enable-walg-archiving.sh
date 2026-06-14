#!/usr/bin/env bash
#
# enable-walg-archiving.sh — turn ON WAL-G continuous archiving on the NEW
# (Netcup) box. STAGED, not auto-run: enable this AFTER the cutover data is in
# place and ideally right after the DNS flip, so the base backup captures the
# live post-cutover data and the new box owns its own WAL timeline.
#
# Run ON the new box (docker is direct there, no `sg` shim):
#     bash /opt/lecrm/deploy/enable-walg-archiving.sh
#
# Why it's off by default: the new cluster archives to WALG_S3_PREFIX
# (s3://lecrm-wal/netcup-demo — a DISTINCT prefix from OVH's …/demo) so it can
# never corrupt OVH's live backup timeline while OVH is still running. Once OVH
# is decommissioned you may reclaim the …/demo prefix (edit walg.env, re-run).
#
set -euo pipefail
cd /opt/lecrm

PGDATA_CONF=/var/lib/postgresql/data/pgdata/postgresql.conf
INCLUDE="include_dir = '/etc/postgresql/conf.d'"
log(){ printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }

# 1. walg.env must be readable by container uid 70, or archive_command/
#    backup-push silently skip sourcing it ("Failed to find any configured
#    storage"). Owner=70 is bulletproof (the host ACL is not honored through
#    the bind mount on this box). chown does not replace the inode, so the
#    single-file bind mount stays valid.
log "Making walg.env readable by uid 70…"
chown 70:70 deploy/postgres/walg.env
chmod 640 deploy/postgres/walg.env
docker exec -u 70 lecrm-postgres test -r /etc/postgres/walg.env \
  || { echo "walg.env still unreadable by uid 70 — aborting"; exit 1; }
log "WAL-G target: $(grep WALG_S3_PREFIX deploy/postgres/walg.env)"

# 2. Append the conf.d include to the data-dir postgresql.conf (idempotent).
#    archive_mode/archive_command live in /etc/postgresql/conf.d/10-wal-archiving.conf
#    (baked into the image); Postgres only loads it once this include is present.
if docker exec lecrm-postgres grep -qxF "$INCLUDE" "$PGDATA_CONF"; then
  log "include_dir already present in postgresql.conf."
else
  log "Appending include_dir to postgresql.conf…"
  docker exec -u 70 lecrm-postgres sh -c \
    "printf '\n# WAL-G archiving enabled out-of-band\n%s\n' \"$INCLUDE\" >> $PGDATA_CONF"
fi

# 3. Restart postgres — archive_mode is a restart-only GUC.
log "Restarting lecrm-postgres…"
docker restart lecrm-postgres >/dev/null
for i in $(seq 1 30); do
  [ "$(docker inspect -f '{{.State.Health.Status}}' lecrm-postgres 2>/dev/null)" = healthy ] && break
  sleep 2
done

AM=$(docker exec lecrm-postgres psql -U postgres -d lecrm -tAc "SHOW archive_mode")
log "archive_mode = $AM"
[ "$AM" = on ] || { echo "archive_mode is not 'on' — aborting"; exit 1; }

# 4. Start the backup sidecar (weekly base backup + one at boot).
log "Starting wal-g-backup sidecar…"
docker compose --env-file deploy/.env.staging -f deploy/compose/postgres.yml up -d wal-g-backup >/dev/null

# 5. Force an immediate base backup of the live (post-cutover) data.
log "Pushing an initial base backup…"
docker exec lecrm-walg-backup su postgres -c /usr/local/bin/lecrm/backup-push.sh

# 6. Verify.
log "Backups in R2 (wal-g backup-list):"
docker exec lecrm-walg-backup su postgres -c "wal-g backup-list" || true
log "Archiver health (want failed_count=0, last_archived_wal advancing):"
docker exec lecrm-postgres psql -U postgres -d lecrm -xc \
  "SELECT archived_count, failed_count, last_archived_wal, last_failed_wal, stats_reset FROM pg_stat_archiver"
log "Done — WAL archiving is ON."
