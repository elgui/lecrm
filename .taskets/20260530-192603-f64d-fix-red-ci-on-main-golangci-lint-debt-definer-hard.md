---
id: 20260530-192603-f64d
title: Fix red CI on main — golangci-lint debt + definer_hardening/tombstone integration tests
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
category: tooling
origin: "session: repo housekeeping (automation-report relocation) — surfaced pre-existing red CI"
done: 2026-05-30
---

CI (`.github/workflows/ci.yml`) has been failing on every push to `main` for ~2 weeks. Two independent, pre-existing root causes (out of scope for the housekeeping run that surfaced this; tracked here).

## Job `go-test+lint+sec` — golangci-lint (apps/api), 29 issues
- **errcheck** (~15): unchecked `resp.Body.Close()` / `conn.Close()` in integration tests; `fmt.Fprintf` in `apps/api/capability/writesafety.go:227`; unchecked return in `internal/db/tenant_pool.go:237`.
- **errorlint** (~5): `!=` / type-assertion error comparisons that fail on wrapped errors — `capability/intentops_integration_test.go` (258,388,406) and `internal/metadata/validate_test.go` (285,316). Use `errors.Is` / `errors.As`.
- **revive package-comments** (3): `internal/crm/activity.go`, `internal/domain/crm.go`, `internal/logging/context.go`.
- **staticcheck S1016** (3): struct-literal conversions — `internal/auth/service_token_handler.go:99`, `internal/auth/session_v2.go:67,193`.
- **unused** (3): `contactFromCapResult`, `companyFromCapResult`, `dealFromCapResult` in `internal/crm/handlers.go:345,354,363`.

## Job `build-admin` — go test ./... (apps/admin), internal/tenant pkg
- `TestRegistryRejectsInvalidSlug` (all subcases: uppercase, too-short, starts-with-digit, starts-with-hyphen, too-long, empty, special-chars) + `TestRegistryRejectsInvalidEmail` + `TestRegistryRejectsNullSlug` — `definer_hardening_test.go:86/119/191`: registry ACCEPTS invalid slugs/emails that the tests expect rejected. Investigate whether the validation hardening these tests assert was never merged (test-ahead-of-impl) vs a real regression.
- `TestTombstoneWorkspace` + `TestTombstoneAlreadyTombstoned` — `tombstone_integration_test.go:35,82`: `audit tombstone: ERROR: could not determine data type of parameter $1 (SQLSTATE 42P08)` — untyped query parameter in the tombstone audit SQL (Postgres can't infer the type; add an explicit cast or pass a typed arg).

## How it was found
`gh run view 26692644399 --log-failed` on the merge commit for PR #6. Same failures appear on every recent `main` push.

## Acceptance
- [ ] `golangci-lint run` clean in `apps/api` (fix, or justified `//nolint` with a reason comment).
- [ ] `go test ./...` green in `apps/admin` — decide per-test: ship the slug/email validation the tests expect, or correct the tests; fix the SQLSTATE 42P08 untyped param in the tombstone audit query.
- [ ] CI green on `main`.

Priority: high — `main` has been red for ~2 weeks, masking new regressions.
