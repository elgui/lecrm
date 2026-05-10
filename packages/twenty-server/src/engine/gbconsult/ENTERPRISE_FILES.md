# Inventory of upstream `@license Enterprise` files

This file is the canonical list of Twenty source files whose top-of-file marker is `/* @license Enterprise */`. These files are **not** licensed under AGPL-3.0; per Twenty's `LICENSE`, they are subject to a separate commercial license.

## Source of truth

The companion file [`ENTERPRISE_FILES.list`](./ENTERPRISE_FILES.list) is the machine-readable list of paths (one per line). To regenerate from a fresh upstream checkout:

```bash
grep -rln "@license Enterprise" packages/ --include="*.ts" --include="*.tsx" | sort > packages/twenty-server/src/engine/gbconsult/ENTERPRISE_FILES.list
```

The list is regenerated on every upstream rebase and committed alongside the rebase patch.

## Counts (as of `twenty-2.2.0+lecrm.0`)

297 files total across `packages/twenty-front`, `packages/twenty-server`, `packages/twenty-website`, and shared modules. The top concentration is in:

- `engine/metadata-modules/row-level-permission-predicate/` and `flat-row-level-permission-predicate/` — RLS predicate engine (≈ 140 files)
- `engine/core-modules/auth/` — SSO controllers, OIDC + SAML guards, OIDC + SAML strategies, enterprise-features guard
- `engine/core-modules/enterprise/` — `EnterprisePlanService`, license validation cron, license JWT verification
- `engine/core-modules/sso/` — SSO identity provider service
- `engine/core-modules/admin-panel/` — admin panel and billing surfaces
- `engine/core-modules/domain/custom-domain-manager/` — custom domain validation
- `engine/metadata-modules/ai/ai-billing/` — AI usage billing
- `front/src/modules/auth/` — front-end SSO selection UI
- `front/src/modules/settings/domains/` — front-end custom domain settings UI
- `front/src/modules/settings/roles/...rowLevelPermissionPredicate*` — front-end RLS UI

## Compliance posture

Per [ADR-002 §5](../../../../../../../README.md#license--attribution), leCRM:

1. Vendors these files **as-is** in the public fork repository to satisfy AGPL §13 source-availability for the AGPL portions of the codebase. The `@license Enterprise` headers are preserved unchanged.
2. Does **not** modify them in `gbconsult/` patches. The patch directory contains independent leCRM implementations of the same surfaces (clean-room).
3. Replaces their runtime behaviour through the DI override pattern (`gbconsult.module.ts`) where the file exposes a NestJS provider, or through controller/route replacement (TBD) where it exposes HTTP routes.

## Pre-commit guard (ADR-002 TO RESOLVE item 2)

A `pre-commit` hook (planned, not yet implemented) will fail any commit that:

- Modifies a file present in `ENTERPRISE_FILES.list`
- Adds a new file with the `/* @license Enterprise */` header outside upstream-rebase commits

This guard is tracked as a discrete sub-tasket; see CHANGES.md at the repo root.

## Operational implication

Until our clean-room SSO controller + OIDC guard land (sub-tasket B-track follow-up), runtime OIDC traffic still executes against upstream's enterprise files. Operating leCRM in this mode requires either:

- A commercial Twenty license (covers @license Enterprise file usage at runtime)
- A leCRM-owned replacement of the SSO routing surface (controllers + guards + strategy invocation)

The decision and timeline are tracked in the architecture project's TO RESOLVE list under ADR-002.
