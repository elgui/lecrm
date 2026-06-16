---
id: 1168
title: [Fix] Sequence state machine + Transition() + in-txn audit emission
status: done
priority: p0
created: 2026-06-15
updated: 2026-06-15
done: 2026-06-15
category: tooling
group: lecrm-v1-build
order: 4
plan: true
---

# Remediate Workflow

You are executing a remediation task injected by the automation system.
A previous task failed, and you must fix the issue it left behind.

## Failure Context

The previous task #20260614-154815-ff66 ("Sequence state machine + Transition() + in-txn audit emission") failed.

### Original Error

Task did not complete successfully.

### What to Fix

The master completed the research and design phase but did not write implementation code. No migration files, no transition.go changes, no tests were created. Only go.work.sum changed (dependency checksum). Implement following the approved SECURITY DEFINER pattern: (1) Migration 0026 creating core.lecrm_emit_audit(...) SECURITY DEFINER function with session_user guard, granted PUBLIC; (2) transition.go Transition(ctx, tx, enrollmentID, to, reason, ...opts) with SELECT...FOR UPDATE, validation, state update, in-tx audit emission via the SDF, and river job enqueue; (3) unit tests (valid paths, invalid panic, mock worker role) + integration test (real workspace-role tx, rollback atomicity, prod 500 audit).

## Instructions

1. **Analyze** — Read the error context and understand what went wrong
2. **Locate** — Find the relevant files or state that needs fixing
3. **Fix** — Implement the minimum change needed to resolve the issue
4. **Verify** — Confirm the fix works (run tests, check output, etc.)

## Rules

- Focus ONLY on the described fix — do not refactor or improve unrelated code
- If the fix requires something only a human can provide (API key, credentials, access), note it clearly and mark the task done with a comment explaining what's needed
- Make the smallest possible change that resolves the issue
- If you cannot determine the fix, document what you found and what you tried

## Common GA Anti-Patterns (check if the failure involves any of these)

If the remediation description mentions any of these patterns, apply the specific fix:
- **Hardcoded credentials as defaults** → Replace with empty string or env var lookup, never a real key
- **SQL string interpolation** → Convert to parameterized queries ($1, ?) or use a safe column whitelist
- **Python `bool(string)` coercion** → Replace `bool(value)` with `value.lower() in ("true", "yes", "1")`
- **Race condition / lazy init** → Move dict/list initialization to `__init__` or setup method, add `FOR UPDATE` to SELECT-then-UPDATE patterns
- **Duplicated code** → Import from the existing module instead of copying; remove the duplicate
- **`console.log` in production** → Replace with framework structured logger
- **Hardcoded user paths** → Use relative paths, env vars, or server-provided values
