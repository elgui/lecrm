---
id: 20260510-202450-a5d3
title: "leCRM v0 — Week-2 Go ramp checkpoint (BINDING decision gate: continue Go or switch to TypeScript+Hono)"
status: later
priority: p0
created: 2026-05-10
updated: 2026-05-14
category: engineering
group: lecrm-v0-scaffolding-v2
group_order: 1
order: 1
---

## Read this cold — full context inline

This tasket is a **binding decision gate at end of week 2** of the leCRM v0 build. It is intentionally short and runs as a separate session from the scaffolding work in tasket order 1 (`lecrm-v0-scaffolding`). The outcome is one of two irrevocable paths: **continue in Go** or **switch the entire backend to TypeScript+Hono**.

## Prerequisite

The order-1 tasket (`lecrm-v0-scaffolding` order 1 — housekeeping + scaffolding) must be complete enough that the three litmus tests below are runnable. That means at minimum: `go.work` initialized, `apps/api` module with `sqlc` configured, a local Postgres reachable, `golangci-lint` installed.

If order-1 is not yet at that state, **do not start this tasket** — return to order 1 and complete the scaffolding first.

## Why this exists

[ADR-009](docs/adr/ADR-009-stack-and-license.md) §1 selects Go as the primary backend on weighted criteria (4.57/5.00 vs TypeScript's 3.73). The decisive evidence — Go's $0.50/run vs TypeScript's $0.62/run on the InfoQ April 2026 Claude Code benchmark, less type-friction, single-static-binary deploy story — assumes Guillaume can ramp on Go fast enough that the velocity advantage materialises in the 11-13 week build.

The council's engineer voice flagged this as the single biggest schedule lever: **if the Go ramp is slow in weeks 1-2, the entire velocity advantage inverts** and TypeScript+Hono becomes the conservative call. The dossier and ADR-009 §1.1 bind a week-2 checkpoint with three concrete tests. **This tasket is that checkpoint.**

Calibration: the council's researcher voice (Ava) verified that the InfoQ benchmark task (simplified Git CLI) is *not directly representative* of leCRM's HTTP/multi-tenant CRUD workload — generalisability is suggestive, not proven. So this checkpoint is also where empirical evidence from leCRM-specific work overrides the benchmark.

## The three litmus tests (must ALL pass)

Run each test in `apps/api`, on the local scaffolded Postgres provisioned in order 1. Time each test honestly (use `time` or a stopwatch). Document blockers as you hit them.

### Test 1 — `sqlc`-typed query through an HTTP handler

Build a single `/v1/_test/workspaces` GET handler that:

1. Acquires a Postgres connection from a pool.
2. Issues `SELECT id, slug, created_at FROM core.workspaces LIMIT 10` via an `sqlc`-generated method.
3. Marshals the result to JSON and returns it.

Pass criteria: handler returns valid JSON; `sqlc` generation cleanly produces types from the SQL; Claude Code did not need to be corrected on Go context or pool acquisition idiom.

**Time budget: 90 minutes.** If blocked > 4 working hours despite Claude Code assistance, this test FAILS.

### Test 2 — Workspace-scoping middleware with idiomatic context propagation

Build a middleware that:

1. Reads the subdomain from `r.Host`.
2. Looks up the corresponding `workspace_id` in `core.workspaces` (cached or direct, doesn't matter).
3. Stores a typed `WorkspaceContext { ID uuid.UUID; Slug string; RoleName string }` in `context.WithValue(...)`.
4. Downstream handlers retrieve it via a typed getter `WorkspaceFromContext(ctx) (*WorkspaceContext, error)`.
5. The handler from Test 1 uses this middleware and refuses to run if no workspace is in context.

Pass criteria: middleware compiles and runs; the `context.WithValue` key uses an unexported type per Go idiom (not a `string` key); the typed getter pattern is used; Claude Code generated the right idiom on first or second prompt.

**Time budget: 90 minutes.** If blocked > 4 working hours, this test FAILS.

### Test 3 — Lint and vet clean

Run on the scaffolding:

```bash
golangci-lint run ./...
go vet ./...
```

Pass criteria: zero warnings, zero errors. No `//nolint:` exceptions added to suppress noise.

**Time budget: 30 minutes** (if Tests 1 and 2 produced clean code; longer otherwise — but if you need > 4 hours to make a freshly-scaffolded codebase lint-clean, that itself is the signal).

## The decision (binding, end of week 2)

| Outcome | Decision |
|---|---|
| All three tests PASS within budget | **CONTINUE in Go.** Proceed to ADR-010 (metadata engine, week 5) and the contacts/companies/deals CRUD work. Record outcome on this tasket. |
| Any test BLOCKED > 4 working hours | **SWITCH to TypeScript + Hono** (Hono + Drizzle + Atlas + `openid-client` v6). The decision is irrevocable; do not relitigate at week 5. Re-scope all downstream taskets against the new stack. |

The 4-hour rule is per-test, not cumulative. The point is to detect "fighting the language" early — not to budget total scaffolding time.

## What to record

On completion of this tasket, append an **Architecture Decision Note** to ADR-009 (or as a closure comment if Tasket supports it) with this shape:

```
## Week-2 Go ramp checkpoint outcome (YYYY-MM-DD)

- Test 1 (sqlc handler): PASS/FAIL — N minutes elapsed — notes
- Test 2 (workspace middleware): PASS/FAIL — N minutes elapsed — notes
- Test 3 (lint clean): PASS/FAIL — N minutes elapsed — notes
- Decision: CONTINUE Go / SWITCH to TypeScript+Hono
- Decided by: Guillaume
- Decided on: YYYY-MM-DD
- Notes: <any qualitative observations on Claude Code's Go fluency vs prior projects>
```

If SWITCH is decided, immediately create a follow-up tasket: "Re-scope `lecrm-v0-scaffolding` and all downstream v0/v1 taskets against TypeScript+Hono stack."

## Acceptance criteria

- [ ] All three litmus tests run and outcomes recorded with elapsed time.
- [ ] Decision (CONTINUE Go / SWITCH to TS+Hono) recorded as an Architecture Decision Note appended to ADR-009.
- [ ] If SWITCH: follow-up re-scoping tasket created in the appropriate group (likely `lecrm-v0-scaffolding-ts` or `lecrm-v0-rebuild`).
- [ ] If CONTINUE: tasket marked `status: done` and the next tasket (ADR-010 metadata engine planning, target week 5) is queued.

## Out of scope for this tasket

- Custom-object metadata engine (ADR-010, week 5).
- Email integration (week 11-12; Gmail OAuth review starts week 5-6).
- MCP adapter wire-format implementation (week 13).
- Any deployment or production work.

## Notes for the executor

Honesty here is the highest-leverage thing you do this quarter. The council was explicit: **P50 build schedule is achievable, P80 is not**. The Go choice is calibrated on benchmark evidence that does not directly map to leCRM's workload. If after two weeks of real CRUD work the language is fighting you, the right call is to switch — not to push through. The runner-up (TypeScript+Hono) is fully specified in ADR-009 and the dossier; the switch costs ~3-5 days of re-scaffolding and saves potentially weeks of compounded velocity loss.

Conversely, if Go works as advertised, do not relitigate. The decision is binding at end of week 2 either way — leaving this question open past week 2 is the worst outcome.
