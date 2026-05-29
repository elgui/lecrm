# syntax=docker/dockerfile:1.7
#
# Postgres 17 + WAL-G + GnuPG image for leCRM Phase 1 (VPS-per-client).
#
# Adds, on top of postgres:17-alpine:
#   - wal-g (continuous WAL archiving + base backups)
#   - gnupg (client-side encryption of WAL segments and base backups)
#   - the lecrm-backup public key (baked at build time from
#     deploy/postgres/gpg/lecrm-backup.pub.asc)
#
# Per ADR-006: WAL-G is the canonical backup tool (pgBackRest archived
# 2026-04); GPG encryption preserves key sovereignty against bucket
# provider compromise; archive_timeout=60 caps RPO at ~60 s.
#
# Per ADR-009 §2.5: WAL-G remains canonical.
#
# Per STRATEGIC-OVERVIEW §2: object-storage target is OVH Object Storage
# (S3-compatible, EU). WAL-G's S3 driver is configured at runtime via
# AWS_ENDPOINT, AWS_S3_FORCE_PATH_STYLE, etc. — see
# deploy/postgres/walg.env.example.

ARG POSTGRES_VERSION=17-alpine
# v3.0.7 is the latest release that still names assets
# `wal-g-pg-ubuntu-<ver>-<arch>` (v3.0.8 dropped the `ubuntu-` infix, which
# would break the download line below). Bump the asset pattern too if moving
# past v3.0.7.
ARG WALG_VERSION=v3.0.7

FROM alpine:3.20 AS walg

ARG WALG_VERSION
ARG TARGETARCH

RUN apk add --no-cache curl ca-certificates \
 && case "${TARGETARCH}" in \
      amd64) WALG_ARCH=amd64 ;; \
      arm64) WALG_ARCH=aarch64 ;; \
      *)     echo "unsupported arch: ${TARGETARCH}" && exit 1 ;; \
    esac \
 && curl -fsSL -o /tmp/wal-g.tar.gz \
      "https://github.com/wal-g/wal-g/releases/download/${WALG_VERSION}/wal-g-pg-ubuntu-22.04-${WALG_ARCH}.tar.gz" \
 && tar -xzf /tmp/wal-g.tar.gz -C /tmp \
 && mv "/tmp/wal-g-pg-ubuntu-22.04-${WALG_ARCH}" /usr/local/bin/wal-g \
 && chmod +x /usr/local/bin/wal-g
# NB: no `wal-g --version` here — the binary is glibc-linked and this
# builder is bare alpine (musl), so it cannot execute yet. It is validated
# in the final stage below, which installs gcompat (the glibc shim).

FROM postgres:${POSTGRES_VERSION}

# wal-g release tarballs are linked against glibc; alpine ships musl.
# Use gcompat to provide a glibc-compatible shim. Alternatively swap the
# base to postgres:17 (debian) — kept alpine for parity with the existing
# deploy/compose/postgres.yml.
RUN apk add --no-cache gnupg ca-certificates gcompat libstdc++

COPY --from=walg /usr/local/bin/wal-g /usr/local/bin/wal-g

# Validate the glibc binary runs under gcompat (the smoke test that cannot
# run in the musl builder stage).
RUN /usr/local/bin/wal-g --version

# Bake the lecrm-backup public key into the image. WAL-G will reference
# it via WALG_PGP_KEY (armored PEM) at runtime, but having the file
# present lets operators verify the fingerprint inside the container.
COPY deploy/postgres/gpg/lecrm-backup.pub.asc /etc/postgres/gpg/lecrm-backup.pub.asc

# Postgres config drop-ins (archive_command, archive_timeout, etc).
COPY deploy/postgres/conf.d/ /etc/postgresql/conf.d/

# Helper scripts (archive_command wrapper, base-backup cron entry point).
COPY deploy/postgres/scripts/ /usr/local/bin/lecrm/
RUN chmod +x /usr/local/bin/lecrm/*.sh

# Make sure the postgres user can read the key file and config.
RUN chown -R postgres:postgres /etc/postgres/gpg /etc/postgresql/conf.d \
 && chmod 0644 /etc/postgres/gpg/lecrm-backup.pub.asc

# initdb payload — BAKED, not bind-mounted. A whole-dir bind mount of the
# migrations over /docker-entrypoint-initdb.d would shadow these baked files
# (it silently disabled WAL archiving before this was fixed) and can't host a
# nested :ro hook mount. Baking keeps the image self-contained: `up --build`
# on a fresh box runs the full schema + bootstrap with no extra mounts.
#
#   *.sql           — schema migrations, applied alphabetically as the
#                     postgres superuser on a fresh data dir (0010/0013 need
#                     superuser, so this is the canonical fresh-DB applier).
#   zz-bootstrap.sh — sorts last: sets the lecrm_api password, conditionally
#                     enables WAL archiving (only with a real walg.env), and
#                     records applied migrations in core.schema_migrations.
#
# (The old 00-include-confd.sh is superseded: zz-bootstrap.sh now appends the
# conf.d include conditionally instead of unconditionally.)
COPY packages/db/migrations/ /docker-entrypoint-initdb.d/
COPY deploy/postgres/initdb/zz-bootstrap.sh /docker-entrypoint-initdb.d/zz-bootstrap.sh
RUN chmod +x /docker-entrypoint-initdb.d/zz-bootstrap.sh
