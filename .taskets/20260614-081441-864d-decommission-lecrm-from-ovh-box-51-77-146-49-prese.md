---
id: 20260614-081441-864d
title: Decommission leCRM from OVH box 51.77.146.49 (preserve co-tenant apps)
status: later
priority: p2
created: 2026-06-14
updated: 2026-06-14
category: tooling
group: lecrm-staging-deploy
group_order: 220
order: 6
plan: true
tags: [deploy, staging, decommission, ovh, cleanup, co-tenant-safety]
---

## Goal
Cleanly remove the **leCRM stack** from the OVH box `51.77.146.49` (`vps-25b8e3b3`) now that staging runs on Netcup, **without disrupting the co-tenant apps that share the box**.

## Context (as of 2026-06-14)
- leCRM staging migrated to **Netcup `152.53.143.175`** (`lecrm-staging`, arm64). Wildcard `*.lecrm.gbconsult.me` DNS flipped + propagated; verified live (12/12 functional tests + OIDC login flow). WAL-G archiving ON (R2 prefix `s3://lecrm-wal/netcup-demo`, 2 base backups, `failed_count=0`).
- OVH `lecrm-api` is already **FENCED (stopped)** by the cutover script; the rest of the OVH lecrm containers are still present.
- ⚠️ **The OVH box is SHARED.** Co-tenants: `tele-claude`, `vps-mngr`, `aaraume-*`, `agentsim-redis` (all 127.0.0.1-bound). They **must not** be disrupted.
- ⚠️ **Scope everything to `lecrm-*` resources only. NEVER run `docker system prune -a` or any global prune/volume-prune** — it would wipe co-tenant images/volumes.
- Docker on OVH needs the group shim: `sg docker -c "docker ..."`.

## Pre-checks (before touching anything)
1. Netcup box healthy: `curl https://demo.lecrm.gbconsult.me/healthz` → 200; all 3 workspaces serve.
2. Backups still running on Netcup: `pg_stat_archiver.failed_count=0`, `wal-g backup-list` (with `set -a; . /etc/postgres/walg.env`) shows recent base backups.
3. DNS fully off OVH (public DoH shows `*.lecrm.gbconsult.me → 152.53.143.175`); no lecrm traffic hitting OVH.
4. **Grace period:** keep OVH lecrm containers *stopped but present* for ~7 days as a rollback path before deleting any volumes.

## Teardown — lecrm-scoped only
5. Stop + remove the lecrm containers via the compose files (NOT manual prune), from `/home/gui/Projects/leCRM`:
   `sg docker -c "docker compose --env-file deploy/.env.staging -f deploy/compose/postgres.yml -f deploy/compose/authentik.yml -f deploy/compose/api.yml -f deploy/compose/caddy.yml down"`
   Covers `lecrm-postgres`, `lecrm-api`, `lecrm-authentik-{server,worker,postgres}`, `lecrm-caddy`, `lecrm-walg-backup`. Also stop/remove `lecrm-pg-test` if still present.
6. **After the grace period + confirmed Netcup backups**, remove the lecrm volumes (hold the OLD data — irreversible): `postgres_data`, `authentik_db`, `caddy_data`, `caddy_config` (compose-prefixed names, e.g. `compose_postgres_data`). `docker volume rm` them explicitly by name.
7. Remove lecrm images (`lecrm/postgres`, `lecrm-api`, `lecrm-caddy:ovh`) by name and the `lecrm` docker network (`docker network rm lecrm`). Never touch co-tenant images.

## Secrets / files
8. Scrub leCRM secrets from the OVH checkout: `deploy/.env.staging`, `deploy/.env.staging.enc`, `deploy/postgres/walg.env`, and the **disposable staging SOPS age key** `~/.config/sops/age/keys.txt`. Decide whether to keep or archive the `/home/gui/Projects/leCRM` checkout itself.
9. **DO NOT revoke shared credentials** — they are REUSED by the live Netcup box: the OVH DNS-01 API token (`OVH_APPLICATION_KEY` / `OVH_APPLICATION_SECRET` / `OVH_CONSUMER_KEY`) and the Cloudflare R2 WAL-G keys. The OVH-side disposable SOPS age key is per-box and may be discarded.

## Host edge
10. Confirm `deploy/nginx/stream-lecrm.conf` was **never applied** (per `deploy/README.md` it was staged-only). If any lecrm nginx vhost/stream config is live, remove it and reload nginx **carefully** (the host nginx serves the co-tenant apps on :80/:443).

## Post-checks
11. Verify co-tenants still healthy: `tele-claude`, `vps-mngr`, `aaraume-*`, `agentsim-redis` (containers up; their endpoints respond). Confirm disk reclaimed and the box otherwise intact.

## Rollback (valid until volumes are deleted in step 6)
- Revive OVH: `sg docker -c "docker start lecrm-postgres lecrm-api lecrm-authentik-server lecrm-authentik-worker lecrm-authentik-postgres lecrm-caddy lecrm-walg-backup"` and revert the wildcard `*.lecrm.gbconsult.me` A-record back to `51.77.146.49`. Once step 6 deletes the volumes, this is no longer possible.
