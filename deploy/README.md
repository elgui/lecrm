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

- `compose/postgres.yml` — Postgres 17, runs the bootstrap migration
  (`packages/db/migrations/0001_init.sql`) on first start; subsequent
  schema changes are managed by Atlas via `cmd/lecrm-migrate`.
- `compose/authentik.yml` — Authentik 2025.10 server + worker. No Redis
  service (2025.10 removed the dependency). Postgres-backed cache.
- `compose/lgtm.yml` — Loki + Grafana + Tempo + Prometheus + OTel
  Collector. ~1.1 GB RAM. Wired in Sprint 11; metrics labelled with
  `workspace_id`.
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

LGTM and the Caddy edge are deferred to Sprint 11; they boot after the
Compose stack is the primary entry point (production deploy).

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
