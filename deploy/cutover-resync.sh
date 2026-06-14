#!/usr/bin/env bash
#
# cutover-resync.sh — final OVH→Netcup data re-sync, run AT cutover time.
#
# Run this ON the OVH box (51.77.146.49 / vps-25b8e3b3). It:
#   1. FENCES OVH (stops lecrm-api) so no new writes land after the snapshot
#      — this is what guarantees zero data loss / no split-brain.
#   2. Dumps the live OVH `lecrm` DB (pg_dump -Fc) + the current workspace
#      roles (discovered dynamically, so workspaces added since the first
#      migration are included).
#   3. Streams the dump over SSH into the NEW box, drops+recreates `lecrm`
#      there and pg_restore's it (API stopped during the swap).
#   4. Verifies row-count parity (core + every workspace schema). Aborts
#      nonzero on any mismatch.
#
# It does NOT touch DNS and does NOT decommission OVH. After it prints
# "PARITY OK", flip the wildcard DNS record:
#     *.lecrm.gbconsult.me  A  51.77.146.49 -> 152.53.143.175
# (leave the apex lecrm.gbconsult.me alone — it is a separate redirect.)
#
# OVH's lecrm-api is left STOPPED on success (so nobody writes to the old DB
# while DNS propagates). Rollback at any time: `sg docker -c "docker start
# lecrm-api"` on this box restores the OVH instance.
#
# Flags (env):
#   YES=1        skip the interactive confirmation
#   NO_FREEZE=1  do NOT stop OVH api; rely on pg_dump's consistent snapshot.
#                Faster / no demo outage, but any OVH write between the dump
#                and DNS propagation is LOST. Not recommended.
#
set -euo pipefail

# ---- connection params (current values; override via env if they change) ----
NEW_IP="${NEW_IP:-152.53.143.175}"
SSH_KEY="${SSH_KEY:-/home/gui/.ssh/lecrm_netcup_ed25519}"
KNOWN_HOSTS="${KNOWN_HOSTS:-/home/gui/.ssh/known_hosts_lecrm_netcup}"
WORK="$(mktemp -d /tmp/lecrm-cutover.XXXXXX)"
DUMP="$WORK/lecrm.dump"
ROLES="$WORK/roles-workspaces.sql"
SNAP_SQL="$WORK/snapshot.sql"
OVH_SNAP="$WORK/ovh.snap"
NEW_SNAP="$WORK/new.snap"
FROZE=0
SUCCESS=0

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }

ovh()  { sg docker -c "$1"; }                                    # docker on OVH (group shim)
ssh_new() { ssh -i "$SSH_KEY" -o UserKnownHostsFile="$KNOWN_HOSTS" \
                -o StrictHostKeyChecking=yes "root@$NEW_IP" "$1"; }   # docker direct on new box

# Roll back the freeze if we die before succeeding, so demo isn't left down.
cleanup() {
  if [[ "$FROZE" == 1 && "$SUCCESS" != 1 ]]; then
    log "FAILED after freezing OVH — restarting OVH lecrm-api so demo stays up on OVH."
    ovh "docker start lecrm-api" >/dev/null 2>&1 || \
      log "WARN: could not auto-restart OVH lecrm-api — run: sg docker -c 'docker start lecrm-api'"
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# ---- normalized count snapshot (identical SQL for both boxes) ----------------
cat > "$SNAP_SQL" <<'SQL'
CREATE TEMP TABLE _snap(k text);
INSERT INTO _snap VALUES ('workspaces='||(SELECT count(*) FROM core.workspaces));
INSERT INTO _snap VALUES ('users='||(SELECT count(*) FROM core.users));
INSERT INTO _snap VALUES ('members='||(SELECT count(*) FROM core.workspace_members));
DO $$
DECLARE r record; t text; n bigint;
BEGIN
  FOR r IN SELECT slug, role_name FROM core.workspaces LOOP
    FOREACH t IN ARRAY ARRAY['contacts','companies','deals','activities'] LOOP
      EXECUTE format('SELECT count(*) FROM %I.%I', r.role_name, t) INTO n;
      INSERT INTO _snap VALUES (format('ws:%s:%s=%s', r.slug, t, n));
    END LOOP;
  END LOOP;
END $$;
SELECT k FROM _snap ORDER BY k;
SQL

# ---- preflight ---------------------------------------------------------------
log "Preflight: checking OVH + new box health…"
ovh "docker exec lecrm-postgres pg_isready -U postgres -d lecrm" >/dev/null \
  || die "OVH lecrm-postgres not ready"
[[ "$(ssh_new 'docker inspect -f {{.State.Health.Status}} lecrm-postgres 2>/dev/null')" == healthy ]] \
  || die "new-box lecrm-postgres not healthy (or unreachable)"
log "Both clusters reachable."

if [[ "${YES:-0}" != 1 ]]; then
  echo "" >&2
  echo "  This will OVERWRITE the database on the NEW box ($NEW_IP) with a fresh" >&2
  echo "  dump of OVH, and (unless NO_FREEZE=1) STOP OVH's lecrm-api to fence writes." >&2
  echo "  OVH api stays stopped on success — flip DNS right after." >&2
  echo "" >&2
  read -r -p "  Proceed? [y/N] " ans
  [[ "$ans" =~ ^[Yy]$ ]] || die "aborted by operator"
fi

# ---- 1. fence OVH ------------------------------------------------------------
if [[ "${NO_FREEZE:-0}" != 1 ]]; then
  log "Fencing OVH: stopping lecrm-api (no further writes)…"
  ovh "docker stop lecrm-api" >/dev/null
  FROZE=1
else
  log "NO_FREEZE=1 — relying on pg_dump consistent snapshot; OVH stays writable (residual-write risk)."
fi

# ---- 2. snapshot + dump the frozen OVH state ---------------------------------
log "Snapshotting OVH counts…"
ovh "docker exec -i lecrm-postgres psql -U postgres -d lecrm -qtA -f -" < "$SNAP_SQL" > "$OVH_SNAP"
log "OVH snapshot: $(tr '\n' ' ' < "$OVH_SNAP")"

log "Dumping OVH lecrm (pg_dump -Fc)…"
ovh "docker exec lecrm-postgres pg_dump -U postgres -Fc -d lecrm" > "$DUMP"
log "Dump size: $(stat -c%s "$DUMP") bytes"

log "Capturing current workspace roles (dynamic)…"
mapfile -t BASES < <(ovh "docker exec lecrm-postgres psql -U postgres -d lecrm -tAc \"SELECT role_name FROM core.workspaces\"" | tr -d '\r')
[[ "${#BASES[@]}" -ge 1 ]] || die "no workspaces found on OVH"
PAT="$(IFS='|'; echo "${BASES[*]}")"
ovh "docker exec lecrm-postgres pg_dumpall -U postgres --roles-only" \
  | grep -E "($PAT)" > "$ROLES" || true
log "Workspace roles captured for: ${BASES[*]}"

# ---- 3. restore onto the new box ---------------------------------------------
log "New box: stopping lecrm-api for the swap…"
ssh_new "docker stop lecrm-api" >/dev/null 2>&1 || true

log "New box: ensuring workspace roles exist…"
ssh_new "docker exec -i lecrm-postgres psql -U postgres -d postgres -v ON_ERROR_STOP=0 -f -" < "$ROLES" >/dev/null 2>&1 || true

log "New box: drop + recreate empty lecrm…"
ssh_new "docker exec lecrm-postgres psql -U postgres -d postgres -c 'DROP DATABASE lecrm WITH (FORCE)'" >/dev/null
ssh_new "docker exec lecrm-postgres psql -U postgres -d postgres -c 'CREATE DATABASE lecrm'" >/dev/null

log "New box: pg_restore (streamed over SSH)…"
if ssh_new "docker exec -i lecrm-postgres pg_restore -U postgres -d lecrm" < "$DUMP" 2> "$WORK/restore.err"; then
  log "pg_restore OK"
else
  log "pg_restore reported errors:"; cat "$WORK/restore.err" >&2; die "restore failed"
fi

log "New box: restarting lecrm-api…"
ssh_new "docker start lecrm-api" >/dev/null

# ---- 4. verify parity --------------------------------------------------------
log "Snapshotting new-box counts…"
# brief settle so the API reconnect doesn't race the count query
sleep 3
ssh_new "docker exec -i lecrm-postgres psql -U postgres -d lecrm -qtA -f -" < "$SNAP_SQL" > "$NEW_SNAP"
log "New snapshot: $(tr '\n' ' ' < "$NEW_SNAP")"

if diff -u "$OVH_SNAP" "$NEW_SNAP" >/dev/null; then
  SUCCESS=1
  echo "" >&2
  log "================  PARITY OK  ================"
  log "New box ($NEW_IP) matches the frozen OVH state exactly."
  log "OVH lecrm-api is STOPPED (fenced). NEXT STEP — flip DNS:"
  log "    *.lecrm.gbconsult.me  A  ->  $NEW_IP   (leave the apex alone)"
  log "Rollback (re-enable OVH): sg docker -c 'docker start lecrm-api'"
  echo "" >&2
else
  echo "" >&2
  log "!!! PARITY MISMATCH — NOT safe to flip DNS. Diff (OVH vs new):"
  diff -u "$OVH_SNAP" "$NEW_SNAP" >&2 || true
  die "counts differ; investigate before cutover"
fi
