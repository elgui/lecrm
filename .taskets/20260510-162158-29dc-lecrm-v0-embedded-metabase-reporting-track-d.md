---
id: 20260510-162158-29dc
title: leCRM v0 — Embedded Metabase reporting (Track D)
status: later
priority: p2
created: 2026-05-10
updated: 2026-05-10
tags: [reporting, metabase, v0]
category: project
group: lecrm-v0-build
order: 2
plan: true
---

# leCRM v0 — Embedded Metabase reporting (Track D)

## Why this tasket exists
v0 needs a basic per-client reporting surface (deal counts, activity volume, conversion funnels) without building it from scratch. Self-hosted Metabase pointed at Twenty's Postgres with workspace-scoped SQL queries, embedded via iframe in a Twenty extension, is the lowest-effort bridge.

Reference: FEASIBILITY-MEMO §3 Track D, ARCHITECTURE.md §6.1.

## Done criteria
- [ ] Metabase container added to the per-client docker-compose template (memory limit 512M).
- [ ] Postgres read-only role per workspace (`workspace_<id>_readonly`).
- [ ] Metabase auto-bootstrap script: creates a baseline dashboard from the Twenty schema (deals by stage, deals by owner, recent activities, conversion funnel).
- [ ] Twenty extension package mounting the Metabase iframe inside a "Reports" tab; signed embed URL with workspace_id parameter.
- [ ] Cube.dev replacement is v1+ — explicitly out of scope here.
