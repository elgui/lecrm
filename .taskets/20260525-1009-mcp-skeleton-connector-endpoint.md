---
id: 20260525-1009-mcp-skeleton-connector-endpoint
title: "MCP adapter skeleton + chatboting connector event endpoint"
status: todo
priority: p2
created: 2026-05-25
category: project
group: crm-frontend-rbac-export
group_order: 70
order: 3
plan: true
tags: [mcp, connector, chatboting, adr-011, sprint-9]
---

# MCP adapter skeleton + chatboting connector event endpoint

## Pre-flight: Verify Previous Taskets

Before starting, verify service tokens and entity handlers exist:

1. `grep -c 'service_tokens' packages/db/queries/*.sql` -- token queries exist
2. `ls apps/api/internal/http/contacts.go` -- entity handlers exist
3. `cd apps/api && go test -race -count=1 ./...` -- all tests pass

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

Two deliverables in one tasket because they share the service token auth layer:

1. **MCP adapter** (`apps/mcp/cmd/lecrm-mcp/`) — separate binary exposing CRM data to AI agents via the Model Context Protocol. Reads from a constrained PG role.

2. **Connector event endpoint** (ADR-011 §4) — `POST /v1/connectors/:source/events` on the main API. Receives async events from chatboting (and future connectors) and creates/updates CRM entities. Service token authenticated.

Sprint 9 work per `docs/sprint-plan.md` + ADR-011.

Source of truth: `docs/sprint-plan.md` Sprint 9, `docs/adr/ADR-011-chatboting-connector-boundary.md`
Working directory: `/home/gui/Projects/leCRM`

## Steps

### MCP Adapter

1. Add `apps/mcp` to `go.work`:
   ```
   use ./apps/mcp
   ```

2. Initialize module: `cd apps/mcp && go mod init github.com/gbconsult/lecrm/apps/mcp`

3. Implement `apps/mcp/cmd/lecrm-mcp/main.go`:
   - Import `mark3labs/mcp-go`
   - Streamable HTTP transport (not stdio — runs as Compose service)
   - Connect to Postgres with a read-mostly constrained role (separate from API's role)
   - Rate limit per (workspace_id, token_id) tuple

4. Define initial MCP tools:
   - `read_contact(id)` → contact with custom properties
   - `list_contacts(filter, cursor)` → paginated contact list
   - `read_deal(id)` → deal with stage info + custom properties
   - `list_deals(filter, cursor)` → paginated deal list
   - `list_pipeline_stages()` → all stages with deal counts
   - `search_contacts(query)` → pg full-text search

5. Add `deploy/compose/mcp.yml`:
   - Separate service, connects to same Postgres
   - Constrained role: SELECT on workspace tables, no INSERT/UPDATE/DELETE
   - Health check endpoint

### Connector Event Endpoint

6. Implement `POST /v1/connectors/:source/events` in main API:
   - Service token auth (must have `connector.push_events` scope)
   - Parse event envelope per ADR-011 §4:
     ```json
     {
       "event": "candidate.enriched|invitation.sent|invitation.claimed|...",
       "source": "chatboting",
       "timestamp": "...",
       "idempotency_key": "...",
       "workspace": "gbconsult",
       "payload": { ... }
     }
     ```
   - Idempotency: store processed keys, skip duplicates (200 OK, no re-processing)

7. Event handlers (per ADR-011 §4 event table):
   - `candidate.enriched` → upsert Contact (match on `payload.candidate.url` or email), set custom properties (score, CMS, geo, category)
   - `invitation.created` → create Deal at "Discovery" stage, link to Contact
   - `invitation.sent` → move Deal to "Proposal Sent", create Activity
   - `invitation.opened` → create Activity on Deal
   - `invitation.claimed` → move Deal to "Closed-Won", create Activity with tenant link
   - `invitation.expired` → move Deal to "Closed-Lost", create Activity
   - `invitation.reply_positive` → create Activity, flag Deal for follow-up

8. All connector mutations:
   - `actor_type = 'connector'`
   - `source_system = :source` (from URL param)
   - Audit-logged (fail-closed)

9. Tests:
   - Test: push `candidate.enriched` → Contact created with custom properties
   - Test: push `invitation.claimed` → Deal moves to Closed-Won + Activity created
   - Test: duplicate idempotency_key → 200 OK, no duplicate entities
   - Test: invalid event type → 400
   - Test: wrong service token scope → 403
   - Test: cross-tenant isolation (token for workspace A can't push to workspace B)

## Done When

- [ ] MCP adapter binary builds and starts as separate Compose service
- [ ] MCP tools return real CRM data (read_contact, list_deals, etc.)
- [ ] MCP uses constrained read-mostly PG role
- [ ] Connector endpoint accepts events from chatboting
- [ ] All 7 ADR-011 event types create/update correct entities
- [ ] Idempotency key prevents duplicate processing
- [ ] Service token auth with scope enforcement works
- [ ] All tests pass

## Completion Verification

1. `cd apps/mcp && go build ./cmd/lecrm-mcp` -- MCP binary builds
2. `grep -c 'connectors' apps/api/internal/http/` -- connector endpoint registered
3. `cd apps/api && go test -race -count=1 ./...` -- all tests pass
4. Commit: `feat: MCP adapter skeleton + chatboting connector event endpoint (Sprint 9, ADR-011)`

## References

- `docs/adr/ADR-011-chatboting-connector-boundary.md` — event contract, boundary design
- `docs/sprint-plan.md` Sprint 9 — MCP skeleton
- ADR-009 §4.1 — service token spec
- mark3labs/mcp-go — Go MCP SDK
- `apps/mcp/cmd/lecrm-mcp/` — existing scaffold (currently .gitkeep only)
