---
id: 20260529-1003-mcp-intent-write-tools
title: "Ship 3 MCP intent write tools: advance_deal, log_interaction, capture_lead"
status: done
priority: p1
created: 2026-05-29
updated: 2026-05-29
done: 2026-05-29
tags: [mcp, write-tools, intent-tools, adr-012, increment-1]
category: project
group: mcp-native-capability-layer
group_order: 210
order: 4
plan: true
---

# Ship 3 MCP intent write tools: advance_deal, log_interaction, capture_lead

## Pre-flight: Verify Previous Taskets

Before starting, verify taskets order:1, order:2, order:3 completed:

1. `export PATH=$PATH:/usr/local/go/bin`
2. `test -d packages/crm-adapter || ls apps/api/internal/crm/service` -- capability layer (order:1)
3. `cd apps/mcp && go build ./cmd/lecrm-mcp` -- MCP links capability layer (order:2)
4. `test -f docs/mcp/write-safety-contract.md` -- write-safety contract (order:3)
5. `cd apps/api && go test -race -count=1 ./...` -- all pass

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

ADR-012 §3 (tool taxonomy), §4 (user stories), §10 Increment 1.3. This is the headline deliverable: the first **read-write** MCP surface — the conversational write layer the client chatbot needs (story S1: *"mark the Acme deal won, they signed today"*).

These are **intent-shaped composites**, NOT a CRUD mirror of REST (ADR-012 §2). Each encapsulates a real user story as one safe, idempotent, auditable call, composing the safety primitives from tasket order:3 and dispatching to capability-layer operations from tasket order:1.

Working directory: `/home/gui/Projects/leCRM`. Source of truth: `docs/adr/ADR-012-mcp-native-capability-layer.md`.

## The three tools (ADR-012 §3)

- `advance_deal(deal, to_stage, note?, mark_closed_at?)` — stage transition + activity, with stage-name fuzzy match. (Story S1.)
- `log_interaction(contact_or_company, summary, outcome?)` — upserts contact if needed + appends an activity. (Story S2/S3.)
- `capture_lead(name, email?, company?, source)` — contact (dedup by email) + optional deal in first stage. (Story S2.) **Reuse the capability ops behind the connector-event `candidate.enriched`/`invitation.created` path** (`apps/api/internal/crm/connectors.go`) — same capability calls, different door. Do not re-implement the upsert.

## Steps

1. Register the three tools in the MCP catalog (`apps/mcp/internal/mcpserver/tools.go`) behind the write scope from tasket order:3. Rich, prompt-quality descriptions and input schemas (descriptions ARE the interface for LLMs — ADR-012 §2).
2. Implement dispatch for each, calling capability-layer write ops with a `Principal` carrying the token scope + `actor_type=mcp_agent`.
3. Apply every §6 control via the order:3 helpers: scope gate (read-only token → denied), idempotency key, `dry_run` preview, fail-closed audit attribution. `advance_deal` with `mark_closed_at` and any multi-entity effect honors the confirmation handshake if classified destructive/bulk.
4. Tests:
   - `advance_deal` moves stage + writes activity; fuzzy stage-name match; `dry_run` mutates nothing.
   - `log_interaction` upserts contact when absent, appends activity.
   - `capture_lead` dedups by email; creates deal in first stage; goes through the same capability op as the connector path.
   - read-only-scoped token → all three denied (403/isError), no mutation.
   - idempotency-key replay → no duplicate entities.
   - cross-tenant: token for workspace A cannot write to workspace B.
   - audit row with `actor_type=mcp_agent` on every successful mutation.

## Done When

- [ ] `advance_deal`, `log_interaction`, `capture_lead` in the MCP tool catalog, behind the write scope.
- [ ] All dispatch through capability-layer ops (no logic duplication); `capture_lead` shares the connector-event upsert path.
- [ ] §6 controls enforced: scope gate, idempotency, dry-run, fail-closed audit (`actor_type=mcp_agent`).
- [ ] Read-only token denied; cross-tenant write blocked; idempotent replay safe — all tested.
- [ ] `apps/mcp` builds; full suite passes.

## Completion Verification

1. `export PATH=$PATH:/usr/local/go/bin`
2. `grep -E "advance_deal|log_interaction|capture_lead" apps/mcp/internal/mcpserver/tools.go | wc -l` -- expect ≥3
3. `cd apps/mcp && go build ./cmd/lecrm-mcp`
4. `cd apps/mcp && go test -race -count=1 ./... && cd ../api && go test -race -count=1 ./...` -- all pass
5. Commit: `feat(mcp): intent write tools — advance_deal, log_interaction, capture_lead (ADR-012 §3)`

## References

- `docs/adr/ADR-012-mcp-native-capability-layer.md` §2, §3, §4, §6
- `docs/mcp/write-safety-contract.md` — safety primitives (tasket order:3)
- `apps/mcp/internal/mcpserver/tools.go` — tool catalog + dispatch
- `apps/api/internal/crm/connectors.go` — connector-event upsert path to reuse for `capture_lead`
