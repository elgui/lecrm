---
id: 20260510-202450-b844
title: leCRM v0 — Twenty-fork tasket housekeeping + Week 1 scaffolding (Parts A+B, fresh session)
status: later
priority: p0
created: 2026-05-10
updated: 2026-05-14
category: engineering
review: "part-a-complete: housekeeping pass done in commit 6f490e0 on 2026-05-10. Skip straight to Part B (Week 1 scaffolding). The 9 lecrm-v0-build sub-taskets have been classified (3 deleted, 6 re-scoped with new bodies, 2 meta-taskets were already done)."
group: lecrm-v0-scaffolding
group_order: 1
order: 1
---

## Read this cold — full context inline

This tasket is the **first execution session** after the leCRM stack decision landed. It runs the housekeeping pass on stale Twenty-fork taskets and stands up the v0 scaffold per the locked ADR-009 architecture. The binding **week-2 Go ramp checkpoint** is a separate tasket (`lecrm-v0-scaffolding` group, order 2) — do **not** run it here; this tasket ends when the scaffold is up and CI is green.

## What just happened (background)

leCRM was previously planned as an AGPL fork of Twenty CRM. On 2026-05-10 a four-round multi-agent council pivoted to **clean-room reimplementation** ([ADR-008](docs/adr/ADR-008-clean-room-reimplementation.md)). A second five-voice council validated the stack research and produced [ADR-009](docs/adr/ADR-009-stack-and-license.md), now Accepted. The locked stack:

- **Backend**: Go 1.23 + `net/http` + Chi + `sqlc` + Atlas + `river` + `zitadel/oidc`. TypeScript+Hono is the runner-up, selected only if the week-2 Go ramp checkpoint (separate tasket, order 2) fails.
- **Database**: PostgreSQL 17, schema-per-tenant via **per-workspace Postgres role** with role-level `ALTER ROLE search_path`, provisioned through a single `SECURITY DEFINER` function (`lecrm_provision_workspace(uuid)`). Phase 1 self-hosted on OVH VPS; Phase 2 OVH Managed Postgres or Ubicloud-on-Hetzner-DE fallback.
- **API**: REST (`/v1/…`) + thin MCP adapter (separate `cmd/lecrm-mcp/` binary in the same Go module). GraphQL deferred to v2.
- **Frontend**: React 19 + Vite + TanStack Router/Query + shadcn/ui + Radix, embedded in the Go binary via `//go:embed dist/*`.
- **Auth**: Authentik 2025.10 self-hosted at v0 (Redis-free since 2025.10); Zitadel Cloud EU at v1.
- **Observability**: LGTM Compose stack at v0 (Loki + Grafana + Tempo + Prometheus + OTel Collector); Grafana Cloud EU at v1.
- **License**: Apache 2.0 (FSL-2.0-Apache-2.0 as upgrade path).
- **Background jobs**: `river` (Postgres-native, no Redis at v1). Per-tenant `river_<workspace_base36>` schema.

The existing `lecrm-v0-build` tasket group (created 2026-05-10 at 16:21, eight sub-taskets) was scoped against the Twenty-fork posture. Most are dead or need re-scoping. That housekeeping is **Part A** of this tasket.

## Part A — Housekeep the stale Twenty-fork taskets (~2-4 hours)

Inspect each tasket in `.taskets/`. Use the Tasket Housekeep workflow rule: **never `rm` files; the safe terminal action is `status: deleted` via the API**.

**API for status updates**:
```bash
curl -s -X PUT "http://localhost:8871/api/taskets/project/status?project_root=/home/gui/Projects/leCRM&task_id=<ID>" \
  -H "Content-Type: application/json" \
  -d '{"status": "deleted"}'
```

(For body edits use `PUT /api/taskets/project/<task_id>` with a JSON payload containing the new `body` field.)

### Classification

**DEAD — Twenty-fork-specific, no clean-room analogue** (`status: deleted`):
- `20260510-162158-6149-lecrm-v0-pre-commit-guard-against-license-enterpri.md` — Enterprise-license file guard; irrelevant under clean-room.
- `20260510-162158-8550-lecrm-v0-gbconsult-oidc-controller-replacement.md` — Twenty auth-module DI override; we build OIDC from scratch with `zitadel/oidc`.
- `20260510-162158-9466-lecrm-v0-agpl-13-footer-mounting-via-twenty-sdk-ex.md` — AGPL §13 footer; no §13 obligation under Apache 2.0.

**SURVIVES IN SPIRIT — re-scope against the new stack** (edit body via `PUT /api/taskets/project/<id>`):
- `20260510-162158-1023-lecrm-v0-secrets-management-baseline-sops-age-trac.md` — SOPS+age secrets baseline; ADR-007 retained. Re-scope additions: `lecrm_provisioner` Postgres credential (Tier-0 secret, annual rotation per ADR-009 §2.1) and Authentik admin credential.
- `20260510-162158-499c-lecrm-v0-brevo-transactional-api-integration-track.md` — Brevo; ADR-003 retained. Re-scope: integration via Go HTTP client (not NestJS service).
- `20260510-162158-d1ba-lecrm-v0-backup-baseline-wal-g-gpg-hetzner-object.md` — WAL-G + GPG; ADR-006 retained. Re-scope: target **OVH Object Storage** (S3-compatible) per the §2 OVH-first decision; verify WAL-G against OVH's S3 endpoint.
- `20260510-162158-29dc-lecrm-v0-embedded-metabase-reporting-track-d.md` — Reporting iframe. Re-scope: ADR-009 §9 names Cube.dev iframe as the v0 bridge; keep as Metabase if it scopes faster, otherwise switch to Cube.dev.

**SURVIVES UNCHANGED (post-v0)**:
- `20260510-162158-aa6f-lecrm-v1-native-sequences-engine-track-f-post-firs.md` — v1 work, not v0.
- `20260510-155549-11e5-lecrm-v2-prototype-telegram-bot-to-twenty-graphql.md` — v2; re-scope from "Twenty GraphQL" to "leCRM REST + MCP adapter" per ADR-009 §4.

**META-taskets to retire** (`status: done` with closure note):
- `001-technical-deep-dive.md`
- `002-v0-build-kickoff.md`

Both are superseded by ADR-008 + ADR-009 + this tasket group. Closure note: "Superseded by `.taskets/20260510-202020-...` (lecrm-v0-scaffolding group)."

## Part B — Stand up the v0 scaffold (~5 days)

### Monorepo layout (per ADR-009 §8.1)

```
lecrm/
├── go.work
├── apps/
│   ├── api/cmd/lecrm-api/      # main HTTP server (REST + embedded SPA)
│   ├── mcp/cmd/lecrm-mcp/      # MCP adapter (separate Compose service)
│   ├── migrate/cmd/lecrm-migrate/  # Atlas runner (Compose pre-deploy job)
│   └── web/                    # React 19 + Vite + TanStack
├── packages/
│   ├── db/                     # sqlc-generated Go + Atlas HCL
│   ├── crm-adapter/            # CrmAdapter Go interface (shared api ↔ mcp)
│   ├── shared-types/           # OpenAPI-generated TS types for web/
│   └── tools/                  # mage tasks
├── deploy/compose/
│   ├── postgres.yml            # Postgres 17
│   ├── authentik.yml           # 2025.10 Redis-free
│   ├── lgtm.yml                # Loki + Grafana + Tempo + Prometheus + OTel Collector
│   └── caddy/                  # DNS-01 wildcard cert config
├── LICENSE                     # Apache-2.0 verbatim
├── NOTICE                      # Copyright (c) 2026 GB Consult SARL
└── .github/workflows/
```

Frontend builds to `apps/web/dist/`; Go API embeds via `//go:embed dist/*`. Caddy terminates TLS and proxies all traffic to the single Go binary, which routes `/v1/*` → REST handlers and `/*` → embedded SPA.

### Checklist

- [ ] `mkdir -p apps/api/cmd/lecrm-api apps/mcp/cmd/lecrm-mcp apps/migrate/cmd/lecrm-migrate apps/web packages/db packages/crm-adapter packages/shared-types packages/tools deploy/compose deploy/caddy`
- [ ] `go work init && go work use ./apps/api ./apps/mcp ./apps/migrate ./packages/db ./packages/crm-adapter ./packages/tools` (skip packages with no `go.mod` initially)
- [ ] Per-package `go mod init github.com/gbconsult/lecrm/apps/api` etc.; add `chi`, `sqlc`, `zitadel/oidc`, `river`, `mark3labs/mcp-go`, `golang.org/x/time` as deps where they belong.
- [ ] `pnpm create vite apps/web --template react-ts` + add TanStack Router/Query + shadcn/ui CLI init + Tailwind. Lock React 19 + Vite latest.
- [ ] `LICENSE` (Apache 2.0 verbatim) and `NOTICE` (`Copyright (c) 2026 GB Consult SARL`) at repo root from the first commit.
- [ ] `deploy/compose/postgres.yml`: Postgres 17 with `POSTGRES_USER=lecrm_provisioner` (or separate admin role for the DDL grant on `pg_proc`). Volume-mounted data dir.
- [ ] `deploy/compose/authentik.yml`: Authentik 2025.10 server + worker (no Redis service — 2025.10 dropped Redis). Postgres-backed session store. Configure with Google Workspace OIDC upstream as a single source for v0; Microsoft Entra deferred to v1.
- [ ] `deploy/compose/lgtm.yml`: Loki + Grafana + Tempo + Prometheus + OTel Collector. Memory budget ~1.1 GB; runs comfortably on a CX22/equivalent OVH VPS.
- [ ] `deploy/caddy/Caddyfile`: DNS-01 wildcard cert for the chosen v0 domain (e.g. `*.lecrm.test` for local; `*.lecrm.fr` for the first deployed env). Cloudflare DNS API token in SOPS-encrypted env file.
- [ ] Atlas HCL schema in `packages/db/migrations/`:
  - `core.workspaces (id uuid pk, slug text unique, created_at, ...)`
  - `core.audit_log` per ADR-007 §3
  - **`lecrm_provision_workspace(p_workspace_id uuid)` SECURITY DEFINER function** per ADR-009 §2.1 — owned by `lecrm_provisioner` role; encapsulates CREATE ROLE + CREATE SCHEMA + ALTER ROLE search_path + ALTER DEFAULT PRIVILEGES + grants + per-tenant `river_*` schema. Must be idempotent.
- [ ] First OIDC login flow end-to-end:
  - Authentik admin → create OIDC client `lecrm-api`
  - Go `zitadel/oidc` RP in `apps/api/internal/auth/`
  - Session cookie set with **`Domain=<workspace>.lecrm.fr; HttpOnly; Secure; SameSite=Strict`** (per ADR-009 §5.2 cookie-scope mandate — never a parent-domain wildcard cookie; cross-tenant leakage risk).
  - Store identity as `(issuer, sub)` tuple per ADR-009 §7.1, never raw `sub`.
- [ ] Smoke test: provision a `workspace_acme` workspace via `lecrm_provision_workspace`, verify role + schema + grants exist, run an idempotent re-call, then connect AS `workspace_acme` and confirm `SHOW search_path` returns `workspace_acme, public`.
- [ ] CSP header configured in Caddy or the Go static handler: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'`.
- [ ] CI scaffold (`.github/workflows/ci.yml`): on every PR run `go test ./...` (with testcontainers-go on Postgres), `pnpm test`, `atlas migrate lint`, `golangci-lint run`, `gosec`, `govulncheck ./...`, `go mod verify`. Multi-stage Docker build (Vite → Go → final scratch image).
- [ ] OVH VPS provisioned for the first Phase-1 deploy target. Compose stack pulled and running. OIDC login reachable at the first test subdomain (e.g. `dev.lecrm.fr`).

### Reading-order shortlist

Don't try to absorb every doc at once. In this order:

1. [ADR-009](docs/adr/ADR-009-stack-and-license.md) §1 (backend) → §2 (database + provisioning function — read §2.1 carefully, it's the most subtle part) → §8 (monorepo layout) → §5 (frontend) → §7 (auth).
2. [ADR-001](docs/adr/ADR-001-tenancy-model.md) (read the 2026-05-10 amendment banner first — three sections are SUPERSEDED; the current mechanism is in ADR-009 §2).
3. [docs/research/stack-selection.md §11](docs/research/stack-selection.md) (council validation transcript — read the architect's R1-R11 refinements; many are baked into the §8.1 layout and §2.1 function).
4. [STRATEGIC-OVERVIEW §2 (Technical)](docs/STRATEGIC-OVERVIEW.md) (revised 2026-05-10 — OVH-first hosting, 11-13 week timeline, schedule gates).
5. [Tasket Housekeep workflow](~/.claude/skills/Tasket/workflows/Housekeep.md) — for the right API patterns when classifying old taskets in Part A.

## Out of scope for this tasket

- **The week-2 Go ramp checkpoint** — that's `lecrm-v0-scaffolding` order 2 (next tasket). Run it as a separate session at end of week 2 once the scaffolding here is up and the three litmus tests are runnable.
- Custom-object metadata engine (deferred to ADR-010, week 5).
- Email integration scaffolding (Gmail OAuth app review starts week 5-6, not now).
- MCP adapter implementation body (week 13 work; the binary + package skeleton are in Part B, the wire-format work is not).
- Production OVH Managed Postgres provisioning (Phase-2 work; v0 runs Postgres self-hosted on the OVH VPS).

## Acceptance criteria

- [ ] All taskets in `.taskets/20260510-162158-*` classified per Part A and updated via the Tasket API.
- [ ] `001-technical-deep-dive.md` and `002-v0-build-kickoff.md` marked `status: done` with a closure note pointing at the `lecrm-v0-scaffolding` group.
- [ ] `go.work` initialized; `apps/web` Vite scaffold runs `pnpm dev` cleanly with shadcn/ui imported.
- [ ] Compose stack up (Postgres + Authentik + LGTM + Caddy) — `docker compose up` succeeds, Authentik admin reachable, OIDC client provisioned, login reachable from `apps/web` dev server.
- [ ] Session cookie set with per-subdomain `Domain=` (verified via browser devtools).
- [ ] `lecrm_provision_workspace(uuid)` function applied via Atlas; idempotent re-call confirmed; `SHOW search_path` returns the expected per-workspace value.
- [ ] CI pipeline green on a no-op PR.
- [ ] OVH VPS provisioned and reachable; first test subdomain serves the OIDC login.
- [ ] LICENSE + NOTICE files at the repo root.

## Notes for the executor

Bound this to **5 working days max** including Part A. If any single checklist item in Part B blocks > 4 working hours despite Claude Code assistance — and the blocker is Go-language-specific (idiom struggle, type-system fight, cargo-cult patterns) — **promote that observation into the order-2 Go ramp checkpoint immediately**, don't wait until end of week 2. The fallback to TypeScript+Hono exists; honest evaluation of velocity is the highest-leverage decision in the plan.

If hooked into an automation loop: this tasket ends at "scaffold up + CI green." It does not include implementing CRM domain models, REST handlers beyond auth, or the MCP adapter wire format. Those are downstream taskets that the order-2 checkpoint either green-lights (continue in Go) or redirects (rewrite in TS+Hono).
