---
id: 20260529-1002-mcp-write-safety-contract
title: "MCP write-safety contract: scope→RBAC mapping + dry-run/confirmation handshake"
status: done
priority: p1
created: 2026-05-29
updated: 2026-05-29
done: 2026-05-29
tags: [mcp, write-safety, rbac, idempotency, dry-run, adr-012, increment-1]
category: project
group: mcp-native-capability-layer
group_order: 210
order: 3
plan: true
---

# MCP write-safety contract: scope→RBAC mapping + dry-run/confirmation handshake

## Pre-flight: Verify Previous Tasket

Before starting, verify tasket order:1 (capability layer) completed:

1. `export PATH=$PATH:/usr/local/go/bin`
2. `test -d packages/crm-adapter || ls apps/api/internal/crm/service` -- capability package exists
3. `cd apps/api && go test -race -count=1 ./...` -- all pass

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

ADR-012 §6 (write-safety model) + §7 (auth-plane / tool-plane separation), TO RESOLVE 3 & 4. This tasket defines and implements the **shared safety primitives** every MCP write tool will reuse — *before* any write tool exists — so the next tasket just composes them. It is the MCP equivalent of the OpenAPI+service-token contract tasket (`20260525-1006`).

Two outputs: (a) a short spec doc, and (b) reusable helpers in the capability/auth layer.

Working directory: `/home/gui/Projects/leCRM`. Source of truth: `docs/adr/ADR-012-mcp-native-capability-layer.md`.

## Design constraints (from ADR-012)

- **Tools authorize against `Principal`, never against the auth mechanism** (§7). The scope→role mapping must work identically whether the `Principal` came from a service token (today) or a future OAuth token. Do not hardcode `actor_type=mcp_agent` as the only possible writer.
- **Read-only tokens cannot reach write tools** — scope gating is the primary blast-radius control (§8).
- Reuse existing infra: service-token scopes (`apps/api/internal/auth/service_token.go`, ADR-009 §4.1), RBAC roles (`apps/api/internal/rbac/role.go`), `core.idempotency_keys`, fail-closed audit (`core.audit_logs`).

## Steps

1. Write `docs/mcp/write-safety-contract.md` specifying:
   - the MCP write scope(s) and the **scope → RBAC role** mapping table;
   - the `dry_run: true` request flag and the **preview response shape** (the would-be effect / diff, no mutation);
   - the **confirmation-token handshake** for destructive/bulk ops (dry-run returns a confirmation token; the real call must echo it);
   - idempotency-key handling for MCP tool calls (how the key is supplied/derived; reuse `core.idempotency_keys`);
   - audit attribution (`actor_type=mcp_agent` + token id) — fail-closed.
2. Implement in the capability/auth layer:
   - a scope-check helper that maps token scope → `Principal` role and authorizes a write op (returns a clean denial for read-only scope);
   - dry-run plumbing: capability write ops accept a `dryRun` flag and return a structured preview instead of mutating;
   - a confirmation-token mechanism for ops flagged destructive/bulk.
3. Unit-test each helper: read-only scope denied; write scope allowed; dry-run mutates nothing; confirmation-token required-and-validated; idempotent replay returns cached result.
4. Mark ADR-012 TO RESOLVE 3 & 4 resolved (add a one-line resolution note pointing at this tasket's commit + the spec doc).

## Done When

- [ ] `docs/mcp/write-safety-contract.md` exists with scope→RBAC table, dry-run shape, confirmation handshake, idempotency + audit rules.
- [ ] Scope-check, dry-run, and confirmation-token helpers implemented in the capability/auth layer.
- [ ] Helpers are `Principal`-based and mechanism-agnostic (no service-token-only assumption; no hardcoded sole `actor_type`).
- [ ] Unit tests cover deny/allow/dry-run/confirm/idempotent-replay.
- [ ] ADR-012 TO RESOLVE 3 & 4 marked resolved.

## Completion Verification

1. `export PATH=$PATH:/usr/local/go/bin`
2. `test -f docs/mcp/write-safety-contract.md` -- spec exists
3. `grep -qi "scope" docs/mcp/write-safety-contract.md && grep -qi "dry" docs/mcp/write-safety-contract.md && grep -qi "confirm" docs/mcp/write-safety-contract.md` -- spec covers all three
4. `cd apps/api && go test -race -count=1 ./...` -- helper tests pass
5. Commit: `feat(mcp): write-safety contract — scope→RBAC, dry-run, confirmation (ADR-012 §6/§7)`

## References

- `docs/adr/ADR-012-mcp-native-capability-layer.md` §6, §7, §8
- `apps/api/internal/auth/service_token.go` — scope model (ADR-009 §4.1)
- `apps/api/internal/rbac/role.go` — RBAC roles
- `apps/api/internal/crm/connectors.go` — existing idempotency + fail-closed audit pattern to mirror
- `core.idempotency_keys`, `core.audit_logs`
