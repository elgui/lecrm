---
id: 1122
title: "[Fix] Contact + Company + Deal REST handlers with CRUD"
status: done
priority: p2
created: 2026-05-28
updated: 2026-05-28
done: 2026-05-28
tags: [crm, handlers, gofmt, remediation]
category: project
group: crm-crud-complete
group_order: 10
order: 1
remediates: 20260525-1003-entity-rest-handlers-crud
---

## Remediation outcome

The previous task `20260525-1003-entity-rest-handlers-crud` was flagged `partial_success` for two reasons:

1. The idempotency + audit-log work for CRM handlers had been written/tested but not committed.
2. `apps/api/internal/crm/handlers_test.go` had a gofmt violation that would have blocked the commit.

Verification confirms (1) was actually committed prior to this session:

- Commit `a9fa5507 feat(api): Idempotency-Key + fail-closed audit on CRM mutations (Sprint 7)` lands the full Sprint 7 scope (migration 0014, `crm/audit.go`, `crm/idempotency.go`, refactored Contact/Company/Deal mutations, unit + integration tests).

This remediation tasket addresses (2) — the residual gofmt drift on `handlers_test.go` (column-aligned comments on the UUID negative-case cases). Applied `gofmt -w` and committed as `7aea2012 style(crm): gofmt handlers_test.go`.

### Build / test sanity

- `go build ./...` — clean (no output)
- `go test ./internal/crm/...` — `ok github.com/gbconsult/lecrm/apps/api/internal/crm 0.019s`
- `gofmt -l apps/api/internal/crm/` — clean (no remaining violations)

Remediates: `20260525-1003-entity-rest-handlers-crud`
