# deploy/ — Compose stubs and edge config

## What this directory is

Compose-file stubs and Caddyfile for the leCRM v0 edge. Validated as
syntactically correct YAML/Caddy DSL; **not yet booted** as of Day-1
hand-off. Three external dependencies must land before
`docker compose up` succeeds:

1. **SOPS-encrypted `.env`** — secrets baseline tasket
   `20260510-162158-1023` (Sprint 3). Required environment variables:
   `POSTGRES_SUPERUSER_PASSWORD`, `AUTHENTIK_SECRET_KEY`,
   `AUTHENTIK_POSTGRES_PASSWORD`, `AUTHENTIK_BOOTSTRAP_PASSWORD`,
   `AUTHENTIK_BOOTSTRAP_EMAIL`, `GRAFANA_ADMIN_PASSWORD`,
   `CLOUDFLARE_API_TOKEN`.
2. **OIDC client provisioned in Authentik** — `lecrm-api` client with
   appropriate redirect URIs per workspace subdomain pattern.
3. **Custom Caddy build with `cloudflare.dns` module** — built via
   `xcaddy build --with github.com/caddy-dns/cloudflare` or the
   `caddy:builder` Docker image.

## Files

- `compose/postgres.yml` — Postgres 17 + WAL-G sidecar, runs the
  bootstrap migration (`packages/db/migrations/0001_init.sql`) on
  first start; subsequent schema changes are managed by Atlas via
  `cmd/lecrm-migrate`. The image is built from `deploy/postgres/`
  (Postgres + WAL-G + GnuPG + the lecrm-backup public key). Backup
  destination is OVH Object Storage; see
  `ops/runbooks/backup-bootstrap.md` for one-time provisioning and
  `ops/runbooks/restore.md` for restore procedures.
- `postgres/` — image layer for the WAL-G-enabled Postgres. Holds the
  Dockerfile, `postgresql.conf` drop-ins (archive_command,
  archive_timeout=60), the lecrm-backup GPG public key, and the
  wal-push/fetch/backup-push helper scripts.
- `postgres/walg.env.example` — OVH Object Storage env template
  (endpoint, region, IAM keys, GPG key path, brotli compression).
  Copy → fill → SOPS-encrypt before deploy. Per ADR-006 §1.
- `compose/authentik.yml` — Authentik 2025.10 server + worker. No Redis
  service (2025.10 removed the dependency). Postgres-backed cache.
- `compose/lgtm.yml` — Loki + Grafana + Tempo + Prometheus + OTel
  Collector. ~1.1 GB RAM. **Deferred** — not part of the default v0
  stack. v0 observability uses structured slog JSON to stdout + Grafana
  Cloud free tier (see `ops/observability.md`). Kept for optional local
  deep-debugging when needed.
- `caddy/Caddyfile` — wildcard DNS-01 TLS, CSP header per ADR-009 §5.2,
  reverse-proxy to `lecrm-api` and `authentik-server`.

## Local dev bring-up (verified Day-2)

```
# 1. One-time: external Compose network and dev env file.
docker network create lecrm
cp deploy/.env.dev.example deploy/.env.dev
# fill secrets (see comments at the top of the example file)

# 2. Postgres.
docker compose --env-file deploy/.env.dev -f deploy/compose/postgres.yml up -d
# 0001_init.sql + 0002_identity.sql are auto-applied on a fresh data dir.

# 3. Authentik.
docker compose --env-file deploy/.env.dev -f deploy/compose/authentik.yml up -d
# Wait ~60s for the initial migrations + bootstrap.

# 4. Provision the OIDC client in Authentik.
docker cp scripts/authentik-provision-oidc-client.py \
   lecrm-authentik-worker:/tmp/provision.py
docker exec lecrm-authentik-worker \
   ak shell -c "exec(open('/tmp/provision.py').read())"
# Capture CLIENT_SECRET from the output into deploy/.env.dev
# (LECRM_OIDC_CLIENT_SECRET=...).

# 5. Seed a workspace fixture.
psql "$LECRM_DATABASE_URL" -c \
   "SET ROLE lecrm_provisioner;
    SELECT core.lecrm_provision_workspace('11111111-1111-1111-1111-111111111111'::uuid);
    INSERT INTO core.workspaces (id, slug, role_name)
    VALUES ('11111111-1111-1111-1111-111111111111', 'acme',
            'workspace_11111111111111111111111111111111')
    ON CONFLICT (slug) DO NOTHING;"

# 6. Build + run lecrm-api.
go -C apps/api build -o /tmp/lecrm-api ./cmd/lecrm-api
set -a; source deploy/.env.dev; set +a
/tmp/lecrm-api

# 7. Smoke test the OIDC handshake.
curl -sS -D - -o /dev/null \
   -H 'Host: acme.lecrm.test:8080' \
   http://127.0.0.1:8080/auth/login
# Expect: 302 with Location=...Authentik authorize URL... and
# Set-Cookie: lecrm_oauth_state=... Domain=acme.lecrm.test
```

### End-to-end OIDC test (Day 3)

A build-tagged Go test drives the full flow — including the
post-Authentik half that the curl smoke test can't reach. Stop any
running lecrm-api on :8080 first; the test spawns its own.

```
# 1. Provision the test user (idempotent).
docker cp scripts/authentik-provision-test-user.py \
   lecrm-authentik-worker:/tmp/provision-user.py
docker exec lecrm-authentik-worker \
   ak shell -c "exec(open('/tmp/provision-user.py').read())"

# 2. Build + run the e2e test.
set -a; source deploy/.env.dev 2>/dev/null; set +a
~/.local/go/bin/go -C apps/api build -o /tmp/lecrm-api ./cmd/lecrm-api
LECRM_API_BIN=/tmp/lecrm-api \
   ~/.local/go/bin/go -C apps/api test -tags e2e -count 1 -v \
     -run TestE2EOIDCFlow ./internal/auth
```

The test asserts four properties of a completed login: the
`lecrm_session` cookie has `Domain=acme.lecrm.test` (not a parent-domain
wildcard); `core.users` has exactly one row keyed on the `(issuer,
subject)` tuple from the Authentik ID token; `core.workspace_members`
links that user to the `acme` workspace; `GET /auth/me` returns
`{user_id, workspace_id}` with both populated. The test is idempotent —
`UpsertUser` and `EnsureMember` collapse repeat runs into the same row.

LGTM is deferred indefinitely — the council review (2026-05-24) agreed
the ~1.1 GB RAM cost is unjustifiable at v0 with <5 clients. Observability
uses structured JSON slog to stdout + Grafana Cloud free tier instead
(see `ops/observability.md`). `compose/lgtm.yml` remains for optional
local deep-debugging.

The Caddy edge is deferred to Sprint 11; it boots after the Compose stack
is the primary entry point (production deploy).

## Decision log

- **Authentik over Zitadel Cloud EU** — see ADR-009 §7.1. v0 has zero
  users, so the migration cost (`(issuer, sub)` mapping + MFA
  re-enrolment, the 4-h gate from §7.1) is not yet incurred. The
  Postgres-backed cache in 2025.10 keeps the operational surface tight.
- **No Redis** — Authentik 2025.10 dropped the requirement;
  background jobs use `river` (Postgres-native, no Redis at v1 per
  ADR-009 §8.3).
- **Wildcard cert via DNS-01** — required because per-workspace
  subdomains can be created at any time without DNS propagation lag.
  Cloudflare DNS API token is a Tier-1 secret.
