# `ops/` — leCRM operational tooling

Per-client Docker Compose templates and provisioning scripts for **Phase 1** (VPS-per-client, see ADR-001 §Phase 1).

## Layout

```
ops/
├── README.md                          this file
├── provision-client.sh                renders a per-client stack from templates
├── templates/
│   ├── docker-compose.template.yml   compose file with ${VAR} interpolation
│   ├── Caddyfile.template            Caddy reverse-proxy config (auto-HTTPS)
│   └── .env.template                 per-client env vars (rendered with secrets)
└── clients/                           rendered per-client stacks (.gitignored)
    └── <client-slug>/
        ├── docker-compose.yml
        ├── Caddyfile
        └── .env                       chmod 0600 — secrets
```

## Quick start

```bash
./ops/provision-client.sh \
    --client-slug acme \
    --subdomain acme.lecrm.fr \
    --client-name "Acme SARL" \
    --client-domain acme.fr

cd ops/clients/acme
docker compose up -d

# Smoke test once Caddy has the cert (~30s)
curl -fsS https://acme.lecrm.fr/api/version
```

## What the script does

1. Validates the client slug (lowercase, alphanumeric + hyphen, 2–31 chars).
2. Refuses to overwrite an existing `ops/clients/<slug>/` directory.
3. Generates a fresh 48-byte URL-safe `APP_SECRET` and Postgres password.
4. Renders `.env`, `docker-compose.yml`, and `Caddyfile` from templates.
5. Sets `chmod 0600` on the `.env` file (secrets).

## Sizing target

The compose template is sized for a **Hetzner CX22** (2 vCPU, 4 GB RAM):

| Container | Memory limit |
|---|---|
| server  | 1.5 GB |
| worker  | 0.75 GB |
| db      | 1.0 GB |
| redis   | 0.32 GB |
| caddy   | 0.13 GB |
| **sum** | **~3.7 GB** |

This leaves ≈300 MB headroom on a 4 GB VM — tight but adequate for 3–15 users per client. If memory pressure shows up in monitoring, the next step is CX32 (8 GB).

## What's NOT in v0

- **WAL-G backups + GPG client-side encryption** → ADR-006 backup baseline sub-tasket (G).
- **sops / age secret management** → ADR-007 secrets baseline sub-tasket (H).
- **Brevo Transactional API** (replacing the SMTP path) → email-track sub-tasket (B).
- **Embedded Metabase** → reporting sub-tasket (D).
- **Native sequences engine** (Brevo inbound parse + Gmail Watch + Graph) → sub-tasket (F), v1+.

Each is a discrete sub-tasket queued for after the v0 spine lands.
