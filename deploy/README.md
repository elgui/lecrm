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

> **Status (2026-05-29):** **full stack LIVE (order:3).**
> `https://demo.lecrm.gbconsult.me` is a working, populated CRM:
> `/healthz` 200, SPA served, `/auth/login` 302 → Authentik with the
> state cookie scoped to `demo.lecrm.gbconsult.me`. Running:
> `lecrm-postgres` (healthy), `lecrm-authentik-{server,worker,postgres}`,
> `lecrm-api`, `lecrm-caddy`. Edge (order:2) unchanged: custom Caddy +
> nginx L4 SNI on `:443` over the OVH DNS-01 wildcard.
> **WAL-G backups: LIVE (2026-05-30)** — pivoted from OVH Object Storage
> to **Cloudflare R2** (`lecrm-wal` bucket); base backup pushed, WAL
> archiving green (see "WAL-G backups" below).
> **Léo access: LIVE (2026-05-30)** — Authentik user `leo`
> (leo@vernayo.com) provisioned; verified a full browser login through
> Authentik → `/auth/callback` → seeded CRM (auto-created his `core.users`
> row + `member` of the `demo` workspace). See "Access / auth model" below.
> **Still TODO:** host firewall (80/443/22-only).
> **TEMPORARY stopgap** —
> migrates to a fresh Hetzner CAX11 by **2026-06-12** (order:5) for true
> infra isolation; the council knowingly waived the isolation gate on OVH.

### order:3 bring-up — what runs and how (live runbook)

Bring-up uses `.env.staging` (SOPS-decrypted on host, mode 0600). At ≤10
tenants there is **no PgBouncer** — `LECRM_DATABASE_URL` points straight at
`postgres:5432` as `lecrm_api` (ops/connection-pooling.md). Postgres is
published on host `127.0.0.1:54320` (admin only; 5432/5433/5434 are taken
by other tenants on the box).

```
# 1. Postgres (fresh data dir → initdb applies 0001..NNNN + zz-bootstrap).
docker compose --env-file deploy/.env.staging -f deploy/compose/postgres.yml up -d --build postgres
# 2. Authentik (wait ~90s for bootstrap).
docker compose --env-file deploy/.env.staging -f deploy/compose/authentik.yml up -d
# 3. OIDC client (staging redirect regex → all *.lecrm.gbconsult.me callbacks):
docker cp scripts/authentik-provision-oidc-client.py lecrm-authentik-worker:/tmp/p.py
docker exec -e LECRM_OIDC_REDIRECT_URI_REGEX='^https://[a-z0-9-]+\.lecrm\.gbconsult\.me/auth/callback$' \
  lecrm-authentik-worker ak shell -c "exec(open('/tmp/p.py').read())"
# → put CLIENT_SECRET in .env.staging (LECRM_OIDC_CLIENT_SECRET) + re-encrypt .enc.
# 4. API (builds embedded-SPA image; depends on postgres health).
docker compose --env-file deploy/.env.staging -f deploy/compose/postgres.yml -f deploy/compose/api.yml up -d --build api
# 5. Provision the demo workspace (superuser calls the wrapper; template seeds the 5 pipeline stages):
docker exec lecrm-postgres psql -U postgres -d lecrm -tAc \
  "SELECT core.lecrm_provision_workspace_with_registry(gen_random_uuid(),'demo','<email>','<email>','gbconsult-default')"
# 6. Seed demo data into the workspace schema (idempotent):
SCHEMA=$(docker exec lecrm-postgres psql -U postgres -d lecrm -tAc "SELECT role_name FROM core.workspaces WHERE slug='demo'")
docker exec -i lecrm-postgres psql -U postgres -d lecrm -v schema="$SCHEMA" -f /dev/stdin < deploy/seed/demo.sql
```

**Migrations apply via initdb, not the migrate-runner, on a fresh DB.**
0010 (`ALTER EXTENSION`) and 0013 require superuser, so the migrate-runner
(which connects as `lecrm_provisioner`) cannot apply them from scratch. The
postgres image therefore **bakes** the migrations + `zz-bootstrap.sh` into
`/docker-entrypoint-initdb.d`; they run as the superuser, then
`zz-bootstrap.sh` records them in `core.schema_migrations` so the runner
handles only incremental deploys on an existing DB. The `lecrm_api`
application role + its per-workspace grants are created by
`0017_app_role.sql` (it was missing entirely — the API authenticates as
`lecrm_api`); its password is set out-of-band by `zz-bootstrap.sh` from
`LECRM_PGBOUNCER_AUTH_PASS`.

**Bugs fixed during this bring-up** (this was the first end-to-end boot):
`deploy/postgres/Dockerfile` (wal-g `v3.0.3`→`v3.0.7`, drop the musl-stage
smoke test, bake initdb payload instead of a shadowing bind-mount);
`0016_service_tokens.sql` (partial index used non-IMMUTABLE `now()`);
`pgbouncer.yml` image tag (`1.22.0`→`1.22.1-p0`).

### WAL-G backups — LIVE (Cloudflare R2, 2026-05-30)

WAL archiving is **ON**; base backup pushed and verified. We pivoted the
destination from OVH Object Storage to **Cloudflare R2** (easier to mint
S3 creds). State on the box:

- **Bucket:** `lecrm-wal` (R2, WEUR). Account S3 endpoint
  `https://38346469bb9dc53151669b6cd6490009.r2.cloudflarestorage.com`
  (no bucket suffix — the bucket lives in `WALG_S3_PREFIX`). Per-workspace
  prefix `s3://lecrm-wal/demo`. R2 layout: `basebackups_005/`, `wal_005/`.
- **`deploy/postgres/walg.env`** filled with R2 knobs (gitignored, `0600`):
  `AWS_REGION=auto`, `AWS_S3_FORCE_PATH_STYLE=true`, account endpoint,
  Object-Read&Write token keys. Adapted from `.example` (which is
  OVH-specific: `AWS_REGION=gra` + OVH endpoint — **do not** copy those).
- **GPG key generated** — the repo previously shipped a *placeholder*
  `gpg/lecrm-backup.pub.asc` (decoded to literal `PLACEHOLDER`). A real
  rsa4096 keypair was generated (fpr in `gpg/lecrm-backup.fingerprint`,
  encryption subkey present). Public key committed; **private key is
  AES256-wrapped in Bitwarden** ("leCRM — backup GPG private key"), never
  on the host. Restore needs that private key (ADR-006).

**Two gotchas that bit the bring-up (keep for the Hetzner migration):**

1. **`walg.env` must be readable by container-uid 70.** The bind-mounted
   `walg.env` is `0600` owned by host uid 1001, but Postgres (and the
   sidecar's `su postgres`) run as **uid 70** inside the image, so
   `archive_command`/`backup-push.sh` silently skip sourcing it
   (`[[ -r ]]` false → `prefix=unset` → "Failed to find any configured
   storage"). Fix without world-reading the secret or sudo: a POSIX ACL —
   `setfacl -m u:70:r deploy/postgres/walg.env` (base mode stays 0600,
   `other::---`). The ACL is lost if the file inode is replaced (rewrite),
   so re-apply after editing `walg.env`. **On a fresh box this is the #1
   thing to redo.**
2. **Single-file bind mounts pin an inode.** After rewriting `walg.env`
   the running container still saw the *old* content until
   `docker restart lecrm-postgres` re-resolved the mount.

**Public key into the running containers (durability note):** the real
pub key was `docker cp`'d into `lecrm-postgres` + `lecrm-walg-backup` at
`/etc/postgres/gpg/lecrm-backup.pub.asc` (the image still has the baked
placeholder). This survives `restart`/reboot but is **lost on container
recreate** — rebuild the image to bake it permanently:
`sg docker -c "docker build -t lecrm/postgres:v0 deploy/postgres"` then
recreate. The committed repo file means the next build picks it up.

**Enabling archiving on an existing data dir (no volume reset):** the
data dir was initialized while `walg.env` was placeholders, so
`zz-bootstrap.sh` did **not** append the conf.d include. Added it manually
(never re-init — that wipes the demo): appended
`include_dir = '/etc/postgresql/conf.d'` to `$PGDATA/postgresql.conf` and
`docker restart lecrm-postgres` (archive_mode is restart-only).

**Run / re-run a backup:**
`sg docker -c 'docker exec lecrm-walg-backup su postgres -c /usr/local/bin/lecrm/backup-push.sh'`
The `wal-g-backup` sidecar (busybox crond) also runs it weekly Sun 03:00
UTC and once at boot. **Verify:** `wal-g backup-list`; archiver health via
`SELECT * FROM pg_stat_archiver` (want `failed_count=0`, `last_archived_wal`
advancing). These R2 S3 creds are separate from the OVH **DNS** creds in
`.env.staging`.

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
  **DNS records (verified 2026-05-29):** apex `lecrm.gbconsult.me` → A
  `54.37.157.49` (pre-existing redirect, **left untouched** — workspaces
  live on subdomains, the app does not own the apex). `*.lecrm.gbconsult.me`
  has **no record yet** — order:2 adds a wildcard **A → `51.77.146.49`**
  (A-only by decision; DNS-only, OVH has no CF-style proxy). DNS-01 cert
  issuance is independent of the record type.
- **Edge built (order:2, this tasket):**
  - Custom Caddy image **`lecrm-caddy:ovh`** (Caddy **v2.10.2**, built via
    `deploy/caddy/Dockerfile` = `caddy:2.10-builder` + `xcaddy --with
    caddy-dns/ovh --with caddy-dns/cloudflare`). Both providers bundled so
    one image serves prod (`*.lecrm.fr`/Cloudflare) and staging.
    Rebuild: `sg docker -c "docker build -t lecrm-caddy:ovh deploy/caddy"`.
  - **`deploy/caddy/Caddyfile.staging`** — `*.lecrm.gbconsult.me` wildcard,
    OVH DNS-01 `tls`/`acme_dns` blocks, `@auth host` → `authentik-server:9000`,
    everything else → `lecrm-api:8080` with the ADR-009 §5.2 CSP. A single
    wildcard cert covers `auth.*` too (no per-host certs). The production
    `deploy/caddy/Caddyfile` (Cloudflare/`*.lecrm.fr`) is left intact.
  - **`deploy/compose/caddy.yml`** — `lecrm-caddy` service on the `lecrm`
    network, publishes `127.0.0.1:8080`/`8443` only (host nginx fronts it),
    injects only the `OVH_*` creds (not the full env), persists `caddy_data`.
  - **`deploy/nginx/stream-lecrm.conf` + `README.md`** — staged (NOT
    applied) layer-4 SNI passthrough: host nginx `:443` → Caddy `:8443` for
    `*.lecrm.gbconsult.me`, all other SNI → relocated nginx vhosts on
    `127.0.0.1:8444`. Applying it touches the shared host's public `:443`
    (tele-claude/aaraume/conversation/drawlk) → runbook + go-ahead required.
- **⚠️ Still blocked — OVH API token.** Secret vars in `.env.staging` are
  **empty placeholders**: `OVH_APPLICATION_KEY`, `OVH_APPLICATION_SECRET`,
  `OVH_CONSUMER_KEY` (only `OVH_ENDPOINT=ovh-eu` is set). Mint a token
  scoped to `GET/PUT/POST/DELETE /domain/zone/gbconsult.me/*` at
  <https://eu.api.ovh.com/createToken/>; **verify before use.** Fill them,
  re-encrypt `.env.staging.enc`, then `up` the edge → Caddy issues the
  wildcard via DNS-01. `CLOUDFLARE_API_TOKEN` is **not** used for staging.

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

### Access / auth model (staging)

- **Login flow:** `https://<ws>.lecrm.gbconsult.me/auth/login` → Authentik
  (`auth.lecrm.gbconsult.me`, OIDC app `lecrm-api`) → `/auth/callback`.
  On callback the API does `UpsertUser` + `EnsureMember(workspace, user)`
  — first login auto-provisions the `core.users` row and a `member`
  binding to the workspace named by the subdomain.
- **⚠️ Open door — by-design gap to close before real clients.** The
  `lecrm` Authentik application has **no policy binding** (default-allow)
  and the callback has **no email allowlist/invite gate**. Net effect:
  **any** Authentik account that authenticates becomes a `member` of the
  workspace it logs into. Acceptable for this stopgap because Authentik is
  `127.0.0.1`-bound, self-registration is off, and users are provisioned
  by hand. **Before prod / Hetzner:** bind the `lecrm` app to a per-client
  group (or add an invite/allowlist check in `Callback`).
- **Provisioned users:** `akadmin` (Authentik admin), `guillaume-e2e`
  (dev e2e fixture), and **`leo`** (leo@vernayo.com — GB Consult partner,
  demo viewer). Add users via `ak shell` on `lecrm-authentik-worker`
  (pattern: `scripts/authentik-provision-test-user.py`).
- **e2e test caveat:** `apps/api/internal/auth/e2e_test.go`
  (`TestE2EOIDCFlow`) is **dev-only** — it hard-skips on any non-`.test`
  TLD and its redirect regex is pinned to `:8080`/`lecrm.test`, so it does
  **not** cover the staging host. Verify staging logins in a real browser.

### ⚠️ Current running state — reconcile in order:2

A **dev** bring-up is already running here (~2 weeks): `lecrm-postgres` +
`lecrm-authentik-{server,worker,postgres}`, all healthy, booted with
**`.env.dev`** secrets (hash-confirmed) and holding only dev-fixture data
(1 workspace / 1 user). This is **not** a real staging instance and its
secrets ≠ `.env.staging`. order:2 should **tear down the dev-fixture
stack and re-up against `.env.staging` on fresh volumes** (the fixture
data is disposable; nothing of value is lost).
