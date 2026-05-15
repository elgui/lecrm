# ADR-010 Scope Research Brief — DDL vs JSONB Metadata Engine

**Prepared for:** Winston (Architect), ADR-010 authoring
**Date:** 2026-05-15
**Stack context:** Go 1.23+, Chi, sqlc, PostgreSQL 17, Atlas v1.0, schema-per-tenant
**Scale context:** 3–15 users/workspace, ≤30 custom properties/workspace, 1–50 workspaces over 24 months
**Entities in scope:** Contact and Deal only (v0 PRD fence)

---

## Executive Summary

The DDL-primary path has a realistic mid-range estimate of **7–10 days** for a solo developer with Claude Code assistance on this stack — materially above the 5-day ADR-009 threshold. The overrun is driven almost entirely by a single factor: sqlc is a build-time code generator that cannot see runtime-added columns, forcing either a bespoke hybrid scan layer or a complete bypass of sqlc for the custom-property read surface. JSONB-primary, by contrast, lands at **2–3 days** end-to-end with a straightforward typed-access helper and GIN index that performs acceptably at the stated scale. At ≤50 workspaces and ≤30 properties per workspace, GIN query latency is sub-millisecond and never the bottleneck. The structural recommendation is JSONB-primary, with an explicit project-log note that v2 may either accept JSONB permanently or budget a dedicated 4–8 week migration epic.

---

## A. DDL-primary scope

### A.1 Migration generation

Under Atlas v1.0, the multi-tenant migration model is designed around **fleet-wide sweeps**, not per-tenant on-demand DDL. The canonical pattern — documented in the GopherCon Israel 2025 talk [1] and the database-per-tenant guide series [2][3][4] — is the `for_each` meta-argument in `atlas.hcl`, which expands one `env` block into N instances, one per tenant, all sharing the same migration directory:

```hcl
env "prod" {
  for_each = toset(local.tenants)
  url      = "postgres://.../${each.value}"
  migration {
    dir = "file://migrations"
  }
}
```

Running `atlas migrate apply --env prod` then sweeps all matched tenants. The `deployment` block ([4]) adds group-level parallelism and `on_error = CONTINUE` semantics:

```hcl
group "free_tier" {
  match    = var.tier == "FREE"
  parallel = 10
  on_error = CONTINUE
}
```

`on_error = CONTINUE` logs a failure and proceeds with remaining targets in the group rather than aborting the sweep [4].

**Critical shape mismatch:** The DDL-primary custom-property model requires applying an `ALTER TABLE` to **exactly one workspace schema** at the moment a user clicks "Add field" in the UI — not as part of a fleet sweep. Atlas's documented model does not support selective single-tenant application at runtime in this way. The Go SDK (`atlasexec`, `MigrateApply`) [5] does expose a `URL` field in `MigrateApplyParams` that accepts a specific schema-scoped connection string, making it technically possible to call `MigrateApply` with `URL = postgres://.../lecrm_<uuid>`. However, this approach requires:

1. Dynamically generating a **new migration file** per custom-property creation event (or accumulating a pending migration), because the sweepable migration directory must contain deterministic SQL.
2. Tracking per-tenant migration state across all workspace schemas (Atlas's `atlas_schema_revisions` table exists per-target, which is correct, but management tooling is absent for this ad-hoc pattern).
3. Ensuring the schema cache for that workspace is invalidated immediately after the `MigrateApply` returns.

No Atlas documentation describes or endorses this flow. It is an undocumented use of the SDK. The GopherCon talk [1] and all guide pages [2][3][4] exclusively describe migrations that are pre-authored and applied identically to all tenants. Custom-property DDL is structurally the opposite shape: runtime-generated, workspace-specific, non-repeatable across the fleet.

**Practical conclusion on migration generation:** A DDL-primary implementation must either (a) embed Atlas as a runtime DDL executor with custom per-tenant file management, or (b) bypass Atlas entirely for the custom-property surface and execute raw `ALTER TABLE` statements via `pgx.Exec` directly. Option (b) is simpler but creates an untracked schema surface outside Atlas's migration graph, which complicates the canary-tenant and pgroll patterns in ADR-009. Neither option is trivial. **Estimate: 1.5–2 days** for migration infrastructure alone.

### A.2 ALTER TABLE locking under concurrent reads

PostgreSQL 17 locking semantics for the two relevant `ADD COLUMN` variants [6][7][8]:

| Operation | Lock level | Table rewrite | Effective duration |
|---|---|---|---|
| `ADD COLUMN col TEXT` (nullable, no default) | `ACCESS EXCLUSIVE` | No — metadata only | Milliseconds [7][8] |
| `ADD COLUMN col TEXT NOT NULL DEFAULT 'constant'` | `ACCESS EXCLUSIVE` | No — PG11+ fast path for non-volatile defaults [8][9] | Milliseconds |
| `ADD COLUMN col TEXT NOT NULL DEFAULT now()` (volatile) | `ACCESS EXCLUSIVE` | Yes — full table rewrite | Minutes on large tables |

The key PG11+ optimization: adding a column with a **non-volatile** (constant) default stores the default in `pg_attribute` system catalog rather than physically rewriting every row. Existing rows return the stored default at query time without being touched on disk [7][9]. This applies to all non-function literal defaults (`'email'`, `0`, `false`, etc.) — the common case for CRM enum and boolean custom fields.

For the leCRM scale (3–15 concurrent users, O(thousands) of rows per contact/deal table), the `ACCESS EXCLUSIVE` lock is held for tens to low hundreds of milliseconds. The ADR-009 provisioning function sets `lock_timeout = '5s'` per role, which bounds the worst case: if a long-running transaction holds a conflicting lock, the `ALTER TABLE` fails fast rather than queuing and blocking the entire tenant. A retry loop with exponential backoff is standard practice [6].

**Risk at this scale:** Low, but non-zero. The `depesz` analysis [6] notes the real danger is not the ALTER itself (12ms in their benchmark) but the lock acquisition wait if an existing query holds a row-level lock. At 3–15 users, the probability of a conflicting long-running transaction during a user-initiated "Add field" action is low. A `lock_timeout = '2s'` + 3-retry loop in the provisioning handler is adequate.

**Estimate for lock-safety wrappers + retry handler:** 0.5 days.

### A.3 sqlc + dynamic schema

This is the largest unknown and the primary driver of the day estimate. sqlc is an explicit build-time tool: it reads `.sql` query files, inspects the database schema at code-generation time, and emits typed Go structs and functions [10]. A column added at runtime via `ALTER TABLE` is invisible to sqlc — it will never appear in generated code.

The pgx `Rows.FieldDescriptions()` and `Rows.Values()` API [11] provides a runtime escape hatch: when scanning result rows from a `.Query()` call, the application can inspect column names and values dynamically. This is the standard pgx pattern for unknown column sets. However, it requires:

- Abandoning sqlc's generated `GetContact` / `GetDeal` query functions for any query that must return custom columns.
- Writing a hybrid scanner: typed scan for known columns (id, name, email, stage, etc.) + dynamic `map[string]any` scan for columns whose names match the `custom_property_definitions` table for that workspace.
- Managing `INFORMATION_SCHEMA.columns` or a local cache of "which custom columns exist for workspace X" so the dynamic query builder knows which additional columns to `SELECT`.

Options evaluated:

**(a) `SELECT *` + typed/dynamic split scan:** Simplest SQL, but `SELECT *` order is fragile across schema versions and requires post-scan column-name dispatch. Ergonomically awkward with sqlc since the `*` cannot appear in a sqlc-annotated query file.

**(b) `row_to_json` aggregation:** `SELECT id, name, ..., row_to_json(c) AS custom_data FROM contacts c` returns a JSON blob for custom columns. Eliminates dynamic scanning — custom data arrives as `map[string]any` after one `json.Unmarshal`. This is arguably the most ergonomic DDL-primary read pattern, but it defeats the core value proposition of DDL (type-safe per-column storage readable by SQL tools) since reads arrive as JSONB anyway.

**(c) Two-step query:** Typed sqlc query for known columns + separate `pgx.Query` with dynamic SQL for custom columns, then merge in application code. Two round-trips per read; schema cache required.

**(d) Bypass sqlc entirely for the custom-property surface:** Use raw `pgx.Exec` / `pgx.Query` for all custom-property reads and writes, keeping sqlc only for the static contact/deal core. This is the most pragmatic option — it keeps the codebase coherent and avoids the hybrid scanner complexity — but it means a meaningful surface area of the data layer lives outside sqlc's type safety.

The pgx maintainer (jackc) explicitly recommends `Rows.FieldDescriptions()` + `Rows.Values()` for dynamic column scenarios [11]. The community consensus is that truly dynamic columns are a known gap in sqlc's model and the project maintainers have explicitly stated this is out of scope [12].

**Ergonomic cost vs JSONB:** The JSONB typed-access helper is a single `Get(ctx, parentType, parentID, key string) (any, error)` function over a `custom_property_definitions` metadata table plus a simple `jsonb_extract_path_text` or Go-side map access. Zero build-time tooling friction. The DDL path requires either option (b)'s JSON-on-read (ironically converging with JSONB) or option (d)'s raw pgx layer — both materially more complex than the JSONB helper.

**Estimate for sqlc workaround + dynamic read layer:** 2.5–3.5 days (the range reflects whether option (b) or (d) is chosen and how much test coverage is required for the schema-cache invalidation path).

### A.4 End-to-end day estimate — DDL-primary

| Work item | Low | Mid | High |
|---|---|---|---|
| Migration template authoring + Atlas SDK integration (per-tenant `MigrateApply` with dynamic file management) | 1.0d | 1.5d | 2.0d |
| Lock-safety wrapper + retry-on-lock-timeout handler | 0.25d | 0.5d | 0.75d |
| sqlc workaround for runtime-added columns (hybrid scanner or raw pgx layer) | 2.0d | 3.0d | 4.0d |
| Per-tenant schema-cache invalidation (knowing which custom columns exist for workspace X) | 0.5d | 0.75d | 1.0d |
| Custom-property CRUD endpoints (Create, Read-list, Delete property definition) | 0.75d | 1.0d | 1.25d |
| Tests: cross-tenant isolation, concurrent-mutation correctness, cache invalidation | 1.0d | 1.5d | 2.0d |
| **Total** | **5.5d** | **8.25d** | **11.0d** |

**The low-end (5.5d) assumes:** option (d) bypass-sqlc is chosen immediately, no schema-cache bugs surface in testing, and the Atlas SDK integration is a thin wrapper. The mid-range (8.25d) is the realistic estimate for a solo developer who discovers the Atlas sweep/per-tenant shape mismatch after initial scaffolding and has to revisit the migration strategy. The high-end (11d) reflects discovery of a second-order bug in cache invalidation or a lock-contention scenario in integration testing.

**All three estimates exceed the 5-day ADR-009 threshold.** Even the low-end (5.5d) assumes perfect up-front routing decisions that are unlikely without prior experience with this exact pattern.

---

## B. JSONB-primary scope

The fallback shape from the G3 runbook §5.2.1:

```sql
CREATE TABLE lecrm_<workspace_uuid>.objects (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    object_type  text NOT NULL,
    parent_type  text NULL,
    parent_id    uuid NULL,
    data         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX objects_type_parent_idx ON ... (object_type, parent_type, parent_id);
CREATE INDEX objects_data_gin_idx ON ... USING gin (data);
```

### B.1 GIN index query performance

PostgreSQL 17 documents the `@>` containment operator as the primary indexed access path for JSONB GIN indexes [13]. The default `jsonb_ops` GIN operator class supports `?`, `?|`, `?&`, `@>`, `@?`, and `@@` operators [13]. For queries of the form:

```sql
SELECT * FROM objects WHERE object_type = 'contact' AND data @> '{"preferred_contact_method": "email"}';
```

the composite index `(object_type, parent_type, parent_id)` handles the equality predicate efficiently, and the GIN index handles the JSONB containment.

**Performance at leCRM scale:** The `coussej` JSONB vs EAV benchmark [14] measured GIN-indexed `@>` queries at **0.153ms** — approximately 15,000x faster than EAV without indexes, and well within interactive response time budgets. This benchmark used a realistic dataset size. At ≤30 custom properties per workspace and O(thousands) of contact/deal records per workspace, JSONB GIN performance is not a constraint. The `jsonb_path_ops` operator class [13] is more space-efficient and faster for path-based containment queries if the leCRM query patterns are known and stable, but the default `jsonb_ops` is correct for the v0 unknown-query-shape case.

**Write performance note:** GIN indexes have higher write amplification than B-tree due to the inverted posting lists. At 3–15 concurrent users, this is negligible. The `fastupdate` GIN parameter (enabled by default) batches pending list writes to reduce per-row overhead [13].

**Conclusion:** GIN index performance is not a material risk at leCRM v0 scale. The approach does not regress UX.

### B.2 Type safety story

Without a typed DDL column, the JSONB approach must enforce that `preferred_contact_method` is one of `['email', 'phone', 'sms']` at the application layer. Three options:

**(a) `custom_property_definitions` metadata table** — a workspace-scoped table (or rows in a shared `property_definitions` table) storing `{ property_key, property_type, allowed_values[] }`. The API layer reads this table on write to validate enum membership before inserting/updating the JSONB payload. This is the correct design regardless of DDL vs JSONB, since even DDL-primary needs to know which custom columns exist per workspace. sqlc handles the metadata table queries cleanly since it is a static, known schema.

**(b) JSON Schema validator** — validate the incoming `data` blob against a workspace-specific JSON Schema at the HTTP handler layer. More expressive (supports string patterns, numeric ranges) but heavier to implement. Not warranted for v0.

**(c) Trust the client.** Wrong approach; not recommended.

**Recommendation:** Option (a) — `custom_property_definitions` table — fits the 11-13-week solo timeline. It is already required for the DDL-primary path (to know which ALTER TABLEs to run), so it is not incremental cost in JSONB-primary. The validation logic is a simple lookup + enum check, implementable in 0.5 days.

### B.3 v1→v2 migration cost honestly stated

If leCRM ships v0 + v1 on JSONB and v2 or a post-Series A decision requires per-tenant typed DDL columns, the migration procedure for each live workspace is:

1. **Schema introspection:** Read `custom_property_definitions` to enumerate all custom properties and their types for workspace X.
2. **DDL generation:** Emit `ALTER TABLE contacts ADD COLUMN <key> <pg_type>` statements for each property. In PG17 with non-volatile defaults, these are metadata-only and complete in milliseconds per column [7][8][9].
3. **Backfill:** `UPDATE contacts SET <key> = (data->><key>)::<pg_type> WHERE data ? '<key>'` — batch-processed to avoid lock accumulation. At O(thousands) of rows per workspace, a single batch works; at tens of thousands, batching with LIMIT 10,000 + sleep is the pattern [15].
4. **Read-path rewrite:** Replace all `data->>'<key>'` accesses in Go code with typed column references. Since the DDL-primary path would have its own read layer, this is not a partial change — it is a full replacement of the custom-property access layer.
5. **Column removal from JSONB (optional):** `UPDATE contacts SET data = data - '<key>'` to de-duplicate. Can be deferred.
6. **Index updates:** Drop the GIN index if no longer needed; add per-column B-tree indexes for high-cardinality properties.

The PRD Exec Summary assertion that this is "not a sprint" is accurate. The backfill SQL itself is fast at leCRM scale — the cost is in the **read-path rewrite**, which touches every query that accesses custom properties, plus the test suite update, plus the per-tenant rollout sequencing across 1–50 workspaces. A realistic estimate is **3–6 weeks of focused engineering** (2–4 sprints depending on sprint length), not months — but it is a dedicated epic, not a background task. This is material but not catastrophic at leCRM's scale; the number of workspaces and entities is small enough that the migration itself is not technically risky, only time-consuming.

The honest framing: "JSONB is load-bearing through v1; v2 either accepts JSONB permanently or budgets a 4–8 week migration epic."

### B.4 End-to-end day estimate — JSONB-primary

| Work item | Low | Mid | High |
|---|---|---|---|
| `objects` table + GIN index migration (Atlas versioned SQL, applies to all tenants at provisioning) | 0.25d | 0.25d | 0.5d |
| `custom_property_definitions` metadata table + sqlc queries | 0.5d | 0.5d | 0.75d |
| Typed-access helper: `GetCustomProps`, `SetCustomProp`, `DeleteCustomProp` | 0.25d | 0.5d | 0.5d |
| Custom-property CRUD HTTP endpoints | 0.5d | 0.75d | 1.0d |
| API-layer enum validation via `custom_property_definitions` | 0.25d | 0.5d | 0.5d |
| Tests: cross-tenant isolation, GIN query correctness, type validation | 0.5d | 0.75d | 1.0d |
| **Total** | **2.25d** | **3.25d** | **4.25d** |

All three estimates are below the 5-day ADR-009 threshold. The mid-range (3.25d) is the realistic estimate for a solo developer starting from the stack scaffold described in ADR-009.

---

## C. Risk asymmetry

The ADR-009 §9 gate defines the decision tree: attempt DDL-primary through G3 Wk-6; if cumulative work exceeds 5 days, fall back to JSONB. Evaluated honestly against the day estimates:

**If DDL-primary is attempted and hits the ceiling:**
- Sunk cost: 5 days of DDL infrastructure (migration tooling, lock-safety wrappers, partial sqlc bypass layer).
- Fallback cost: 2–3 days to implement JSONB-primary from scratch (some components like `custom_property_definitions` are reusable).
- Total exposure if DDL-primary fails at the gate: **7–8 days** of cumulative metadata-engine work before shipping.
- Risk: The 5-day gate may not trigger until after the sqlc workaround complexity is discovered (mid-implementation), at which point the developer is past the threshold.

**If JSONB-primary is adopted at the start:**
- Implementation cost: 2.25–3.25 days. Done.
- Future exposure: A 4–8 week migration epic at v2 if typed columns are required. At leCRM's scale (1–50 workspaces, thousands of records each), the migration is technically tractable — it is a time cost, not a correctness risk.
- The JSONB path does not foreclose DDL adoption at v2; it defers it to a point where (a) the business case for the cost is validated by paying customers and (b) the data model is stable enough to know which properties warrant typed columns.

**Is "DDL-primary is cheaper to be wrong about"?** No. The G3 gate logic implicitly assumed DDL-primary could be attempted cheaply and abandoned if it exceeded 5 days. But the research shows the primary cost driver (sqlc + dynamic schema) is not discovered until mid-implementation, after 3–4 days of migration infrastructure work. The fallback from DDL-primary to JSONB is not free — it is 7–8 days total vs 3.25 days if JSONB is chosen first.

**JSONB-primary asymmetry:** The downside of choosing JSONB-primary incorrectly (i.e., if typed columns were always necessary for v0) would be: ship v0 with slightly weaker tooling ergonomics for data analysts (no `\d contacts` showing custom columns), then pay the migration cost at v2. At ≤50 workspaces and ≤30 properties each, this is a known, bounded, tractable cost. The DDL-primary downside is: lose 7–8 days of v0 critical-path schedule, which in an 11-13 week solo-dev window is catastrophic.

The asymmetry favors JSONB-primary: **cheaper to choose, cheaper to be wrong about in either direction at this scale.**

---

## D. Recommendation

**Adopt JSONB-primary for v0 and v1.** The DDL-primary path carries a realistic mid-range estimate of 8+ days — exceeding the 5-day ADR-009 threshold by 60% — primarily because sqlc's build-time code generation is architecturally incompatible with runtime-added columns, forcing a bespoke hybrid read layer that adds 3–4 days of uninstructed complexity. JSONB-primary delivers the same functional outcome (user-defined custom properties on Contact and Deal) in 3 days, uses Atlas's standard sweep model without per-tenant file management hacks, and performs acceptably at leCRM's scale (GIN-indexed `@>` queries measured at sub-millisecond latency in comparable benchmarks). The strongest counterargument is long-term technical debt: JSONB custom properties are opaque to SQL introspection tools, complicate future per-property indexing, and require a dedicated migration epic at v2 if typed columns become necessary. That counterargument is valid but does not change the recommendation — it changes the project log entry, which should explicitly record: "JSONB is load-bearing through v1; v2 either accepts JSONB permanently or budgets a 4–8 week migration epic."

---

## Sources

1. Atlas blog — GopherCon Israel 2025 talk recap, "Building scalable multi-tenant applications in Go": https://atlasgo.io/blog/2025/05/26/gophercon-scalable-multi-tenant-apps-in-go
2. Atlas Guides — Database-per-Tenant Architectures (introduction): https://atlasgo.io/guides/database-per-tenant/intro
3. Atlas Guides — Deploying Schema Migrations to Database-per-Tenant Architecture: https://atlasgo.io/guides/database-per-tenant/deploying
4. Atlas Guides — Staged Rollout Strategies for Multi-Tenant Schema Migrations: https://atlasgo.io/guides/database-per-tenant/rollout
5. Atlas Go SDK (`atlasexec`) package documentation — `MigrateApplyParams`, `SchemaApplyParams`: https://pkg.go.dev/ariga.io/atlas-go-sdk/atlasexec
6. depesz — "How to run short ALTER TABLE without long locking concurrent queries": https://www.depesz.com/2019/09/26/how-to-run-short-alter-table-without-long-locking-concurrent-queries/
7. tutorialpedia — "Does Adding a Null Column to a Postgres Table Cause a Lock?": https://www.tutorialpedia.org/blog/does-adding-a-null-column-to-a-postgres-table-cause-a-lock/
8. DEV Community — "Which ALTER TABLE Operations Lock Your PostgreSQL Table?": https://dev.to/mickelsamuel/which-alter-table-operations-lock-your-postgresql-table-1082
9. oneuptime blog — "How to Add Columns Without Locking in PostgreSQL" (2026-01-21): https://oneuptime.com/blog/post/2026-01-21-postgresql-add-columns-no-lock/view
10. sqlc official documentation: https://docs.sqlc.dev/en/latest/
11. pgx GitHub — Discussion #1785 "Dynamic Select Queries": https://github.com/jackc/pgx/discussions/1785
12. sqlc GitHub — Discussion #364 "Support dynamic queries": https://github.com/sqlc-dev/sqlc/discussions/364
13. PostgreSQL 17 documentation — "JSON Types" (JSONB GIN indexes, `@>` operator, `jsonb_path_ops`): https://www.postgresql.org/docs/17/datatype-json.html
14. coussej — "Replacing EAV with JSONB in PostgreSQL" (GIN index benchmark: 0.153ms `@>` query): https://coussej.github.io/2016/01/14/Replacing-EAV-with-JSONB-in-PostgreSQL/
15. averagedevs — "Zero-Downtime Database Migrations for TypeScript SaaS: Expand, Backfill, and Cut Over Safely": https://www.averagedevs.com/blog/zero-downtime-database-migrations-typescript-saas
16. Atlas Docs — Applying Schema Migrations (`atlas migrate apply` CLI and SDK): https://atlasgo.io/versioned/apply
17. Atlas Docs — Managing Multi-Tenant Migrations with Atlas Cloud Control Plane: https://atlasgo.io/guides/database-per-tenant/control-plane
18. incident.io blog — "Migrating JSONB columns in Go" (expand-contract pattern, three-phase migration): https://incident.io/blog/migrating-jsonb-columns-in-go
