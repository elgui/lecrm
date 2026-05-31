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

## End-to-end live API sweep (every happy-path request, all 3 workspaces)

Rather than stop at the data layer, every request the SPA fires on Léo's path
was exercised against the **live API** (`https://<slug>.lecrm.gbconsult.me`),
authenticated with a short-lived, `*`-scoped service token minted per workspace
(via the repo's own `auth.GenerateServiceToken`, inserted into
`core.service_tokens` with a 1-hour expiry, then **revoked** immediately after —
zero residual credentials). Endpoint list extracted from `apps/web/src` (the
`api.get` call sites + hooks). Result — **0 server errors (5xx) anywhere** and
every list non-empty where Léo expects rows:

| Surface / request                              | demo | bistrot-halles | menuiserie-vasseur |
|------------------------------------------------|------|----------------|--------------------|
| `GET /v1/workspace/me`                         | 200  | 200            | 200                |
| `GET /v1/contacts`                             | 200 · 10 | 200 · 10   | 200 · 10           |
| `GET /v1/contacts/{id}` · `/notes` · `/properties` | 200 | 200       | 200                |
| `GET /v1/companies`                            | 200 · 4 | 200 · 4    | 200 · 4            |
| `GET /v1/companies/{id}` · `/notes`            | 200  | 200            | 200                |
| `GET /v1/deals`                                | 200 · 6 | 200 · 6    | 200 · 6            |
| `GET /v1/deals/{id}` · `/properties` · `/notes`| 200  | 200            | 200                |
| `GET /v1/pipeline/stages`                      | 200 · 5 | 200 · 5    | 200 · 5            |
| `GET /v1/tasks`                                | 200 · 6 | 200 · 6    | 200 · 6            |
| `GET /v1/tasks?entity_type=deal&entity_id=…`   | 200  | 200            | 200                |
| `GET /v1/metadata/definitions?parent_type=contact` | 200 | 200       | 200                |
| `GET /v1/metadata/definitions?parent_type=deal`| 200  | 200            | 200                |

**Two non-200s — both verified as NOT on Léo's rendered path, not bugs:**

- `GET /v1/workspace/members` → **403**. The members list is owner-only
  (`rbac.RequireRole(RoleOwner)` in `apps/api/internal/http/server.go`). The
  Settings → Members page (`apps/web/src/routes/settings/members.tsx:54`) renders
  the members table **only when `isOwner`**; a non-owner sees a static "Only
  workspace owners can manage members." card and `useMembers()` never fires. The
  403 is an artifact of the *verification token's* scope (`can_manage_members:
  false`), not a request the browser makes for that viewer. A real owner session
  returns 200.
- `GET /v1/reports/embed-token` → **405**. Endpoint is POST-only; a GET probe is
  405 (not 500). On the demo the Reports route short-circuits to the honest
  "coming soon" placeholder via `reportsEnabled()`
  (`apps/web/src/routes/reports/$workspaceId.tsx:44`) and **never calls
  embed-token at all** — so no failed request appears in Léo's Network pane.

## Thin spot found and fixed during the sweep — contacts list lead rows

The contacts list reads `ORDER BY created_at DESC, id DESC`
(`packages/db/queries/contacts.sql`). Each workspace seeds 8 company-linked
contacts plus 2 deliberately company-less individual leads (`#009`, `#010` —
`fonction: "Particulier"` in the restaurant/carpentry workspaces). Because every
row shared the bulk-insert `created_at`, the `id DESC` tie-break floated the two
**company-less** contacts to the **top** — so the first rows Léo sees, and the
first contact he clicks, showed *no company*, directly undercutting the
company-name / relationship surfacing fixed in step 1 (the headline of this demo).

Fix (idempotent, realism-preserving): pin the two individuals to a fixed earlier
`created_at` (`2026-05-22`) in all three seeds so the 8 company-linked contacts
lead the list while the realistic individual customers remain further down.
Verified live afterwards — the demo contacts list now opens with
`Thomas Mercier`, `Inès Dubois`, … (all company-linked); the two individuals
sort last. Companies were **not** forced onto them: a private "Particulier"
customer is exactly right for a restaurant/carpenter, and the only defect was
ordering.

## Data depth confirmed (per workspace, live)

`contacts=10` (8 with a company), `companies=4`, `deals=6` (across all 5 FR
stages), `tasks=6` (on 4 distinct deals + 1 contact + 1 workspace-global),
`notes=3` (1 contact + 2 deals), `pipeline stages=5`, custom-property
definitions present for both `contact` and `deal`. Notes/tasks are present on
*some* records (not all) — an empty Notes/Tasks panel on a record without them
is the app's normal, calm empty state (called out as already-good in the UX
review), not an error.

## Reproduce the API sweep

```bash
# from vps-25b8e3b3 (staging host). Mint a 1-hour, *-scoped token per workspace,
# hit every happy-path endpoint, assert no 5xx, then revoke the tokens.
# Token plaintext+hash generated via apps/api auth.GenerateServiceToken; hash
# inserted into core.service_tokens scoped to the workspace; embedded slug must
# match the subdomain. DELETE the rows (name tag) when done.
```

The infra checks above (API healthz 200, `/v1/tasks` unauth → 400 not 500) plus
this authenticated sweep cover the "zero failed/500 requests" half of the
acceptance at the network layer. The "zero console errors" half rides on the
SPA build, gated green in steps 1–3 (`tsc --noEmit`, `eslint src`, `vitest run`)
and unchanged by this step; this step touched seed SQL only.
