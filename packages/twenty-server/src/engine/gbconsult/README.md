# `gbconsult/` — leCRM patch directory

This directory contains all GB Consult modifications applied on top of upstream Twenty CRM. The pattern is documented in [ADR-002 — Twenty Fork Management](https://github.com/elgui/lecrm/blob/main/CHANGES.md) (the architecture document is private; the operational summary is in CHANGES.md at the repo root).

## What lives here

The contract: every leCRM customisation lands here. The upstream tree is touched in **exactly one** place — `packages/twenty-server/src/app.module.ts` imports `GBConsultModule` last so its providers shadow upstream defaults.

```
gbconsult/
├── README.md                      this file
├── ENTERPRISE_FILES.md            inventory of upstream @license Enterprise files (advisory)
├── gbconsult.module.ts            single NestJS override module (the only thing app.module.ts imports)
├── auth/
│   ├── oidc-strategy.ts           clean-room Passport OIDC strategy using openid-client
│   └── auth.module.override.ts    auth-related providers wired into the override module
└── enterprise/
    └── plan-service-stub.ts       EnterprisePlanService always-valid stub
```

## Override pattern (NestJS DI custom providers)

NestJS resolves providers by token. When two modules export a provider for the same token, the module imported **later** wins. `GBConsultModule` is imported last in `app.module.ts`, so its providers shadow the upstream `EnterpriseModule`/`AuthModule` defaults.

Concretely, the stub is wired as:

```typescript
{
  provide: EnterprisePlanService,
  useClass: GBConsultEnterprisePlanServiceStub,
}
```

This means every consumer that injects `EnterprisePlanService` (e.g. `EnterpriseFeaturesEnabledGuard`) gets the stub instead of upstream's licensed implementation.

## Status of OIDC override (work in progress)

The OIDC strategy file (`auth/oidc-strategy.ts`) is a clean-room re-implementation, not a copy of upstream's `@license Enterprise` `oidc.auth.strategy.ts`. The remaining work to make the OIDC login flow fully run through our clean-room code (rather than upstream's enterprise files) is tracked as a separate sub-tasket. Specifically, the upstream `SSOAuthController` and `OIDCAuthGuard` (both `@license Enterprise`) instantiate `OIDCAuthStrategy` directly via `new OIDCAuthStrategy(...)` rather than through DI — this means a DI override alone does not redirect the runtime path. A follow-up will either:

1. Replace `SSOAuthController` with a leCRM-owned controller that uses our `gbconsult/auth/oidc-strategy.ts`, or
2. Rebuild Twenty without compiling the `@license Enterprise` SSO files (TypeScript `paths` exclusion + a leCRM-owned route module).

Until that follow-up lands, the EnterprisePlanService stub is sufficient to **unblock** the gate guards (so OIDC login does not fail with "enterprise features not enabled"), and OIDC at runtime executes against upstream's enterprise files. Operating in this mode requires a commercial Twenty license — a Guillaume decision recorded as a TO RESOLVE item.

## Adding a new override

1. Decide via the [ADR-002 §3 module-placement decision tree](https://github.com/elgui/lecrm) whether the change belongs here, in a `twenty-sdk` extension, or in a separate microservice.
2. If here: add the implementation file under the appropriate subdirectory (`auth/`, `enterprise/`, etc.).
3. Wire it into `gbconsult.module.ts` providers.
4. Add a Jest test that asserts `app.get(SomeService)` returns the leCRM implementation (per ADR-002 TO RESOLVE item 3).
5. Update `CHANGES.md` at the repo root.
