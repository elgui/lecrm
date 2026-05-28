#!/usr/bin/env bash
# restore_command wrapper. PostgreSQL passes %f (segment to fetch) and
# %p (destination path on disk).
#
# wal-g wal-fetch downloads from OVH Object Storage, decrypts with
# WALG_PGP_KEY, and writes to %p. Exit non-zero if the segment is not
# yet available (PostgreSQL handles the retry / promote logic).
set -euo pipefail

if [[ -r /etc/postgres/walg.env ]]; then
  # shellcheck disable=SC1091
  set -a; . /etc/postgres/walg.env; set +a
fi

WAL_NAME="${1:?WAL filename required (PostgreSQL %f)}"
DEST_PATH="${2:?destination path required (PostgreSQL %p)}"

exec /usr/local/bin/wal-g wal-fetch "${WAL_NAME}" "${DEST_PATH}"
