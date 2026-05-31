# Scenario 02 — Administrate different client accounts (workspace switching)

## Objective
The user can see they have access to **more than one client account
(workspace)** and can **switch into another one to administrate it** — the core
"integrator" experience we want Leo to feel.

## Preconditions
- Logged in (Scenario 01 PASS).

## Steps
1. From the app shell, `take_snapshot` and locate the **workspace switcher** — a
   trigger button (`aria-haspopup="listbox"`, `ChevronsUpDown` icon). For an
   integrator its label reads `GB Consult · administrating {slug}`.
2. If present, click it. `take_snapshot` the dropdown (`role="listbox"`).
3. Read the listed workspaces (current shows a ✓ + role badge; others are links).
   Record how many there are and their roles.
4. Click a **different** workspace (an `<a role="option">`). This is a full-page
   navigation to another subdomain. `wait_for` load; `take_snapshot`.
5. Verify the switch took effect: the switcher now shows the new slug, and app
   data (pipeline/deals) is that workspace's. `take_screenshot`.
6. `list_console_messages`.

## Pass/Fail assertions
- PASS when: the switcher is visible with **≥2 workspaces**, AND clicking another
  lands authenticated on that workspace (no re-login dead-end, no error), AND the
  UI clearly reflects the new active account.
- FAIL if: switching errors, loops back to login, or lands on a broken page.
- **BLOCKED (expected risk):** if the switcher is **absent** because the demo user
  has only ONE workspace. Per the code the switcher hides when `workspaces ≤ 1`.
  If so, this journey is **not exercisable on the current demo** — report it
  loudly: Leo cannot experience account-switching as the invite implies.

## Evidence to capture
- Whether the switcher renders; the workspace count + roles; before/after slugs;
  screenshots of the dropdown and the post-switch page.

## If it fails / is blocked
This is one of the two journeys the user explicitly wants Leo to have. If BLOCKED
(single workspace), the fix is product/provisioning work: grant the demo user
integrator access to ≥2 client workspaces (see taskets re: integrator grants /
elevation / auto-grant-on-provision). Call this out as the top gap.
