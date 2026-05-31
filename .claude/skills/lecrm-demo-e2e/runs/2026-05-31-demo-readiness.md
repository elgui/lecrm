# leCRM demo E2E — run report

- **Date (UTC):** 2026-05-31
- **Target:** https://demo.lecrm.gbconsult.me (+ bistrot-halles, menuiserie-vasseur)
- **Runner:** Claude Code (chrome-devtools MCP) + on-host log/code/DB agents
- **Demo user / workspace:** leo / demo  (role: admin; integrator on the 2 client WS)
- **Verdict:** ✅ READY — login, demo data, pipeline, custom properties, AND
  multi-account switching all verified working end-to-end.

> Correction note: an early first-pass verdict ("NOT READY — login loop / empty
> demo") was **wrong**. Those symptoms were artifacts of (a) a reused dirty
> browser session replaying stale OIDC state (consuming the auth code twice) and
> (b) a migration-lag window earlier that day. A clean isolated-context login as
> `leo` succeeds and the demo is fully populated. Lesson baked into the skill:
> always drive logins in a fresh isolated browser context.

## Scenario results
| # | Scenario | Result | Notes |
|---|----------|--------|-------|
| 01 | Login & landing | ✅ PASS | Clean OIDC login via branded Authentik ("Bienvenue sur leCRM"); lands on dashboard (10 contacts, 4 companies, 4 open deals, €112,500). |
| 02 | Administrate client accounts (switch) | ✅ PASS | Switcher lists 3 accounts: demo (admin), bistrot-halles (integrator), menuiserie-vasseur (integrator). Switched to bistrot-halles: new subdomain SSO'd, label became "GB Consult · administrating bistrot-halles", API served **bistrot's own** data (open pipeline €94,700, distinct companies/deals) — workspace isolation verified. |
| 03 | Pipelines & custom properties | ⚠️ MOSTLY PASS | A: kanban + 5 French stages + 6 deals; move-deal API works. B: still **no UI to create a pipeline/stages** (product gap). C: **custom properties WORK** — list columns + deal-detail editor show real values; `/v1/metadata/properties` fixed (was 500). |

## What was fixed/built this run
1. **[FIXED+DEPLOYED] `metadata.GetMany` 500** — `[]uuid.UUID` unencodable under
   the simple-protocol pool. `apps/api/internal/metadata/set.go:87` → pass ids as
   `[]string` + `ANY($2::uuid[])`. Committed + **merged to `main`** (464a45fa),
   built, unit-tested, deployed to staging, verified live.
2. **[PROVISIONED] Two client workspaces + integrator grants** so Leo can switch:
   - `bistrot-halles` (Le Bistrot des Halles — événementiel/traiteur)
   - `menuiserie-vasseur` (Menuiserie Vasseur — chantiers/devis)
   Each seeded with 4 companies / 10 contacts / 6 deals across the 5 stages +
   themed custom properties; `leo@vernayo.com` granted `integrator` on both.
   Seed files committed at `deploy/seed/demo-{bistrot-halles,menuiserie-vasseur}.sql`.
   (Provisioning via `core.lecrm_provision_workspace_with_registry`; no infra
   change needed — wildcard DNS/TLS + OIDC redirect regex already cover new subdomains.)

## Known issues / follow-ups (non-blocking)
- **No UI to create a pipeline / stages** (product gap — stages pre-provisioned).
- **Re-login on workspace switch:** Authentik prompts for credentials again when
  switching to a new subdomain (no silent cross-workspace SSO). UX friction; not a
  blocker, but worth a look (Authentik session/cookie scope).
- **Stale dashboard stats on switch:** right after switching, the dashboard
  briefly showed the previous workspace's totals (React-Query cache); correct
  numbers appear on reload. The underlying API/data are correctly isolated.
- **Switcher a11y nit:** non-current options render `aria-selected="true"` (should
  be `false`).
- **Deep links to /deals/<id> bounce to dashboard** on hard nav (client-side
  routing); in-app clicks work.
- **Read-only empty-state CTA** ("Create your first deal" with no button) when
  `useMe` permissions haven't resolved; leo is admin so unaffected.
- **Drift:** the workspace-switcher frontend (`WorkspaceSwitcher.tsx`, the
  `__root.tsx` import, `use-workspaces.ts`) is in-flight UNCOMMITTED work in the
  tree (taskets `frontend-workspace-switcher-ui`). Left untouched per the working
  agreement; it is what staging currently runs. Should be committed by its author.

## Data created / cleaned up
- Created 2 workspaces + their seed data + 2 integrator grants (intentional,
  durable demo state). One throwaway probe workspace was created and fully
  dropped. No demo-workspace rows mutated.

## Recommendation
Demo is ready to send. Before/with the invite: (1) tell Leo the 3 accounts are
his to explore (demo + 2 client tenants he can switch between as integrator);
(2) optionally smooth the re-login-on-switch; (3) "create a pipeline" remains a
roadmap feature — set expectations or build the stage-management UI. Commit the
in-flight switcher frontend so `main` matches the deployed image.
