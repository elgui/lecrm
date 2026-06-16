---
id: 20260530-150314-8142
title: Frontend workspace switcher UI
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
tags: [lecrm, integrator, rbac, multi-tenant, auth]
category: project
group: lecrm-integrator-switching
order: 4
plan: true
---

# Frontend workspace switcher UI

## Pre-flight: Verify Previous Tasket
Before starting, verify Tasket 3 ("Login elevation + /auth/workspaces") completed:
1. `cd apps/api && go test ./internal/auth/... -count=1` -- pass
2. Manual/staging: `GET /auth/workspaces` returns `[{slug, role, url}]` for an authenticated session
3. `git log --oneline -15 | grep -i "auth/workspaces\|integrator"` -- Tasket 3 commit exists

**If any check fails, STOP and report. Do not proceed.**

## Context
Final slice: the UI that lets Léo see and jump between his client workspaces. Mechanism is **full navigation to the target subdomain** → existing Authentik SSO makes the re-auth silent → he lands in that client's CRM with a fresh, correctly-scoped session cookie. No SPA-side workspace state, no wildcard cookie — fully consistent with ADR-009 §5.2.

Working directory: `/home/gui/Projects/leCRM` (frontend in `apps/web`).

## Approach
- New hook `apps/web/src/hooks/use-workspaces.ts`: TanStack Query fetch of `GET /auth/workspaces` via the existing API/fetch wrapper (`apps/web/src/lib/api.ts`). Type it in `apps/web/src/lib/types.ts` (`AccessibleWorkspace { slug; role; url }`).
- Switcher component (shadcn dropdown / `DropdownMenu`) in the sidebar or top bar of `apps/web/src/routes/__root.tsx`:
  - **Render only when the list has > 1 entry** (single-workspace clients see nothing — no behavior change for them).
  - Mark the current workspace (compare against `/auth/me`'s `workspace_slug`).
  - Other entries are real anchor links: `<a href={ws.url}>` (full page navigation, NOT TanStack router navigation — we must cross the subdomain boundary).
  - Integrator framing: when the current user's role is `integrator`, label it e.g. `GB Consult · administrating {current client}` and group/style the list as client accounts.

## Steps
1. `use-workspaces.ts` hook + `AccessibleWorkspace` type.
2. Build `WorkspaceSwitcher.tsx` (shadcn DropdownMenu) under `apps/web/src/components/`.
3. Mount it in `__root.tsx` header/sidebar; hide when `<= 1` workspace.
4. Highlight current; render others as `<a href>` full-nav links; integrator label/styling.
5. Manual verify on staging with Léo's account across ≥2 granted tenants.

## Done When
- [ ] With ≥2 accessible workspaces, the switcher renders and lists them; current is marked.
- [ ] Clicking another workspace full-navigates to its subdomain and (via SSO) lands Léo in that client's CRM.
- [ ] Single-workspace users see no switcher.
- [ ] `cd apps/web && pnpm build` (or the project's build cmd) clean; no type errors.

## Completion Verification
1. `cd apps/web && pnpm build` -- builds clean (use the repo's actual build command if not pnpm)
2. `cd apps/web && pnpm lint` (if configured) -- clean
3. Manual on staging: log in as Léo → switcher shows his tenants → click one → lands in that workspace
4. Commit: `feat(web): integrator workspace switcher`

## References
- `apps/web/src/hooks/use-auth.ts` / `use-me.ts` — existing /auth/me + role hooks (pattern to mirror)
- `apps/web/src/lib/api.ts` — fetch wrapper
- `apps/web/src/lib/types.ts` — shared types
- `apps/web/src/routes/__root.tsx` — root layout to mount the switcher
- Tasket 3 (this group) — `GET /auth/workspaces` contract `{slug, role, url}`
