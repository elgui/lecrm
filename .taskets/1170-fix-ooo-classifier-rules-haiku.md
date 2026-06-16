---
id: 1170
title: [Fix] OOO classifier (rules + Haiku)
status: done
priority: p0
created: 2026-06-15
updated: 2026-06-15
done: 2026-06-15
category: tooling
group: lecrm-v1-build
order: 7
plan: true
---

# Remediate Workflow

You are executing a remediation task injected by the automation system.
A previous task failed, and you must fix the issue it left behind.

## Failure Context

The previous task #20260614-154815-a81e ("OOO classifier (rules + Haiku)") failed.

### Original Error

Task did not complete successfully.

### What to Fix

A follow-up Claude Code session should run the full test suite on the new classifier code (`go test ./apps/api/internal/sequences/ooo...`), verify the build succeeds, inspect test results to confirm the fixture reaches the 95% precision target, and commit all untracked files with a message referencing ADR-004 rev 2 §5.

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
