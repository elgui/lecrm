---
title: G3 Schedule Gate — Metadata-Engine Scope Verification Runbook
status: prep complete; verification blocked until end-of-Wk-6 timing AND ADR-010 DOR satisfied
tasket: 20260514-114245-d3a8 (G3 schedule gate)
adr-source: docs/adr/ADR-009-stack-and-license.md §9 G3
adr-target: docs/adr/ADR-010-metadata-engine.md (does not exist yet; authored by tasket 20260514-114217-3c84 at Sprint 4 / Wk 4)
created: 2026-05-14
target-firing-window: end of Sprint 6 / Wk 6 (~2026-06-23, contingent on Wk-1 baseline 2026-05-12)
NOT-before: Sprint 6 close. Premature firing falsifies the gate's evidence basis (incomplete picture per the tasket body).
NOT-after: Sprint 7 open. Late firing flips the gate from "scope-check" to "sunk-cost rationalisation".
---

# G3 — Metadata-Engine Scope Verification Runbook

This document is the **fire-at-Wk-6 kit** for the G3 schedule gate per ADR-009 §9 G3. The verification itself is a single-session task at end of Sprint 6; this runbook codifies the methodology so the session is mechanical, evidence-based, and immune to sprint-pressure motivated reasoning in either direction (premature GREEN to keep velocity, premature RED to escape difficulty).

## 1. What this gate does

Per [ADR-009 §9](../adr/ADR-009-stack-and-license.md) G3:

> **Wk 6 metadata-engine scope gate (binding).** ADR-010 (custom-object metadata engine) authored by **end of week 5**, not week 7. If the per-tenant DDL pattern hits a complexity ceiling by week 6 (signal: cumulative metadata-engine work > 5 days), **fall back to JSONB `data` column on a generic `objects` table per workspace schema**. Faster, less elegant, acceptable for v1 scale (3-15 users × ≤30 custom objects per workspace).

The PRD Exec Summary failure-mode framing (Winston, round 2):

> "JSONB fallback ships and is load-bearing through v1; v2 then either accepts JSONB permanently or budgets a dedicated migration epic — not a sprint, given live tenant data backfill plus read-path rewrite."

**The gate exists because the schedule cannot be held hostage to an implementation that's growing under pressure.** The fallback is built in so we can pull the lever cleanly rather than pushing through a path that's quietly becoming non-viable.

## 2. DOR for firing this gate

All three must be true at the moment the gate fires. If any is false, do not run the verification — the gate's evidence basis is incomplete and the result will be motivated reasoning, not measurement.

| Prerequisite | Source of truth |
|---|---|
| **ADR-010 committed** with a binary decision (DDL-primary OR JSONB-primary) | `git log -- docs/adr/ADR-010-metadata-engine.md` — must show a commit before Sprint 4 close per the sprint plan, but at minimum before this verification fires. Tasket `20260514-114217-3c84` marks the commit. |
| **Metadata-engine implementation underway** since ADR-010 commit | Commits touching `apps/api/internal/metadata/` (Go branch) or `apps/api/src/metadata/` (TS branch — should not exist post-G1) since the ADR-010 commit date. At least one feature surface (custom-property CRUD endpoint, migration generator, JSONB typed-access helper) must exist. |
| **End of Sprint 6 / Wk 6** | Sprint plan Monday-cadence puts Sprint 6 close around 2026-06-19 to 2026-06-23 depending on Wk-1 baseline. Do not fire before the Friday of Sprint 6. Do not fire later than the Monday of Sprint 7. |

## 3. Day-counting methodology (the load-bearing part)

The threshold is "cumulative effort ≤5 days". Honest measurement is the single most important property of this gate, because sprint-pressure incentivises the future self to round down. Use **all three** of the following sources and take the maximum:

### 3.1 Git-based count

```bash
# All commits touching the metadata-engine surface since ADR-010 commit
git log --since="<ADR-010-commit-date>" --pretty=format:"%h %ad %s" --date=short -- \
    apps/api/internal/metadata/ \
    packages/db/migrations/ \
    packages/db/queries/ \
    | grep -iE "metadata|custom[_-]propert|object[_-](schema|type)|jsonb"
```

Count **calendar days** that contain any such commit (not commit count, not hours). One commit on a day = 1 day. Multiple commits on one day = 1 day. Use this as the lower bound.

### 3.2 Session-transcript count

Search Claude Code session transcripts (in `~/.claude/projects/-home-gui-Projects-leCRM/`) for sessions that worked on metadata-engine code:

```bash
# Quick heuristic
grep -lriE "metadata.engine|custom.propert|ADR-010|JSONB column" \
    ~/.claude/projects/-home-gui-Projects-leCRM/ 2>/dev/null | sort -u
```

For each matching session file, check the session's start timestamp and count the calendar day. Include sessions that produced no commits (dead ends, exploratory rewrites, abandoned branches — the tasket body explicitly says these count).

### 3.3 Tasket-body count

Inspect any taskets in the `lecrm-v0-*` groups whose body or commits relate to metadata engine. Count their `created` → `done`/`later` span in calendar days. If a tasket sat at `wip` for three days and only one day produced output, still count three.

### 3.4 Final figure

Take **max(3.1, 3.2, 3.3)** as the cumulative-effort count. Write all three numbers into the ADR-010 addendum even when they agree. The honest number is the max, and the spread is signal.

## 4. Threshold check

Two questions, both must be yes for GREEN:

1. **Is cumulative effort to date ≤5 days?** (per §3 above)
2. **Is the remaining work to reach a workable v0 metadata surface ≤2 days?**

"Workable v0 metadata surface" means: custom-property create / list / read / update / delete works against at least one entity (Contact or Deal), the API shape is stable, and the tests in the test-strategy doc (tasket `20260514-114210-9b41` category (c)) are passing.

If either question is no, the result is RED.

## 5. Decision tree

### 5.1 GREEN — continue with the chosen pattern

1. Append a **Verification Outcome (Wk 6, YYYY-MM-DD)** section to `docs/adr/ADR-010-metadata-engine.md` using the template in §6 below. State: cumulative days, projected completion date, remaining-work checklist.
2. Cross-reference this tasket: "G3 verification per tasket `20260514-114245-d3a8` — GREEN."
3. Mark tasket `20260514-114245-d3a8` `done`.
4. No downstream taskets change shape.
5. Update `docs/sprint-plan.md` Sprint 6 row: "G3: GREEN — continued with <DDL|JSONB>-primary; cumulative <N>d, remaining <M>d."

### 5.2 RED — execute the JSONB fallback

The fallback is **specific** per ADR-009 §9 G3: JSONB `data` column on a generic `objects` table **per workspace schema**. Not "JSONB on tenant.contacts.custom_fields" — a separate generic table. This matters because the fallback's whole point is "no per-tenant DDL ambition, no migrations on custom-property creation".

#### 5.2.1 If ADR-010 chose DDL-primary

Schema shape (per workspace schema, where `lecrm_<workspace_uuid>` is the workspace schema name):

```sql
CREATE TABLE lecrm_<workspace_uuid>.objects (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    object_type  text NOT NULL,         -- e.g., 'custom_lead_source', 'custom_industry_tag'
    parent_type  text NULL,             -- 'contact' | 'deal' | null
    parent_id    uuid NULL,             -- FK to contact/deal in the same schema
    data         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX objects_type_parent_idx
    ON lecrm_<workspace_uuid>.objects (object_type, parent_type, parent_id);
CREATE INDEX objects_data_gin_idx
    ON lecrm_<workspace_uuid>.objects USING gin (data);
```

Migration generator (the thing that was hurting under DDL-primary) becomes a no-op. Custom-property creation becomes an INSERT into the workspace's `objects` table (no DDL, no migration, no schema-cache invalidation).

Typed access helpers in `apps/api/internal/metadata/` should provide:

- `Get(ctx, objectType, parentType, parentID) ([]Object, error)`
- `Set(ctx, objectType, parentType, parentID, data map[string]any) error`
- `Find(ctx, objectType, jsonbQuery JSONBQuery) ([]Object, error)` — JSONB path predicates

#### 5.2.2 If ADR-010 chose JSONB-primary and IT is the one bleeding scope

The fallback is **even simpler**: same shape as 5.2.1 but with all per-tenant ambition stripped (e.g., if the JSONB-primary design tried to keep a registered-properties metadata table, drop it; if it had typed views per object type, drop those too). Pure unstructured `data jsonb` with typed-access helpers. Document this explicitly in the ADR-010 addendum: "JSONB-primary as authored was over-engineered; reduced to unstructured per workspace schema."

#### 5.2.3 ADR-010 addendum (required, verbatim paragraph)

Append the failure-mode paragraph verbatim. Even if it duplicates the existing ADR-010 prose, it must appear in the addendum context so the next reader doesn't have to scroll back up:

> **JSONB fallback ships and is load-bearing through v1; v2 then either accepts JSONB permanently or budgets a dedicated migration epic — not a sprint, given live tenant data backfill plus read-path rewrite.**

#### 5.2.4 Downstream taskets that must update

- **Tasket `20260514-114210-9b41` (test-strategy)** — non-negotiable category (c) "JSONB regression coverage" stops being conditional and becomes load-bearing. The Go-branch test pack (`apps/api/internal/metadata/*_test.go`) must include: round-trip persistence of all JSONB-stored property types, GIN-index-backed query correctness, concurrent-write isolation per workspace.
- **Tasket `20260514-114231-8a67` (integrator-handoff-capabilities)** — methodology config storage shape collapses to JSONB on the same `objects` table (or a sibling). Update its body to describe the JSONB shape, not the DDL shape.
- **Sprint plan `docs/sprint-plan.md`** — Sprint 6 row gets "G3: RED — JSONB fallback executed. Cumulative <N>d, fallback work <M>d." Sprint 7-10 rows of the active branch (Go or TS) lose any DDL-specific implementation detail. Sprint 13 "Pre-deploy" gets a JSONB-regression-suite verification line.

#### 5.2.5 Fallback-execution tasket

If the fallback work is non-trivial (≥1 day), spawn a child tasket via the **Tasket skill** (not direct markdown write — see [[feedback_tasket_skill_required]]) titled "G3 RED — execute JSONB fallback on objects table per workspace schema". Set parent: `20260514-114245-d3a8`, priority p0, category engineering. Otherwise, complete the fallback in this same session and document under §5 of the addendum.

#### 5.2.6 Mark G3 tasket done

Mark tasket `20260514-114245-d3a8` `done` (regardless of GREEN or RED — the decision IS the work). Add `done: YYYY-MM-DD`. Add `outcome: RED-fallback-executed` (or `GREEN-continued`) to the frontmatter.

## 6. ADR-010 addendum template

Append below the existing ADR-010 body, as a new top-level section:

```markdown
---

## Verification Outcome — G3 Schedule Gate (Wk 6, YYYY-MM-DD)

**Verified by:** Guillaume (or session ID)
**Date:** YYYY-MM-DD
**Tasket:** `20260514-114245-d3a8`
**Outcome:** GREEN — continue with <DDL|JSONB>-primary | RED — JSONB fallback executed

### Day-counting evidence (per runbook §3)

| Source | Day count |
|---|---|
| Git-based (3.1) | <N> |
| Session-transcript (3.2) | <N> |
| Tasket-body (3.3) | <N> |
| **Final (max)** | **<N>** |

Spread between sources: <reasoning if non-zero>.

### Remaining work to workable v0 metadata surface

- [ ] <item 1>
- [ ] <item 2>

Estimated remaining: <M> days.

### Decision rationale

<One paragraph. State the threshold check result (≤5d cumulative AND ≤2d remaining) and the decision branch taken. If RED, name which §5.2.x sub-branch (5.2.1 DDL-primary → fallback / 5.2.2 JSONB-primary already over-engineered → simplify).>

### Downstream tasket updates applied

- [ ] `20260514-114210-9b41` (test-strategy) — category (c) status change
- [ ] `20260514-114231-8a67` (integrator-handoff) — methodology config shape
- [ ] `docs/sprint-plan.md` — Sprint 6 row outcome + downstream sprints reconciled
- [ ] Sprint 13 pre-deploy row — JSONB-regression verification (if RED)

### Verbatim load-bearing paragraph (required if RED, or if ADR-010 was JSONB-primary)

> JSONB fallback ships and is load-bearing through v1; v2 then either accepts JSONB permanently or budgets a dedicated migration epic — not a sprint, given live tenant data backfill plus read-path rewrite.
```

## 7. What this runbook does NOT do

Explicitly out of scope (preserving the gate's evidence basis):

- It does **not** decide the outcome in advance. The outcome is whatever §3-4 produce at end of Wk 6.
- It does **not** pre-bias toward GREEN ("we've sunk too much to fall back") or RED ("DDL was always too ambitious"). The threshold is the threshold.
- It does **not** substitute for ADR-010 itself. ADR-010 sets the pattern; this runbook verifies the pattern's scope at Wk 6.

## 8. Cross-references

- [ADR-009 §9](../adr/ADR-009-stack-and-license.md) — G3 gate definition (source of authority)
- [ADR-010](../adr/ADR-010-metadata-engine.md) — the pattern this gate verifies (does not exist yet; authored Sprint 4 / Wk 4)
- Tasket `20260514-114217-3c84` — ADR-010 authoring tasket
- Tasket `20260514-114210-9b41` — test strategy (downstream consequence of RED branch)
- Tasket `20260514-114231-8a67` — integrator-handoff (downstream consequence of RED branch)
- Tasket `20260514-114203-7e2f` — sprint plan (Sprint 6 row gets the outcome)
- [docs/sprint-plan.md](../sprint-plan.md) §"Sprint 6 (Wk 6) — G3 verify + G4 submission + CRUD continues"
