---
id: 20260515-192005-fde9
title: "leCRM v0 — Sprint plan reconciliation: ADR-010 JSONB-primary propagation (Sprint 3, ~5 min)"
status: done
priority: p2
created: 2026-05-15
updated: 2026-05-18
done: 2026-05-18
tags: [sprint-3, housekeeping, adr-010, documentation]
category: engineering
group: lecrm-v0-sprint-3
group_order: 3
order: 6
plan: true
---

## Read this cold — full context inline

Mechanical wording fix on `docs/sprint-plan.md` — collapse the conditional "DDL-primary OR JSONB-primary" / "if JSONB chosen" framing across Sprints 4-6 entries to declarative "JSONB-primary per ADR-010 §1". ADR-010 §TO RESOLVE-5 names this as the follow-up.

## Why this exists

ADR-010 committed 2026-05-15 (commit `e875fb8`) selected JSONB-primary on a generic `objects` table per workspace schema. The decision is binary, not conditional. The sprint plan's Sprint 4-6 entries still hedge with "DDL OR JSONB" / "if JSONB chosen" language inherited from the pre-ADR design. Stale conditional language confuses any future reader (or automated dependency tracking) about what was decided.

Priority p2 because it's a documentation refresh, not blocking any code work. Mechanical fix, ~5 minutes.

## Prerequisite (DOR)

- ADR-010 committed (done at commit `e875fb8`, 2026-05-15).

## Approach

Edit `docs/sprint-plan.md`:

### Sprint 4 row (L115)
Current: "**G2 fires PROACTIVELY** — tasket `20260514-114217-3c84` | ADR-010 authored at Wk 4, not Wk 5; binary decision DDL-primary OR JSONB-primary; if JSONB chosen, ADR-010 carries the load-bearing-through-v1 paragraph verbatim"

New: "**G2 fired proactively at Wk 3** — tasket `20260514-114217-3c84` `done` (2026-05-15). ADR-010 selected JSONB-primary on `objects` table per workspace schema. Load-bearing-through-v1 paragraph in ADR-010 §6."

### Sprint 5 row (L127)
Current: "Metadata engine implementation continues | DDL-primary or JSONB depending on ADR-010"

New: "Metadata engine implementation continues per ADR-010 (JSONB-primary on `objects` + `custom_property_definitions` per workspace schema)"

### Sprint 5 row (L130)
Current: "**JSONB regression test coverage** IF ADR-010 chose JSONB | Concurrent mutation, schema drift, query correctness — non-negotiable test category (c)"

New: "**JSONB regression test coverage** (non-negotiable test category (c), load-bearing per ADR-010) | Concurrent mutation, schema drift against `custom_property_definitions`, GIN-index query correctness — ≥8 tests per `docs/test-strategy.md` §4.3"

### Sprint 6 row (L140)
Current: "**G3 verification fires end of sprint** — tasket `20260514-114245-d3a8` | Count actual days of metadata-engine effort honestly; if >5d cumulative, execute JSONB fallback per ADR-009 §9 G3 (JSONB `data` column on generic `objects` table per workspace schema)"

New: "**G3 verification fires end of sprint** — tasket `20260514-114245-d3a8` | JSONB-scope sanity check per G3 runbook §5.2.2 (LIVE path): is cumulative JSONB metadata-engine work staying inside the 3.25d projection? Are non-negotiable (c) tests passing? Runbook §5.2.1 (DDL→JSONB switch) is historical."

### Sprint 6 G3 fallback consequences (L145)
Current: "**G3 fallback consequences:** if G3 forces JSONB fallback at this sprint, downstream tasket `8a67` Phase 2 (methodology config) must reconcile to the JSONB shape; test-strategy non-negotiable category (c) becomes load-bearing rather than conditional."

New: "**G3 outcome already determined by ADR-010:** ADR-010 committed JSONB-primary 2026-05-15 (proactive G2). Downstream alignment landed in commit `e875fb8`: tasket `731a` Phase 2 (methodology config) body updated; test-strategy category (c) already load-bearing; G3 runbook §5.2.1 marked historical. Sprint 6 G3 fire is the JSONB-scope sanity check only."

## Done When

- [ ] L115, L127, L130, L140, L145 of `docs/sprint-plan.md` updated per above
- [ ] `git diff docs/sprint-plan.md` reviewed; no other conditional ADR-010 language elsewhere in the file
- [ ] Commit: `docs(sprint-plan): reconcile to ADR-010 JSONB-primary (ADR-010 TR-5)`

## References

- `docs/adr/ADR-010-metadata-engine.md` §TO RESOLVE-5
- `docs/adr/ADR-010-metadata-engine.md` §6 (verbatim load-bearing paragraph)
- `docs/gates/G3-metadata-engine-scope-verification-runbook.md` (§5.2.1 historical, §5.2.2 LIVE)
- Commit `e875fb8` (ADR-010 + research brief + downstream alignment)
