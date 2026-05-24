---
id: 20260515-192005-dd81
title: "leCRM v0 — Auth foundation: zitadel/oidc + (issuer,sub) user-key table + workspace-scoped session cookies (Sprint 3)"
status: done
priority: p1
created: 2026-05-15
updated: 2026-05-18
done: 2026-05-18
tags: [sprint-3, auth, oidc, authentik, session-cookies]
category: engineering
group: lecrm-v0-sprint-3
group_order: 3
order: 4
plan: true
---

## Read this cold — full context inline

Sprint 3 Auth track. Wires `zitadel/oidc` against Authentik (v0 IDP per ADR-009 §7.1), implements the `(issuer, sub)` user-key table that keeps the v0→v1 IDP migration a mapping table (not a destructive rewrite), and binds session cookies to specific workspace subdomains.

## Why this exists

ADR-009 §7.1: "Default v0 (≤4 clients): Authentik 2025.10 self-hosted on Hetzner. Single Compose service (Redis dependency removed in 2025.10; Postgres-backed caching). Built-in TOTP MFA. OIDC upstream for Google Workspace + Microsoft Entra."

ADR-009 §7.1 also binds the identity-key shape: "users are keyed on `(issuer, sub)` tuple, not raw `sub`. Authentik issues UUID-format `sub`; Zitadel issues numeric strings. The tuple makes the v0→v1 migration a mapping table, not a destructive rewrite."

ADR-009 §5.2: session cookies must be scoped to the workspace subdomain (`Domain=acme.lecrm.fr`), never the parent domain — wildcard would leak sessions across workspaces. CSP header on embedded SPA is binding.

## Prerequisite (DOR)

- Build-tagged OIDC e2e test from commit `098664f` is the baseline scaffolding.
- Authentik admin credential in SOPS secret manifest per tasket `20260510-162158-1023`.
- Workspace middleware live per commit `f69d24a` (`apps/api/internal/workspace/`).
- Authentik 2025.10 instance reachable from the dev environment (Docker compose or hosted).

## Approach

### A. OIDC client integration (zitadel/oidc)
1. Add `github.com/zitadel/oidc/v3` dependency.
2. `apps/api/internal/auth/oidc.go`:
   - RP setup against Authentik OIDC discovery endpoint.
   - Callback handler verifying state + PKCE.
   - On successful auth: look up or create user by `(issuer, sub)` tuple — see §B.
3. Hardening: HTTPS-only callback URL; nonce + state per RFC 6749; no refresh_token in URL.

### B. `(issuer, sub)` user-key table
1. New migration `packages/db/migrations/0003_users.sql` adds:
   ```sql
   CREATE TABLE core.user_identities (
     id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
     user_id     UUID NOT NULL,
     issuer      TEXT NOT NULL,
     sub         TEXT NOT NULL,
     email       TEXT,
     created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
     UNIQUE (issuer, sub)
   );
   ```
2. sqlc queries in `packages/db/queries/users.sql`:
   - `GetUserByIssuerSub(issuer, sub)` — returns user_id or nil
   - `UpsertUserIdentity(user_id, issuer, sub, email)` — idempotent on `(issuer, sub)`

### C. Session cookies scoped to workspace subdomain
1. Session middleware in `apps/api/internal/auth/session.go`:
   - On successful OIDC callback: set `Set-Cookie: session=...; Domain=acme.lecrm.fr; SameSite=Strict; Secure; HttpOnly; Path=/; Max-Age=86400`
   - Domain derived from the request Host header (workspace subdomain), NOT hardcoded `lecrm.fr`.
   - Static check: any attempt to set `Domain=lecrm.fr` (parent domain) fails the handler immediately.

### D. CSP header on embedded SPA (ADR-009 §5.2)
1. Static handler middleware sets `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'`.
2. The `unsafe-inline` for styles is shadcn/Tailwind necessity; scripts must remain `'self'` only.

## Done When

- [ ] `apps/api/internal/auth/oidc.go` implements RP flow against Authentik, callback verifies state + PKCE
- [ ] `core.user_identities` table migrated + sqlc queries generated
- [ ] OIDC callback creates/looks up user by `(issuer, sub)` — verified by build-tagged e2e test
- [ ] Session cookies set with workspace-subdomain-scoped Domain, SameSite=Strict, Secure, HttpOnly
- [ ] Test: cross-workspace session leakage attempt rejected (Domain mismatch)
- [ ] CSP header present on all SPA-served responses

## References

- ADR-009 §7.1 (Authentik default v0, `(issuer, sub)` tuple, zitadel/oidc choice), §5.2 (cookie + CSP)
- ADR-007 (secrets — Authentik admin credential storage)
- Commit `098664f` (OIDC e2e test scaffolding)
- Commit `f69d24a` (workspace middleware)
- Tasket `20260510-162158-1023` (secrets baseline)
