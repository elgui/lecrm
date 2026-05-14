---
id: 20260514-114203-7e2f
title: "leCRM v0 — Sprint plan with explicit Wk-2 Go-vs-TS fork branch"
status: later
priority: p1
created: 2026-05-14
updated: 2026-05-14
category: engineering
group: lecrm-v0-execution-discipline
group_order: 2
order: 1
---

## Read this cold — full context inline

This tasket produces a planning artefact, not code. It exists because the Wk-2 stack-decision gate (ADR-009 §1.1, tracked as G1 / tasket `20260510-202450-a5d3`) is **irrevocable** — one of two paths gets taken and the other is closed forever. Without a sprint plan that absorbs this gate structurally, the Wk-2 decision is chaotic: every downstream tasket body has to be rewritten depending on which path won. The fix is to draft both branches in advance.

## Why this exists

From PRD step-02 round-2 council debate (Winston, 2026-05-14):

> "Flag table works. The 'Irrevocable; sprint plan reprices at Wk 2' column carries the weight — *provided the sprint plan actually shows the reprice branch*. If the sprint plan downstream doesn't have a Wk-2 fork ('Go path / TS path'), the table is decorative. I'd want to see that branch concretely before signing off this is mitigated."

This tasket is the artefact that lets the mitigation column stop being decorative.

## Prerequisite (DOR)

- `docs/adr/ADR-009-stack-and-license.md` is committed (the canonical reference for the gate condition).
- Tasket `20260510-202450-b844` Part B (Week-1 scaffolding) is either underway or about to start — the sprint plan must reflect the actual scaffolding tasks, not theoretical ones.
- The 8 v0 features from the PRD Executive Summary are locked: Contacts, Companies, Deals, Pipeline Kanban, Gmail sync, Notes / activity, Tasks, Custom properties on Contact + Deal, Multi-user with role-based permissions, Tenant CSV export.

## Approach

1. **Inventory sprints 1-2.** These are stack-agnostic enough that they need minimal forking (scaffolding + G1 litmus tests).
2. **Draft Go-path sprint plan for sprints 3-13.** Each sprint includes: features being built, tests written, fixture surface, schedule-gate triggers (G2 Wk 5, G3 Wk 6, G4 Wk 5-6).
3. **Draft TS+Hono-path sprint plan for sprints 3-13.** Same structure but explicitly call out: (a) re-pricing of test/fixture investment (Go test harness work is dead — Vitest/Bun/etc. takes its place), (b) any features that get easier/harder in TS+Hono, (c) ADR-010 metadata-engine-pattern decision still applies but the implementation strategy differs.
4. **Make the divergence point at Wk 2 explicit** — both branches share sprints 1-2 and end with the same deliverable (v0 ships with the 8-feature core at Wk 11-13).
5. **One paragraph at the top** explains the Wk-2 decision mechanism and what triggers each branch (referencing ADR-009 §1.1 and tasket `a5d3`).

## Done When

- [ ] `docs/sprint-plan.md` (or equivalent location) exists in repo
- [ ] Both Go-path and TS+Hono-path branches are written with sprint-by-sprint detail (no placeholders)
- [ ] Test/fixture re-pricing called out explicitly per branch
- [ ] G2 / G3 / G4 schedule-gate triggers placed in their correct sprints on BOTH branches
- [ ] Top paragraph explains the Wk-2 decision mechanism
- [ ] Short architect-style review pass (Winston-lens: are there hidden assumptions about Go-only patterns that would break in TS+Hono?)

## References

- `docs/adr/ADR-009-stack-and-license.md` §1.1 — Wk-2 litmus tests
- `{output_folder}/planning-artifacts/prd.md` — Exec Summary failure mode + flag table mitigation column
- Tasket `20260510-202450-a5d3` — the binding G1 decision gate this plan serves
- Tasket `20260510-202450-b844` Part B — Week-1 scaffolding (sprints 1-2 source)
