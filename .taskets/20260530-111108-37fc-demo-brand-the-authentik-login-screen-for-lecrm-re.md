---
id: 20260530-111108-37fc
title: "Demo: brand the Authentik login screen for leCRM (reproducible script)"
status: done
priority: p2
created: 2026-05-30
updated: 2026-05-31
done: 2026-05-31
tags: [demo, authentik, branding, first-impression, polish]
category: project
group: lecrm-demo-polish
group_order: 250
order: 3
plan: true
---

## Context

The login screen at `auth.lecrm.gbconsult.me` shows STOCK Authentik branding: orange "authentik" wordmark, default aerial-road background, title "Welcome to authentik!". Verified live 2026-05-30. Literal first screen before the leCRM app.

## Why

An unbranded third-party login is an off-brand first touch for an outward-facing demo. Should say leCRM, not authentik. Cheap and reproducible. Not a blocker.

## Approach

1. Customize the Authentik Brand (Tenant/Brand): leCRM logo, favicon, neutral/branded background, title. Remove visible "authentik" wordmark from the default brand.
2. Do it via an `ak shell` script (like `scripts/authentik-provision-oidc-client.py`) so it is REPRODUCIBLE and carries to the Hetzner migration (order:5), not lost click-ops.
3. Commit `scripts/authentik-brand-lecrm.py`; note in `deploy/README.md`.

## Done When

- [x] Login shows leCRM logo/title; no "authentik" wordmark visible to a logging-in user
- [x] Applied via committed idempotent `ak shell` script (re-runnable on fresh Authentik)
- [x] `deploy/README.md` notes the branding step

## Verified (2026-05-31)

Executed `scripts/authentik-brand-lecrm.py` against the live
`lecrm-authentik-worker` on host `51.77.146.49`:

- `GET https://auth.lecrm.gbconsult.me/api/v3/core/brands/current/` →
  `branding_title: "leCRM"`, `branding_logo`/`branding_favicon` decode to the
  leCRM SVG wordmark/mark.
- `default-authentication-flow.title` = `"Bienvenue sur leCRM"`.
- Public login page `/if/flow/default-authentication-flow/` → `HTTP 200`,
  served `<title>leCRM</title>` (was "authentik"). Remaining "authentik"
  strings are internal framework asset paths, not user-visible branding.

## References

- `scripts/authentik-provision-oidc-client.py`, `authentik-provision-test-user.py` (ak shell pattern)
- Live Authentik: host `51.77.146.49`, container `lecrm-authentik-worker` (`ak shell`)
- `deploy/README.md` -> Access / auth model
- Infra task (not Leo-facing) per memory feedback_leo_scope_lecrm
