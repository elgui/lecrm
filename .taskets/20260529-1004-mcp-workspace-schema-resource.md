---
id: 20260529-1004-mcp-workspace-schema-resource
title: "Expose lecrm://workspace/schema MCP Resource (self-describing custom-property schema)"
status: todo
priority: p2
created: 2026-05-29
category: project
group: mcp-native-capability-layer
group_order: 210
order: 5
plan: true
tags: [mcp, resources, metadata-engine, self-describing, adr-012, increment-1]
---

# Expose lecrm://workspace/schema MCP Resource (self-describing custom-property schema)

## Pre-flight: Verify Previous Tasket

Before starting, verify tasket order:1 (capability layer) and order:2 (MCP links it) completed:

1. `export PATH=$PATH:/usr/local/go/bin`
2. `test -d packages/crm-adapter || ls apps/api/internal/crm/service` -- capability layer
3. `cd apps/mcp && go build ./cmd/lecrm-mcp` -- MCP binary builds
4. `cd apps/api && go test -race -count=1 ./...` -- all pass

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

ADR-012 §5 (self-describing schema — the differentiator) + §9 (use MCP Resources, not just Tools), §10 Increment 1.4, TO RESOLVE 5. This is independent of the write tools (order:3/4) and could run in parallel; it is ordered last-but-one because it is a smaller, lower-risk addition.

A generic CRM MCP exposes a fixed schema. leCRM has a **per-workspace metadata engine** (ADR-010, custom-property definitions wired in tasket `20260525-1001`). Exposing the connecting workspace's *actual* custom-property schema lets an LLM use real fields correctly (e.g. discover that this workspace tracks `cms`, `geo`, `lead_score` on contacts). A closed CRM structurally cannot do this. The MCP adapter today uses only Tools — this adds the first **Resource**.

Working directory: `/home/gui/Projects/leCRM`. Source of truth: `docs/adr/ADR-012-mcp-native-capability-layer.md`.

## Steps

1. Implement MCP `resources/list` and `resources/read` in `apps/mcp/internal/mcpserver/` (mirror the existing tools dispatch structure in `server.go`/`tools.go`).
2. Add a capability-layer read op that returns the workspace's `custom_property_definitions` (parent_type, property_key, property_type, allowed_values) — from the metadata engine (`apps/api/internal/metadata/`, migration `0003_metadata_engine.sql`).
3. Expose it as resource URI `lecrm://workspace/schema`, scoped to the caller's resolved workspace (per-workspace isolation — never leak another workspace's schema). Serialize compactly for LLM consumption (token-efficient).
4. Tests: `resources/list` includes the schema resource; `resources/read` returns this workspace's definitions; a second workspace sees only its own; empty-definitions workspace returns a valid empty schema.

## Done When

- [ ] `resources/list` + `resources/read` implemented in the MCP adapter.
- [ ] `lecrm://workspace/schema` returns the connecting workspace's custom-property definitions via the capability layer.
- [ ] Per-workspace scoping verified (no cross-workspace schema leak).
- [ ] Serialization is compact/token-efficient.
- [ ] `apps/mcp` builds; tests pass.

## Completion Verification

1. `export PATH=$PATH:/usr/local/go/bin`
2. `grep -ri "resources/read\|resources/list\|workspace/schema" apps/mcp/internal/mcpserver/ | wc -l` -- expect ≥2
3. `cd apps/mcp && go build ./cmd/lecrm-mcp`
4. `cd apps/mcp && go test -race -count=1 ./...` -- pass
5. Commit: `feat(mcp): self-describing workspace schema resource (ADR-012 §5/§9)`

## References

- `docs/adr/ADR-012-mcp-native-capability-layer.md` §5, §9
- `apps/api/internal/metadata/` — custom-property definitions engine (ADR-010, tasket `20260525-1001`)
- `packages/db/migrations/0003_metadata_engine.sql` — `custom_property_definitions`
- `apps/mcp/internal/mcpserver/server.go`, `tools.go` — dispatch structure to mirror
