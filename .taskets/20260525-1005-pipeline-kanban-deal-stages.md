---
id: 20260525-1005-pipeline-kanban-deal-stages
title: Pipeline Kanban skeleton + Deal stage transitions
status: done
priority: p1
created: 2026-05-25
updated: 2026-05-28
tags: [crm, pipeline, kanban, frontend, dnd-kit, sprint-6-8]
category: project
group: crm-crud-complete
group_order: 60
order: 3
plan: true
done: 2026-05-28
---

# Pipeline Kanban skeleton + Deal stage transitions

Operator-facing Kanban UX for moving deals across pipeline stages, plus the
HTTP endpoints that back it. Independent of the Cube reports track (sprint-12,
already shipped): Cube reads `deals.stage_id` and `pipeline_stages` directly,
so this tasket is **not** a blocker for the Reports route. Once it ships,
stage transitions surface immediately in the `deals-by-stage` and
`conversion-funnel` dashboards because they write to the same DB tables.

Working directory: `/home/gui/Projects/leCRM`

## Pre-flight

Run from repo root. ALL must pass; STOP and report if any fail.

```bash
# Deal CRUD handler exists in the CRM package
test -f apps/api/internal/crm/handlers.go && \
  grep -q "func (h \*Handler) ListDeals" apps/api/internal/crm/handlers.go

# Deals list route exists on the frontend
test -f apps/web/src/routes/deals/index.tsx

# Pipeline-stages DB artefacts exist (migrations 0004 + 0008)
grep -q "CREATE TABLE IF NOT EXISTS %I.pipeline_stages" \
  packages/db/migrations/0004_workspaces_admin_email_registry.sql
grep -q "stage_id" packages/db/migrations/0008_crm_entities.sql

# Cube schema expects `Activities` rows on the `objects` table with
# object_type='activity'. Confirm the contract before we write.
grep -q "object_type = 'activity'" deploy/cube/schema/Activities.js

# Build & tests are green on main before we start
export PATH=$PATH:/usr/local/go/bin
(cd apps/api && go build ./... && go test ./...)
```

## Context

- **DB tables already shipped:**
  - `pipeline_stages` (per-workspace schema) — migration `0004` seeds 5 default
    stages for the `gbconsult-default` template:
    Discovery, Qualified, Proposal Sent, Negotiation, Closed-Won/Lost.
  - `deals.stage_id uuid` — migration `0008` (`apps/api/internal/sqlcgen/models.go`
    already exposes `PipelineStage`).
- **HTTP scaffolding:** all `/v1/deals/*` routes are mounted via
  `crm.Handler.RegisterRoutes` (`apps/api/internal/crm/handlers.go:34`).
  Add the new routes there, not under `apps/api/internal/http/`.
- **Activity log:** no dedicated `activities` table yet. The agreed v0
  shape lands as a row in the workspace `objects` table with
  `object_type='activity'`, `parent_type='deal'`, `parent_id=<deal_id>`,
  and `data` matching the JSONB contract in `deploy/cube/schema/Activities.js`.
  See `apps/api/internal/metadata/set.go:118` for the insert pattern.
- **Frontend tool:** the web app uses **bun** (`apps/web/package.json`),
  not pnpm. Run `bun add` / `bun run` / `bun test`.
- **Sprint-12 status:** Cube backend (`040558a6`) and frontend (`718b16ba`)
  shipped. This tasket is purely additive — no Cube changes required.

## Steps

### 1. Backend — SQL queries (`packages/db/queries/pipeline_stages.sql`, new file)

Add three named queries (search_path is set to `workspace_<uuid>` by
the middleware, so table names are unqualified — see `deals.sql` for the
established style):

```sql
-- name: ListPipelineStages :many
SELECT id, name, order_index, created_at
FROM pipeline_stages
ORDER BY order_index ASC, name ASC;

-- name: GetPipelineStage :one
SELECT id, name, order_index, created_at
FROM pipeline_stages
WHERE id = $1;

-- name: UpdateDealStage :one
UPDATE deals
SET stage_id   = sqlc.arg('stage_id')::uuid,
    updated_at = now()
WHERE id = sqlc.arg('id')::uuid
RETURNING id, title, amount, currency, stage_id, contact_id, company_id,
          owner_id, expected_close_date, closed_at, created_at, updated_at;
```

Regenerate sqlc bindings:

```bash
(cd apps/api && sqlc generate)
```

### 2. Backend — Handlers (`apps/api/internal/crm/handlers.go`)

Add three handlers next to the existing deal handlers and wire them in
`RegisterRoutes` (line 34):

| Route                             | Handler              | Notes |
|-----------------------------------|----------------------|-------|
| `GET    /v1/pipeline/stages`      | `ListPipelineStages` | Reads via `readTx` + `sqlcgen.New(tx).ListPipelineStages` |
| `PATCH  /v1/deals/{id}/stage`     | `TransitionDealStage`| Body: `{"stage_id":"<uuid>"}`. See §3. |
| `GET    /v1/pipeline/board`       | `GetPipelineBoard`   | Optional — if you skip, document the choice in the commit message and have the frontend group client-side. |

Each handler resolves the workspace via `h.ws(w, r)` (existing helper,
line 269) and uses `readTx` / a `BeginTx(... AccessMode: pgx.ReadWrite)`
transaction for the write path. Match the error envelope used by
`writeErr` / `writeJSON` (defined in the same file).

### 3. Backend — `TransitionDealStage` semantics

In a single `pgx.Tx`:

1. `GetPipelineStage(stage_id)` — 400 if not found (cross-tenant: a stage
   from another workspace's schema cannot be resolved because the role's
   `search_path` excludes it).
2. `GetDeal(id)` — 404 if not found. Capture `oldStageID`.
3. If `oldStageID == newStageID`, return 200 with the unchanged deal
   (idempotent — no activity row).
4. `UpdateDealStage(id, stage_id)` — get the updated row.
5. If `oldStageID != nil`, resolve the old stage name with
   `GetPipelineStage(oldStageID)` (best-effort — log+continue if it's
   been deleted).
6. Insert the activity row into `objects`:

   ```go
   data := map[string]any{
       "kind":         "stage_change",
       "subject":      deal.Title,
       "occurred_at":  time.Now().UTC().Format(time.RFC3339),
       "actor_id":     actorID, // pull from auth context
       "old_stage":    oldStageID,    // nullable
       "new_stage":    newStageID,
       "old_stage_name": oldStageName, // nullable
       "new_stage_name": newStageName,
   }
   // INSERT INTO objects (object_type, parent_type, parent_id, data)
   // VALUES ('activity', 'deal', $1, $2)
   ```

   Use the same `INSERT` shape as `apps/api/internal/metadata/set.go:118`.

7. Commit. On any error in steps 4–6, roll back and return 500 — the
   stage update and activity row are atomic by contract.

Return `200` with the updated deal (same shape as `UpdateDeal`).

### 4. Backend — Tests (`apps/api/internal/crm/handlers_test.go`)

Use the existing `testfixtures/tenantpair` harness (already imported in
the file — see `provision_ro_role_test.go` for usage). Cover:

- Happy path: transition writes deal + activity in one tx; response
  body is the updated deal.
- Idempotency: PATCH with the current stage_id returns 200 but writes
  no activity row (`SELECT count(*) FROM objects WHERE object_type='activity'`
  unchanged).
- Invalid stage_id (random UUID) → 400, no DB writes.
- Cross-tenant isolation: workspace A cannot transition a deal in
  workspace B, and cannot use a stage_id from workspace B's schema
  (provision two workspaces via `tenantpair.Provision`).
- `ListPipelineStages` returns the 5 seeded stages in `order_index`
  order for a `gbconsult-default` workspace.

Run with `(cd apps/api && go test -race -count=1 ./internal/crm/...)`.

### 5. Frontend — Dependencies

```bash
cd apps/web && bun add @dnd-kit/core @dnd-kit/sortable @dnd-kit/utilities
```

### 6. Frontend — Route + hooks

- New route: `apps/web/src/routes/pipeline/$workspaceId.tsx` (mirror the
  Reports route layout — `apps/web/src/routes/reports/$workspaceId.tsx`
  is the canonical pattern committed in `718b16ba`).
- Add a top-nav link in `apps/web/src/routes/__root.tsx` next to the
  existing Reports link.
- Extend `apps/web/src/hooks/use-deals.ts` with:
  - `usePipelineStages()` — `GET /v1/pipeline/stages` via `api.get`.
  - `useTransitionDealStage()` — `useMutation` calling
    `api.patch('/v1/deals/<id>/stage', {stage_id})`. On success,
    `queryClient.invalidateQueries({queryKey: ['deals']})`.
- Pipeline board layout: one column per stage, ordered by `order_index`.
  Deals fetched via `useDeals()`, grouped client-side by `stage_id`.
- Deal card shows: title, `amount` + `currency` (use `Intl.NumberFormat`),
  contact name if joined, `expected_close_date`. Overdue indicator:
  `expected_close_date < today && closed_at == null`.
- Click a card → navigate to `/deals/$dealId`.

### 7. Frontend — Drag-and-drop

- Wrap the board in `<DndContext>` with `closestCenter` collision
  detection.
- Each column is a `useDroppable` zone keyed by `stage.id`.
- Each card is a `useDraggable` keyed by `deal.id`.
- On drop:
  1. Optimistically update the local React Query cache: `setQueryData`
     on the deals list, mutating the dropped card's `stage_id`.
  2. Fire `useTransitionDealStage().mutate({id, stage_id})`.
  3. On error: `queryClient.invalidateQueries(['deals'])` (server
     truth wins) and surface a `<Toast>` via the existing toast util.
- Keyboard accessibility: `KeyboardSensor` with `sortableKeyboardCoordinates`.

### 8. Frontend — Tests

Add Vitest specs alongside the new files (same pattern as
`apps/web/src/routes/reports/reports-body.test.tsx`):

- `pipeline-board.test.tsx`: renders 5 columns from a seeded stages
  fixture; cards land in the correct column based on `stage_id`;
  empty columns show the empty-state copy.
- `use-pipeline-stages.test.ts`: 200 happy path + 401/503 error paths
  (mirror `use-embed-token.test.ts`).
- `transition.test.tsx`: mock `useMutation`; simulate a drop event and
  assert the mutation was called with the correct `{id, stage_id}`;
  on simulated error, assert the card snaps back.

## Done When

- [ ] `GET /v1/pipeline/stages` returns the workspace's stages, ordered.
- [ ] `PATCH /v1/deals/{id}/stage` transitions atomically + writes a
      `stage_change` activity row into `objects`.
- [ ] Idempotent PATCH (same stage) does not write an activity row.
- [ ] Cross-tenant isolation test passes.
- [ ] `/pipeline/$workspaceId` route renders a Kanban with one column
      per stage and one card per deal.
- [ ] Drag-and-drop moves a card and persists via the PATCH endpoint;
      optimistic update + rollback on error works.
- [ ] Sprint-12 dashboards `deals-by-stage` and `conversion-funnel`
      reflect the new transitions (manual check — no code change).
- [ ] `(cd apps/api && go build ./... && go test -race -count=1 ./...)` green.
- [ ] `(cd apps/web && bun run typecheck && bun test)` green.

## Completion Verification

```bash
# Backend
test -f packages/db/queries/pipeline_stages.sql
grep -q "TransitionDealStage" apps/api/internal/crm/handlers.go
grep -q "/v1/pipeline/stages" apps/api/internal/crm/handlers.go
grep -q "/v1/deals/{id}/stage" apps/api/internal/crm/handlers.go
export PATH=$PATH:/usr/local/go/bin
(cd apps/api && go build ./... && go test -race -count=1 ./internal/crm/...)

# Frontend
grep -q '"@dnd-kit/core"' apps/web/package.json
test -f apps/web/src/routes/pipeline/\$workspaceId.tsx
(cd apps/web && bun run typecheck && bun test)
```

Commit message:
`feat(pipeline): Kanban board with drag-and-drop deal stage transitions`

## References

- Existing CRM handler & test patterns: `apps/api/internal/crm/handlers.go`,
  `apps/api/internal/crm/handlers_test.go`
- Workspace context middleware: `apps/api/internal/workspace/middleware.go`
- Multi-tenant test fixtures: `apps/api/internal/testfixtures/tenantpair/`
- Activity JSONB contract: `deploy/cube/schema/Activities.js`
- `objects` insert pattern: `apps/api/internal/metadata/set.go:118`
- DB artefacts:
  `packages/db/migrations/0004_workspaces_admin_email_registry.sql` (stages),
  `packages/db/migrations/0008_crm_entities.sql` (`deals.stage_id`)
- Frontend route pattern (mirror this):
  `apps/web/src/routes/reports/$workspaceId.tsx` (commit `718b16ba`)
- DnD Kit: https://docs.dndkit.com/
