# ADR-001 — Tenancy Model: VPS-per-Client → Schema-per-Tenant

**Status:** Accepted (amended 2026-05-10 by ADR-009 — three sections superseded; see banner below)
**Date:** 2026-05-10
**Deciders:** Guillaume

> **2026-05-10 amendment from [ADR-009](ADR-009-stack-and-license.md).** Three sections of this ADR are now superseded by ADR-009's clean-room stack reframe:
>
> 1. **"Tenant boundary security" — `SET LOCAL search_path` in TypeORM's queryRunner** is **[SUPERSEDED by ADR-009 §2.1](ADR-009-stack-and-license.md): per-workspace Postgres role with `ALTER ROLE workspace_<id> SET search_path = workspace_<id>, public` set at provisioning time, via a single `SECURITY DEFINER` function `lecrm_provision_workspace(uuid)`. The application connects AS the workspace role; `search_path` is inherited from the role, mode-agnostic.**
> 2. **"Operational specifics" — phase-3 PgBouncer 1.20+ `track_extra_parameters = search_path` transaction-mode plan** is **[SUPERSEDED by ADR-009 §2.2](ADR-009-stack-and-license.md): the `ALTER ROLE` pattern eliminates the PgBouncer-version dependency and is mode-agnostic.**
> 3. **TO RESOLVE item 1 (verify PgBouncer ≥1.20 for `track_extra_parameters`)** is **[CLOSED — obsolete under ADR-009 §2 ALTER ROLE pattern]**. Replacement TO RESOLVE under ADR-009 TO RESOLVE-13: verify Ubicloud Standard-2 PgBouncer config uses `auth_query` mode (not static `auth_file`) before Phase-2 cut-over.
>
> The "fork of Twenty" framing in §Context below is also superseded by ADR-008 (clean-room reimplementation). The schema-per-tenant primitive (`workspace_<base36(uuid)>`) is read from Twenty as architectural reference, not inherited as code.

---

## Context

leCRM serves French/EU SMBs (3–15 users per client) on a forked Twenty CRM (NestJS + PostgreSQL). The tenancy decision is the foundation that every other architectural choice depends on: it sets the blast radius of bugs, the GDPR posture toward auditors, the operational cost per client, and the per-tenant restore mechanics.

Three options exist on paper:
- **A — VPS-per-client:** one Docker Compose stack per client. Maximum physical isolation, maximum operational cost per client.
- **B — Shared cluster, schema-per-tenant:** one PostgreSQL cluster, one schema per workspace. This is **Twenty's actual native model** — confirmed by reading `packages/twenty-server/src/engine/workspace-datasource/utils/get-workspace-schema-name.util.ts` in the upstream repo, which constructs schema names as `workspace_${uuidToBase36(workspaceId)}` and calls `queryRunner.createSchema(schemaName, true)` at workspace signup. See `docs/research/multi-tenant-postgres-patterns.md` §2.
- **C — Shared schema with `workspace_id` discriminator:** Twenty does **not** use this pattern. Adopting it would require forking Twenty's data layer and offers the weakest isolation. Rejected without further consideration.

A factual correction in the brief was that Twenty was thought to use the shared-schema-with-discriminator pattern. Reading the source disproved this. Twenty's `workspace_id` lives in the `core.workspace` identity table; per-client data lives in a dedicated PostgreSQL schema. This is a critical correction because it changes the calculus: model B does not require a fork delta, model C does. (`docs/research/multi-tenant-postgres-patterns.md` §2.)

The leCRM constraints that drive the decision:

- **Solo operator with AI-augmented dev velocity.** Operability matters more than feature breadth. Multi-stack ops at >5 clients consumes ops time at a rate that compounds against scaling.
- **EU data residency and GDPR Art. 28 defensibility.** CNIL guidance does not mandate physical isolation; logical isolation is acceptable if explicitly described in the DPA (`docs/research/multi-tenant-postgres-patterns.md` §5).
- **Cost-per-client at scale <20% of MRR through phase 3.**
- **First Design Partner live in 4–6 weeks.** v0 must ship before the multi-tenant question is solved end-to-end.

---

## Decision

leCRM adopts a **two-phase tenancy model with pure consolidation**.

### Phase 1 (≤4 clients): VPS-per-client (Model A)

Each client runs on a dedicated Hetzner CX21 (or equivalent) with the full stack as a Docker Compose deployment. Twenty runs in single-workspace mode (`IS_MULTIWORKSPACE_ENABLED=false`). Backups, secrets, upgrades, and monitoring are per-VPS.

### Phase 2 (5+ clients): Shared cluster, schema-per-tenant (Model B)

When the fifth client signs, leCRM consolidates **every** existing client to a single shared cluster running Twenty in multiworkspace mode (`IS_MULTIWORKSPACE_ENABLED=true`). No premium "dedicated VPS" tier remains in phase 2 — pure consolidation. Twenty's native schema-per-workspace model (`workspace_<base36(uuid)>`) provides the tenant boundary.

Pure consolidation is binding. Tiered offerings (some clients on dedicated VPS, others on shared) double operational overhead and consume the margin consolidation buys. If a regulated prospect demands physical isolation in phase 2+, they are a non-fit and are politely declined or referred. A future "leCRM Sovereign" SKU may reintroduce dedicated infrastructure but is out of scope for this ADR.

### Operational specifics

**PgBouncer configuration (phase 2):**
- Pool mode: **session** (initial). Session mode preserves `search_path` across the connection lifecycle; transaction mode does not, and `search_path` leakage between transactions is the single biggest operational hazard of schema-per-tenant (`docs/research/multi-tenant-postgres-patterns.md` §3 — "PgBouncer + search_path interaction (Critical)").
- Pool size per database: 25.
- Per-role connection limit: 10 (`ALTER ROLE workspace_<id> CONNECTION LIMIT 10`).
- Per-role `statement_timeout = '30s'`, `work_mem = '16MB'`, `lock_timeout = '5s'`.

**PostgreSQL sizing (phase 2):**
- `max_connections = 250`.
- Sizing arithmetic: 10 clients × 10 conns + worker pool (20) + headroom = 130; comfortably under 250.
- At phase 3 (20 clients): 20 × 10 + 20 + headroom = 220; still under 250 but with less margin → upgrade to PgBouncer 1.20+ with `track_extra_parameters = search_path` and switch to transaction pooling, which collapses real connection count. **[SUPERSEDED by [ADR-009](ADR-009-stack-and-license.md) §2.2: the `ALTER ROLE` pattern is mode-agnostic and eliminates this PgBouncer-version dependency. CVE-2025-12819 (December 2025) also affected `track_extra_parameters = search_path`; the role-level pattern is CVE-clean as a side effect.]**

**Migration trigger:** the **fifth signed client** triggers consolidation, OR (whichever comes first) ops time spent on multi-stack maintenance crosses 4 h/week. Both signals point at the same operational tipping point.

### Phase 1 → Phase 2 migration runbook (per client)

1. Pre-condition check: source VPS Twenty version equals target shared cluster Twenty version. If not, upgrade source VPS first (per [ADR-002](ADR-002-twenty-fork-management.md)).
2. `pg_dump -Fc -h <source-vps> -U twenty -d twenty_db > /tmp/<client>.dump`.
3. Provision workspace on shared cluster via Twenty signup API. This auto-creates `workspace_<base36>` and runs standard-object migrations.
4. `pg_restore --data-only -n workspace_<base36> -d twenty_db /tmp/<client>.dump` (DDL is already in place from step 3).
5. Run row-count + checksum diff against source VPS (`SELECT COUNT(*) FROM ... ; SELECT md5(string_agg(...)) FROM ...`).
6. Cloudflare API call: update CNAME `<client>.lecrm.fr` → shared Edge VPS Floating IP.
7. Smoke test: login, sample read, sample write. Sequences and AI features (if enabled) get their own smoke step.
8. 7-day quiescence on the source VPS (still reachable but not authoritative; DNS already cut over).
9. Decommission source VPS after quiescence.

Per-client window: 30–60 min, with the only client-visible interruption being the DNS cut-over (~2 min). Other clients are unaffected throughout.

### Backup mechanics by phase

- **Phase 1:** per-VPS WAL-G with GPG client-side encryption to Hetzner Object Storage. Per-client S3 prefix `s3://lecrm-wal/<client-slug>/`. Restore is trivially per-client. ([ADR-006](ADR-006-backup-dr.md))
- **Phase 2:** WAL-G on the shared cluster + nightly `pg_dump -n workspace_<id>` per workspace as a surgical-restore supplement. Per-tenant restore: `pg_restore -n workspace_<id>` into the live cluster after `DROP SCHEMA "workspace_<id>" CASCADE`. Native PostgreSQL semantics; no custom tooling. ([ADR-006](ADR-006-backup-dr.md))

### Tenant boundary security

> **[SUPERSEDED by [ADR-009](ADR-009-stack-and-license.md) §2.1]** — the `SET LOCAL search_path` / TypeORM mechanism described below is replaced by per-workspace Postgres role + role-level `ALTER ROLE search_path` set at provisioning. Read the bullets below as historical context; the current mechanism is in ADR-009.

- Schema isolation enforced at every connection by `SET LOCAL search_path = workspace_<id>` in TypeORM's queryRunner (Twenty's existing behaviour). **[SUPERSEDED — current mechanism is per-workspace Postgres role + `ALTER ROLE workspace_<id> SET search_path = workspace_<id>, public`. See ADR-009 §2.1.]**
- PgBouncer in session mode preserves the path across requests on the same connection.
- Per-role PostgreSQL `CONNECTION LIMIT` and `statement_timeout` provide noisy-neighbour mitigation (`docs/research/multi-tenant-postgres-patterns.md` §6).
- `application_name = 'workspace_<id>'` on every connection enables per-tenant filtering in `pg_stat_activity` and `pg_stat_statements` for monitoring.

---

## Consequences

### Positive

- v0 ships fast: phase 1 is operationally simple and the GDPR story for early prospects is unambiguous (physical isolation).
- Phase 2 inherits Twenty's native data-layer model: zero fork delta on the tenancy mechanism. Less code to maintain, fewer divergences to rebase against upstream.
- Schema-per-tenant gives surgical per-tenant backup/restore via native `pg_dump -n workspace_<id>` and `pg_restore -n` — no custom ETL.
- Cost-per-client trajectory holds the <20%-of-MRR target through phase 3 (see ARCHITECTURE.md §7.3).
- Migration B → A is straightforward if a regulated client later upgrades to a dedicated environment: schema-per-tenant makes extraction clean.

### Negative

- The Phase 1 → Phase 2 migration is real engineering work. Per-client windows of 30–60 min × number of clients. Mitigated by automating the runbook end-to-end before the fifth client signs.
- PgBouncer session mode reduces pooling efficiency vs transaction mode. Acceptable at ≤10 clients; at ≤20 clients we transition to PgBouncer 1.20+ transaction mode with `track_extra_parameters = search_path` (see `docs/research/multi-tenant-postgres-patterns.md` §3).
- Pure consolidation refuses regulated-isolation prospects in phase 2. Revenue lost is bounded; ops cost saved is unbounded.
- Per-tenant DDL changes (custom objects, custom fields) must run schema-by-schema. Twenty's `WorkspaceManagerService` already iterates workspaces during upgrades, but partial-failure handling needs a tested rollback path before the first phase 2 upgrade.

### Neutral

- All client subdomains share an SSL wildcard (`*.lecrm.fr`) in phase 2. SSL ops simplify but a wildcard cert leak affects every workspace; mitigated by Cloudflare-managed origin cert + Let's Encrypt automation, and by storing the cert in Vault (v1+).
- Cross-tenant analytics (e.g., aggregated usage telemetry) require explicit `UNION ALL` across schemas — acceptable at ≤20 clients.

---

## Alternatives Considered

### Alt 1: Shared cluster from day one (skip phase 1)

Rejected because v0 must ship in 4–6 weeks. Schema-per-tenant requires Traefik subdomain routing, PgBouncer config, per-role tuning, the migration runbook tested against a real workspace, and per-tenant backup tooling. None of this is the 4-week critical path. Phase 1 is operationally heavier per-client but architecturally simpler — the right trade for ≤4 clients.

### Alt 2: Stay on VPS-per-client forever

Rejected because at 20 clients the ops time would exceed the time available from a solo operator. 20 stacks × 1 h/month per stack for routine maintenance = 20 h/month, which is half a working week before any feature work. At >5 clients, consolidation pays back its migration cost within 2 months.

### Alt 3: Database-per-tenant (one PostgreSQL cluster, N databases)

Rejected. Schema-per-tenant is operationally equivalent at 5–20 clients with strictly less connection-pool overhead. Database-per-tenant multiplies pool size by N and offers marginal extra isolation that isn't worth the cost (`docs/research/multi-tenant-postgres-patterns.md` §4). Neon and Supabase recommend it at scale because their models are different — Neon recommends project-per-tenant for cloud Postgres-as-a-service, and Supabase gives each project a dedicated cluster. Neither maps to leCRM's self-hosted topology.

### Alt 4: Shared schema with `workspace_id` discriminator (model C)

Rejected. Twenty does not use this pattern; adopting it forks the data layer significantly. The blast radius of a missing `WHERE workspace_id = ?` is the entire tenant base. PostgreSQL Row-Level Security can defend against this but is fragile in TypeORM (any raw query bypasses RLS unless explicitly wrapped). The CNIL accepts logical isolation, but model C is the weakest form of logical isolation. (`docs/research/multi-tenant-postgres-patterns.md` §5.)

### Alt 5: Tiered offering (shared cluster + premium dedicated VPS)

Rejected as binding architectural choice. Tiered offerings double the deploy/upgrade pipeline and split the ops surface. The marginal revenue from a "premium dedicated" tier is dwarfed by the ops cost. If a future SKU emerges (leCRM Sovereign), it will be a separate product, not a tier on the same product.

---

## References

- `docs/research/multi-tenant-postgres-patterns.md` (entire document; §2 has the schema-per-workspace correction; §3 covers PgBouncer; §6 covers noisy-neighbour mitigation; §7 covers backup mechanics).
- `docs/research/dr-security.md` §3 (per-client restore granularity in shared cluster).
- Twenty source: [`get-workspace-schema-name.util.ts`](https://github.com/twentyhq/twenty/blob/main/packages/twenty-server/src/engine/workspace-datasource/utils/get-workspace-schema-name.util.ts), [`workspace-datasource.service.ts`](https://github.com/twentyhq/twenty/blob/main/packages/twenty-server/src/engine/workspace-datasource/workspace-datasource.service.ts).
- [Citus multi-tenant patterns](https://docs.citusdata.com/en/stable/use_cases/multi_tenant.html).
- [Crunchy Data — designing Postgres for multi-tenancy](https://www.crunchydata.com/blog/designing-your-postgres-database-for-multi-tenancy).
- [PgBouncer 1.20 `track_extra_parameters` announcement (Citus blog)](https://www.citusdata.com/blog/2024/04/04/pgbouncer-supports-more-session-vars/).
- [CNIL — recommandations cloud (PDF)](https://www.cnil.fr/sites/default/files/typo/document/Recommandations_pour_les_entreprises_qui_envisagent_de_souscrire_a_des_services_de_Cloud.pdf).
- Related ADRs: [ADR-002](ADR-002-twenty-fork-management.md) (fork management — version parity required for phase-1→phase-2 migration), [ADR-006](ADR-006-backup-dr.md) (backup mechanics), [ADR-007](ADR-007-encryption-secrets-audit.md) (LUKS, audit).

---

## TO RESOLVE

1. **PgBouncer version on target VPS.** Verify Hetzner image / Debian APT version is ≥1.20 to enable `track_extra_parameters` at phase 3. If not, pin phase 3 to session mode and budget the upgrade as a separate task. (`docs/research/multi-tenant-postgres-patterns.md` §10 item 1) **[CLOSED 2026-05-10 — obsolete under [ADR-009](ADR-009-stack-and-license.md) §2.2 `ALTER ROLE` pattern. Replacement: ADR-009 TO RESOLVE-13 — verify Ubicloud Standard-2 PgBouncer config uses `auth_query` mode, not static `auth_file`.]**
2. **Twenty migration runner per-schema audit.** Before the phase 1 → phase 2 cut-over of any client, test Twenty's `database:migrate` against a 2-workspace cluster on a staging environment to confirm it iterates `workspace_*` schemas correctly and rolls back cleanly on partial failure. (`docs/research/multi-tenant-postgres-patterns.md` §10 item 7)
3. **Subdomain wildcard SSL** before phase 2 onboarding. Confirm Caddy + DNS-01 challenge against Cloudflare API works for `*.lecrm.fr`, including renewal cron. (`docs/research/multi-tenant-postgres-patterns.md` §10 item 6)
4. **CNIL DPA template wording** that explicitly describes schema-per-tenant logical isolation as the operative mechanism in phase 2+. Draft language: *« les données de chaque client sont isolées dans un schéma PostgreSQL dédié, inaccessible aux autres clients tant au niveau applicatif qu'au niveau base de données. »* This is a legal task tracked in `docs/LEGAL-PLAYBOOK.md` but called out here so the architecture commits to a phrasing the legal posture can defend. (`docs/research/multi-tenant-postgres-patterns.md` §10 item 3)
5. **Connection-limit arithmetic at phase 3 worst case.** Validate `max_connections = 250` against actual phase-3 traffic (20 clients × 15 users active × peak request rate). If insufficient, increase to 500 or migrate to transaction-mode pooling.
