---
id: 1148
title: "[Fix] L2: Dashboard — replace redundant nav tiles with one live 'what needs attention'"
status: done
priority: p1
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend, remediation]
category: project
group: lecrm-leo-demo-polish
group_order: 7
order: 7
remediates: 20260531-145435-f381
---

## Remediation outcome

The previous task `20260531-145435-f381` was flagged **partial_success**: the
dashboard rework (drop nav tiles → live "what needs attention" panels, commit
`af77073c`) passed `tsc`/`eslint`, but `vitest` could **not be run to completion**
on the host because the 6 GB `ulimit -v` hard cap makes V8 fail WASM-memory
allocation (`RangeError: WebAssembly.instantiate(): Out of memory`). The
acceptance criterion "vitest green" was therefore only *inferred*. This
remediation actually ran the suite — and that surfaced a real failure the
inference had missed.

**Commit:** `bcc32e15` — *test(web): fix attention.test fixtures clobbering
prefixed ids* (branch `auto/lecrm-leo-demo-polish-20260531`).

### What was found

Run in a memory-adequate `node:24-bookworm` container (escapes the host's 6 GB
`RLIMIT_AS`), the suite reported **5 failing tests** in the new
`apps/web/src/lib/attention.test.ts` — exactly the id-equality assertions:

```
expected [ 'deal-soon' ] to deeply equal [ 'soon' ]   (and 4 similar)
```

Root cause was a **test-fixture bug**, not a production-code bug: the `task()` /
`deal()` helpers set a prefixed `id` (`'task-…'` / `'deal-…'`) and then spread
`...overrides` *afterwards*, so an explicit `id` in the overrides **overwrote the
prefix**, leaving bare ids (`'soon'`) while the assertions expected the prefixed
form (`'deal-soon'`). The production helper `attention.ts` was correct — sort
order and overdue flags already matched; only the fixture-built ids were wrong.

### What was done

Destructure `id` out of `overrides` before the spread in both `task()` and
`deal()` so the prefix survives:

```ts
const { id, ...rest } = overrides;
return { id: 'task-' + (id ?? …), …, ...rest };
```

A 6-line, test-only change — no production code touched.

### Verification

- `vitest run` → **93/93 passed** (16 files), run in `node:24-bookworm`
  container to dodge the host vmem OOM. Before the fix: 5 failed / 88 passed.
- `tsc --noEmit -p tsconfig.app.json` ✓ (host)
- `eslint src/lib/attention.test.ts` ✓ (host)
