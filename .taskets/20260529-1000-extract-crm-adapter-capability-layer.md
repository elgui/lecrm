---
id: 20260529-1000-extract-crm-adapter-capability-layer
title: Extract packages/crm-adapter capability layer; route REST handlers through it
status: done
priority: p1
created: 2026-05-29
updated: 2026-05-29
done: 2026-05-29
tags: [mcp, crm-adapter, capability-layer, refactor, adr-012, increment-1]
category: project
group: mcp-native-capability-layer
group_order: 210
order: 1
plan: true
done: 2026-05-29
---

# Extract packages/crm-adapter capability layer; route REST handlers through it

## Context

Foundational refactor for ADR-012 (`docs/adr/ADR-012-mcp-native-capability-layer.md` §1, §10 Increment 1.1, TO RESOLVE 1). **No user-visible behavior change** — this is the structural precondition for write-capable MCP.

Today CRM business logic lives coupled to HTTP handlers in `apps/api/internal/crm/handlers.go` (15 CRUD endpoints, delivered by tasket `20260525-1003`), and the MCP adapter carries a *separate* read implementation in `apps/mcp/internal/store/`. ADR-009 §8.1 planned a shared `packages/crm-adapter/` to avoid exactly this divergence. This tasket realizes it as a **protocol-agnostic capability layer** so REST, MCP, and connector-events all become thin projections over one source of truth.

Working directory: `/home/gui/Projects/leCRM`. Source of truth: `docs/adr/ADR-012-mcp-native-capability-layer.md`.

## Approach

The capability layer depends only on the store + domain types — never on `net/http`, chi, or JSON-RPC. Each operation:
- takes a resolved `Principal` (workspace + role + scopes + `actor_type`) — see `apps/api/internal/rbac/role.go` — never an `http.Request`;
- enforces RBAC, idempotency (`core.idempotency_keys`), and fail-closed audit (`core.audit_logs`) **inside** the call, not in the adapter;
- returns domain results, not wire formats.

Match the existing module layout (`go.work`). Either `packages/crm-adapter/` (per ADR-009 §8.1) or an `apps/api/internal/crm/service` package linkable by both binaries — pick whichever keeps `go.work` clean and is importable by `apps/mcp`; record the choice in the commit body. Reads AND writes move into it.

## Steps

1. Create the capability package (e.g. `packages/crm-adapter/`); wire into `go.work` if a new module.
2. Define capability operations with `Principal`-first signatures, covering what `handlers.go` does today: contacts/companies/deals list+get+create+update+delete, deal stage transition, pipeline stages, contact full-text search.
3. Move business logic from `apps/api/internal/crm/handlers.go` into the capability layer: RBAC authorization, the `readTx`/`writeTx` workspace-scoped transaction wrappers, idempotency (`core.idempotency_keys`), and fail-closed audit emission.
4. Reduce `handlers.go` to thin HTTP adapters: decode request → build `Principal` from context → call capability op → encode response/error. No business logic remains in handlers.
5. Confirm the connector-event path (`apps/api/internal/crm/connectors.go`) routes its mutations through the same capability operations where they overlap (e.g. contact upsert, deal stage move) — do not leave a third divergent mutation implementation.
6. Run the full suite; fix any drift. Behavior must be identical.

## Done When

- [ ] Capability package exists, `Principal`-first, no `net/http` import.
- [ ] `apps/api/internal/crm/handlers.go` contains no business logic — only decode/encode + capability calls.
- [ ] RBAC + idempotency + fail-closed audit enforced inside the capability layer.
- [ ] Connector-event mutations reuse the same capability ops (no third mutation impl).
- [ ] All existing `apps/api` tests pass unchanged (no behavior change).
- [ ] `golangci-lint` clean.

## Completion Verification

1. `export PATH=$PATH:/usr/local/go/bin`
2. `test -d packages/crm-adapter || ls apps/api/internal/crm/service` -- capability package exists
3. `grep -L "net/http" packages/crm-adapter/*.go 2>/dev/null` -- (sanity) capability files do not import net/http
4. `cd apps/api && go build ./... && go test -race -count=1 ./...` -- all pass
5. `golangci-lint run ./...` -- clean
6. Commit: `refactor(crm): extract protocol-agnostic capability layer (ADR-012 §1)`

## References

- `docs/adr/ADR-012-mcp-native-capability-layer.md` §1, §10 — capability-layer decision + increment plan
- `docs/adr/ADR-009-stack-and-license.md` §8.1 — planned `packages/crm-adapter/`
- `apps/api/internal/crm/handlers.go` — current CRUD logic to extract (tasket `20260525-1003`)
- `apps/api/internal/crm/connectors.go` — connector-event mutations to converge
- `apps/api/internal/rbac/role.go` — `Principal` / role hierarchy
- `core.idempotency_keys`, `core.audit_logs` — reuse (ADR-007)
