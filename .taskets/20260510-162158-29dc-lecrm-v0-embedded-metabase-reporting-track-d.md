---
id: 20260510-162158-29dc
title: leCRM v0 — Cube.dev backend (per-workspace RO role + signed embed)
status: done
priority: p1
created: 2026-05-10
updated: 2026-05-28
done: 2026-05-28
tags: [reporting, cube-dev, backend, v0]
category: project
group: lecrm-v0-sprint-12
group_order: 12
order: 1
plan: true
---

# leCRM v0 — Cube.dev backend (per-workspace RO role + signed embed)

## Why

v0 needs a per-client reporting surface. ADR-009 §9 names Cube.dev as the bridge: semantic layer (define `measures`/`dimensions` once, reuse across dashboards) + JWT `securityContext` carrying `workspace_id` + LLM-ready for v2 "ask your CRM". This tasket is the **backend half** of sprint-12; the React route + dashboard configs land in `12b`.

## Done criteria

- [ ] **Per-workspace read-only Postgres role** `workspace_<id>_ro` with `GRANT SELECT` on the workspace schema. Created via an extension to `core.lecrm_provision_workspace` (SECURITY DEFINER, `search_path=core,pg_catalog` per migration 0006). New migration `packages/db/migrations/0013_workspace_ro_role.sql`.
- [ ] **Cube.dev container** in `deploy/compose/cube.yml`, ~512 MB memory, image `cubejs/cube:latest`. Connects to Postgres as `workspace_<id>_ro` (one connection per workspace, picked at JWT verification time).
- [ ] **Cube schema** in `deploy/cube/schema/` defining measures + dimensions over: `deals`, `contacts`, `companies`, `activities`. Includes `deal_stage` dimension (depends on tasket 20260525-1005 having shipped — verify first).
- [ ] **Signed embed JWT endpoint** `POST /api/v1/reports/embed-token` on `apps/api`: returns short-lived (5 min TTL) JWT with claims `{workspace_id, exp, aud:"cube"}`, signed with `LECRM_CUBE_JWT_SECRET`. Workspace-scoped resolver — 403 if caller doesn't belong to the workspace.
- [ ] **Audit event** `report.embed_token.issued` with `workspace_id` + `actor_id` written to `core.audit_log` on every token mint.
- [ ] **Tests**: provisioning function migration smoke test (RO role can SELECT, cannot UPDATE); embed-token handler tests (happy path, cross-workspace 403, expired token rejected by Cube container in an integration test).

## Pre-flight (verify before starting)

1. `ls packages/db/migrations/0011_external_sync.sql` — sync seam migration present.
2. `git log --oneline -20 | grep -i "deal.*stage\|pipeline"` — tasket 20260525-1005 has shipped (deal-stage domain exists).
3. If #2 fails, STOP and report. Do not proceed — Cube schema's `deal_stage` dimension requires the domain.

## Out of scope (deferred to 12b)

- React `reports/$workspaceId.tsx` route.
- Cube.dev dashboard configs (deals by stage, deals by owner, recent activities, conversion funnel).

## References

- ADR-009 §9 (Cube.dev as v0 dashboard bridge).
- ADR-001 (workspace schema isolation).
- Tasket 20260525-1005 (deal-stage domain — hard dependency).
