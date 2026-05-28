#!/usr/bin/env bash
# archive_command wrapper.
#
# PostgreSQL passes %p (absolute path to the WAL segment) and %f (the
# segment filename). We pass %p to wal-g wal-push and let it stream the
# segment, GPG-encrypt it (WALG_PGP_KEY), and upload to OVH Object
# Storage (s3://lecrm-wal/<client-slug>/).
#
# IMPORTANT: archive_command MUST exit non-zero on failure. PostgreSQL
# will retry; the WAL segment stays on disk until the upload succeeds.
# Do NOT trap-and-swallow errors here.
#
# Env loaded from /etc/postgres/walg.env (mounted by Compose). See
# deploy/postgres/walg.env.example for the OVH-flavoured template.
set -euo pipefail

if [[ -r /etc/postgres/walg.env ]]; then
  # shellcheck disable=SC1091
  set -a; . /etc/postgres/walg.env; set +a
fi

WAL_PATH="${1:?WAL path required (PostgreSQL %p)}"
# %f is unused but logged for traceability.
WAL_NAME="${2:-$(basename "${WAL_PATH}")}"

exec /usr/local/bin/wal-g wal-push "${WAL_PATH}"
