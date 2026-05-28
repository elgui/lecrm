---
id: 20260528-142602-10c3
title: v0 ship-gate verification — confirm CRM critical path shipped
status: parked
priority: p1
created: 2026-05-28
updated: 2026-05-28
tags: [v1-readiness, v0-ship-gate, verification]
category: project
group: lecrm-v1-readiness
group_order: 80
order: 2
plan: true
---

## Why

v1 (native sequences engine) assumes a feature-complete v0: idempotency, fail-closed audit, activity timeline, RBAC, OpenAPI spec + service tokens, MCP/connector boundary, single-binary deploy. This tasket is a verification-only gate that confirms each prerequisite has shipped before v1 work is unparked.

**Status `parked` until the upstream taskets close.** Flip to `next` when all checks below pass.

## Upstream dependencies (status as of 2026-05-28)

| Tasket | Group | Required scope | Currently |
|---|---|---|---|
| 1003 | crm-crud-complete | `Idempotency-Key` header + fail-closed audit-log txn | residual (base CRUD shipped in PR#5) |
| 1004 | crm-crud-complete | `activities` + `notes` + `tasks` tables + handlers | not started |
| 1005 | crm-crud-complete | Pipeline Kanban + `PATCH /v1/deals/:id/stage` | next (refreshed for cold execution) |
| 1006 | crm-crud-complete | OpenAPI 3.1 spec + service tokens + contract tests | not started |
| 1007 | crm-frontend-rbac-export | `RequireRole` middleware + member-mgmt endpoints + UI gating | residual (DB foundation shipped in migration 0002) |
| 1008 | crm-frontend-rbac-export | Frontend feature-complete + `go:embed` + CSV export | not started |
| 1009 | crm-frontend-rbac-export | MCP adapter + `POST /v1/connectors/:source/events` | residual (sync seam shipped in sprint-11) |

## Verification block

Run each from repo root. Every assertion MUST pass before flipping this tasket to `done`.

```bash
export PATH=$PATH:/usr/local/go/bin
set -e

# --- 1003 residual: idempotency + audit ---
grep -q "idempotency_keys" packages/db/migrations/*.sql
grep -rq "Idempotency-Key" apps/api/internal/crm/
test -d apps/api/internal/audit

# --- 1004: activities / notes / tasks tables ---
grep -q "CREATE TABLE.*activities" packages/db/migrations/*.sql
grep -q "CREATE TABLE.*notes" packages/db/migrations/*.sql
grep -q "CREATE TABLE.*tasks" packages/db/migrations/*.sql

# --- 1005: stage transition route ---
grep -q "/v1/deals/{id}/stage\|/v1/pipeline/stages" apps/api/internal/crm/handlers.go
grep -q '"@dnd-kit/core"' apps/web/package.json

# --- 1006: OpenAPI + service tokens ---
test -f docs/openapi.yaml -o -f apps/api/openapi.yaml
grep -q "service_tokens" packages/db/migrations/*.sql
test -s packages/shared-types/index.ts -o -d packages/shared-types/src

# --- 1007: RBAC middleware + member-mgmt ---
grep -rq "RequireRole" apps/api/internal/auth/
grep -q "/v1/workspace/members" apps/api/internal/auth/*.go apps/api/internal/admin/*.go 2>/dev/null

# --- 1008: frontend feature-complete + go:embed + CSV ---
grep -rq "go:embed" apps/api/cmd/lecrm-api/
grep -rq "encoding/csv" apps/api/internal/

# --- 1009: MCP binary + connector endpoint ---
test -f apps/mcp/cmd/lecrm-mcp/main.go
grep -q "/v1/connectors" apps/api/internal/sync/*.go apps/api/internal/http/server.go 2>/dev/null

# --- end-to-end build/test sweep ---
(cd apps/api && go build ./... && go test -race -count=1 ./...)
(cd apps/admin && go build ./... && go test ./...)
(cd apps/migrate && go build ./... && go test ./...)
(cd apps/web && bun run typecheck && bun test)
```

## Done when

- Every check in the verification block above exits 0.
- The `lecrm-v0-build` group is fully `done`.
- This tasket's body is updated with an evidence section quoting the commit hashes that closed each upstream item.

## What flipping this tasket to `done` triggers

- Unparks `lecrm-v1-readiness/order:4` (v1 kickoff signal).
- Signals the operator that v0 is shippable to a first paying client.
- Does NOT itself start v1 — see the kickoff signal tasket for the final flip.
