# ADR-010 — Custom-Object Metadata Engine Pattern

**Status:** Accepted
**Date:** 2026-05-15 (Proposed); 2026-05-15 (Accepted after scope-research brief at `docs/research/adr-010-metadata-engine-scope.md`)
**Deciders:** Guillaume
**Related:** [ADR-009 §9](ADR-009-stack-and-license.md) (G2 schedule gate — this ADR fulfills it proactively from Wk 3 rather than reactively at Wk 5). [ADR-001](ADR-001-tenancy-model.md) (schema-per-tenant baseline — preserved entirely). Tasket `20260514-114217-3c84` (authoring tasket). Tasket `20260514-114245-d3a8` (G3 Wk-6 verification gate — semantics adjusted by this ADR; see §6).

---

## Context

[ADR-009 §9](ADR-009-stack-and-license.md) specifies a binding schedule gate: the custom-object metadata-engine pattern must be authored by end of Wk 5, with JSONB on a generic `objects` table per workspace schema as the fallback if per-tenant DDL hits a complexity ceiling (cumulative work > 5 days) by Wk 6.

The round-2 council framing from Winston (2026-05-13/14) sharpened this: "JSONB fallback doesn't de-risk complexity — it *defers* it. The honest fork isn't 'DDL or JSONB' — it's 'DDL or accept JSONB as the v0+v1 reality and plan the DDL migration as a v2 epic.' Decide that BEFORE Wk 5, not during. Once you have a `custom_fields JSONB` column with three Design Partners' worth of data in it, migrating back to per-tenant DDL means writing a per-tenant extraction-and-typing pipeline. That's not 5 days — that's a sprint."

This ADR executes that decision proactively at start of Wk 3 (today 2026-05-15, Wk-1 baseline 2026-05-12), based on a scope-research brief produced 2026-05-15 with concrete day estimates against the locked stack (Go 1.23+, `sqlc`, Atlas v1.0, PostgreSQL 17, schema-per-tenant per ADR-001/ADR-009 §2).

Scale constraints (ADR-009 §9): 1–50 workspaces over 24 months, 3–15 users per workspace, **≤30 custom properties per workspace**. Entity surface at v0 (PRD `v0-capability-constraints`): **Contact and Deal only**.

---

## Decision

### 1. Pattern: JSONB-primary on a generic `objects` table per workspace schema

Custom properties on Contact and Deal at v0 are persisted as JSONB on a single workspace-scoped `objects` table, with type-safety enforced at the application layer via a `custom_property_definitions` metadata table.

Per-tenant DDL for custom-property creation is **rejected** for v0 and v1. The decision is binary: no DDL-primary attempt with JSONB fallback at Wk 6; JSONB is the primary path from line 1.

### 2. Why DDL-primary was rejected (decisive evidence)

The scope-research brief (`docs/research/adr-010-metadata-engine-scope.md`) estimates DDL-primary at **5.5d / 8.25d / 11d** (low / mid / high) against the locked stack. All three estimates exceed the ADR-009 §9 5-day threshold. The overrun is driven by two compounding architectural mismatches with the stack chosen in ADR-009, not by Postgres locking or schema-cache concerns (those are tractable):

1. **`sqlc` is build-time codegen.** Runtime-added columns are invisible to it. Workarounds (hybrid scanner over `pgx.Rows.FieldDescriptions()`, `row_to_json` aggregation that defeats the typed-storage value proposition, or bypassing `sqlc` for the custom-property surface) add 2.5–3.5 days and undocumented territory. The `sqlc` maintainers have explicitly stated runtime dynamic columns are out of scope (sqlc GitHub Discussion #364).
2. **Atlas v1.0's sweep model is fleet-wide, not per-tenant on-demand.** The documented multi-tenant pattern (`for_each` over `local.tenants` in `atlas.hcl`, `group` blocks with `parallel + on_error = CONTINUE`) applies one migration to all matched tenants. Per-tenant on-demand DDL at runtime is the opposite shape: dynamic file management with `atlasexec.MigrateApply` called against a single schema URL is technically possible but undocumented, and creates a schema surface outside Atlas's migration graph (which complicates the canary-tenant and `pgroll` patterns in ADR-009 §2.4). Adds 1.5–2 days of bespoke tooling.

Risk asymmetry: if DDL-primary were attempted and hit the G3 ceiling, cumulative cost is **7–8 days** (≈5 days sunk DDL infrastructure + 2–3 days JSONB-from-scratch). vs **3.25 days** if JSONB is chosen up front. The G3 gate's implicit assumption — that DDL can be cheaply abandoned at Wk 6 — does not hold once you account for *when* the `sqlc`-shape problem gets discovered (mid-implementation, after the migration infrastructure is already partially built).

### 3. Schema shape (binding)

Provisioned in `core.lecrm_provision_workspace` (ADR-009 §2.1) as an extension to the per-workspace schema creation, or as a separate provisioning step run at workspace-create time:

```sql
CREATE TABLE workspace_<role_name>.objects (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    object_type   text NOT NULL,            -- 'custom_lead_source', 'custom_industry_tag', etc.
    parent_type   text NULL,                -- 'contact' | 'deal' | NULL
    parent_id     uuid NULL,                -- FK semantics (no DB FK — see below)
    data          jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX objects_type_parent_idx
    ON workspace_<role_name>.objects (object_type, parent_type, parent_id);
CREATE INDEX objects_data_gin_idx
    ON workspace_<role_name>.objects USING gin (data);
```

`parent_id` is not a DB-enforced foreign key. The application enforces referential integrity because `parent_type` selects which sibling table (`contacts` or `deals`) the id refers to, and Postgres FKs cannot conditionally target multiple tables. Orphan rows are detected by a periodic janitor job (deferred to post-v0; not load-bearing at ≤30 properties × low-thousands of records).

GIN operator class is the default `jsonb_ops` for v0 (broader operator coverage; query shape not yet stable). Re-evaluate `jsonb_path_ops` at v1 when query patterns are observed.

### 4. Type safety: `custom_property_definitions` metadata table (binding)

In each workspace schema:

```sql
CREATE TABLE workspace_<role_name>.custom_property_definitions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_type     text NOT NULL,       -- 'contact' | 'deal'
    property_key    text NOT NULL,       -- 'preferred_contact_method'
    property_type   text NOT NULL,       -- 'string' | 'number' | 'boolean' | 'enum' | 'date'
    allowed_values  jsonb NULL,          -- ['email', 'phone', 'sms'] for enum; NULL otherwise
    required        boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (parent_type, property_key)
);
```

This is a static, known schema — `sqlc` handles it cleanly. The API layer reads from this table on every custom-property write to validate the incoming `data` payload (enum membership, type match, required-presence). Validation is **fail-closed**: a payload that does not pass validation is rejected with `400 Bad Request`, never silently stored.

### 5. Typed access surface (binding)

Go package `apps/api/internal/metadata/` exposes:

```go
// Get returns all custom properties for one parent record.
func Get(ctx context.Context, parentType string, parentID uuid.UUID) (map[string]any, error)

// Set upserts the entire custom-property payload for one parent record.
// Validates against custom_property_definitions before writing.
func Set(ctx context.Context, parentType string, parentID uuid.UUID, data map[string]any) error

// Find queries records by JSONB containment predicate.
// Uses the GIN index via the @> operator.
func Find(ctx context.Context, parentType string, query map[string]any) ([]Object, error)
```

`Get` and `Set` operate on the full property bag per record, not per individual property — this matches how CRM UIs render custom fields (block of fields in a form) and reduces round-trip count. Individual-property mutation is composable on top of `Set`.

`Find` returns the full `Object` rows (id, parent_id, data); the calling handler is responsible for joining back to `contacts` / `deals` if it needs the parent record body. v0 join is application-side (two queries, merge in Go) rather than SQL; revisit at v1 if N+1 patterns surface.

### 6. Verbatim load-bearing paragraph

**JSONB is load-bearing through v1; v2 either accepts JSONB permanently or budgets a dedicated migration epic — live tenant data backfill plus read-path rewrite, not a sprint.**

This paragraph appears verbatim per the authoring tasket's explicit requirement. The migration-epic estimate is **3–6 weeks of focused engineering** at leCRM scale (1–50 workspaces × thousands of records each) per the scope-research brief §B.3. The cost is dominated by the read-path rewrite (every query that accesses custom properties touched, plus test-suite update, plus per-tenant rollout sequencing), not by the SQL backfill itself — that's fast at this scale.

### 7. Schedule-gate semantics: G2 honored proactively; G3 adjusted

- **G2 (Wk 5 ADR-010 authored).** Honored proactively at Wk 3 by this ADR. ADR-009 §9 G2 closes.
- **G3 (Wk 6 metadata-engine scope verification, tasket `20260514-114245-d3a8`).** The runbook at `docs/gates/G3-metadata-engine-scope-verification-runbook.md` was prepared on the assumption that ADR-010 might choose DDL-primary and that G3 would be the DDL→JSONB switch point. With this ADR pinning JSONB-primary from line 1, G3 becomes a **JSONB-scope sanity check** (is cumulative JSONB metadata-engine work staying inside the 3.25d projection? are tests for non-negotiable category (c) — JSONB regression — actually passing?) rather than a pattern-switch gate. Runbook §5.2.2 ("If ADR-010 chose JSONB-primary and IT is the one bleeding scope") is the live branch; §5.2.1 (DDL→JSONB) is dead code preserved only as historical record.

The G3 gate is **not removed** — it's the integrity check that v0 is actually shipping on JSONB-primary as planned, not silently drifting into something more elaborate under sprint pressure.

---

## Consequences

### Positive

- **Stays comfortably inside the ADR-009 §9 5-day budget.** Mid-estimate 3.25d (vs DDL-primary 8.25d). Frees ≥5 days of v0 critical-path schedule for higher-value work (integrator-handoff, OAuth submission, sequences scaffolding).
- **`sqlc` and Atlas stay in their native sweet spot.** `sqlc` handles the static `custom_property_definitions` table cleanly; Atlas runs the `objects` + definitions tables as part of the standard sweep at workspace provisioning. No undocumented SDK use, no per-tenant migration file management.
- **GIN-indexed JSONB performance is not a constraint at v0 scale.** Public benchmarks measure `@>` containment queries at sub-millisecond latency (~0.15ms) on comparable datasets. ≤30 properties × low-thousands of records per entity is well inside the comfort envelope.
- **Custom-property storage shape is stable from v0.** The `objects` + `custom_property_definitions` pair is the same shape at v0, v1, and (if accepted permanently) v2. No mid-v1 schema upheaval.
- **Type safety preserved at the API boundary** via `custom_property_definitions` + fail-closed validation. The "JSONB has no type safety" critique is correct at the storage layer but neutralized at the access layer.

### Negative

- **Custom-property data is opaque to SQL introspection.** `\d contacts` does not show `preferred_contact_method`. Analyst queries need to know to look in `objects.data` and use JSONB operators. Acceptable v0 cost given the v0 audience is the application + AI agents, not analysts. Revisit at v1 if a Design Partner pushes back.
- **v2 carries a 3–6-week migration epic** if/when typed columns become necessary (e.g., per-property indexing for high-cardinality lookups, BI-tool compatibility, schema-introspection demands from an enterprise buyer). This is the load-bearing-through-v1 reality; it cannot be wished away. The mitigation is honesty in the project log (see §6).
- **Per-property indexing is harder.** A high-cardinality custom property that wants its own B-tree index requires either an `EXPRESSION INDEX` on `(data->>'<key>')` (per-workspace, runtime DDL — same shape we were trying to avoid) or accepting the GIN index's broader coverage. v0 makes no per-property indexing commitment.
- **Cross-tenant analytics fenced.** Already a v0 capability constraint per PRD (`v0-capability-constraints`); this ADR reinforces it.
- **GIN write amplification** is higher than B-tree. Negligible at 3–15 concurrent users per workspace; `fastupdate` is on by default to batch pending-list writes. Worth re-measuring at v1 if write throughput grows.

### Neutral

- **ADR-001 schema-per-tenant primitive entirely preserved.** The `objects` table lives inside the workspace schema and inherits the per-workspace Postgres role's `search_path` (ADR-009 §2.1). Cross-tenant isolation properties unchanged.
- **ADR-007 audit-log shape unchanged.** Custom-property writes audit via the standard `payload JSONB` field with `event = 'metadata.property.upsert'` (catalogue addition tracked under §TO RESOLVE).
- **ADR-009 §9 G2 closes; G3 semantics adjusted but gate survives.** See §7.

---

## Alternatives Considered

### DDL-primary (per-tenant ALTER TABLE per custom-property creation)

The shape ADR-009 §9 hoped to land on. Rejected for the two architectural-mismatch reasons in §2 above. The brief estimates 5.5–11d (mid 8.25d). Even the low-end assumes perfect up-front routing decisions that are unlikely without prior experience with this exact pattern on this exact stack. PostgreSQL 17 `ADD COLUMN` locking (`ACCESS EXCLUSIVE`, but milliseconds for nullable or non-volatile-default columns) is **not** the problem; the problem is `sqlc` + Atlas shape mismatch.

### Hybrid (DDL for "high-value" properties, JSONB for "long-tail")

Adds a taxonomy decision to v0 critical path ("which properties are high-value enough to warrant DDL?"). Doubles read-path complexity (typed scan + JSONB scan, both for the same record type). Postpones but does not eliminate the v2 migration epic. Combines the worst of both at v0. Rejected.

### Twenty fork-style metadata engine

Twenty's metadata extension API uses workspace-aware NestJS providers + per-tenant DDL via TypeORM. Stack-incompatible (ADR-009 §1 selected Go + `sqlc`, not TS/NestJS/TypeORM). ADR-008 ruled out copying Twenty's implementation patterns regardless. Reference only.

### `data JSONB` column on the existing `contacts` / `deals` tables (no `objects` table)

Functionally equivalent to the chosen pattern at v0 scale, with one downside: it tightly couples standard-field migrations with custom-field storage. The generic `objects` table keeps the two concerns physically separated, which (a) makes the v2 DDL migration easier if we take it (custom fields are already isolated), (b) keeps the `contacts` / `deals` schemas stable and `sqlc`-friendly without a `data` column appearing in every generated struct, and (c) lets the same table host non-record-bound custom objects later (`object_type = 'integration_config'`) if we want. Marginal cost (one extra join when reading a record + its custom fields) is acceptable. Rejected the per-table approach in favor of the generic `objects` table.

---

## References

- `docs/research/adr-010-metadata-engine-scope.md` — full scope-research brief (3,415 words, 18 sources) underlying this decision.
- [ADR-009 §9](ADR-009-stack-and-license.md) — G2 schedule gate (executed proactively here) and G3 Wk-6 verification (semantics adjusted in §7).
- [ADR-001](ADR-001-tenancy-model.md) — schema-per-tenant baseline preserved entirely.
- [ADR-007](ADR-007-encryption-secrets-audit.md) — audit-log catalogue; receives `metadata.property.upsert` event addition.
- `docs/gates/G3-metadata-engine-scope-verification-runbook.md` — Wk-6 verification (now a JSONB-scope sanity check, runbook §5.2.2 live, §5.2.1 historical).
- Tasket `20260514-114217-3c84` — this ADR's authoring tasket.
- Tasket `20260514-114245-d3a8` — G3 Wk-6 verification (semantics adjusted by this ADR).
- Tasket `20260514-114210-9b41` — test strategy non-negotiable category (c) (JSONB regression coverage) becomes load-bearing.
- Tasket `20260514-114231-8a67` — integrator-handoff methodology config storage shape (lands on JSONB, not DDL).
- [PostgreSQL 17 JSON Types documentation](https://www.postgresql.org/docs/17/datatype-json.html) — GIN operator classes, `@>` containment semantics.
- [Atlas Guides — Database-per-Tenant Architecture](https://atlasgo.io/guides/database-per-tenant/intro) — confirms the fleet-wide sweep model that drove the DDL-primary rejection.
- [sqlc GitHub Discussion #364](https://github.com/sqlc-dev/sqlc/discussions/364) — maintainer position that runtime dynamic columns are out of scope.

---

## TO RESOLVE

1. **Provisioning function extension.** Extend `core.lecrm_provision_workspace` (or add a sibling provisioning step) to create the workspace's `objects` and `custom_property_definitions` tables + indexes alongside the schema and role. Tracked as a Sprint 4 implementation tasket.
2. ~~**Audit-log catalogue addition.**~~ **Resolved (Sprint 4).** `metadata.property.upsert` added to ADR-007 §3 catalogue and implemented in `apps/api/internal/metadata/set.go`. The JSONB write and audit emission share a single Postgres transaction; audit failure rolls back the metadata write (ADR-009 §7.2 fail-closed invariant). Fail-closed integration test in `apps/api/internal/metadata/fail_closed_test.go`.
3. **G3 runbook annotation.** Update `docs/gates/G3-metadata-engine-scope-verification-runbook.md` to flag §5.2.1 as historical (DDL→JSONB switch path that is dead code per this ADR) and §5.2.2 as the live path (JSONB-scope sanity check). Done as part of Step 3 of authoring tasket `20260514-114217-3c84`.
4. **Downstream tasket alignment** (Step 3 of authoring tasket `20260514-114217-3c84`):
   - Tasket `20260514-114210-9b41` (test-strategy) — non-negotiable category (c) "JSONB metadata mutation regression path" was conditional on this ADR's outcome; now load-bearing. Minimum test count to be specified in that tasket's `done`-state amendment.
   - Tasket `20260514-114231-8a67` (integrator-handoff) — methodology config storage shape collapses to JSONB on the `objects` table (`object_type = 'integrator_methodology_config'` or sibling). Update tasket body.
5. **Sprint plan reconciliation.** `docs/sprint-plan.md` Sprint 4-6 entries should reference ADR-010 JSONB-primary as the active pattern (not "DDL with JSONB fallback"). Sprint 6 row should reference G3 as a sanity check, not a switch point.
6. **`jsonb_path_ops` re-evaluation at v1.** Once query patterns are observed in production (≥1 Design Partner live), evaluate whether `jsonb_path_ops` would be a strict improvement over `jsonb_ops` for the dominant query shape.
7. **Janitor job for orphan `objects` rows** — deferred to post-v0. Not load-bearing at v0 scale; revisit when a Design Partner reports referential drift.

---

## Verification Outcome — G3 Schedule Gate

To be appended at end of Sprint 6 / Wk 6 per the runbook §6 template. Expected outcome: GREEN — JSONB-primary held inside scope envelope. If RED (JSONB itself bleeding past 3.25d projection), runbook §5.2.2 prescribes the simplification path (strip any per-tenant ambition that crept in).
