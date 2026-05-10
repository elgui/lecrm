# Changes vs. upstream Twenty

This file documents leCRM's modifications relative to upstream [Twenty CRM](https://github.com/twentyhq/twenty), in fulfilment of AGPL-3.0 §5(a).

## Format

For each release tag `twenty-<UPSTREAM>+lecrm.<PATCH>`, the modifications applied on top of the named upstream version are listed, grouped by area (auth, data layer, ops, AI, etc.). A pinned upstream-vs-fork compare link is also useful for reviewers:

**`https://github.com/twentyhq/twenty/compare/<UPSTREAM_TAG>...elgui:lecrm:<RELEASE_TAG>`**

---

## `twenty-2.2.0+lecrm.0` — v0 spine (2026-05-10)

First release after the seed commit. Vendored upstream Twenty `v2.2.0` and added the leCRM patch directory + ops baseline.

### `gbconsult/` patch directory (new)

Lives at `packages/twenty-server/src/engine/gbconsult/`. All server-side modifications go here per ADR-002 §2.

- `gbconsult.module.ts` — single NestJS override module imported last in `app.module.ts`. Replaces upstream `EnterprisePlanService` via the standard custom-providers pattern.
- `enterprise/plan-service-stub.ts` — clean-room `EnterprisePlanService` always-valid stub. AGPL-3.0 leCRM code, written from scratch against the public method signatures of upstream's `@license Enterprise` class.
- `auth/oidc-strategy.ts` — clean-room Passport OIDC strategy using `openid-client`. Functional replacement for upstream's `@license Enterprise` `oidc.auth.strategy.ts`. **Wiring status:** scaffolded; the runtime SSO controller surface still uses upstream's enterprise files. Follow-up sub-tasket replaces the controller.
- `auth/auth.module.override.ts` — auth-module override exporting the OIDC strategy.
- `version/version.controller.ts` — `GET /api/version` endpoint returning the upstream + leCRM revision pair (AGPL §13 source-build correspondence anchor).
- `version/version.constants.ts` — single source of truth for the running version strings.
- `__tests__/gbconsult.module.spec.ts` — Jest test asserting the DI override resolves to the leCRM stub. Per ADR-002 TO RESOLVE item 3, runs on every PR.
- `README.md` — pattern + status documentation.
- `ENTERPRISE_FILES.md` + `ENTERPRISE_FILES.list` — inventory of 297 upstream `@license Enterprise` files (per ADR-002 TO RESOLVE item 2). Pre-commit guard against accidental modification is a tracked sub-tasket.

### Upstream file edits (the only ones)

- `packages/twenty-server/src/app.module.ts` — added one `import { GBConsultModule }` line and one entry in the `imports` array (placed last so its providers shadow upstream defaults). This is the **only** upstream file edited per ADR-002 §2.

### Frontend patch directory (new)

- `packages/twenty-front/src/lecrm/AGPLFooter.tsx` — clean-room AGPL §13 attribution footer component.
- `packages/twenty-front/src/lecrm/README.md` — mounting strategy and follow-up sub-tasket pointers (`twenty-sdk` extension preferred over modifying upstream layouts).

### Operations (new)

- `ops/templates/docker-compose.template.yml` — per-client Docker Compose template (Phase 1, VPS-per-client). Sized for Hetzner CX22 (4 GB RAM). Services: server, worker, postgres 16, redis 7, caddy.
- `ops/templates/Caddyfile.template` — Caddy reverse proxy with auto-HTTPS, security headers, JSON access logs.
- `ops/templates/.env.template` — per-client environment template.
- `ops/provision-client.sh` — provisioning script. Validated end-to-end with a dry client; `docker compose config` returns a valid configuration.
- `ops/README.md` — operational quick-start + sizing math.

### Repository hygiene

- `.gitignore` — combined upstream Twenty's verbatim ignore list with leCRM additions (`ops/clients/`, secrets, editor noise).
- `README.md` — updated to reflect post-import layout and corrected the AGPL §13 footer URL to `github.com/elgui/lecrm` (the canonical location; `gbconsult/` org transfer is deferred to the first paying client).
- `UPSTREAM-README.md` — Twenty's original `README.md` preserved verbatim.
- `LICENSE` — kept upstream's (contains `@license Enterprise` carve-outs that match the actual files vendored).

### Outstanding

- **OIDC runtime path replacement.** The DI override stubs the licence gate; OIDC at runtime still executes against upstream's enterprise files. Tracked as a sub-tasket. Operating in this mode requires either a commercial Twenty licence or the controller-replacement follow-up to land first.
- **AGPL §13 footer mounting.** Component scaffolded; the mounting hook (`twenty-sdk` extension preferred) is a sub-tasket.
- **Pre-commit guard against `@license Enterprise` modifications.** Tracked.
- **Twenty CLA / licence-ratchet monitoring.** Tracked (ADR-002 TO RESOLVE item 1).

A pinned compare link for this release:
**`https://github.com/twentyhq/twenty/compare/v2.2.0...elgui:lecrm:twenty-2.2.0+lecrm.0`**

---

## Pre-release (seed)

**2026-05-10 — Repository initialised.** Seed only: LICENSE, NOTICE, README, CHANGES, empty `gbconsult/` patch directory. No code from Twenty had been imported yet.
