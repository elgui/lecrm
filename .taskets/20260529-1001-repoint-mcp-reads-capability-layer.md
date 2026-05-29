---
id: 20260529-1001-repoint-mcp-reads-capability-layer
title: Repoint apps/mcp reads at the capability layer; delete divergent internal/store
status: done
priority: p1
created: 2026-05-29
updated: 2026-05-29
done: 2026-05-29
tags: [mcp, crm-adapter, consolidation, adr-012, increment-1]
category: project
group: mcp-native-capability-layer
group_order: 210
order: 2
plan: true
---

# Repoint apps/mcp reads at the capability layer; delete divergent internal/store

## Pre-flight: Verify Previous Tasket

Before starting, verify tasket order:1 ("Extract packages/crm-adapter capability layer") completed:

1. `export PATH=$PATH:/usr/local/go/bin`
2. `test -d packages/crm-adapter || ls apps/api/internal/crm/service` -- capability package exists
3. `cd apps/api && go test -race -count=1 ./...` -- all pass
4. `git log --oneline -15 | grep -i "capability layer"` -- extraction commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

ADR-012 §1 consolidation requirement, §10 Increment 1.2, TO RESOLVE 2. The MCP adapter today has its own read implementation in `apps/mcp/internal/store/store.go` — a divergent second copy of "list contacts" etc. Now that the capability layer exists (tasket order:1), the MCP binary must consume it as a library and the divergent read logic must be deleted. This is harmless at 6 read tools but fatal once write tools land — there must be exactly one implementation.

The separate-binary topology (ADR-009 §4.2) is **binding and preserved**: `apps/mcp` still builds as its own binary and deploys as its own Compose service (`deploy/compose/mcp.yml`). What changes is that it *links* the capability layer instead of carrying its own store.

Working directory: `/home/gui/Projects/leCRM`. Source of truth: `docs/adr/ADR-012-mcp-native-capability-layer.md`.

## Approach

Repoint the 6 existing read tools (`read_contact`, `list_contacts`, `read_deal`, `list_deals`, `list_pipeline_stages`, `search_contacts` — see `apps/mcp/internal/mcpserver/tools.go`) to dispatch to capability-layer read operations. **Preserve the DB-level read-only guarantee for read-only-scoped tokens**: read-only tokens must still resolve to a `workspace_<id>_ro` Postgres connection (migration `0013_workspace_ro_role.sql`). The capability layer should accept the connection/role appropriate to the caller's scope, so the `_ro` hardening survives for reads while the write path (later taskets) uses a read-write role.

## Steps

1. Add the capability package as a dependency of `apps/mcp` (import path; `go.work` already has both).
2. In `apps/mcp/internal/mcpserver/tools.go` `dispatchTool`, replace `s.reader.*` calls with capability-layer read calls, passing a `Principal` built from the resolved workspace + token scope.
3. Ensure read-only-scoped tokens drive the `workspace_<id>_ro` connection (DB-enforced read-only preserved). Document how the capability layer selects the role/connection by scope.
4. Delete the now-redundant read implementation in `apps/mcp/internal/store/store.go` (keep only a thin connection/pool provider if still needed; remove the duplicated query logic).
5. Keep tool input/output schemas stable — MCP clients should see no contract change for reads.
6. Run MCP build + the read-tool tests.

## Done When

- [ ] The 6 read tools dispatch through the capability layer (no `apps/mcp/internal/store` query duplication).
- [ ] Divergent read logic in `apps/mcp/internal/store/store.go` deleted.
- [ ] Read-only-scoped tokens still use the `workspace_<id>_ro` connection (DB-level read-only preserved); how the role is selected is documented.
- [ ] `apps/mcp` still builds as a separate binary; read tool contracts unchanged.
- [ ] Tests pass.

## Completion Verification

1. `export PATH=$PATH:/usr/local/go/bin`
2. `cd apps/mcp && go build ./cmd/lecrm-mcp` -- MCP binary builds
3. `grep -rc "SELECT" apps/mcp/internal/store/ 2>/dev/null | awk -F: '{s+=$2} END{print s+0}'` -- expect 0 (no duplicated read SQL left)
4. `cd apps/mcp && go test -race -count=1 ./... && cd ../api && go test -race -count=1 ./...` -- all pass
5. Commit: `refactor(mcp): route reads through shared capability layer; drop divergent store (ADR-012 §1)`

## References

- `docs/adr/ADR-012-mcp-native-capability-layer.md` §1, §10
- `apps/mcp/internal/mcpserver/tools.go` — the 6 read tools + `dispatchTool`
- `apps/mcp/internal/store/store.go` — read impl to delete
- `packages/db/migrations/0013_workspace_ro_role.sql` — `_ro` role to preserve for reads
- `docs/adr/ADR-009-stack-and-license.md` §4.2 — separate-binary binding (preserved)
