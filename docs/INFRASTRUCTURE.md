# Infrastructure, CI & Deployment

**Canonical map of where leCRM runs, how it gets there, and what CI does.**
Last verified against live hosts: **2026-05-30**.

This is the high-level reference. For operational step-by-step runbooks it
points to the authoritative sources rather than duplicating them:
- Bring-up / deploy commands → [`deploy/README.md`](../deploy/README.md)
- Edge / nginx SNI front → [`deploy/nginx/README.md`](../deploy/nginx/README.md)
- Backups, restore, DR, secret rotation → [`ops/runbooks/`](../ops/runbooks/)
- Integrator (Léo) tenant CLI → [`docs/integrator-handoff.md`](integrator-handoff.md)
- Stack/license/CI decisions → [`docs/adr/ADR-009-stack-and-license.md`](adr/ADR-009-stack-and-license.md)

---

## TL;DR — how many instances are running?

**One.** A single **staging** stack runs the leCRM application
(`*.lecrm.gbconsult.me`). There is **no production instance yet**.

The `lecrm-admin` Dokku app on `54.37.157.49` is **not** a second instance
of the app — it is a one-shot tenant-provisioning CLI (`Running: false`,
0 processes) invoked on demand via `dokku run`. See "Tooling" below.

---

## Environment matrix

| Environment | Status | Host | Deploy model | Public domain | Env file |
|---|---|---|---|---|---|
| **dev** | on-demand | local laptop | `docker compose` + native binary | `*.lecrm.test:8080` | `deploy/.env.dev` |
| **staging** | **LIVE** | `51.77.146.49` (`vps-25b8e3b3`) | `docker compose` from the host git checkout | `*.lecrm.gbconsult.me` | `deploy/.env.staging` (SOPS) |
| **production** | **not deployed (planned)** | dedicated Hetzner CAX11 (migration target) | `docker compose` (same files) | `*.lecrm.fr` (Cloudflare DNS-01) | TBD |

Notes:
- **Production does not exist.** The migration to a dedicated Hetzner CAX11
  for true infra isolation is the planned production substrate (target
  **2026-06-12** per `deploy/README.md`). Staging on shared OVH is an
  explicitly council-waived stopgap.
- Earlier docs (`docs/integrator-handoff.md`, CI comments) reference a
  "production Dokku host `54.37.157.49`". That phrasing applies **only** to
  the `lecrm-admin` CLI image, *not* the API/web app. The API is not, and is
  not planned to be, served from Dokku — it runs via Compose. See "Known
  divergences".

---

## Staging stack (live, inspected 2026-05-30)

Host `51.77.146.49` (`vps-25b8e3b3`). Compose project **`compose`**, files
`deploy/compose/postgres.yml` + `deploy/compose/api.yml`, working dir
`/home/gui/Projects/leCRM/deploy/compose`. All app ports bind to
**loopback only** — the public entry point is the edge (below).

| Container | Image | Host port (loopback) | Role |
|---|---|---|---|
| `lecrm-api` | `lecrm-api:staging` (built from `deploy/Dockerfile`) | `127.0.0.1:8088→8080` | REST `/v1/*` + embedded React SPA |
| `lecrm-postgres` | `lecrm/postgres:v0` (Postgres 17 + WAL-G) | `127.0.0.1:54320→5432` | Primary DB (all tenant schemas) |
| `lecrm-walg-backup` | WAL-G sidecar | — | Continuous WAL archiving → Cloudflare R2 (`lecrm-wal`) |
| `lecrm-authentik-server` | `ghcr.io/goauthentik/server:2025.10` | `127.0.0.1:9000→9000`, `9443` | OIDC IdP |
| `lecrm-authentik-worker` | `ghcr.io/goauthentik/server:2025.10` | — | Authentik background worker |
| `lecrm-authentik-postgres` | `postgres:17-alpine` | — | Authentik's own DB |
| `lecrm-caddy` | `lecrm-caddy:ovh` | `127.0.0.1:8443→443`, `8080→80` | TLS edge, wildcard cert, reverse proxy |
| `lecrm-pg-test` | `postgres:16` | `127.0.0.1:5434→5432` | Integration/test DB (loopback-bound, see [[feedback_test_postgres_localhost_only]]) |

Health: `https://demo.lecrm.gbconsult.me/healthz` → `200`. The distroless
API image has no shell, so readiness is checked out-of-band via the edge,
not a container healthcheck.

At ≤10 tenants there is **no PgBouncer** — `LECRM_DATABASE_URL` points
straight at `postgres:5432` as `lecrm_api`. The `pgbouncer.yml` layer is
added only at ≥10 tenants (`ops/connection-pooling.md`).

### Edge / networking (Edge Option B)

The host's shared systemd nginx already terminates TLS for several
`*.gbconsult.me` vhosts on `0.0.0.0:443`. Caddy must own the
`*.lecrm.gbconsult.me` wildcard (DNS-01) and terminate its own TLS, so
nginx runs an **L4 SNI passthrough** in front:

```
client ──:443──► host nginx (ssl_preread, no TLS termination)
                 ├─ *.lecrm.gbconsult.me ─► 127.0.0.1:8443  (lecrm-caddy)
                 └─ everything else       ─► 127.0.0.1:8444  (nginx vhosts)
```

Caddy then routes on the `lecrm` Docker network:
- `auth.lecrm.gbconsult.me` → `authentik-server:9000`
- every other `*.lecrm.gbconsult.me` (workspace subdomains) → `lecrm-api:8080`

A single wildcard cert (`*.lecrm.gbconsult.me`) serves every workspace
subdomain. **DNS-01 provider is OVH** (the `gbconsult.me` zone lives on OVH
`dns104/ns104.ovh.net`), not Cloudflare. Applied 2026-05-29; rollback
procedure in `deploy/nginx/README.md`.

---

## Tooling (not app instances)

| Thing | Host | State | Purpose |
|---|---|---|---|
| `lecrm-admin` Dokku app | `54.37.157.49` (main Dokku) | `Deployed: true`, **`Running: false`**, 0 processes, no web vhost | One-shot tenant-provisioning CLI. Léo runs it via `dokku run lecrm-admin tenant ...` (his `gb-tenant` alias). Built from `apps/admin/Dockerfile`; `web=0` so it never auto-starts. See `docs/integrator-handoff.md`. |

---

## CI pipeline

`.github/workflows/ci.yml`. Triggers: PRs to `main` and pushes to `main`.
**CI builds and tests only — it does not deploy or push images anywhere.**

| Job | What it does |
|---|---|
| `go` | `apps/api`: `go mod verify`, `go test -race ./...` (testcontainers Postgres), `golangci-lint`, `gosec`, `govulncheck` |
| `build-admin` | `apps/admin`: service-container Postgres 16, `lecrm-migrate apply`, integration tests, lint, then a tenant create/verify smoke test |
| `atlas` | `atlas migrate lint` on the newest migration (pinned to Atlas v0.37 — last unauthenticated lint release) |
| `web` | `apps/web`: `pnpm typecheck`, `pnpm test`, `pnpm build` |
| `docker` | Multi-stage build of `lecrm-api:ci` (Vite → Go → distroless). **`push: false`** |
| `docker-admin` | Build `lecrm-admin:ci`. **`push: false`** |

There is **no deploy job**. Image promotion was always described as "a
separate deploy workflow" that has not been built. A red `main` therefore
blocks future automated promotion but does **not** take down staging
(staging was brought up manually and is not redeployed by CI).

---

## Deployment process (reality)

Staging is updated **manually on the host**, not by CI:

1. SSH to the host: `ssh gui@51.77.146.49` (user `gui`; `debian`/`dokku`/`root` are denied).
2. Update the checkout at `/home/gui/Projects/leCRM` (e.g. `git pull`).
3. Decrypt secrets: `sops -d deploy/.env.staging.enc > deploy/.env.staging` (mode 0600).
4. Build + restart from source:
   ```bash
   docker compose --env-file deploy/.env.staging \
     -f deploy/compose/postgres.yml \
     -f deploy/compose/api.yml up -d --build
   ```

The full, authoritative bring-up (Postgres → Authentik → OIDC client →
API → workspace seed) is in **`deploy/README.md` → "Staging" section**.

---

## Backups & observability

- **Backups: LIVE (2026-05-30).** WAL-G archives base backups + WAL to
  **Cloudflare R2** bucket `lecrm-wal` (pivoted from OVH Object Storage).
  Provisioning + restore: `ops/runbooks/backup-bootstrap.md`,
  `ops/runbooks/restore.md`. GPG-encrypted, brotli-compressed.
- **Observability:** structured slog JSON to stdout + Grafana Cloud free
  tier (`ops/observability.md`). The full LGTM stack (`compose/lgtm.yml`,
  ~1.1 GB RAM) is **deferred indefinitely** per the 2026-05-24 council
  review — kept only for optional local deep-debugging.

---

## Known divergences & caveats

1. **"Production Dokku" wording is about the CLI, not the app.**
   `docs/integrator-handoff.md` and CI comments call `54.37.157.49` the
   "production Dokku host". That is correct *only* for the `lecrm-admin`
   provisioning CLI. The API/web app does not run on Dokku and is not
   planned to — production will be a Compose stack on a dedicated Hetzner
   CAX11.
2. **CI does not deploy.** Staging is hand-deployed; keep that in mind when
   reasoning about what a green/red `main` actually gates.
3. **Host working-tree drift.** The staging checkout
   (`/home/gui/Projects/leCRM`) has had uncommitted local modifications to
   Go source/test files. Because deploys build from that working tree, the
   running image can diverge from `main`. Prefer a clean `git pull` + build,
   or pin via `LECRM_IMAGE_TAG`, before relying on the deployed artifact.
4. **Shared-host stopgap.** Staging co-exists with unrelated apps
   (`aaraume-*`, `agentsim-redis`, …) on the OVH box. The isolation gate was
   knowingly waived pending the Hetzner migration (target 2026-06-12).
