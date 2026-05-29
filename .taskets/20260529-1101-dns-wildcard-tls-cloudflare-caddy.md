---
id: 20260529-1101-dns-wildcard-tls-cloudflare-caddy
title: "Staging: DNS + wildcard TLS for *.lecrm.gbconsult.me (Cloudflare DNS-01 + custom Caddy)"
status: done
priority: p1
created: 2026-05-29
category: project
group: lecrm-staging-deploy
group_order: 220
order: 2
plan: true
tags: [deploy, staging, dns, tls, cloudflare, caddy, leo-test]
---

# Staging: DNS + wildcard TLS for *.lecrm.gbconsult.me (Cloudflare DNS-01 + custom Caddy)

## Pre-flight: Verify Previous Tasket

1. `grep -i "lecrm.gbconsult.me\|staging" deploy/README.md` -- host + edge strategy recorded (tasket order:1)
2. `ssh <host> "docker network ls | grep lecrm"` -- network exists
3. Confirm the staging env file holds a `CLOUDFLARE_API_TOKEN`, and **verify its scope via the API** (Rook gate #2): `curl -s -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" https://api.cloudflare.com/client/v4/user/tokens/verify` → active, and confirm it is DNS:Edit limited to the gbconsult.me zone (not an account-global token).

**If any check fails, STOP and report. Do not proceed.**

> **Human-in-the-loop:** Creates real DNS records on a live domain. Run interactively.

## Context

Per-workspace subdomains mean leCRM needs a **wildcard cert** (`*.lecrm.gbconsult.me`) — issued via **Cloudflare DNS-01**, because subdomains can be created at any time without waiting on HTTP-01 per-host validation (see `deploy/README.md` decision log). The stock Caddy binary lacks the Cloudflare DNS provider; a custom build is required.

Decisions: domain **`*.lecrm.gbconsult.me`**; gbconsult.me is on Cloudflare. Edge option from tasket order:1.

Working directory: `/home/gui/Projects/leCRM`.

## Steps

1. **DNS records (Cloudflare, gbconsult.me zone).** Point the staging host IP (from order:1):
   - **`lecrm.gbconsult.me` already has a redirection** (per Guillaume). **Do NOT clobber it.** Workspaces live on subdomains, so the app does not need the apex. Leave the existing apex record/redirect in place; only reconcile it if you explicitly decide the app should own the apex (then record that decision and what the redirect pointed to before changing it).
   - `*.lecrm.gbconsult.me` → A/AAAA → host IP — **the record this tasket actually adds** (covers `demo.lecrm.gbconsult.me`, `auth.lecrm.gbconsult.me`, etc.). Confirm the existing apex redirect does not also match `*` in a way that intercepts subdomains.
   - Decide Cloudflare proxy mode: **DNS-only (grey cloud)** is simplest with Caddy-managed certs + DNS-01; proxied (orange) needs origin-cert handling — prefer grey-cloud for staging. Record the choice.
2. **Custom Caddy with the Cloudflare DNS module.** Build via `xcaddy build --with github.com/caddy-dns/cloudflare` or the `caddy:builder` Docker image; produce the image/binary the compose edge service will run. Wire it into the edge (per the order:1 edge option).
3. **Caddyfile.** Update `deploy/caddy/Caddyfile`:
   - real domain `lecrm.gbconsult.me` + wildcard `*.lecrm.gbconsult.me`;
   - `tls { dns cloudflare {env.CLOUDFLARE_API_TOKEN} }`;
   - reverse-proxy app subdomains → `lecrm-api`, auth host (`auth.lecrm.gbconsult.me`) → `authentik-server`;
   - keep the CSP header per ADR-009 §5.2 (`frame-ancestors 'none'`, `script-src 'self'`).
4. **Issue + verify the wildcard cert.** Boot just the edge (or `caddy validate` + a dry issuance), confirm Caddy obtains a `*.lecrm.gbconsult.me` cert via DNS-01 (watch logs for the ACME DNS challenge succeeding). If edge option B (behind Dokku nginx), also confirm nginx forwards `lecrm.gbconsult.me` to Caddy's port.

## Done When

- [ ] `lecrm.gbconsult.me` and `*.lecrm.gbconsult.me` resolve to the staging host (DNS propagated).
- [ ] Custom Caddy with `caddy-dns/cloudflare` built and wired into the edge.
- [ ] `deploy/caddy/Caddyfile` updated: real domain, DNS-01 tls block, proxies to api + authentik, CSP retained.
- [ ] Wildcard cert for `*.lecrm.gbconsult.me` issued successfully (ACME DNS-01 challenge green in logs).

## Completion Verification

1. `dig +short demo.lecrm.gbconsult.me` and `dig +short lecrm.gbconsult.me` -- return the host IP
2. `grep -q "lecrm.gbconsult.me" deploy/caddy/Caddyfile && grep -qi "dns cloudflare" deploy/caddy/Caddyfile` -- Caddyfile updated
3. `ssh <host> "docker logs <caddy-container> 2>&1 | grep -i 'certificate obtained\|wildcard'"` -- cert issued
4. `curl -sS -o /dev/null -w '%{http_code} %{ssl_verify_result}\n' https://demo.lecrm.gbconsult.me` -- TLS handshakes (verify_result 0)
5. Commit: `feat(deploy): wildcard TLS for *.lecrm.gbconsult.me via Cloudflare DNS-01`

## References

- `deploy/caddy/Caddyfile` — edge config to update
- `deploy/README.md` — DNS-01 / wildcard decision log
- ADR-009 §5.2 — CSP + cookie scoping requirements
- Cloudflare skill (`~/.claude` skills) — DNS record management for gbconsult.me
