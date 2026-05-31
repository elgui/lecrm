---
name: lecrm-demo-e2e
description: Drive a real browser (chrome-devtools MCP) through the leCRM LIVE demo to verify the experience an invited user (e.g. Leo) will actually get. USE WHEN asked to e2e-test the demo, validate/QA the demo before sending the invite link, check that account-switching / pipeline / custom-property journeys work end to end, or "test the leCRM demo". Project-scoped, extensible scenario catalog.
---

# leCRM demo E2E

Browser-driven, human-like end-to-end verification of the **live staging demo**
at `https://demo.lecrm.gbconsult.me`, run via the **chrome-devtools MCP** server.

**Why this exists:** before we hand the demo link to a real person, confirm they
can have a *full, meaningful, bug-free* first experience. The named, must-work
journeys are: (1) log in, (2) switch into different client accounts to
administrate them, (3) build a basic pipeline with custom properties. The
catalog grows as features ship.

## What this is / isn't
- IS: a walkthrough of the **real deployed app**, asserting key journeys work and
  surfacing anything that would embarrass us in front of an invited user.
- ISN'T: unit/integration tests (Go + vitest already cover those) and not a
  headless CI suite. It runs interactively in a Claude Code session and writes a
  dated report under `runs/`.

## Prerequisites (check first, stop if unmet)
1. **chrome-devtools MCP** connected — tools `mcp__chrome-devtools__*` exist. If
   not, tell the user and stop (don't fake a run).
2. **Demo up:** `curl -s -o /dev/null -w '%{http_code}' https://demo.lecrm.gbconsult.me/healthz` → `200`.
3. **Credentials — NEVER hardcode or commit them.** Resolve in order:
   - env `LECRM_DEMO_USER` / `LECRM_DEMO_PASSWORD`, else
   - the untracked invite `deploy/leo-demo-invite.html` (parse "Identifiant" /
     "Mot de passe").
   Login is **Authentik (OIDC)**: app redirects to the IdP, which asks for
   username, then password, then redirects back.

## How to run
1. Read every file in `scenarios/` in numeric order. Each has: Objective,
   Preconditions, Steps, Pass/Fail assertions, and an "If it fails" note.
2. Open a page (`new_page`) and execute scenarios in order. **Locate elements via
   `take_snapshot` (accessibility tree) by visible label/role**, not brittle CSS —
   the app is a TanStack Router SPA and the IdP page is dynamic.
3. After each scenario record **PASS / FAIL / BLOCKED** + evidence: a
   `take_screenshot`, the specific assertion that failed, and any console output.
4. After key steps run `list_console_messages` — JS errors count as findings even
   if the UI "looked" fine. Optionally `list_network_requests` to catch 4xx/5xx.
5. Write a report from `TEMPLATE-report.md` to `runs/<UTC-date>-<slug>.md` and give
   the user a one-line **verdict** (rubric below).

## Verdict rubric
- **READY** — all in-scope scenarios PASS, no console errors on happy paths.
- **SEND WITH CAVEATS** — only cosmetic issues; core journeys work.
- **NOT READY** — any in-scope core journey FAILs or is BLOCKED (missing UI,
  error, dead end). Spell out exactly what blocks the invited user.

## Safety / etiquette
- The demo is a shared sandbox an invited human will land on. Prefer
  **non-destructive** checks; if a scenario creates data, clean it up (or name it
  obviously, e.g. `e2e-temp-*`) and say so in the report.
- Don't commit credentials, screenshots with secrets, or the invite file.

## Known design facts (verify in-run — code changes; don't assume)
- Routes: `/`, `/pipeline/$workspaceId`, `/deals`, `/contacts`, `/companies`,
  `/settings/custom-fields`, `/settings/members`, `/reports/$workspaceId`, `/tasks`.
- **Workspace switcher** (`apps/web/src/components/WorkspaceSwitcher.tsx`) renders
  **only if the user has >1 accessible workspace** (`GET /auth/workspaces`).
  Integrator trigger label: `GB Consult · administrating {slug}`; dropdown is
  `role="listbox"`, other workspaces are `<a href={ws.url} role="option">`
  (full-page nav to another subdomain).
- **Custom fields** (`/settings/custom-fields`) need `permissions.can_write`
  (admin). Create = `POST /v1/metadata/definitions`; form ids `cf-key`, `cf-type`,
  `cf-allowed`; key regex `^[a-z][a-z0-9_]*$`.
- **Pipeline board** `/pipeline/$workspaceId`: `data-testid="pipeline-board"`,
  columns `pipeline-column-{stageId}` (`data-stage-name`), cards
  `pipeline-card-{dealId}`; drag = `PATCH /v1/deals/{id}/stage`.
- **No UI to create pipeline *stages*** as of authoring — stages are
  pre-provisioned. Treat "create a pipeline" as a gap to **confirm**, not assume.

## Extending (add scenarios as features ship)
- Copy the closest `scenarios/NN-*.md`, bump the number, keep the section headers.
- One journey per file; keep each independent (each may log in fresh) so the suite
  stays resilient.
- Every step = action + target (by visible label) + expected. Make assertions
  **observable in the browser** (text appears, row added, workspace/URL changes,
  no console error).
- Add the new file to the catalog below.

## Scenario catalog
- `scenarios/01-login-and-landing.md` — log in via Authentik, land on the app.
- `scenarios/02-administrate-client-accounts.md` — switch into different client
  workspaces to administrate them (integrator flow).
- `scenarios/03-pipelines-and-custom-properties.md` — inspect/build a pipeline and
  create a custom property.
