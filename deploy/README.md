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
  The DNS-01 API credential is a Tier-1 secret. **Provider is
  domain-dependent:** production `*.lecrm.fr` may use Cloudflare, but
  **staging `*.lecrm.gbconsult.me` uses the OVH provider** — the
  `gbconsult.me` zone is hosted on OVH (`dns104/ns104.ovh.net`), not
  Cloudflare. See the Staging section below.

## Staging (lecrm.gbconsult.me)

> **Status (2026-05-29):** host + secrets + edge strategy laid (this
> tasket). Full bring-up (DNS/TLS, api/caddy/pgbouncer, Léo access) is
> tasket order:2+. **TEMPORARY stopgap** — migrates to a fresh Hetzner
> CAX11 by **2026-06-12** (tasket order:5) for true infra isolation; the
> council knowingly waived the isolation gate on OVH (see the tasket).

### Host

- **`51.77.146.49`** (hostname `vps-25b8e3b3`), OVH VPS. Chosen as the
  **least-sensitive co-tenant box** per the council ruling. Co-tenants:
  `tele-claude`, `vps-mngr`, `aaraume-*`, `agentsim-redis` — all bound to
  `127.0.0.1` only.
- **⚠️ Infra-inventory correction:** `~/.claude/CLAUDE.md` lists "Main
  Dokku `54.37.157.49`" and "Static Sites `vps-4f99e005`" as separate
  machines — they are the **same physical box** (`hostname -I` confirms),
  co-hosting goali + chatboting (customer data) + voicekit + okolok. That
  box was therefore the *worst* blast-radius choice; `51.77.146.49` is the
  correct least-sensitive host.
- Capacity: 23 GB RAM (~6 GB free under current load), Docker 28.5.1.
  **Disk is tight — 87 % full, ~25 GB free** on `/`; watch image/volume
  growth, prune before bringing up cube/lgtm.

### Isolation (temporary OVH mitigations; real fix is the Hetzner migration)

- Dedicated `lecrm` Docker **bridge** network (not shared with co-located
  apps). No shared volumes or secrets with `tele-claude`/`vps-mngr`.
- **All DB ports are `127.0.0.1`-only** (verified on the running
  containers): `lecrm-postgres` → `127.0.0.1:54320`, `authentik-postgres`
  internal-only. No `0.0.0.0` publish anywhere on the host.
- **TODO (order:2):** host firewall allowing only 80/443 (+22 from the
  operator IP). Note nginx already serves co-located apps on 80/443, so
  the rule must keep those open.

### Edge — Option B (Caddy behind host nginx)

The host runs **systemd nginx** binding `0.0.0.0:80`/`:443` for the
co-located apps, so Caddy cannot own 80/443 directly (Option A is
unavailable). Plan: Caddy binds high ports; host nginx routes
`*.lecrm.gbconsult.me` to it. **Recommended for order:2:** an nginx
`stream {}` SNI passthrough (`*.lecrm.gbconsult.me` → Caddy's internal
443) so Caddy still terminates TLS and owns the DNS-01 wildcard — avoids
splitting TLS ownership.

### TLS / DNS-01 — OVH provider (not Cloudflare)

- `gbconsult.me` is on **OVH DNS** (`dns104.ovh.net`/`ns104.ovh.net`).
  `lecrm.gbconsult.me` has no record yet.
- Caddy must use **`caddy-dns/ovh`** — rebuild with
  `xcaddy build --with github.com/caddy-dns/ovh` (the current
  `caddy/Caddyfile` is hardcoded to Cloudflare + `*.lecrm.fr/.test`; it
  needs a staging variant for `*.lecrm.gbconsult.me` +
  `auth.lecrm.gbconsult.me`).
- Secret vars (in `.env.staging`, currently **empty placeholders** —
  fill before order:2): `OVH_ENDPOINT=ovh-eu`, `OVH_APPLICATION_KEY`,
  `OVH_APPLICATION_SECRET`, `OVH_CONSUMER_KEY`. Mint a token scoped to
  `GET/PUT/POST/DELETE /domain/zone/gbconsult.me/*` at
  <https://eu.api.ovh.com/createToken/>; **verify before use.**
  `CLOUDFLARE_API_TOKEN` is **not** used for staging.

### Secrets

- **`deploy/.env.staging.enc`** — SOPS-encrypted, lives on the host
  (gitignored; not committed — see below). Holds strong generated
  Postgres/Authentik/session secrets; OIDC client secret and OVH DNS-01
  creds are placeholders (filled in order:3 / order:2).
- **Age key:** a **dedicated, DISPOSABLE staging key** in
  `~/.config/sops/age/keys.txt` on this host — **not** the operator key
  (which stays on Guillaume's workstation/YubiKey per ADR-007 §2). Lowest
  blast radius for an internet-exposed stopgap; revoke/discard at the
  Hetzner migration. The recipient is wired as the first creation rule in
  `ops/secrets/.sops.yaml`.
- **Decrypt at boot:**
  ```
  SOPS_AGE_KEY_FILE=~/.config/sops/age/keys.txt \
    sops --config ops/secrets/.sops.yaml -d deploy/.env.staging.enc \
    > deploy/.env.staging      # mode 0600, gitignored
  ```
- **Not committed:** `.env.staging.enc` is gitignored (overriding the
  global `!.env.*.enc` allow) because it is encrypted to a host-only
  disposable key — no value baking it into permanent history.
- **sops 3.13.1 gotcha:** path_regex matches the **absolute** path, so the
  repo's `^secrets/…`/`^deploy/…` rules don't match under 3.13.1. The
  staging rule uses a `(^|/)`-boundary anchor. Flagged for the
  secrets-baseline tasket to fix the other rules.
- Tooling: `age` v1.3.1 + `sops` 3.13.1 installed via `go install` to
  `~/go/bin` (userspace, no sudo). `gui` added to the `docker` group.

### ⚠️ Current running state — reconcile in order:2

A **dev** bring-up is already running here (~2 weeks): `lecrm-postgres` +
`lecrm-authentik-{server,worker,postgres}`, all healthy, booted with
**`.env.dev`** secrets (hash-confirmed) and holding only dev-fixture data
(1 workspace / 1 user). This is **not** a real staging instance and its
secrets ≠ `.env.staging`. order:2 should **tear down the dev-fixture
stack and re-up against `.env.staging` on fresh volumes** (the fixture
data is disposable; nothing of value is lost).
