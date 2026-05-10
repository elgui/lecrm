---
id: 20260510-202018-c458
title: Week 1-2 — leCRM v0 scaffolding + Twenty-fork tasket cleanup + Go ramp checkpoint
status: deleted
priority: p0
created: 2026-05-10
updated: 2026-05-10
category: engineering
group: lecrm-v0-scaffolding
order: 1
---

## Context

[ADR-009](docs/adr/ADR-009-stack-and-license.md) was Accepted on 2026-05-10 after five-voice council validation. The stack is locked: Go 1.23 + Postgres 17 schema-per-tenant via per-workspace role + REST + thin MCP adapter + React 19 + Vite + TanStack + shadcn/ui + Apache 2.0 + Authentik (v0) → Zitadel Cloud EU (v1) + LGTM observability + WAL-G backups. Hosting is OVH-first per the revised STRATEGIC-OVERVIEW §2 (OVH VPS at Phase 1, OVH Managed Postgres at Phase 2, Ubicloud-on-Hetzner-DE as documented fallback).

The existing `lecrm-v0-build` tasket group (created 2026-05-10 162158, eight sub-taskets) was scoped against the now-superseded Twenty-fork posture. Most are dead or need re-scoping. The 8 Twenty-fork sub-taskets in `.taskets/20260510-162158-*` reference AGPL §13 footers, gbconsult/oidc controller replacement, Enterprise-license file guards — all irrelevant under clean-room Apache 2.0.

This tasket bundles the two unblock-the-build activities: (1) tasket-group housekeeping so the dashboard is not misleading; (2) the week 1-2 scaffolding work per ADR-009 §8.1 layout, culminating in the binding **week-2 Go ramp checkpoint** per ADR-009 §1.1 (three concrete tests; switch to TypeScript+Hono if any blocks > 4 working hours).

## Work breakdown

### Part A — Housekeeping (day 1, ~2-4 hours)

For each tasket in `.taskets/20260510-162158-*` and the `00X-*` meta-taskets, classify:

1. **DEAD** (Twenty-fork-specific, no clean-room analogue):
   - `20260510-162158-6149-lecrm-v0-pre-commit-guard-against-license-enterpri.md` — Enterprise-license file guard; irrelevant.
   - `20260510-162158-8550-lecrm-v0-gbconsult-oidc-controller-replacement.md` — Twenty auth-module override; we build OIDC from scratch with `zitadel/oidc`.
   - `20260510-162158-9466-lecrm-v0-agpl-13-footer-mounting-via-twenty-sdk-ex.md` — AGPL §13 footer; no §13 under Apache 2.0.
   - Mark `status: deleted` via Tasket API (`PUT /api/taskets/project/status?...`) with explicit user approval per the Housekeep workflow rule. Never `rm` the file; the API write is the safe action.

2. **SURVIVES IN SPIRIT** (re-scope against the new stack):
   - `20260510-162158-1023-lecrm-v0-secrets-management-baseline-sops-age-trac.md` — SOPS+age secrets baseline; ADR-007 retained. Re-scope: add `lecrm_provisioner` Postgres credential (Tier-0 secret, annual rotation) per ADR-009 §2.1; add Authentik admin credential.
   - `20260510-162158-499c-lecrm-v0-brevo-transactional-api-integration-track.md` — Brevo; ADR-003 retained. Re-scope: integration is now via Go HTTP client (not NestJS service).
   - `20260510-162158-d1ba-lecrm-v0-backup-baseline-wal-g-gpg-hetzner-object.md` — WAL-G + GPG; ADR-006 retained. Re-scope: target is **OVH Object Storage** (or OVH-equivalent S3-compatible) not Hetzner Object Storage, given the §2 OVH-first hosting decision; verify WAL-G S3 compat works against OVH.
   - `20260510-162158-29dc-lecrm-v0-embedded-metabase-reporting-track-d.md` — Reporting iframe bridge. Re-scope: ADR-009 §9 names Cube.dev iframe as the v0 bridge; either rename to Cube.dev or keep Metabase if it scopes faster — decide in re-scope review.

3. **SURVIVES UNCHANGED (post-v0)**:
   - `20260510-162158-aa6f-lecrm-v1-native-sequences-engine-track-f-post-firs.md` — v1 work, not v0.
   - `20260510-155549-11e5-lecrm-v2-prototype-telegram-bot-to-twenty-graphql.md` — v2 work, re-scope from "Twenty GraphQL" to "leCRM REST + MCP adapter" per ADR-009 §4.

4. **META-taskets to retire**:
   - `001-technical-deep-dive.md`, `002-v0-build-kickoff.md` — superseded by ADR-008 + ADR-009 + this tasket group. Mark `status: done` with a closure note pointing at this tasket group.

For each surviving-in-spirit tasket, edit the body via `PUT /api/taskets/project/...` to reference ADR-009 and the new stack constraints.

### Part B — Week 1 scaffolding (days 2-5)

Per ADR-009 §8.1 layout:

```
lecrm/
├── go.work
├── apps/
│   ├── api/cmd/lecrm-api/
│   ├── mcp/cmd/lecrm-mcp/
│   ├── migrate/cmd/lecrm-migrate/
│   └── web/                   # React 19 + Vite + TanStack
├── packages/
│   ├── db/                    # sqlc-generated Go + Atlas HCL
│   ├── crm-adapter/           # CrmAdapter Go interface (shared api ↔ mcp)
│   ├── shared-types/
│   └── tools/
├── deploy/compose/
│   ├── postgres.yml
│   ├── authentik.yml          # 2025.10 Redis-free
│   ├── lgtm.yml               # Loki + Grafana + Tempo + Prometheus + OTel Collector
│   └── caddy/
└── .github/workflows/         # go test, pnpm test, atlas lint, golangci-lint, gosec, govulncheck, go mod verify
```

Concrete checklist:
- [ ] `go work init && go work use ./apps/api ./apps/mcp ./apps/migrate ./packages/...`
- [ ] `pnpm create vite apps/web --template react-ts` + TanStack Router/Query install + shadcn/ui init
- [ ] Compose stack up: Postgres 17 + Authentik 2025.10 + LGTM stack + Caddy with DNS-01 wildcard cert for `*.lecrm.fr` (or chosen v0 domain).
- [ ] First OIDC login flow end-to-end: Authentik admin → create OIDC client → Go `zitadel/oidc` RP → session cookie set with `Domain=acme.lecrm.fr; HttpOnly; Secure; SameSite=Strict` (per ADR-009 §5.2 cookie-scope mandate).
- [ ] Atlas HCL schema for `core.workspaces` table + `lecrm_provision_workspace(uuid)` SECURITY DEFINER function per ADR-009 §2.1; smoke-test by provisioning a `workspace_acme` role + schema + grants + idempotency check.
- [ ] CI scaffold: GitHub Actions running `go test`, `pnpm test`, `atlas migrate lint`, `golangci-lint run`, `gosec`, `govulncheck ./...`, `go mod verify` on every PR.
- [ ] OVH VPS provisioned for first Phase-1 deploy target; Caddy + Compose pulled and run; OIDC login reachable at the first test subdomain.

### Part C — Week-2 Go ramp checkpoint (end of week 2 — BINDING decision point)

Per ADR-009 §1.1, three concrete litmus tests must pass:

1. Minimal HTTP handler running an `sqlc`-typed query against local Postgres, returning JSON.
2. Workspace-scoping middleware using idiomatic Go context propagation (`context.WithValue` for `WorkspaceContext`).
3. `golangci-lint run` and `go vet ./...` clean on the scaffolding.

**If any of the three is blocked > 4 working hours despite Claude Code assistance, switch to the TypeScript+Hono fallback** (Hono + Drizzle + Atlas + `openid-client` v6). The decision is irrevocable by end of week 2; do not relitigate at week 5.

Record the checkpoint outcome as an Architecture Decision Note appended to ADR-009 or as a closure comment on this tasket.

## Out of scope for this tasket

- Custom-object metadata engine (deferred to ADR-010, week 5).
- Email integration scaffolding (Gmail OAuth app review starts week 5-6, not now).
- MCP adapter implementation body (week 13, after the CrmAdapter interface is fleshed out).
- Production OVH Managed Postgres provisioning (Phase-2 work; v0 runs Postgres self-hosted on the OVH VPS).
- Anything Apache-2.0-license-related beyond the `LICENSE` + `NOTICE` files at the root commit.

## Acceptance criteria

- [ ] All Twenty-fork-specific taskets in `.taskets/20260510-162158-*` are either `status: deleted` (explicit Twenty references) or have re-scoped bodies pointing at ADR-009.
- [ ] `001-technical-deep-dive.md` and `002-v0-build-kickoff.md` marked `status: done` with a closure note referencing this tasket group.
- [ ] `go.work` + `apps/web` Vite scaffold + Compose stack (Postgres + Authentik + LGTM + Caddy) running locally.
- [ ] First OIDC login flow end-to-end with workspace-scoped session cookie (`Domain=` per-subdomain).
- [ ] `lecrm_provision_workspace(uuid)` SECURITY DEFINER function applied via Atlas and smoke-tested.
- [ ] CI pipeline passes on a no-op PR (`go test`, `atlas migrate lint`, `golangci-lint`, `gosec`, `govulncheck`, `go mod verify`).
- [ ] Week-2 Go ramp checkpoint recorded: PASS (continue Go) or FAIL (switch to TypeScript+Hono).

## Reading list (start here)

1. [ADR-009](docs/adr/ADR-009-stack-and-license.md) §1 (backend), §2 (database + provisioning function), §4 (API surface), §5 (frontend), §7 (auth), §8 (build/monorepo), §9 (schedule gates).
2. [ADR-001](docs/adr/ADR-001-tenancy-model.md) (read with the 2026-05-10 amendment banner — three sections superseded; current mechanism is in ADR-009 §2).
3. [STRATEGIC-OVERVIEW §2 (Technical)](docs/STRATEGIC-OVERVIEW.md) (revised 2026-05-10 — OVH-first hosting, 11-13 week timeline, schedule gates).
4. [docs/research/stack-selection.md §11](docs/research/stack-selection.md) (council validation — read the architect's R1-R11 refinements; many are baked into the §8.1 monorepo layout and the §2.1 provisioning function).

## Notes for the executor

This is a hybrid housekeeping + execution tasket. Part A (housekeeping) is ~2-4 hours and can run in parallel with Part B kickoff. Part B is ~5 days. Part C is the binding decision gate.

If the executor is Claude Code in an agent loop, **do not skip Part C** — the Go-vs-TypeScript fallback is the single biggest schedule lever in the plan and must be evaluated honestly, not optimistically.
