---
id: 20260529-1102-boot-staging-stack-workspace-seed
title: "Staging: boot the stack, run migrations, provision demo workspace + seed data + backups"
status: todo
priority: p1
created: 2026-05-29
category: project
group: lecrm-staging-deploy
group_order: 220
order: 3
plan: true
tags: [deploy, staging, compose, migrations, workspace, seed, walg, leo-test]
---

# Staging: boot the stack, run migrations, provision demo workspace + seed data + backups

## Pre-flight: Verify Previous Taskets

1. `grep -q "lecrm.gbconsult.me" deploy/caddy/Caddyfile` -- edge config done (order:2)
2. `dig +short demo.lecrm.gbconsult.me` -- resolves to staging host (order:2)
3. Staging SOPS env present on host with Postgres/Authentik/Cloudflare secrets (order:1)

**If any check fails, STOP and report. Do not proceed.**

> **Human-in-the-loop:** boots a live stack on a real host and provisions secrets/workspaces. Run interactively; no `sudo` without confirmation.

## Context

With the host, secrets, DNS, and TLS in place, bring the full stack up on the OVH box and make `https://demo.lecrm.gbconsult.me` a working, populated CRM. Bring-up order and commands follow `deploy/README.md` (Day-2/Day-3 runbook), adapted from local (`lecrm.test`) to the live domain.

Decisions: **persistent staging** → WAL-G backups ON (OVH Object Storage, per ADR-006 + tasket `20260510-162158-d1ba`). Demo workspace slug `demo` → `demo.lecrm.gbconsult.me`.

Working directory: `/home/gui/Projects/leCRM`. Source of truth: `deploy/README.md`.

## Steps

1. **Postgres** (`deploy/compose/postgres.yml`, WAL-G image). Boot first; `0001_init.sql` + `0002_identity.sql` auto-apply on a fresh data dir. Re-confirm no public 5432 (order:1 guard).
2. **Migrations** via `cmd/lecrm-migrate` (`deploy/compose/migrate.yml`) — Atlas applies the full schema (through `0016_service_tokens.sql` and the ADR-012 capability-layer migrations). Pre-deploy ordering: migrate exits 0 → api starts (ADR-009 §8.2).
3. **Authentik** (`deploy/compose/authentik.yml`). Wait ~60s for bootstrap. Then provision the OIDC client:
   ```
   docker cp scripts/authentik-provision-oidc-client.py <authentik-worker>:/tmp/provision.py
   docker exec <authentik-worker> ak shell -c "exec(open('/tmp/provision.py').read())"
   ```
   Set redirect URIs for the live pattern (`https://demo.lecrm.gbconsult.me/auth/callback`, and the wildcard workspace pattern). Capture `CLIENT_SECRET` → fill `LECRM_OIDC_CLIENT_SECRET` in the SOPS env.
4. **lecrm-api** — build with the embedded SPA (`//go:embed`, ADR-009 §5.1) and run as a compose service on the `lecrm` network (add an `api.yml` or extend an existing compose file if one is not already present). Env from the staging SOPS file. Set the OIDC issuer to `https://auth.lecrm.gbconsult.me`.
5. **Caddy edge** — bring up per the order:1 edge option; confirm it proxies `demo.lecrm.gbconsult.me` → `lecrm-api` and `auth.lecrm.gbconsult.me` → `authentik-server` over the wildcard cert.
6. **Provision the demo workspace** (mirror `deploy/README.md` step 5, live slug):
   ```
   SET ROLE lecrm_provisioner;
   SELECT core.lecrm_provision_workspace('<uuid>'::uuid);
   INSERT INTO core.workspaces (id, slug, role_name)
   VALUES ('<uuid>', 'demo', 'workspace_<uuid-no-dashes>')
   ON CONFLICT (slug) DO NOTHING;
   ```
7. **Seed demo data** so Léo sees a populated CRM (not an empty shell): ~10 contacts, 3–4 companies, 5–6 deals spread across pipeline stages, a few activities/notes. Prefer seeding **through the API** (exercises the real path) or a small idempotent SQL seed script committed under `deploy/seed/demo.sql`.
8. **Backups ON** (persistent staging): fill `deploy/postgres/walg.env` from the example → SOPS; confirm `archive_command` is active; run an initial base backup (`deploy/postgres/scripts/backup-push.sh`) and verify the object lands in OVH Object Storage. Confirm `restore-tenant.sh` is present for the runbook.

## Done When

- [ ] Stack up on the host: postgres, migrate (exited 0), authentik, lecrm-api, caddy.
- [ ] Atlas migrations applied (schema current incl. ADR-012 capability layer).
- [ ] OIDC client provisioned; `LECRM_OIDC_CLIENT_SECRET` set; issuer = `auth.lecrm.gbconsult.me`.
- [ ] `demo` workspace provisioned; reachable at `https://demo.lecrm.gbconsult.me`.
- [ ] Demo data seeded (contacts/companies/deals across stages) — committed seed script or documented API seed.
- [ ] WAL-G base backup succeeded and verified in OVH Object Storage; `archive_command` active.

## Completion Verification

1. `ssh <host> "docker compose ... ps"` -- all services healthy; migrate exited 0
2. `curl -sS -o /dev/null -w '%{http_code}\n' https://demo.lecrm.gbconsult.me/healthz` -- 200
3. `curl -sS -D - -o /dev/null https://demo.lecrm.gbconsult.me/auth/login | grep -i 'location\|set-cookie'` -- 302 to Authentik + state cookie scoped to demo.lecrm.gbconsult.me
4. `psql "$LECRM_DATABASE_URL" -c "SET search_path=workspace_<uuid>; SELECT count(*) FROM contacts;"` -- seeded rows present
5. `ssh <host> "<walg> backup-list | tail"` -- base backup present
6. Commit (config/seed/docs only — never secrets): `feat(deploy): boot lecrm.gbconsult.me staging — stack, demo workspace, seed, backups`

## References

- `deploy/README.md` — full bring-up runbook (adapt lecrm.test → lecrm.gbconsult.me)
- `deploy/compose/{postgres,authentik,migrate,mcp}.yml`, `deploy/caddy/Caddyfile`
- `scripts/authentik-provision-oidc-client.py` — OIDC client provisioning
- `apps/migrate/cmd/lecrm-migrate` — Atlas runner; ADR-009 §8.2 pre-deploy ordering
- `deploy/postgres/scripts/backup-push.sh`, `restore-tenant.sh`, `walg.env.example` — backups (ADR-006)
- `core.lecrm_provision_workspace` — workspace provisioning (ADR-009 §2.1)
