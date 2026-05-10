---
id: 20260510-162158-8550
title: leCRM v0 — gbconsult OIDC controller replacement
status: deleted
priority: p1
created: 2026-05-10
updated: 2026-05-10
tags: [auth, oidc, gbconsult, v0]
category: project
review: "superseded-by: docs/adr/ADR-008-clean-room-reimplementation.md + docs/adr/ADR-009-stack-and-license.md. Twenty-fork-specific work; no analogue under clean-room Apache 2.0 build. Cleared by housekeeping in tasket 20260510-202450-b844 Part A on 2026-05-10."
group: lecrm-v0-build
order: 6
plan: true
---

# leCRM v0 — gbconsult OIDC controller replacement

## Why this tasket exists
The v0 spine (`twenty-2.2.0+lecrm.0`) includes the EnterprisePlanService DI override (always-valid stub) and a clean-room OIDC strategy in `gbconsult/auth/oidc-strategy.ts`. The runtime OIDC path, however, still executes through upstream's `@license Enterprise` `SSOAuthController` and `OIDCAuthGuard` because they instantiate `OIDCAuthStrategy` directly via `new OIDCAuthStrategy(...)` rather than through DI.

Operating in this mode requires either a commercial Twenty licence OR replacing the SSO controller surface with leCRM-owned code. This tasket implements the latter.

Reference: `packages/twenty-server/src/engine/gbconsult/README.md`, ADR-002 §2.

## Done criteria
- [ ] `gbconsult/auth/sso-auth.controller.ts` — leCRM-owned controller mounting on `POST /auth/oidc/{identityProviderId}/initiate` and `GET /auth/oidc/callback`, using `gbconsult/auth/oidc-strategy.ts`.
- [ ] `gbconsult/auth/oidc-auth.guard.ts` — leCRM-owned guard equivalent to upstream's, using our strategy.
- [ ] Module-level configuration in `tsconfig.build.json` to exclude the upstream `@license Enterprise` SSO files from the build (TypeScript paths exclusion + NestJS module-tree pruning).
- [ ] End-to-end OIDC login test against a Google Workspace test tenant (manual smoke test).
- [ ] Documentation in `gbconsult/README.md` updated to remove the "OIDC runtime path replacement" outstanding item.
