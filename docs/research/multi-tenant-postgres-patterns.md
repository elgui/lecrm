# Multi-Tenant PostgreSQL Patterns for leCRM

**Audience:** Solo operator building a managed CRM-as-a-service on a forked Twenty CRM (NestJS + PostgreSQL).  
**Date:** 2026-05-10  
**Scope:** Phases 1–3 (0–20 clients, EU/French SMBs, 3–15 users each).

---

## Executive Summary

Twenty CRM's native model is **schema-per-workspace**, not the shared workspace_id discriminator pattern described in the brief. Each workspace gets a dedicated PostgreSQL schema named `workspace_<base36-uuid>`, provisioned at signup via `WorkspaceManagerService.init()`. This is a critical factual correction: the "shared schema with workspace_id" framing does not match the actual codebase.

For leCRM, the practical question is therefore: **one Docker Compose stack per client** (model A), **or** **shared PostgreSQL cluster with schema-per-client** (model B, which aligns with Twenty's own default). Model C (shared schema + workspace_id discriminator) would require forking away from Twenty's data layer entirely and is not recommended.

---

## 1. Trade-Off Matrix

| Dimension | (A) VPS-per-client | (B) Shared cluster, schema-per-tenant | (C) Shared schema, workspace_id |
|---|---|---|---|
| **Infra cost (5 clients)** | ~€40/mo (5 × €8) | ~€10–15/mo (1 shared VPS) | ~€10–15/mo |
| **Infra cost (20 clients)** | ~€160/mo | ~€20–30/mo | ~€20–30/mo |
| **Operability (solo)** | High burden: 5–20 independent stacks, updates, monitoring | Medium: one cluster, per-schema migration scripts | Low: single migration run |
| **Blast radius of app bug** | Contained to one client | Contained to one schema (IF search_path isolated correctly) | All workspaces if a WHERE clause is missing |
| **Blast radius of DB incident** | One client | All clients on cluster | All clients |
| **GDPR data isolation** | Physical: gold standard for auditors | Logical: schema boundary, defensible | Logical: row-level only, weakest |
| **CNIL stance** | Not explicitly required; risk-assessment driven | Acceptable if documented in DPA/contract | Acceptable with RLS; higher audit scrutiny |
| **Backup granularity** | Per-client `pg_dump` trivially | `pg_dump -n workspace_<id>` — per-schema dump supported natively | Requires filtered dump; no native granularity |
| **Point-in-time restore** | Per-client, surgical | Per-schema: restore schema from dump into live DB | Must restore whole DB then surgically extract rows |
| **Noisy-neighbor risk** | None (dedicated resources) | High without mitigation; one runaway query can lock shared cluster | High; worse because no schema boundary buffers planning |
| **Connection pool pressure** | Low per client; high aggregate | One pool for all tenants; efficient | One pool; most efficient |
| **Query performance at scale** | Consistent per client | Catalog bloat above ~1,000 schemas; acceptable at 5–50 | Best raw performance; no schema lookup overhead |
| **Per-tenant resource quotas** | Trivial: Docker CPU/mem limits | Via pg role limits: `ALTER ROLE ... CONNECTION LIMIT / statement_timeout / work_mem` | Same as schema model but harder to attribute per-tenant |
| **Tenant provisioning time** | 5–15 min (DNS, SSL, compose up, seed) | 30–60 sec (CREATE SCHEMA, run migrations, seed) | ~15 sec (insert workspace row, seed) |
| **Migration risk** | Per-client, isolated rollback | Script iterates schemas; partial failure leaves some schemas ahead | One migration, atomic |
| **Regulatory client upgrade path** | Already isolated; nothing to do | Extract schema → new DB → spin new stack | Must extract rows + spin new stack |

**Bottom line at 5–20 clients:** Model A gives maximum isolation at disproportionate operational cost. Model B (schema-per-tenant) is Twenty's actual native model, meaning zero fork delta, and is what leCRM should evaluate for both phases. Model C requires a significant fork away from Twenty's storage layer and offers the weakest isolation.

---

## 2. Twenty's Actual Architecture (Schema-Per-Workspace)

Twenty does **not** use a shared workspace_id discriminator. The source code confirms:

**`packages/twenty-server/src/engine/workspace-datasource/utils/get-workspace-schema-name.util.ts`**
```typescript
export const getWorkspaceSchemaName = (workspaceId: string): string => {
  return `workspace_${uuidToBase36(workspaceId)}`;
};
```

**`packages/twenty-server/src/engine/workspace-datasource/workspace-datasource.service.ts`**  
`createWorkspaceDBSchema(workspaceId)` calls TypeORM's `queryRunner.createSchema(schemaName, true)` — one PostgreSQL schema per workspace, created at tenant signup.

**`packages/twenty-server/src/engine/workspace-manager/workspace-manager.service.ts`**  
`WorkspaceManagerService.init()` orchestrates: createSchema → run standard object migrations → seed default data → generate SDK client.

The database layout is:
- **`core` schema** — shared tables: `workspace`, `user`, `user_workspace`, roles, billing
- **`metadata` schema** — shared metadata registry (custom objects, fields, views)
- **`workspace_<base36>` schema** — per-client data tables

This is important: leCRM already gets schema-per-client isolation out of the box with Twenty's multiworkspace mode (`IS_MULTIWORKSPACE_ENABLED=true`). The question for v0 is whether to use **one shared cluster** (staying close to upstream) or **one stack per client** for maximum isolation.

Source: [twentyhq/twenty GitHub](https://github.com/twentyhq/twenty)

---

## 3. Schema-Per-Tenant Deep Dive

### How It Works

One PostgreSQL instance hosts N schemas. The application sets `search_path = workspace_<id>` at connection time. All queries then resolve table names against that tenant's schema. No cross-schema data is reachable unless explicitly joined.

### Real-World Experience at SMB Scale (5–50 Tenants)

At 5–50 tenants, schema-per-tenant is entirely tractable. The Arkency team documented production surprises:

1. **PostgreSQL extensions** must be placed in a dedicated `extensions` schema included in the global search_path — they cannot live in each tenant schema. Minor but requires setup discipline. ([Source](https://blog.arkency.com/what-surprised-us-in-postgres-schema-multitenancy/))

2. **Background jobs** need a shared schema for the job queue table. Per-schema job tables multiply unnecessarily. This is already handled in Twenty's `core` schema.

3. **Catalog bloat** is only a concern above ~1,000 schemas. PostgreSQL's `pg_catalog` scans can slow DDL at that scale; at 5–50 this is irrelevant.

4. **Cross-tenant analytics** require `UNION ALL` across schemas or a separate ETL step. Acceptable at leCRM's scale.

Crunchy Data's benchmark: schema-per-tenant "works to ~100s of tenants" before catalog pressure; database-per-tenant hits practical limits at ~10–100. ([Source](https://www.crunchydata.com/blog/designing-your-postgres-database-for-multi-tenancy))

### Migration Cost Per Schema

Migrations must run per-schema. Twenty handles this in `WorkspaceManagerService` — it already iterates workspaces to run schema-level migrations during upgrades. The pattern:

```sql
-- In a migration script
DO $$
DECLARE schema_name text;
BEGIN
  FOR schema_name IN
    SELECT nspname FROM pg_namespace WHERE nspname LIKE 'workspace_%'
  LOOP
    EXECUTE format('SET search_path = %I', schema_name);
    -- run your migration DDL here
  END LOOP;
END $$;
```

At 20 schemas this takes seconds. At 1,000 schemas it takes minutes. Risk: partial failure leaves some schemas ahead. Mitigation: transactional DDL, rollback scripts.

### PgBouncer + search_path Interaction (Critical)

This is the single biggest operational hazard of schema-per-tenant.

**Transaction pooling mode** (the default for performance) does NOT preserve session state between transactions. If `search_path` is set at connection start and the connection is returned to the pool, the next borrower may inherit the previous tenant's search_path. This creates data leakage risk.

**Three safe approaches:**

1. **Session pooling mode** — PgBouncer preserves session state. Safe, but reduces pooling efficiency. Acceptable for 5–20 tenants. ([Source](https://blog.sagarregmi.info.np/transaction-pooling-the-multi-tenant-nightmare))

2. **`SET LOCAL search_path`** — Scopes the change to the current transaction only. Safe with transaction pooling. Requires the ORM to inject `SET LOCAL` at the start of every transaction.

3. **PgBouncer 1.20+ `track_extra_parameters`** — Allows PgBouncer to track `search_path` across transaction boundaries. Effectively restores session semantics in transaction mode. Recommended if upgrading PgBouncer. ([Source](https://www.citusdata.com/blog/2024/04/04/pgbouncer-supports-more-session-vars/))

Twenty uses TypeORM with a `queryRunner` per-request that sets schema context. This is naturally scoped to the request lifecycle, reducing (but not eliminating) pooling risk.

**leCRM recommendation:** Use PgBouncer in **session mode** for phase 1/2 (5–20 tenants). Revisit transaction mode with `track_extra_parameters` at phase 3.

---

## 4. Database-Per-Tenant Deep Dive

### What It Means

One PostgreSQL logical database per client, on the same or separate PostgreSQL cluster. Each database is fully independent: separate `pg_catalog`, separate connection strings, separate backups.

### Trade-Offs vs Schema-Per-Tenant

| Aspect | Database-per-tenant | Schema-per-tenant |
|---|---|---|
| Isolation boundary | DB boundary (strongest logical) | Schema boundary (strong logical) |
| Connection pool | Must pool per-database; multiplied | Single pool; efficient |
| PITR / backup | One `pg_dump -d tenantdb`; trivial | `pg_dump -n workspace_<id>`; also trivial |
| Cross-tenant analytics | Requires FDW or ETL | UNION ALL across schemas |
| Max practical tenants | ~100–500 on one cluster | ~1,000–10,000 |
| Catalog bloat | None (separate catalog per DB) | Minor at 5–50 tenants |
| Migration orchestration | Same as schema: iterate databases | Iterate schemas |
| Neon compatibility | Neon recommends project-per-tenant | Neon does not recommend schema-per-tenant |

Neon explicitly states: database-per-tenant is their recommended model; "schema-per-user doesn't reduce operational complexity or costs if compared to the many-databases approach, but it does introduce additional risks." ([Source](https://neon.com/docs/guides/multitenancy))

**For self-hosted VPS deployment (leCRM's context):** Database-per-tenant on the same PostgreSQL cluster is operationally equivalent to schema-per-tenant at 5–20 clients, with marginally stronger isolation, at the cost of multiplied connection pool overhead. Not worth the connection complexity vs schema-per-tenant unless a regulated client demands it contractually.

**Supabase's model** is worth noting: each Supabase project = a dedicated PostgreSQL cluster (compute + storage), not just a database. This achieves physical isolation while remaining API-managed. ([Source](https://supabase.com/docs/guides/getting-started/architecture)) leCRM's model A (VPS-per-client) is the on-prem analog.

---

## 5. Shared Schema + Workspace ID (Model C) Deep Dive

### Why This Is Not Twenty's Model

Contrary to the brief's framing, Twenty does NOT use `workspace_id` discriminator on shared tables. Twenty's data layer IS schema-per-workspace. The `workspace_id` appears in the `core` schema's `workspace` table (identity record), not as a column on every data row.

### Security Risk Profile of True Shared Schema

If one were to collapse all data into shared tables with a `workspace_id` column and rely on application-layer filtering:

- A single missing `WHERE workspace_id = ?` in any query leaks cross-tenant data.
- NestJS TypeORM interceptors can enforce this globally, but any raw query, migration script, background job, or admin tooling bypass creates risk.
- A bug in one client's session could expose all clients' data simultaneously — maximum blast radius.

### Row-Level Security as Defense-in-Depth

PostgreSQL RLS can enforce tenant isolation at the database engine level, independent of application code:

```sql
ALTER TABLE contacts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contacts
  USING (workspace_id = current_setting('app.workspace_id')::uuid);
```

The NestJS application sets `SET LOCAL app.workspace_id = '<id>'` at the start of each transaction. TypeORM integration via the `rls` package ([Source](https://github.com/Avallone-io/rls)) or `cls-rls` ([Source](https://github.com/nugmanoff/cls-rls)) automates this.

**Interaction with TypeORM:** TypeORM does not natively support RLS. You must wrap connections with a middleware that calls `SET LOCAL` before each query. This works but is fragile: any code path that obtains a raw connection bypasses RLS.

**CNIL stance on logical isolation:** CNIL's cloud guidance does not mandate physical isolation. It requires a risk assessment and contractual clarity. Logical isolation with RLS is defensible if documented in the DPA (Article 28 GDPR contract with the data processor). CNIL emphasizes encryption at rest/transit and clear responsibility allocation in contracts — not a specific isolation architecture. ([Source](https://www.cnil.fr/fr/securite-cloud-informatique-en-nuage)) ([Source](https://www.cnil.fr/sites/default/files/typo/document/Recommandations_pour_les_entreprises_qui_envisagent_de_souscrire_a_des_services_de_Cloud.pdf))

**Conclusion on model C:** Avoid for leCRM. It requires forking Twenty's data layer significantly, offers weakest isolation, and provides no operational advantage at 5–20 tenants.

---

## 6. Noisy-Neighbor Mitigation Playbook (Solo Operator)

At 5–20 tenants on a shared cluster, one runaway client query is the primary risk. Practical controls:

### PostgreSQL-Level Controls (Per-Role)

```sql
-- Create a role per workspace (or use one shared role with GUC overrides)
CREATE ROLE workspace_abc LOGIN PASSWORD '...';

-- Connection cap: prevent one client from exhausting the pool
ALTER ROLE workspace_abc CONNECTION LIMIT 10;

-- Statement timeout: kill runaway queries automatically
ALTER ROLE workspace_abc SET statement_timeout = '30s';

-- work_mem cap: prevent memory exhaustion from large sorts
ALTER ROLE workspace_abc SET work_mem = '16MB';

-- lock_timeout: prevent lock pile-ups
ALTER ROLE workspace_abc SET lock_timeout = '5s';
```

### Monitoring

```sql
-- Per-workspace query stats (schema-per-tenant: use search_path to attribute)
SELECT pid, now() - pg_stat_activity.query_start AS duration, query, state
FROM pg_stat_activity
WHERE state != 'idle' AND query_start < now() - interval '10 seconds';

-- pg_stat_statements per workspace (track via role or app_name)
SELECT rolname, calls, total_exec_time, rows, query
FROM pg_stat_statements
JOIN pg_roles ON pg_roles.oid = pg_stat_statements.userid
ORDER BY total_exec_time DESC
LIMIT 20;
```

Set `pg_stat_statements.track = all` in `postgresql.conf`.

### Application-Layer Controls

- Set `application_name = 'workspace_<id>'` in connection string — enables per-tenant filtering in `pg_stat_activity`.
- Implement async job queues (Bull/BullMQ in Twenty) with per-tenant rate limits.
- Alert on `pg_stat_activity` connections > threshold per role.

### Schema-Level Mirroring

For Twenty's schema-per-workspace, each workspace's data is physically separated in catalogs. A slow query in `workspace_abc` does not lock tables in `workspace_xyz`. The blast radius of query performance issues is contained to the schema — a significant advantage over model C.

Reference: [Neon noisy-neighbor article](https://neon.com/blog/noisy-neighbor-multitenant); [Crunchy Data multi-tenancy guide](https://www.crunchydata.com/blog/designing-your-postgres-database-for-multi-tenancy)

---

## 7. Provisioning + Restore Mechanics Per Model

### Model A: VPS-per-Client (Docker Compose Stack)

**Onboarding (~10–15 min manual, ~5 min scripted):**
1. Create subdomain DNS A record → client VPS IP (or shared VPS with Traefik routing).
2. Provision Let's Encrypt cert (Traefik handles automatically, ~30 sec).
3. Clone/copy Docker Compose template; inject env vars (DB password, SMTP, subdomain, `APP_SECRET`).
4. `docker compose up -d` — pulls images, starts Postgres + Twenty server + worker.
5. Run `twenty-server database:migrate` to initialize schema.
6. Seed default workspace data via `twenty-server workspace:seed` or first-login flow.
7. Configure OIDC if required.

**Per-client backup:**
```bash
docker exec <postgres-container> pg_dump -U twenty twenty_db > client_abc_$(date +%Y%m%d).sql
```

**Point-in-time restore:**
```bash
# Stop app, restore dump to fresh DB, restart
docker exec -i <postgres-container> psql -U twenty twenty_db < client_abc_20260107.sql
```

Fully surgical. Zero risk to other clients.

---

### Model B: Shared Cluster, Schema-Per-Client (Twenty Native)

**Onboarding (~1–2 min scripted):**
1. Create DNS subdomain CNAME → shared VPS (already done; Traefik routes by hostname).
2. Create workspace via Twenty API or UI: `POST /auth/sign-up` with workspace name and subdomain.
3. Twenty automatically calls `WorkspaceManagerService.init()` → creates schema `workspace_<base36>` → runs standard object migrations → seeds default data.
4. Total schema creation time: Twenty's own logs show this measured in milliseconds to seconds.
5. Configure OIDC for client if required.

Automation script skeleton:
```bash
#!/bin/bash
CLIENT=$1
SUBDOMAIN=$2
# 1. Add DNS (via Cloudflare API or manual)
# 2. Call Twenty API to create workspace
curl -X POST https://api.leCRM.fr/auth/sign-up \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@'$CLIENT'","password":"...","workspaceDisplayName":"'$CLIENT'","subdomain":"'$SUBDOMAIN'"}'
# Done — schema, migrations, seed all happen inside Twenty
```

**Per-client backup:**
```bash
# Backup one workspace schema
pg_dump -h localhost -U twenty -d twenty_db \
  -n "workspace_$(echo $WORKSPACE_ID | base36encode)" \
  -Fc -f /backups/workspace_${CLIENT}_$(date +%Y%m%d).dump

# Also backup core + metadata schemas (shared, but small)
pg_dump -h localhost -U twenty -d twenty_db \
  -n core -n metadata \
  -Fc -f /backups/shared_$(date +%Y%m%d).dump
```

**Point-in-time restore (one client, surgical):**
```bash
# 1. Drop the damaged schema
psql -c "DROP SCHEMA \"workspace_abc\" CASCADE;"
# 2. Restore from backup dump
pg_restore -h localhost -U twenty -d twenty_db \
  -n "workspace_abc" /backups/workspace_abc_20260107.dump
```

This is **natively supported** by PostgreSQL `pg_dump -n` and `pg_restore -n`. No custom tooling needed. ([Source](https://nicolaiarocci.com/pg_dump-and-pg_restore-can-backup-and-restore-single-postgres-schemas/))

---

## 8. Migration Paths

### A → B (5 clients on VPS-per-client → consolidate to shared cluster)

**Feasibility:** Moderate complexity. One-time migration per client.

**Steps:**
1. Spin up shared PostgreSQL cluster with Twenty in multiworkspace mode.
2. For each client VPS:
   a. `pg_dump` the client's database.
   b. Create workspace in shared cluster (auto-creates schema).
   c. Restore only data tables (not DDL — schema already created by Twenty): `pg_restore --data-only`.
   d. Validate row counts and data integrity.
   e. Update DNS subdomain to point to shared VPS.
   f. Verify login and data.
   g. Decommission client VPS.
3. Each migration window: ~30–60 min per client; can be done one at a time with zero downtime for other clients.

**Risk:** Schema version must be identical across all source stacks and the target cluster. If client VPS stacks are at different Twenty versions, migrate to the same version first.

---

### B → A (extract one regulated client from shared cluster to dedicated VPS)

**Feasibility:** Straightforward. Schema-per-tenant makes this clean.

**Steps:**
1. Spin new Docker Compose stack for the client.
2. `pg_dump -n workspace_<id>` from shared cluster.
3. `pg_restore` into new isolated DB (with DDL — schema is self-contained).
4. Update DNS. Validate.
5. Delete schema from shared cluster after cutover.

**Time:** 1–2 hours per client. Entirely scripted.

---

### C → A or B (hypothetical: if someone started with shared workspace_id)

**Feasibility:** Painful. Requires extracting per-workspace rows from every table, inserting into a new schema or database. No native `pg_dump` support. Custom ETL script needed for every table. Risk of foreign key ordering issues. **This is the reason to avoid model C from the start.**

---

## 9. Citus Guide — Key Applicable Lessons

The Citus multi-tenant documentation is written for scale-out scenarios (1,000+ tenants on distributed Postgres). Its lessons applicable to leCRM at 5–20 tenants:

1. **Include tenant ID in every primary key** even in schema-per-tenant models — makes future sharding trivial.
2. **Co-locate all tenant data** — in schema-per-tenant this is automatic (everything in the schema).
3. **JSONB for custom fields** — Twenty already uses this for custom objects and fields; validated by Citus docs as the idiomatic approach.
4. **Isolate large tenants** — if one client has 10× the data of others, they may warrant their own VPS (model A) while smaller clients stay on the shared cluster.
5. **`pg_stat_statements` per tenant** — use `application_name` or role attribution for per-workspace query attribution.

Source: [Citus Multi-Tenant Applications docs](https://docs.citusdata.com/en/stable/use_cases/multi_tenant.html)

---

## 10. Recommendation: leCRM Phase 1 / 2 / 3

### Phase 1 (Months 1–6: 1–3 clients)

**Recommended model: A (VPS-per-client)**

At 1–3 clients, the operational overhead of running separate stacks is manageable by a solo operator (3 compose stacks is not burdensome). The benefits:

- Zero risk of data cross-contamination during early, error-prone development.
- Each client can be on a slightly different twenty version while you stabilize the fork.
- Strong GDPR story for early enterprise or regulated prospects — physical isolation is unambiguous in a DPA.
- Per-client `pg_dump` backup is trivially simple.
- If a client churns, deprovision is a single `docker compose down`.

Estimated infra: 3 × €8/mo = €24/mo. Acceptable for early ARR.

**Watch-out:** Do not let more than 5 clients accumulate on model A without starting the migration to B. The operational debt compounds quickly at 6–10 stacks.

---

### Phase 2 (Months 6–14: up to 10 clients)

**Recommended model: B (Shared cluster, schema-per-tenant)**

This is **Twenty's native model** — zero fork delta. Flip `IS_MULTIWORKSPACE_ENABLED=true`, configure Traefik subdomain routing on the shared VPS, and all provisioning becomes a single API call.

Key setup tasks for phase 2:
1. Migrate phase 1 clients (3 stacks → shared cluster). ~2h per client.
2. Configure PgBouncer in **session mode** (safe with search_path; adequate at 10 clients).
3. Set per-workspace PostgreSQL roles with `CONNECTION LIMIT 10`, `statement_timeout = '30s'`, `work_mem = '16MB'`.
4. Automate nightly `pg_dump -n workspace_<id>` per workspace to object storage (S3/R2).
5. Set `application_name` per workspace for `pg_stat_activity` monitoring.

For any client requiring physical isolation (regulated sector, explicit contractual demand), keep or migrate them to model A. This creates a **tiered offering**: standard = shared cluster; premium/enterprise = dedicated VPS.

---

### Phase 3 (Months 14–24: up to 20 clients)

**Continue model B; evaluate PgBouncer transaction mode.**

At 20 clients with 3–15 users each, peak connections are well within a shared PgBouncer pool in session mode. The catalog size (20 schemas × ~100 tables = 2,000 table entries) is negligible.

Investments for phase 3:
1. Upgrade to PgBouncer 1.20+ and enable `track_extra_parameters = search_path` — enables transaction pooling safely with schema-per-tenant.
2. Consider read replica for reporting queries to avoid noisy-neighbor on analytics.
3. Implement per-workspace `pg_stat_statements` dashboard (Grafana or Metabase).
4. Review whether any 2–3 clients warrant dedicated VPS upgrade (data volume, compliance need, or revenue justifying premium tier).

If a client demands SOC 2 or ANSSI-level physical isolation, model A remains the answer. The schema-per-tenant architecture makes extraction trivial.

---

## Items to Resolve (TO RESOLVE)

1. **PgBouncer version on target VPS.** Confirm version supports `track_extra_parameters` (requires 1.20+). If not, pin to session pooling for now.
2. **Twenty fork migration version parity.** Before consolidating clients to shared cluster, ensure all stacks are at the same Twenty version. Document upgrade runbook.
3. **CNIL DPA template.** Draft Article 28 language that explicitly describes the schema-per-tenant logical isolation model — "données de chaque client isolées dans un schéma PostgreSQL dédié, inaccessible aux autres clients au niveau applicatif et base de données." No auditor obligation requires physical isolation, but the DPA must name the mechanism.
4. **Backup encryption.** `pg_dump` outputs must be encrypted at rest before shipping to object storage. Confirm GPG or age encryption in backup script.
5. **Connection limit arithmetic.** Validate: 20 clients × 10 connections (session pool) = 200 connections. PostgreSQL `max_connections = 250` or 500 on the shared VPS. Ensure headroom.
6. **Subdomain wildcard SSL.** Confirm Traefik + Let's Encrypt wildcard cert (`*.lecrm.fr`) or per-subdomain cert automation is in place before phase 2 onboarding.
7. **Twenty migration runner per-schema.** Audit Twenty's upgrade process for handling existing workspaces — confirm it iterates all `workspace_*` schemas on `twenty-server database:migrate`. Test with a 2-workspace scenario before production cutover.

---

## Sources

- [Twenty CRM source — workspace-datasource.service.ts](https://github.com/twentyhq/twenty/blob/main/packages/twenty-server/src/engine/workspace-datasource/workspace-datasource.service.ts)
- [Twenty CRM source — get-workspace-schema-name.util.ts](https://github.com/twentyhq/twenty/blob/main/packages/twenty-server/src/engine/workspace-datasource/utils/get-workspace-schema-name.util.ts)
- [Citus Multi-Tenant Applications](https://docs.citusdata.com/en/stable/use_cases/multi_tenant.html)
- [Neon Multitenancy Guide](https://neon.com/docs/guides/multitenancy)
- [Neon — Noisy Neighbor in Multitenant](https://neon.com/blog/noisy-neighbor-multitenant)
- [Crunchy Data — Designing Postgres for Multi-tenancy](https://www.crunchydata.com/blog/designing-your-postgres-database-for-multi-tenancy)
- [Arkency — What Surprised Us in Postgres Schema Multitenancy](https://blog.arkency.com/what-surprised-us-in-postgres-schema-multitenancy/)
- [PgBouncer Transaction Pooling: The Multi-Tenant Nightmare](https://blog.sagarregmi.info.np/transaction-pooling-the-multi-tenant-nightmare)
- [PgBouncer — track_extra_parameters support (Citus announcement)](https://www.citusdata.com/blog/2024/04/04/pgbouncer-supports-more-session-vars/)
- [Thomas Vanderstraeten — Schema-based multitenancy NestJS + TypeORM](https://thomasvds.com/schema-based-multitenancy-with-nest-js-type-orm-and-postgres-sql/)
- [Avallone-io/rls — TypeORM RLS package](https://github.com/Avallone-io/rls)
- [CNIL — Sécurité Cloud informatique en nuage](https://www.cnil.fr/fr/securite-cloud-informatique-en-nuage)
- [CNIL — Recommandations pour services Cloud (PDF)](https://www.cnil.fr/sites/default/files/typo/document/Recommandations_pour_les_entreprises_qui_envisagent_de_souscrire_a_des_services_de_Cloud.pdf)
- [Nicolaiarocci — pg_dump per-schema backup](https://nicolaiarocci.com/pg_dump-and-pg_restore-can-backup-and-restore-single-postgres-schemas/)
- [Supabase Architecture Docs](https://supabase.com/docs/guides/getting-started/architecture)
- [Adiagr.com — SaaS Postgres Multitenancy Patterns](https://www.adiagr.com/blog/07-saas-postgres-multitenancy-patterns/)
- [Twenty CRM — Multi-Workspace Discussion #5685](https://github.com/twentyhq/twenty/discussions/5685)
