---
id: 20260529-1104-migrate-staging-ovh-to-hetzner
title: "Staging: migrate OVH stopgap → fresh Hetzner CAX11 (hard deadline 2026-06-12)"
status: done
priority: p1
created: 2026-05-29
updated: 2026-06-14
category: project
group: lecrm-staging-deploy
group_order: 220
order: 5
plan: true
tags: [deploy, staging, migration, hetzner, isolation, deadline, leo-test]
---

> **CLOSED 2026-06-14 ✅** — Migrated to **Netcup VPS 1000 ARM G11 `152.53.143.175`**
> (substrate revised from Hetzner→Netcup per the cost/ARM decision; see memory
> `project_staging_hetzner_to_netcup_migration`). Full stack rebuilt arm64-native,
> data restored with row-count parity (3 workspaces), Authentik re-provisioned
> (OIDC + leCRM branding + Léo), Caddy wildcard via OVH DNS-01, WAL-G archiving ON
> (R2 `netcup-demo`). Wildcard DNS flipped + verified (12/12 tests + OIDC login,
> browser login by Léo confirmed). Repeatable scripts: `deploy/cutover-resync.sh`,
> `deploy/enable-walg-archiving.sh`. OVH teardown tracked by tasket `20260614-081441-864d`.

# Staging: migrate OVH stopgap → fresh Hetzner CAX11 (hard deadline 2026-06-12)

## Pre-flight: Verify Previous Taskets

1. `curl -sS -o /dev/null -w '%{http_code}\n' https://demo.lecrm.gbconsult.me/healthz` -- OVH staging is live (order:3)
2. `ls deploy/` -- portable env-parameterized stack artifact committed (order:2/order:3 portability requirement)
3. Léo has been testing on OVH (the stopgap served its purpose) — or the 2-week deadline is approaching regardless.

## Why this tasket exists (council ruling — binding)

The 2026-05-29 council unanimously preferred a **fresh Hetzner box** over OVH for staging. The **decisive reason was isolation**: the OVH boxes run production apps (tele-claude, vps-mngr / static sites), so an internet-exposed, externally-tested CRM stack co-located there shares a blast radius (container escape / shared-kernel / bridge-network pivot → real prod services). Guillaume accepted OVH **only as a sub-2-week stopgap for speed**, with this migration as the agreed remediation.

**Hard deadline: 2026-06-12** (~2 weeks from staging go-live). Rationale (Ava): the failure mode is **psychological inertia** — once staging is "good enough," it never moves. The deadline is the forcing function. Going to Hetzner also satisfies Rook's gate #3 (infra-level isolation, *by construction*) and aligns with ADR-009's Hetzner/EU production target.

> **Human-in-the-loop:** provisions a new VPS and cuts over a live URL Léo uses. Run interactively; coordinate the cutover so Léo isn't mid-session.

## Steps

1. **Provision Hetzner CAX11** (arm64, ~€4/mo, Falkenstein/Helsinki — EU). Ubuntu 24.04; inject SSH key; host firewall = 22 from your IP only, 80/443 public; install Docker; `docker network create lecrm`. Nothing else runs on this box (isolation by construction).
2. **Stand up the stack from the portable artifact** (`deploy/`) — same compose + Caddyfile, new SOPS env (new Postgres password, new Authentik secrets, **re-verify the scoped Cloudflare token**). Because the artifact is env-parameterized (order:2/3), this should be `docker compose up` + secrets, target <30 min.
3. **Data move.** Restore the demo workspace data onto Hetzner: either WAL-G restore from the existing backup destination (preferred — proves the DR path) or a `pg_dump`/`pg_restore` of the demo workspace schema. Verify row counts match OVH.
4. **Re-point WAL-G** backups to run from Hetzner (confirm a fresh base backup lands).
5. **DNS cutover.** Repoint `*.lecrm.gbconsult.me` (+ apex/auth as applicable) from the OVH IP → Hetzner IP in Cloudflare. Watch TTL; Caddy re-issues the wildcard cert via DNS-01 on the new box. Confirm `demo.lecrm.gbconsult.me` serves from Hetzner.
6. **Léo smoke test on Hetzner** — re-run the order:4 critical-path check (login as Léo's local account → view → create → move stage → CSV) against the migrated instance.
7. **Decommission the OVH stopgap** — stop the stack, confirm no public Postgres port was ever left open during its life, remove containers/volumes/secrets from the OVH box, and confirm tele-claude/vps-mngr (the co-tenants) are unaffected.
8. Update `deploy/README.md`: staging now on Hetzner; record the box, the cutover date, and close the isolation-gate waiver from order:1.

## Done When

- [ ] Fresh Hetzner CAX11 running the full stack; nothing else co-located.
- [ ] Demo workspace data migrated; row counts verified; WAL-G backups running from Hetzner.
- [ ] DNS cut over; `https://demo.lecrm.gbconsult.me` serves from Hetzner with a valid wildcard cert.
- [ ] Léo critical-path smoke test green on Hetzner.
- [ ] OVH stopgap fully decommissioned; co-tenant prod apps unaffected; isolation-gate waiver closed in `deploy/README.md`.
- [ ] Completed on or before **2026-06-12**.

## Completion Verification

1. `dig +short demo.lecrm.gbconsult.me` -- returns the Hetzner IP
2. `curl -sS -o /dev/null -w '%{http_code} %{ssl_verify_result}\n' https://demo.lecrm.gbconsult.me/healthz` -- 200, TLS verify 0, served from Hetzner
3. `ssh <hetzner> "<walg> backup-list | tail"` -- base backup present on Hetzner
4. `ssh <ovh-box> "docker ps | grep -i lecrm || echo DECOMMISSIONED"` -- OVH stack gone
5. Commit: `chore(deploy): migrate leCRM staging OVH → Hetzner CAX11 (isolation; council ruling)`

## References

- Council debate 2026-05-29 (this session) — isolation rationale + 2-week deadline
- `deploy/README.md` — staging runbook + isolation-gate waiver (order:1) to close
- `deploy/postgres/scripts/restore-tenant.sh`, WAL-G scripts — data move / DR path
- ADR-009 §2 — Hetzner/Ubicloud as the documented production substrate
- ADR-006 — backup/restore (the migration exercises this)
