#!/bin/sh
# zz-bootstrap.sh — runs LAST in /docker-entrypoint-initdb.d (the "zz" prefix
# sorts after the numbered SQL migrations) on a FRESH data dir only.
#
# Three jobs, all requiring the migrations (0001..0017) to have already run:
#
#   1. Enable WAL archiving ONLY when a real walg.env is mounted. The
#      archive_mode/archive_command live in conf.d/10-wal-archiving.conf,
#      which Postgres loads only if `include_dir = conf.d` is appended to
#      postgresql.conf. We append it conditionally so a box without OVH
#      Object Storage creds boots clean (archive_mode stays off) instead of
#      piling up un-archivable WAL. (This supersedes the baked
#      00-include-confd.sh, which the compose migrations bind-mount shadows.)
#
#   2. Set the lecrm_api LOGIN password. 0017_app_role.sql creates the role
#      with no password (no secret in version control); we set it here from
#      the env so the API and PgBouncer can authenticate. Mirrors the
#      out-of-band ALTER ROLE pattern documented in 0013.
#
#   3. Record the migrations initdb just applied in core.schema_migrations,
#      so the Go migrate-runner (which tracks that table) treats them as
#      done and only applies genuinely new files on later deploys — never
#      re-running 0010's superuser-only ALTER EXTENSION.
#
# Portability: this is what keeps a fresh Hetzner box a `docker compose up`
# away — no manual psql step. Everything it needs comes from the env +
# mounted walg.env.
set -eu

PSQL="psql -v ON_ERROR_STOP=1 --username ${POSTGRES_USER} --dbname ${POSTGRES_DB} -tA"
WALG_ENV=/etc/postgres/walg.env

# --- 1. Conditional WAL archiving -----------------------------------------
# Treat walg.env as "real" only if it exists and still carries neither of the
# template placeholders from walg.env.example.
if [ -r "${WALG_ENV}" ] \
   && ! grep -q 'CLIENT_SLUG_HERE' "${WALG_ENV}" \
   && ! grep -q 'REPLACE_WITH_OVH' "${WALG_ENV}"; then
  CONF="${PGDATA}/postgresql.conf"
  INCLUDE_LINE="include_dir = '/etc/postgresql/conf.d'"
  if ! grep -Fxq "${INCLUDE_LINE}" "${CONF}"; then
    printf '\n# leCRM: load WAL-G archiving drop-ins (zz-bootstrap.sh).\n%s\n' \
      "${INCLUDE_LINE}" >> "${CONF}"
    echo "zz-bootstrap: WAL archiving ENABLED (real walg.env present)"
  fi
else
  echo "zz-bootstrap: WAL archiving left OFF (no real walg.env mounted)"
fi

# --- 2. Application-role password -----------------------------------------
if [ -z "${LECRM_API_DB_PASSWORD:-}" ]; then
  echo "zz-bootstrap: FATAL — LECRM_API_DB_PASSWORD not set; lecrm_api would have no password" >&2
  exit 1
fi
# Inline with SQL-escaped quoting. (psql's :'var' interpolation is unreliable
# via -c; it reached the server literally and errored.) Double any single
# quotes so the literal is safe.
esc_pw=$(printf '%s' "${LECRM_API_DB_PASSWORD}" | sed "s/'/''/g")
${PSQL} -c "ALTER ROLE lecrm_api LOGIN PASSWORD '${esc_pw}'"
echo "zz-bootstrap: lecrm_api password set"

# --- 3. Seed the migration-tracking table ---------------------------------
${PSQL} -c "CREATE SCHEMA IF NOT EXISTS core"
${PSQL} -c "CREATE TABLE IF NOT EXISTS core.schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())"
for f in /docker-entrypoint-initdb.d/[0-9]*.sql; do
  [ -e "${f}" ] || continue
  name=$(basename "${f}")
  ${PSQL} -c "INSERT INTO core.schema_migrations (name) VALUES ('${name}') ON CONFLICT DO NOTHING"
done
echo "zz-bootstrap: core.schema_migrations seeded with applied migrations"
