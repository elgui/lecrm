# Changes vs. upstream Twenty

This file documents leCRM's modifications relative to upstream [Twenty CRM](https://github.com/twentyhq/twenty), in fulfilment of AGPL-3.0 §5(a).

## Format

For each release tag `twenty-<UPSTREAM>+lecrm.<PATCH>`, list the modifications applied on top of the named upstream version. Group by area (auth, data layer, ops, AI). Link to specific commits where useful.

A pinned upstream-vs-fork compare link is also useful for reviewers:
**`https://github.com/twentyhq/twenty/compare/<UPSTREAM_TAG>...gbconsult:lecrm:<RELEASE_TAG>`**

---

## Pre-release (seed, no upstream imported yet)

**2026-05-10 — Repository initialised.** Seed only: LICENSE, NOTICE, README, CHANGES, empty `gbconsult/` patch directory. No code from Twenty has been imported yet; no modifications applied. The fork is established to satisfy AGPL §13 publication discipline ahead of the v0 build.

---

## Planned modifications (v0 build, target 2026 Q3)

Tracked at full detail in the leCRM architecture documents (private). Headline items:

### `@license Enterprise` substitutions (all in `gbconsult/`)
- **`gbconsult/auth/oidc-strategy.ts`** — Passport OIDC strategy, replaces upstream's Enterprise-licensed `core-modules/auth/strategies/oidc.auth.strategy.ts`. Wired via NestJS DI provider override (no upstream file edited).
- **`gbconsult/auth/saml-strategy.ts`** — Passport SAML strategy (added when client demand requires).
- **`gbconsult/enterprise/plan-service-stub.ts`** — `EnterprisePlanService` always-valid stub, replaces the upstream license-gating service via DI override.

### Auth pipeline wiring
- **`gbconsult/auth/auth.module.override.ts`** — Single override module that supplies the providers above. The upstream `auth.module.ts` is **not modified**; the override is loaded after the upstream module via the standard NestJS provider-override pattern.

### AGPL §13 compliance
- **UI footer** — every page served displays *"Powered by Twenty CRM (AGPL-3.0) — source: github.com/gbconsult/lecrm"*. Implementation in a Twenty extension package, no upstream UI edit.
- **Version endpoint** — `/api/version` returns the running upstream + leCRM revision pair.

The complete list of modifications, decision rationale, and rollout order lives in `docs/adr/ADR-002-twenty-fork-management.md` of the architecture project.
