---
id: 20260525-1005-pipeline-kanban-deal-stages
title: "Pipeline Kanban skeleton + Deal stage transitions"
status: todo
priority: p1
created: 2026-05-25
category: project
group: crm-crud-complete
group_order: 60
order: 3
plan: true
tags: [crm, pipeline, kanban, frontend, dnd-kit, sprint-6-8]
---

# Pipeline Kanban skeleton + Deal stage transitions

## Pre-flight: Verify Previous Taskets

Before starting, verify entity handlers exist:

1. `ls apps/api/internal/http/deals.go` -- deal handler exists
2. `ls apps/web/src/routes/deals/index.tsx` -- deals route exists
3. `git log --oneline -20 | grep -i "deal\|REST"` -- handler commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

Feature 2 of v0: Pipeline Kanban view. The `pipeline_stages` table already exists (seeded with 5 defaults: Discovery, Qualified, Proposal Sent, Negotiation, Closed-Won/Lost in migration 0004). Deals already have a `stage_id` FK. This tasket adds the stage transition endpoint and the drag-and-drop Kanban frontend.

Spans Sprint 6 (skeleton) through Sprint 8 (complete).

Source of truth: `docs/sprint-plan.md` Sprint 6 + Sprint 8
Working directory: `/home/gui/Projects/leCRM`

## Steps

1. Backend — Pipeline stages endpoint:
   - `GET /v1/pipeline/stages` — returns all stages for the workspace, ordered by `order_index`
   - Read from `workspace_<uuid>.pipeline_stages` (already exists)

2. Backend — Deal stage transition:
   - `PATCH /v1/deals/:id/stage` — body: `{stage_id: uuid}`
   - Validates stage_id exists in pipeline_stages
   - Updates deal's stage_id
   - Creates activity: `deal.stage_changed` with `{old_stage, new_stage, old_stage_name, new_stage_name}`
   - Audit-logged (fail-closed)

3. Backend — Deals grouped by stage:
   - `GET /v1/pipeline/board` — returns `{stages: [{id, name, order_index, deals: [...]}]}`
   - Or: frontend fetches stages + deals separately and groups client-side (simpler, avoids complex query)

4. Frontend — Install DnD Kit:
   - `cd apps/web && pnpm add @dnd-kit/core @dnd-kit/sortable @dnd-kit/utilities`

5. Frontend — Pipeline Kanban component:
   - `apps/web/src/routes/deals/index.tsx` — or a dedicated `/pipeline` route
   - Columns: one per pipeline stage, ordered by order_index
   - Deal cards: title, amount, contact name, expected close date
   - Drag-and-drop: drag a deal card between columns
   - On drop: call `PATCH /v1/deals/:id/stage` with the target stage_id
   - Optimistic update: move card immediately, revert on API error
   - Empty column state

6. Frontend — Deal card component:
   - Compact card showing key deal info
   - Click to navigate to deal detail page
   - Visual indicator for overdue deals (expected_close_date < today)

7. Tests:
   - Backend: stage transition creates correct activity entry
   - Backend: invalid stage_id returns 400
   - Backend: cross-tenant isolation (can't move deal to another workspace's stage)
   - Frontend: `pnpm typecheck` and `pnpm build` pass

## Done When

- [ ] Pipeline stages endpoint returns workspace stages
- [ ] Deal stage transition endpoint works with audit + activity logging
- [ ] Kanban board renders stages as columns with deal cards
- [ ] Drag-and-drop moves deals between stages (calls API)
- [ ] Optimistic UI update on drag (revert on error)
- [ ] Stage transition creates activity log entry
- [ ] `pnpm typecheck` and `pnpm build` pass
- [ ] Backend tests pass

## Completion Verification

1. `grep -c 'stage' apps/api/internal/http/deals.go` -- stage transition handler present
2. `grep -c 'dnd-kit' apps/web/package.json` -- DnD Kit installed
3. `cd apps/web && pnpm build` -- frontend builds
4. `cd apps/api && go test -race -count=1 ./...` -- backend tests pass
5. Commit: `feat(pipeline): Kanban board with drag-and-drop deal stage transitions (Sprint 6-8)`

## References

- `packages/db/migrations/0004_workspaces_admin_email_registry.sql` — pipeline_stages table + seed
- `apps/api/internal/http/deals.go` — deal handlers (from previous tasket)
- `docs/sprint-plan.md` Sprint 6 (skeleton) + Sprint 8 (complete)
- DnD Kit docs: https://dndkit.com
