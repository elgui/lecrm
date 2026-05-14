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

## Bring-up order (target)

```
docker network create lecrm   # one-time; the postgres.yml also creates it
docker compose -f compose/postgres.yml up -d
# wait for healthcheck green
docker compose -f compose/authentik.yml up -d
docker compose -f compose/lgtm.yml up -d
# Caddy boots last; it depends on the upstream services existing on the
# `lecrm` network.
```

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
