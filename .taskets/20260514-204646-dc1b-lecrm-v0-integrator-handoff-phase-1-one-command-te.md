---
id: 20260514-204646-dc1b
title: "leCRM v0 — Integrator handoff Phase 1: one-command tenant provisioning CLI (Sprint 8)"
status: done
priority: p1
created: 2026-05-14
updated: 2026-05-25
tags: [integrator-handoff, distribution-moat, cli, sprint-8]
category: engineering
group: lecrm-v0-sprint-8
group_order: 8
order: 1
plan: true
done: 2026-05-25
---

## Read this cold — full context inline

Split-out **Phase 1** of the integrator-handoff Distribution-moat work. Originally bundled in tasket `20260514-114231-8a67` (now superseded) with Phase 2 (Sprint 9) and Phase 3 (Sprint 11). Per sprint plan §Sprint 8 (Wk 8), this is the first capability to ship.

## Why this exists

The PRD Executive Summary names **Distribution — Léo's accumulated HubSpot-buyer market intelligence and qualified pipeline** as the 4th moat — the only moat component that cannot be reproduced by a capitalized competitor in 18 months. If Léo is the channel, the codebase must protect HIS unit of work: the integrator handoff.

Round-2 council (Winston, 2026-05-14): "Tenant provisioning has to be a one-command operation, not a checklist."

## Prerequisite (DOR)

- Tenant model committed — schema-per-tenant scaffolding live (b844 Part B — done).
- `lecrm_provision_workspace(uuid)` SECURITY DEFINER function in `packages/db/migrations/0001_init.sql` (committed `63be520`).
- Multi-user RBAC implementation underway (Sprint 8 sibling work — feature 7 per PRD Executive Summary).

## Approach — Capability 1: One-command tenant provisioning

Build (or formalize from scaffolding) a CLI/script that takes minimal input — tenant slug, admin email, Léo-as-creator metadata — and provisions:

- Postgres schema + per-workspace role (via `lecrm_provision_workspace`)
- Default roles for multi-user RBAC
- Default pipeline structure (configurable later via Phase 2 methodology config)
- Audit logging enabled
- OAuth client placeholder
- Default methodology config slot (filled in Phase 2)

**Target:** `lecrm tenant create <slug> --admin <email> [--template <name>]` (or equivalent), **≤30 seconds end-to-end**. NOT a Notion checklist Léo follows by hand.

## Done When

- [ ] `lecrm tenant create` CLI subcommand implemented in `apps/api/cmd/lecrm-api/` (or a dedicated `cmd/lecrm-admin/`)
- [ ] Single command end-to-end ≤30s against a fresh Postgres
- [ ] Documented in `apps/README.md` or a dedicated `docs/integrator-handoff.md`
- [ ] Demoed against a throwaway test tenant; output verified (role exists, schema exists, default privileges set, audit log enabled)
- [ ] Cross-link: Phase 2 (Sprint 9 — methodology config) consumes the tenant produced here

## References

- Supersedes Capability 1 of `20260514-114231-8a67` (integrator handoff parent)
- Sibling Phase 2 (Sprint 9 — methodology config), Phase 3 (Sprint 11 — audit surface)
- `docs/sprint-plan.md` §Sprint 8 (Wk 8) — RBAC + Pipeline Kanban + integrator handoff Phase 1
- `docs/adr/ADR-009-stack-and-license.md` §2.1 — `lecrm_provision_workspace` function
- `${PAI_DATA_DIR}/35_CLIENTS/Leo/README.md` — Léo's integrator methodology reference
