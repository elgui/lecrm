---
id: 20260514-114245-d3a8
title: "leCRM v0 — G3: Wk 6 metadata-engine scope verification gate"
status: blocked
priority: p0
created: 2026-05-14
updated: 2026-05-14
category: engineering
group: lecrm-v0-scaffolding-v2
group_order: 1
order: 3
blocked_on: DOR-prerequisites (ADR-010 must be committed by Sprint 4 / Wk 4 via tasket 20260514-114217-3c84; metadata-engine implementation underway Sprint 4-6) + schedule-timing (we are at end of Wk 2; gate fires end of Wk 6 ~2026-06-23). Premature firing falsifies the gate's evidence basis per the tasket body.
runbook: docs/gates/G3-metadata-engine-scope-verification-runbook.md
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
- `docs/gates/G3-metadata-engine-scope-verification-runbook.md` — the fire-at-Wk-6 kit prepared in this session
- Tasket `20260514-114217-3c84` — ADR-010 metadata-engine commitment (the pattern this gate verifies)
- Tasket `20260514-114210-9b41` — test strategy doc (downstream consequence of GREEN/RED branch)
- Tasket `20260514-114203-7e2f` — sprint plan (downstream update on RED branch)
- `{output_folder}/planning-artifacts/prd.md` — Exec Summary failure mode (metadata-engine paragraph)

---

## Verification Prep Status (2026-05-14)

The automator scheduled this Wk-6 gate during Wk 2 of the build (Wk-2 Go ramp checkpoint committed `c476737`). Running the verification now would falsify the gate's evidence basis: ADR-010 has not been authored (tasket `20260514-114217-3c84` is `later`, scheduled for Sprint 4 / Wk 4 per `docs/sprint-plan.md`), no metadata-engine implementation has started, and there are no calendar days of effort to count. Marking this tasket `done` at Wk 2 would either record a fabricated "GREEN — 0 days, continue" or invent a phantom RED — both falsify the run record and break the ADR-009 §9 G3 schedule gate's purpose. Status is therefore **blocked**, not **done**, by automator rule #4.

### What was completed in this session (commit-tracked)

- **`docs/gates/G3-metadata-engine-scope-verification-runbook.md`** — the fire-at-Wk-6 kit. Codifies the day-counting methodology (git + session-transcript + tasket-body, take the max), the GREEN/RED threshold (≤5d cumulative AND ≤2d remaining), the RED-branch JSONB-fallback schema shape (per-workspace-schema `objects` table with `data jsonb`), the ADR-010 addendum template, and the downstream taskets that must update conditionally. Structurally analogous to the G4 `SUBMISSION-PACKAGE.md` produced in this same group.

### DOR gap (must close before Wk-6 firing can be honest)

1. **ADR-010 committed.** Tasket `20260514-114217-3c84` produces `docs/adr/ADR-010-metadata-engine.md` with a binary DDL-primary OR JSONB-primary decision. Sprint plan places this at Sprint 4 / Wk 4 (proactive G2 per Winston's round-2 lock).
2. **Metadata-engine implementation underway.** Sprint 4-5 work (Go branch: `apps/api/internal/metadata/` per ADR-010's chosen pattern). Without commits in this surface since ADR-010, there's nothing to verify.
3. **End-of-Sprint-6 timing.** Sprint plan Monday-cadence puts Sprint 6 close around 2026-06-23 (contingent on Wk-1 baseline 2026-05-12). Do not fire before the Friday of Sprint 6. Do not fire later than the Monday of Sprint 7 — late firing flips the gate from "scope-check" to "sunk-cost rationalisation".

### Hand-off to Wk-6 trigger

When Sprint 6 arrives and items 1-2 above are green, the verification is a **single-session execute-the-runbook task**: open `docs/gates/G3-metadata-engine-scope-verification-runbook.md`, run §3 measurement, evaluate §4 threshold, take the §5 GREEN or RED branch, fill the §6 ADR-010 addendum, flip this tasket's status to `done` with `outcome: GREEN-continued` or `outcome: RED-fallback-executed` in the frontmatter.

If RED and the fallback work is ≥1 day, a follow-up implementation tasket should be created **via the Tasket skill** (not direct markdown write — see [[feedback_tasket_skill_required]]) per runbook §5.2.5.
