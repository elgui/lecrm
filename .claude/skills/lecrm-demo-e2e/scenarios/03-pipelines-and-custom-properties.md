# Scenario 03 — Pipelines & custom properties

## Objective
The user can work with a **pipeline** (view the kanban, move a deal between
stages) and **create a custom property** — the "taillable sur-mesure" promise.

## Preconditions
- Logged in (Scenario 01 PASS).

## Part A — Pipeline (view + move a deal)
1. Navigate to the pipeline (`/` or `/pipeline/$workspaceId`). `take_snapshot`.
2. Assert the board renders: `data-testid="pipeline-board"` with the expected
   columns (demo: Discovery, Qualified, Proposal Sent, Negotiation,
   Closed-Won/Lost) and deal cards.
3. Move a deal between stages: drag a `pipeline-card-{dealId}` from one
   `pipeline-column-{stageId}` to another (chrome-devtools `drag`). This fires
   `PATCH /v1/deals/{id}/stage`.
4. Verify the card now sits in the target column and the count badges update.
   `wait_for` / `take_snapshot`. `list_console_messages`. `take_screenshot`.
   (Reset: drag it back so the shared demo stays tidy.)

### Pipeline pass/fail
- PASS: board renders, drag persists the move (no error banner `role="alert"`,
  no console error, card stays after re-snapshot).
- FAIL: board empty/broken, drag does nothing or errors.

## Part B — "Create a pipeline"
1. Look for any UI to **create a new pipeline or add/rename/reorder stages**
   (snapshot the pipeline page + `/settings`).
- **EXPECTED GAP:** at authoring time there is **no UI to create pipeline
  stages** — they're pre-provisioned. If none exists, mark **BLOCKED** and report:
  "create a pipeline" is not yet possible in the product. If a UI *has* shipped,
  exercise it (create a stage, confirm it appears on the board) and update this
  scenario.

## Part C — Custom property
1. Navigate to `/settings/custom-fields`. `take_snapshot`.
2. If you see "Only workspace admins can manage custom fields", the demo user
   lacks `can_write` → mark **BLOCKED** (the user can't create properties) and
   report the role gap.
3. Otherwise create one: set parent type (Contact/Deal), fill `cf-key`
   (e.g. `e2e_temp_tier`, must match `^[a-z][a-z0-9_]*$`), pick `cf-type`
   (`string`), submit ("Add field"). Fires `POST /v1/metadata/definitions`.
4. Verify it appears in the fields table. `take_screenshot`. `list_console_messages`.
5. **Cleanup:** delete the `e2e_temp_*` field (ghost delete button →
   `window.confirm`) so the demo data stays clean. Note cleanup in the report.

### Custom-property pass/fail
- PASS: field creates (201), shows in the table, deletes cleanly.
- FAIL: 400/409/500, validation rejects a valid key, or the table doesn't update.
- BLOCKED: page gated behind admin and the demo user isn't admin.

## If it fails / is blocked
Pipeline creation + custom properties are the second journey the user wants Leo
to have. Report each part's status separately so the gap is precise (e.g.
"can move deals ✓, cannot create a pipeline ✗ (no UI), can create custom fields
✓/✗ depending on role").
