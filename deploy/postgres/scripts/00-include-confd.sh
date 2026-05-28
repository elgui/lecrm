#!/usr/bin/env bash
# Runs once on a fresh data dir (the docker-entrypoint-initdb.d/ hook).
# Appends `include_dir = '/etc/postgresql/conf.d'` to postgresql.conf so
# our WAL-archiving drop-in is picked up.
set -euo pipefail

CONF_FILE="${PGDATA}/postgresql.conf"
INCLUDE_LINE="include_dir = '/etc/postgresql/conf.d'"

if ! grep -Fxq "${INCLUDE_LINE}" "${CONF_FILE}"; then
  printf '\n# leCRM: load WAL-G and tenant-tuning drop-ins.\n%s\n' \
    "${INCLUDE_LINE}" >> "${CONF_FILE}"
fi
