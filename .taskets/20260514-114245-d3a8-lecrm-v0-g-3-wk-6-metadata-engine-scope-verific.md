---
id: 20260514-114245-d3a8
title: "leCRM v0 — G3: Wk 6 metadata-engine scope verification gate"
status: later
priority: p0
created: 2026-05-14
updated: 2026-05-14
category: engineering
group: lecrm-v0-scaffolding-v2
group_order: 1
order: 3
---

## Read this cold — full context inline

The G3 schedule gate per ADR-009 §9: at end of Wk 6, verify the implementation scope of the chosen metadata-engine pattern (set by ADR-010 — see tasket `20260514-114217-3c84`) is actually ≤5 days cumulative. If exceeded, execute the documented fallback (JSONB on generic `objects` table per workspace schema). The verification IS the decision — this tasket must NOT be skipped under sprint pressure, and it must NOT be executed earlier (premature) or later (sunk-cost bias takes over).

## Why this exists

From ADR-009 §9 G3 + PRD Exec Summary failure-mode paragraph (Winston, round 1 + round 2):

> "JSONB fallback ships and is load-bearing through v1; v2 then either accepts JSONB permanently or budgets a dedicated migration epic — not a sprint, given live tenant data backfill plus read-path rewrite."

The honest read: if the primary path (DDL or JSONB — set by ADR-010) is bleeding scope past 5 days, the chosen path is wrong for this v0 schedule. The fallback is built into ADR-009 precisely so the schedule isn't held hostage to an implementation that's growing under pressure.

## Prerequisite (DOR)

- **ADR-010 committed** — tasket `20260514-114217-3c84` is `done`. The pattern is named (DDL-primary or JSONB-primary).
- Metadata-engine implementation work has been underway since ADR-010 commit (around Wk 4-5).
- **End-of-Wk-6 timing.** DO NOT execute earlier (incomplete picture) or later (sunk-cost bias).

## Approach

1. **Inventory work done.** Count actual days of effort against the metadata-engine implementation since ADR-010 commit. Be honest — partial days count, exploratory rewrites count, dead-end branches count.

2. **Threshold check.** Is cumulative effort ≤5 days? Are remaining tasks small (≤2 days) to reach a workable v0 metadata surface?

3. **Decision branch:**
   - **GREEN: Continue.** Document the verification result + projected completion. Continue with the chosen pattern.
   - **RED: Execute fallback.** Per ADR-009 §9 G3: fall back to JSONB on a generic `objects` table per workspace schema. If ADR-010 already chose JSONB and IT is the one bleeding scope, the fallback is even simpler JSONB (no per-tenant DDL ambition, no migrations on custom-property creation — just an unstructured `data jsonb` column with typed-access helpers).

4. **Update ADR-010** with the verification outcome appended as an addendum (date + outcome + projected reconciliation, including an explicit cross-reference to this tasket).

5. **Update downstream taskets** — if fallback executed: the `test-strategy-scope-doc` non-negotiable category (c) becomes more important (JSONB regression coverage); subsequent custom-property feature taskets adjust to the simpler shape; the `integrator-handoff-capabilities` methodology config storage shape may need adjustment.

## Done When

- [ ] Verification documented (in ADR-010 addendum or a sibling doc)
- [ ] Decision logged: GREEN (continue) OR RED (fallback executed)
- [ ] If RED: fallback implementation tasket created (or this tasket extended into a fallback-execution session)
- [ ] Cross-link to G2 (ADR-010 source) preserved
- [ ] Sprint plan (tasket `20260514-114203-7e2f`) updated for the post-decision sprints

## References

- `docs/adr/ADR-009-stack-and-license.md` §9 G3
- Tasket `20260514-114217-3c84` — ADR-010 metadata-engine commitment (the pattern this gate verifies)
- Tasket `20260514-114210-9b41` — test strategy doc (downstream consequence of GREEN/RED branch)
- Tasket `20260514-114203-7e2f` — sprint plan (downstream update on RED branch)
- `{output_folder}/planning-artifacts/prd.md` — Exec Summary failure mode (metadata-engine paragraph)
