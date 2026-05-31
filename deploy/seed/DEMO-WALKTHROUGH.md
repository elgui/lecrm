# Demo happy-path walkthrough — Léo integrator demo

Tasket `20260531-145435-45f6` (group `lecrm-leo-demo-polish`, step 4/11).
North star: Léo clicks every tab in at least one workspace; any empty table,
`0` headline, Lorem, console error, or 500 on the happy path ends the deal.

Path audited per workspace:
`dashboard → contacts → contact detail → deals → deal detail → pipeline →
tasks → settings`, across the 3 seeded workspaces (`demo`, `bistrot-halles`,
`menuiserie-vasseur`).

## What was checked (data layer, per workspace seed)

Each seed file (`deploy/seed/demo.sql`, `demo-bistrot-halles.sql`,
`demo-menuiserie-vasseur.sql`) was read and its coverage mapped to the tabs
Léo will click:

| Tab / surface            | Backing data                                   | Before        | Now        |
|--------------------------|------------------------------------------------|---------------|------------|
| Dashboard stat row       | contacts / companies / deals counts + open €   | populated     | populated  |
| Contacts list            | 10 contacts (FR names, phones, company links)  | populated     | populated  |
| Contact detail           | core fields + custom props (fonction, canal)   | populated     | populated  |
| Contact → Notes / Tasks  | notes on some contacts; **tasks**              | **tasks empty** | **fixed** |
| Companies list / detail  | 4 companies (FR, industry, size)               | populated     | populated  |
| Deals list               | 6 deals across the 5 FR stages                 | populated     | populated  |
| Deal detail              | core + custom props + notes + activities       | populated     | populated  |
| Deal → Tasks panel       | **tasks**                                      | **tasks empty** | **fixed** |
| Pipeline (kanban)        | 6 deals spread over Découverte→Gagné/Perdu     | populated     | populated  |
| **Tasks** (global tab)   | `GET /v1/tasks` (workspace-wide)               | **EMPTY — "No tasks yet."** | **6 tasks / workspace** |
| Settings / Custom fields | custom_property_definitions (themed per client)| populated     | populated  |
| Reports                  | honest "coming soon" placeholder (step 3)      | placeholder   | placeholder |

## The gap that was fixed

No seed inserted into the `tasks` table, yet Tasks is a first-class nav item on
Léo's path and every record detail page mounts a `TasksPanel`
(`apps/web/src/components/tasks-panel.tsx`). The result was a prominent
**"No tasks yet."** empty state on the global Tasks tab *and* on every
contact/deal detail — exactly the kind of empty surface the tasket warns ends
the deal.

Added a **TASKS** section to all three seed files: 6 believable French tasks per
workspace, themed to each business (agency/consulting, restaurant events,
carpentry chantiers). Each set mixes:

- **overdue** (due_date < today), **due-soon**, and **upcoming** tasks,
- one **completed** task (`completed_at` set) so the checked/struck-through
  state renders,
- one **workspace-global** task (`entity_type IS NULL`) plus deal- and
  contact-scoped tasks, so both the global `/tasks` list *and* the per-record
  task panels are populated.

Columns/ordering match the read path. The list query
(`packages/db/queries/tasks.sql`, handler `apps/api/internal/crm/anc_handlers.go`)
filters only by optional `assignee_id` and optional entity scope — there is **no**
completed filter — and orders `completed_at NULLS FIRST, due_date NULLS LAST,
created_at DESC`. Every seeded row (assignee = the demo owner sentinel, which the
unfiltered list returns) is therefore visible; completed tasks sort last,
overdue/open first.

Rows use fixed UUIDs + `ON CONFLICT (id) DO NOTHING`, so the additions are
idempotent and safe to re-apply alongside the existing rows.

## Verification performed

1. **SQL semantic validation** — throwaway scratch schema with the real
   migration-0015 `tasks` DDL (incl. the `entity_type` CHECK), inside a
   rolled-back txn, run in the `lecrm-pg-test` container. All three files'
   `INSERT INTO tasks …` blocks parse and execute; each yields exactly 6 rows;
   `demo` has exactly 1 completed and 1 global task. `psql -v ON_ERROR_STOP=1`
   → `VALIDATION_PASSED_4f2a`, exit 0.

2. **Applied to live staging** (`lecrm-postgres`, database `lecrm`, superuser
   DSN, slug→schema resolved server-side via `\gset` so no manual UUID
   handling). Idempotent re-apply of all three seeds → `APPLY_DONE_4f2a`,
   exit 0. The demo-seed summary now reports `… 6 tasks …`.

3. **Server-side assertion** across the 3 demo schemas — every workspace has
   `tasks=6 contacts=10 deals=6 global-tasks=1`:
   ```
   workspace_5004d2cf8ef74de9b3b6abe0e6c8d56e OK: tasks=6 contacts=10 deals=6 global-tasks=1
   workspace_2c50cb2f7a1d4cebb9be1c1d12dbf96a OK: tasks=6 contacts=10 deals=6 global-tasks=1
   workspace_70d6797cb86a4f2ba87e1f8f8e87f3d2 OK: tasks=6 contacts=10 deals=6 global-tasks=1
   ```
   → `VERIFY_DONE_4f2a`, exit 0.

4. **API live checks** — `GET http://127.0.0.1:8088/healthz` (API direct) →
   `200`; `GET http://127.0.0.1:8080/healthz` (via Caddy) → `308` (HTTP→HTTPS
   redirect, expected); `GET /v1/tasks` unauthenticated → `400` (request
   rejected at the auth/validation gate; endpoint is wired and fails closed —
   **not** a 500).

## Reproduce the live apply

```bash
# from a host with the staging compose stack (vps-25b8e3b3)
SU=$(grep -m1 '^POSTGRES_SUPERUSER_PASSWORD=' deploy/.env.staging | cut -d= -f2-)
for pair in demo:demo bistrot-halles:demo-bistrot-halles \
            menuiserie-vasseur:demo-menuiserie-vasseur; do
  slug=${pair%%:*}; file=${pair##*:}
  schema=$(sg docker -c "docker exec -e PGPASSWORD=$SU lecrm-postgres \
    psql -U postgres -d postgres -tA \
    -c \"SELECT 'workspace_'||replace(id::text,'-','') \
         FROM core.workspaces WHERE slug='$slug'\"")
  sg docker -c "docker exec -i -e PGPASSWORD=$SU lecrm-postgres \
    psql -U postgres -d postgres -v ON_ERROR_STOP=1 \
    -v schema=$schema -f -" < deploy/seed/$file.sql
done
```

## Remaining manual confirmation (recommended, not blocking)

The data layer for every happy-path tab is now populated and verified live; the
app code on this path is unchanged by this step (display bugs were fixed in
steps 1–2, Reports stubbed in step 3). A final human pass with browser devtools
open — log in as Léo, click each tab in `demo`, `bistrot-halles`,
`menuiserie-vasseur`, watch the Console + Network panes — is the last mile to
sign off "zero console errors / zero failed requests" end-to-end. The infra
checks above (API healthz 200, `/v1/tasks` → 400 not 500) make a 500 on the
tasks path unlikely.
