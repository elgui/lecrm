---
id: 20260514-114217-3c84
title: leCRM v0 — ADR-010 (metadata-engine pattern), authored PRE-Wk 5
status: done
priority: p0
created: 2026-05-14
updated: 2026-05-15
category: engineering
group: lecrm-v0-scaffolding-v2
group_order: 1
order: 2
done: 2026-05-15
---

## Read this cold — full context inline

The DDL-vs-JSONB metadata-engine decision must NOT be a discovery moment at Wk 5. By the time G2 (ADR-009 §9) fires reactively at end of Wk 5, JSONB has already shipped (because it's the simpler path and the schedule was pushing) and is load-bearing through v1+. The honest framing per Winston: "JSONB fallback ships and is load-bearing through v1; v2 either accepts JSONB permanently or budgets a dedicated migration epic (live tenant data backfill + read-path rewrite — not a sprint)."

This tasket is the formal G2 schedule gate executed PROACTIVELY at Wk 4 (or earlier if the scaffolding makes the metadata surface ready sooner). Decision made on evidence, not on schedule pressure.

## Why this exists

From PRD step-02 round-1 + round-2 council debates (Winston, 2026-05-13/14):

- "JSONB fallback doesn't de-risk complexity — it *defers* it. You still own the eventual migration. I'd rather we mark complexity 'high' now and let the JSONB fallback be the lever that brings it back down when (if) we pull it."
- "The honest fork isn't 'DDL or JSONB' — it's 'DDL or accept JSONB as the v0+v1 reality and plan the DDL migration as a v2 epic.' Decide that BEFORE Wk 5, not during."
- "Once you have a `custom_fields JSONB` column with three Design Partners' worth of data in it, migrating back to per-tenant DDL means writing a per-tenant extraction-and-typing pipeline. That's not 5 days — that's a sprint."

## Prerequisite (DOR)

- Scaffolding has defined the metadata-engine surface concretely enough to scope (at minimum: which entities support custom properties, target API shape, expected query patterns).
- Stack decision locked (G1 / tasket `20260510-202450-a5d3`) — DDL tooling differs Go-vs-TS (sqlc + goose vs Drizzle/Kysely + Atlas).

## Approach

1. **Research the actual scope of per-tenant DDL primary path.** Concrete questions:
   - How many migrations per custom-property creation?
   - What's the locking behaviour during ALTER TABLE under concurrent reads?
   - How is the SQL generated (templated vs ORM-driven)?
   - Target: ≤5 days cumulative across the metadata engine implementation.
   - If estimate exceeds 5 days even with the most pragmatic implementation, JSONB is the structurally honest call.

2. **Author `docs/adr/ADR-010-metadata-engine.md`.** Capture:
   - **Decision: DDL-primary OR JSONB-primary** — one of the two. No "we'll decide later" mealy-mouthed framing.
   - Rationale, with scope estimate from step 1.
   - Fallback condition if DDL-primary chosen (e.g., "if cumulative metadata work exceeds 7 days at Wk 6, execute G3 fallback per tasket `20260514-114245-d3a8`").
   - **Critical paragraph if JSONB chosen, verbatim:** "JSONB is load-bearing through v1; v2 either accepts JSONB permanently or budgets a dedicated migration epic — live tenant data backfill plus read-path rewrite, not a sprint."

3. **Update downstream tasket bodies** that depend on the metadata pattern:
   - `test-strategy-scope-doc` (tasket `20260514-114210-9b41`) — non-negotiable category (c)
   - `integrator-handoff-capabilities` (tasket `20260514-114231-8a67`) — methodology config storage shape
   - G3 verification gate body (tasket `20260514-114245-d3a8`)
   - Any v0 feature implementation taskets that exist by Wk 4

## Done When

- [ ] `docs/adr/ADR-010-metadata-engine.md` committed
- [ ] Decision is binary and explicit (DDL OR JSONB — not "depending on")
- [ ] If JSONB: the load-bearing-through-v1 paragraph is in the ADR verbatim
- [ ] If DDL: fallback trigger condition is precisely specified (day count + behaviour)
- [ ] PRD `tenancy-topology-evolution` flag row in Project Classification table updated if needed
- [ ] Downstream tasket bodies aligned (#2, #5, #7 in this group / scaffolding group)

## References

- `docs/adr/ADR-001-tenancy-model.md` — schema-per-tenant topology baseline
- `docs/adr/ADR-009-stack-and-license.md` §9 G2 — the schedule gate this tasket executes proactively
- `{output_folder}/planning-artifacts/prd.md` — Exec Summary failure mode (JSONB-bleed paragraph)
- Tasket `20260514-114245-d3a8` — G3 scope verification depends on this ADR
