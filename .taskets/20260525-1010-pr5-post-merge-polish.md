---
id: 20260525-1010-pr5-post-merge-polish
title: "PR#5 post-merge polish — remaining low-severity review findings"
status: pending
priority: p3
created: 2026-05-25
category: project
group: crm-entity-foundation
group_order: 50
order: 10
updated: 2026-05-28
plan: false
tags: [code-quality, review-findings, polish]
---

# PR#5 post-merge polish — remaining low-severity review findings

## Context

PR#5 (auto/crm-entity-foundation) was reviewed with `--effort high` and produced 9 findings. Critical (#1-2) and major (#3-4) are fixed. Medium (#5-9) are fixed. Five low-severity items remain — none are bugs, all are code-quality improvements that reduce future risk.

Source: PR#5 code review, 2026-05-25.
Working directory: `/home/gui/Projects/leCRM`

## Items

### 1. deleteRow() uses unqualified table names

`apps/api/internal/crm/handlers.go` — `deleteRow` passes bare table names like `"contacts"` to `tx.Exec`. Safe because `SET LOCAL search_path` scopes the transaction, but inconsistent with the metadata package which schema-qualifies via `pgx.Identifier`.

**Fix:** Switch to sqlc `:execrows` annotation (returns `int64` rows affected), or schema-qualify the table name in `deleteRow` using `pgx.Identifier{schema, table}.Sanitize()`.

### 2. CreateDefinition duplicate detection still uses strings.Contains

`apps/api/internal/metadata/handlers.go` ~line 103 — the `duplicate`/`unique` error branch still matches via `strings.Contains(err.Error(), "duplicate")`. If Postgres changes error message wording, this silently becomes a 500.

**Fix:** Use `pgconn.PgError` with `Code == "23505"` (unique_violation):
```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    writeErr(w, http.StatusConflict, ...)
    return
}
```

### 3. Cursor decoding silently ignores garbage input

`apps/api/internal/crm/handlers.go` — `decodeCursor` returns zero values on error, callers discard the error with `_, _, _ :=`. A garbage `?cursor=xxx` silently shows the first page instead of returning 400.

**Fix:** If `cursor` query param is non-empty and `decodeCursor` fails, return 400 "invalid cursor".

### 4. defCache maxSize=50 thrashes with many workspaces

`apps/api/internal/metadata/cache.go` — cache keyed by `schema:parentType`. With 25+ workspaces × 2 parent types = 50+ keys, cache is always at capacity and every new workspace evicts an active one.

**Fix:** Raise `cacheMaxSize` to 200, or replace hand-rolled LRU with `hashicorp/golang-lru`.

### 5. Company column shows truncated UUID instead of name

`apps/web/src/routes/contacts/index.tsx` — Company column shows `company_id.slice(0,8)…` which is not human-readable.

**Fix:** Add a SQL JOIN in `ListContacts` query to return `company_name`, or add a frontend batch-lookup for company names. JOIN is cleaner and avoids N+1.

## Done When

- [ ] All 5 items addressed or explicitly deferred with rationale
- [ ] `go build ./...` clean
- [ ] `npx tsc --noEmit` clean
- [ ] Existing tests pass
