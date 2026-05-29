---
id: 20260529-1100-provision-ovh-host-secrets-edge
title: "Staging: provision OVH host, secrets, and resolve the 80/443 edge strategy"
status: todo
priority: p1
created: 2026-05-29
category: project
group: lecrm-staging-deploy
group_order: 220
order: 1
plan: true
tags: [deploy, staging, ovh, secrets, caddy, edge, leo-test, gbconsult]
---

# Staging: provision OVH host, secrets, and resolve the 80/443 edge strategy

## Goal of this group

Stand up a **persistent staging instance** of leCRM on an existing OVH VPS via docker-compose, at `*.lecrm.gbconsult.me`, so **Léo can test the app**. This first tasket lays the host + secrets + edge foundation; later taskets do DNS/TLS, boot the stack, and grant Léo access.

> **Human-in-the-loop:** This tasket needs SSH access to an OVH box and creates real secrets. It is **not** a hands-off automator step — run it interactively. Do NOT run `sudo` without explicit confirmation (global policy).

## Context

The deploy stack is fully scaffolded under `deploy/` (see `deploy/README.md`) and locally verified (Day-2/Day-3 bring-up), but **never booted on a real server**. Compose files exist: `deploy/compose/{postgres,authentik,mcp,migrate,cube,pgbouncer}.yml`; edge at `deploy/caddy/Caddyfile`; WAL-G image under `deploy/postgres/`.

Decisions (do not relitigate): **OVH VPS + docker-compose** as a **time-boxed stopgap**, domain **`*.lecrm.gbconsult.me`**, **persistent staging** (WAL-G backups on, tasket order:3).

> **Council ruling (2026-05-29, 4-voice debate).** The council unanimously preferred a **fresh Hetzner CAX11** over OVH — the decisive reason is **isolation**: the OVH boxes already run production apps (tele-claude, vps-mngr / static sites), so an internet-exposed, externally-tested CRM stack there shares a blast radius. Guillaume chose **OVH-now for speed, with a hard commitment to migrate to Hetzner within ~2 weeks** (tasket order:5, deadline 2026-06-12). Consequence: Rook's infra-level-isolation gate is **knowingly waived on OVH** and the migration is its remediation. Therefore this tasket MUST (a) pick the OVH box with the **least-sensitive co-tenants**, (b) enforce strict container/network isolation from co-located apps, and (c) build the stack as a **portable, env-parameterized artifact** (committed under `deploy/`) so the order:5 migration is config-only (<30 min).

Working directory: `/home/gui/Projects/leCRM`.

## The edge decision (resolve FIRST — it blocks tasket order:2)

The OVH boxes in `~/.claude/CLAUDE.md` run **Dokku, whose nginx already binds ports 80/443**. leCRM's Caddy edge wants 80/443 + a Cloudflare DNS-01 wildcard cert. Pick one and record the choice in this tasket and in `deploy/README.md`:

- **Option A — dedicated/free-ports box (recommended):** deploy on a box (or a box where Dokku does not claim 80/443) so Caddy binds 80/443 directly. Cleanest; Caddy owns TLS via DNS-01. Candidates: the secondary box `51.77.146.49` (lighter load — tele-claude/vps-mngr) or the static-sites VPS — **verify nothing else holds 80/443** (`sudo ss -tlnp | grep -E ':80|:443'`).
- **Option B — behind Dokku nginx:** Caddy binds high ports (e.g. 8443) and the existing nginx reverse-proxies `lecrm.gbconsult.me` to it. Adds a hop and splits TLS ownership; only if no box has free 80/443.

## Council security gates (hard — verify before anything ships; from Rook)

1. **Postgres binds `127.0.0.1` only** — never a public port (step 4 below; the crypto-mining scar).
2. **Cloudflare token scoped to the single `gbconsult.me` zone, DNS:Edit only** — and **verified via the Cloudflare API** before use (`GET /user/tokens/verify`), not trusted because "I made it narrow."
3. **Isolation from co-located prod apps** — true infra-level isolation is NOT possible on a shared OVH box (that is why we migrate). Compensate while on OVH: dedicated Docker network (no shared network with tele-claude/vps-mngr), host firewall allowing only 80/443 (+22 from your IP), no shared secrets/volumes. This is a *temporary mitigation*, not a substitute for gate 3 — order:5 is the real fix.

## Steps

1. **Pick the host.** Check capacity on candidate OVH boxes (the stack needs ~2–4 GB RAM: Postgres + Authentik + api + caddy). `ssh <user>@<host> "free -m; df -h /; docker info | grep -i version; ss -tlnp | grep -E ':80|:443' || true"`. Record the chosen host IP + the edge option (A/B) here and in `deploy/README.md`.
2. **Docker + network.** Ensure Docker is present and create the external network the compose files expect: `docker network create lecrm` (idempotent — ignore "already exists").
3. **Secrets (SOPS-encrypted, per ADR-007 + secrets-baseline tasket `20260510-162158-1023`).** Create the staging env file (e.g. `deploy/.env.staging`, SOPS-encrypted; never commit plaintext). Required vars (from `deploy/README.md`):
   - `POSTGRES_SUPERUSER_PASSWORD` — **strong, generated** (`openssl rand -base64 32`).
   - `AUTHENTIK_SECRET_KEY`, `AUTHENTIK_POSTGRES_PASSWORD`, `AUTHENTIK_BOOTSTRAP_PASSWORD`, `AUTHENTIK_BOOTSTRAP_EMAIL`.
   - `CLOUDFLARE_API_TOKEN` — scoped to **DNS:Edit on the gbconsult.me zone only** (used by Caddy DNS-01 in tasket order:2).
   - `LECRM_OIDC_CLIENT_SECRET` — placeholder; filled after OIDC client provisioning in tasket order:3.
   - `GRAFANA_ADMIN_PASSWORD` if LGTM is used (it is deferred per README — skip unless debugging).
4. **Postgres exposure guard (NON-NEGOTIABLE — see memory `feedback_test_postgres_localhost_only`; a prior test DB was crypto-mined after public exposure).** Confirm `deploy/compose/postgres.yml` does **not** publish 5432 to `0.0.0.0`. It must be either no host port (internal `lecrm` network only) or bound to `127.0.0.1:5432`. Fix the compose file if it exposes publicly. Same scrutiny for any pgbouncer/authentik-postgres port.
5. **Document** the host, edge option, and secrets location in `deploy/README.md` under a new "Staging (lecrm.gbconsult.me)" section.

## Done When

- [ ] Host chosen + capacity-verified; recorded in `deploy/README.md`.
- [ ] Edge option (A/B) decided and recorded; if A, 80/443 confirmed free on the host.
- [ ] `lecrm` docker network exists on the host.
- [ ] SOPS-encrypted staging env file present on the host with all required vars; strong Postgres password; Cloudflare token scoped to DNS:Edit on gbconsult.me only.
- [ ] `postgres.yml` (and any sibling DB) verified to NOT expose 5432 publicly (127.0.0.1 or internal-only).

## Completion Verification

1. `grep -nE '5432' deploy/compose/postgres.yml` -- any published port must be `127.0.0.1:5432` or absent (no `0.0.0.0`)
2. `ssh <host> "docker network ls | grep lecrm"` -- network exists
3. `ssh <host> "test -f <path>/.env.staging.sops || sops -d <path>/.env.staging > /dev/null"` -- encrypted env resolves
4. Commit (config/docs only — never secrets): `docs(deploy): staging host + edge strategy for lecrm.gbconsult.me`

## References

- `deploy/README.md` — bring-up runbook, required env vars, decision log
- `deploy/compose/postgres.yml` — DB exposure to verify
- `~/.claude/CLAUDE.md` — OVH infra inventory (hosts), sudo policy
- memory `feedback_test_postgres_localhost_only` — never expose Postgres publicly
- ADR-007 / tasket `20260510-162158-1023` — SOPS secrets baseline
