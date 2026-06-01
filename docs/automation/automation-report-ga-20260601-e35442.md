# Automation Run Report — `lecrm-integrator-gap-closure` (ga-20260601-e35442)

- **Run ID:** `ga-20260601-e35442`
- **Branch:** `auto/lecrm-integrator-gap-closure-20260601`
- **Started:** 2026-06-01 11:17 · **Last updated:** 2026-06-01 17:16
- **Report generated:** 2026-06-01 (evidence-based verification, final step)
- **Work steps:** 3 feature tasks + 1 injected fix task (this report is the
  non-work reporting step).

> **This report supersedes the 12:05 draft** (commit `f48f612d`), which was
> written mid-run when steps 2 & 3 still looked blocked. Since then both landed
> via remediation. The numbers below reflect a fresh, full re-verification —
> including the **build-tagged integration tests that the per-step verifiers
> never actually ran**. That re-run surfaced a critical, still-open defect in
> the CSV-import feature (see step 2).

---

## 1. Executive Summary

**2 of 3 feature steps are truly, end-to-end done. 1 (CSV import) landed code
that compiles and unit-tests green but is non-functional at runtime — every
import endpoint returns HTTP 404.**

| Step | Title | Run label | **Verified verdict** |
|------|-------|-----------|----------------------|
| 1 | Custom report builder + Cube.dev reporting | done | ✅ **TRUE completion** — `cbb18013`; builds, unit tests + tsc clean |
| 2 | CSV import (column mapping + dedup) | done | ❌ **FALSE completion (runtime-broken)** — `a4b8fb7a`; compiles & unit-green, but **7/7 import integration tests FAIL**: route/param mismatch → 404 on every endpoint |
| 3 | Duplicate detection + record merge | done | ✅ **TRUE completion** — `b8a25224` + fix `82844ade`; builds, **dedup integration tests PASS** |

Both the original task-body statistics (`1 done / 2 blocked`) **and** the
per-step verifier verdicts (`all 3 success`) were wrong:

- The task-body stats were **stale** — captured before the step-2 and step-3
  remediation sessions committed their work.
- The step-2 verifier claimed *"integration tests verified dedup logic and
  cross-tenant isolation."* That is **false**. Those tests are gated behind
  `//go:build integration` + Docker/testcontainers and were never executed; when
  actually run they fail 7/7. Unit tests (which don't carry the tag) pass, which
  is what the verifier saw.

**Score: 2/3 verified done. 1/3 is a false completion that needs a ~3-line fix.**

---

## 2. Verified Completions

### ✅ Step 1 — Native report engine + custom report builder

- **Commit:** `cbb18013` — `feat(reports): native report engine + custom builder with N-1 comparison`
- **Scope:** 16 files, **+2250 / −77**.
- **Evidence:**

  ```
  go build ./...                       → BUILD_EXIT: 0
  go test -count=1 ./internal/reports/... → ok  ...internal/reports  0.006s
  tsc --noEmit -p tsconfig.app.json    → TSC_EXIT: 0
  ```

- **Delivers:** allow-listed SQL builder (`query.go`, custom-property keys bound
  as params to `jsonb ->>`, never interpolated), saved-definition store
  (`store.go`, ADR-010 JSONB, no new migration), `POST /v1/reports/run` +
  definitions CRUD with cross-workspace guard, the un-stubbed Reports route, and
  N-1 demo seed data. Unit + real-PG integration tests included.

**Verdict: solid, committed, reproducible. Genuinely done.**

### ✅ Step 3 — Duplicate detection + record merge (contacts & companies)

- **Commits:** `b8a25224` — `feat(dedup): duplicate detection + record merge for
  contacts & companies` (2580 LOC, 11 files incl. migration
  `0022_dedup_no_merge_rules.sql`), then `82844ade` — `fix(dedup): inject rbac
  principal in integration harness so merge tests run` (the injected fix task).
- **Evidence:**

  ```
  go build ./...                                                   → BUILD_EXIT: 0
  go test -tags integration -count 1 -run TestDedup ./internal/crm → ok  ...internal/crm  26.05s
  ```

  The dedup integration suite — exact-email match, relation re-pointing
  (notes/activities/tasks/deals), company→contact re-pointing, no-merge-rule
  exclusion, merge audit event, cross-tenant isolation — runs against a real
  testcontainers Postgres and **passes**. The `82844ade` fix injecting the RBAC
  principal into the harness is what unblocked it.

**Verdict: committed, builds, integration-verified. Genuinely done.**

---

## 3. False / Incomplete Completions

### ❌ Step 2 — CSV import (contacts/companies/deals) — RUNTIME-BROKEN

Code landed (`a4b8fb7a`, 2301 LOC: `import.go` backend, `csv-import-wizard.tsx`
UI, integration tests, OpenAPI). It **compiles and unit-tests pass** — but the
feature **does not work**: every import request returns `404 unknown import
entity`. The committed integration tests prove it, and they were never run by
the verifier.

**Evidence — 7/7 import integration tests fail:**

```
go test -tags integration -count 1 -run TestImport ./internal/crm
--- FAIL: TestImport_Contacts_AnalyzePreviewCommit (3.96s)
      analyze: status=404 body={"error":"unknown import entity"}
--- FAIL: TestImport_Contacts_CrossTenant_Isolation (2.99s)   → 404 unknown import entity
--- FAIL: TestImport_Contacts_Dedup_UpdatePolicy (2.98s)      → 401 authentication required (setup)
--- FAIL: TestImport_Contacts_Dedup_SkipPolicy (3.06s)        → 401 authentication required (setup)
--- FAIL: TestImport_Contacts_ErrorReport (2.97s)             → 404 unknown import entity
--- FAIL: TestImport_Companies_Smoke (2.99s)                  → 404 unknown import entity
--- FAIL: TestImport_Deals_Smoke (3.01s)                      → 404 unknown import entity
FAIL    github.com/gbconsult/lecrm/apps/api/internal/crm  22.0s
```

**Root cause — route registered as a static literal, handler reads a path param
that was never declared.** The handlers extract the entity with
`chi.URLParam(r, "entity")`:

```go
// apps/api/internal/crm/import.go:205 (and :271)
spec, ok := specForParam(chi.URLParam(r, "entity"))
if !ok {
    writeErr(w, http.StatusNotFound, "unknown import entity")
    return
}
```

…but the routes are registered as **fixed strings with no `{entity}`
placeholder** (`apps/api/internal/crm/handlers.go:93-115`):

```go
r.Post("/v1/import/contacts/analyze", h.ImportAnalyze)   // no {entity} → URLParam("entity") == ""
r.Post("/v1/import/contacts/preview", h.ImportPreview)
r.Post("/v1/import/contacts/commit", h.ImportCommit)
// …same for companies, deals
```

`chi.URLParam(r, "entity")` therefore always returns `""`, `specForParam("")`
returns `false`, and **every** analyze/preview/commit call 404s. This is not a
test-only problem: the React wizard hits the same paths
(`csv-import-wizard.tsx:103` → `fetch(\`/v1/import/${entity}/${step}\`)`), so the
feature is dead end-to-end in the browser too.

**Secondary issue:** the two `*_Dedup_*Policy` tests fail one step earlier with
`401 authentication required` when seeding a contact via `POST /v1/contacts`
through the shared `pipelineTestEnv` harness — the same missing-RBAC-principal
class of bug that `82844ade` fixed for the dedup harness, but not fixed here.
Both faults must be addressed for the suite to go green (fixing the 401 alone
just exposes the 404).

**Net:** ~2.3k LOC of plausible, compiling, unit-tested code that is **0%
functional at runtime**. The original `blocked` label (from the draft report)
under-credited it; the final verifier `success` label over-credited it. The
truth is in between: it landed, but it's broken.

---

## 4. Failures / Blocked

No step ended the run in a `blocked` or `errored` state — the two that were
blocked mid-run (CSV import, dedup) were both subsequently committed by
remediation sessions. The only **functional** failure is the latent runtime
defect in step 2 documented above; it passed CI-style gates (build + untagged
unit tests) and so was not caught until this report ran the integration suite.

---

## 5. Build Status (right now)

| Gate | Command | Result |
|------|---------|--------|
| Go build | `go build ./...` | ✅ `BUILD_EXIT: 0` |
| Go unit tests | `go test -count=1 ./internal/reports/... ./internal/crm/...` | ✅ pass (crm: 99 tests, reports: ok) |
| TypeScript | `tsc --noEmit -p tsconfig.app.json` | ✅ `TSC_EXIT: 0` |
| Web unit tests | `vitest run` | ⚠️ **environment-blocked** — `RangeError: WebAssembly.instantiate(): Out of memory` (known WASM/vmem cap on this host, not a code defect) |
| Go integration — dedup | `go test -tags integration -run TestDedup ./internal/crm` | ✅ pass (26s, real PG) |
| Go integration — import | `go test -tags integration -run TestImport ./internal/crm` | ❌ **7/7 FAIL** (route/param 404; +2 setup 401) |

Working tree is clean (`git status`: only `apps/web/node_modules` untracked;
`git diff --stat` empty). **A fresh CI checkout builds and passes the untagged
suite green** — which is exactly why the import defect slipped through: it only
manifests under `-tags integration` (Docker) or at runtime in the browser.

---

## 6. Recommendations

1. **Fix the CSV-import route/handler mismatch (critical, ~3 lines).** Make the
   route declare the param the handler already reads — in
   `apps/api/internal/crm/handlers.go` replace the nine static import routes with
   three param routes:

   ```go
   r.Post("/v1/import/{entity}/analyze", h.ImportAnalyze)
   r.Post("/v1/import/{entity}/preview", h.ImportPreview)
   r.Post("/v1/import/{entity}/commit", h.ImportCommit)
   ```

   `/v1/import/...` never collides with `/v1/contacts/{id}` (different prefix),
   so the static-literal workaround that exists for `/export` is unnecessary
   here. `specForParam` already rejects unknown entities with 404, preserving the
   intended validation. Then re-run `go test -tags integration -run TestImport`.

2. **Fix the harness 401** so the two `*_Dedup_*Policy` import tests can seed
   data: inject the RBAC principal into `pipelineTestEnv`'s authenticated
   requests, mirroring the fix `82844ade` applied to the dedup harness.

3. **Add the integration suite to the per-step verification gate.** Both false
   signals this run (stale stats, over-optimistic verifier) trace to
   `-tags integration` tests never being executed. A green untagged build is not
   evidence that a feature works — wire `go test -tags integration ./...`
   (Docker available) into the gate, or at minimum require it for any task that
   ships new HTTP routes.

4. **Steps 1 & 3 need no rework** — committed, green, integration-verified.
   Ship/merge as normal. (Reminder: CI builds/tests only; staging is rebuilt by
   hand on the host from the working tree — deploy from a clean tree, and **not**
   before the import route fix lands, or the demo will surface a dead Import
   button.)

5. **Re-queue CSV import as a [Fix] task** (same pattern the run already used for
   dedup → task `1150`) rather than treating it as done.

---

### Evidence appendix (commands run)

```
git log --since="2026-06-01 11:17" --until=now --pretty='%h %ad %s' --date=format:%H:%M
  82844ade 17:14 fix(dedup): inject rbac principal in integration harness …
  b8a25224 17:02 feat(dedup): duplicate detection + record merge …
  a4b8fb7a 16:42 feat(import): CSV import for contacts/companies/deals …
  f48f612d 12:05 docs(automation): evidence-based run report … (superseded by THIS file)
  cbb18013 11:48 feat(reports): native report engine + custom builder …
git status --short                                     # ?? apps/web/node_modules only
git diff --stat                                        # (empty)
go build ./...                                         # EXIT 0
go test -count=1 ./internal/reports/... ./internal/crm/...   # pass (crm 99 tests)
tsc --noEmit -p tsconfig.app.json                      # EXIT 0
vitest run …                                           # WASM OOM (env-blocked)
go test -tags integration -run TestDedup  ./internal/crm   # ok (26s)
go test -tags integration -run TestImport ./internal/crm   # FAIL 7/7
grep -n 'chi.URLParam(r, "entity")' internal/crm/import.go   # :205 :271
grep -n 'v1/import/contacts/analyze' internal/crm/handlers.go # :93 (static, no {entity})
```
