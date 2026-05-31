# Scenario 01 — Login & landing

## Objective
An invited user with the demo credentials can log in through Authentik and land
on a working app (the commercial pipeline), with no error page and no console
errors on the way in.

## Preconditions
- Credentials resolved (env or `deploy/leo-demo-invite.html`).
- Demo healthz = 200.

## Steps
1. `new_page` → `navigate_page` to `https://demo.lecrm.gbconsult.me`.
2. Expect a redirect to the login flow (`/auth/login` → Authentik). `take_snapshot`.
3. On the Authentik page, fill the **username/identifier** field with the demo
   user and submit. Authentik is multi-step: a username stage, then a password
   stage — `take_snapshot` again before filling the **password** field, then
   submit.
4. `wait_for` the app to load after the OIDC callback. `take_snapshot`.
5. `list_console_messages` and (optional) `list_network_requests`.
6. `take_screenshot` of the landing page.

## Pass/Fail assertions
- PASS when, after callback, the URL is back on `demo.lecrm.gbconsult.me` (not the
  IdP) AND the app shell is visible (nav with Pipeline / Deals / Contacts /
  Companies) AND the landing content renders (the kanban board
  `data-testid="pipeline-board"` or a clear home view).
- FAIL if: stuck on IdP after correct creds, an error page / blank screen, a
  redirect loop, a 4xx/5xx on `/auth/*` or the SPA bootstrap, or any uncaught
  console error during login.

## Evidence to capture
- Post-login screenshot; the final URL; any console errors; the role/identity
  shown (used by later scenarios).

## If it fails
This blocks everything — the invited user cannot get in. Mark the whole run
BLOCKED and report the exact failure (IdP error text, network status, console).

## Notes for the runner
- Authentik field names vary by theme; rely on the snapshot's visible labels
  ("Username"/"Email", "Password", "Log in"/"Continue") rather than CSS.
- If the demo brands the Authentik screen (see tasket "brand the authentik login
  screen"), confirm the branding renders too — a broken login screen is a first
  impression Leo will see.
