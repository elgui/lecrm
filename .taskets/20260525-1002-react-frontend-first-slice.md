---
id: 20260525-1002-react-frontend-first-slice
title: "React frontend first slice — TanStack Router + Query + shadcn/ui"
status: done
priority: p1
created: 2026-05-25
updated: 2026-05-25
done: 2026-05-25
category: project
group: crm-entity-foundation
group_order: 50
order: 3
plan: true
tags: [frontend, react, tanstack, shadcn, sprint-4-5]
---

# React frontend first slice — TanStack Router + Query + shadcn/ui

## Pre-flight: Verify Previous Tasket

Before starting, verify Tasket 1 ("Contact + Company + Deal domain models") completed:

1. `ls packages/db/queries/contacts.sql` -- contacts query file exists
2. `cd apps/api && go build ./...` -- compiles
3. `git log --oneline -10 | grep -i "contact\|entity"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

The frontend scaffold exists (`apps/web/src/main.tsx` with React 19 + TanStack Router/Query + shadcn/ui) but has only 4 files and no real pages. The API is gaining entity endpoints. This tasket connects the frontend to the backend for the first time.

Can run in parallel with Tasket 2 (custom properties CRUD) — both depend on Tasket 1 only.

Source of truth: `docs/sprint-plan.md` Sprint 4
Working directory: `/home/gui/Projects/leCRM`

## Approach

Build the SPA shell: route tree, layout, auth-aware routing, and a first working list page (contacts). This establishes the frontend patterns that all subsequent pages follow.

## Steps

1. Configure Vite dev proxy in `apps/web/vite.config.ts`:
   ```ts
   server: {
     proxy: {
       '/api': { target: 'http://localhost:8080', changeOrigin: true }
     }
   }
   ```

2. Set up TanStack Router route tree:
   - `routes/__root.tsx` — layout with sidebar navigation
   - `routes/index.tsx` — dashboard (placeholder)
   - `routes/contacts/index.tsx` — contact list
   - `routes/contacts/$contactId.tsx` — contact detail
   - `routes/companies/index.tsx` — company list
   - `routes/deals/index.tsx` — deal list / pipeline view
   - `routes/deals/$dealId.tsx` — deal detail
   - `routes/settings/index.tsx` — workspace settings (placeholder)

3. Build shared layout:
   - Sidebar: nav links to Contacts, Companies, Deals, Settings
   - Top bar: workspace name, user info (from GET /auth/me), logout button
   - Content area with consistent padding/max-width

4. Implement auth-aware routing:
   - Query GET /auth/me on app load
   - If 401 → redirect to auth flow (OIDC login at /auth/login)
   - Show loading state while checking auth
   - Store auth state in TanStack Query cache

5. Build contact list page:
   - TanStack Query hook: `useContacts()` → GET /v1/contacts
   - shadcn/ui Table component with columns: name, email, company, created
   - Cursor-based pagination (load more / infinite scroll)
   - Empty state when no contacts
   - Link to contact detail page

6. Build contact detail page:
   - TanStack Query hook: `useContact(id)` → GET /v1/contacts/:id
   - Display all fields, edit form (shadcn/ui form + react-hook-form + zod)
   - Custom properties section (reads from GET /v1/contacts/:id/properties)
   - Back to list navigation

7. Start dev server and verify:
   - `cd apps/web && pnpm dev` serves on :5173
   - Proxy to Go API on :8080 works
   - Auth redirect works
   - Contact list loads data from API
   - Contact detail shows individual record

## Done When

- [ ] Route tree covers all planned pages (contacts, companies, deals, settings)
- [ ] Sidebar navigation works between all routes
- [ ] Auth check on app load → redirect to login if unauthenticated
- [ ] Contact list page renders data from the API
- [ ] Contact detail page shows individual record with edit form
- [ ] Vite dev proxy to Go backend works
- [ ] `pnpm typecheck` passes
- [ ] `pnpm test` passes

## Completion Verification

1. `ls apps/web/src/routes/__root.tsx` -- route tree exists
2. `ls apps/web/src/routes/contacts/index.tsx` -- contacts page exists
3. `cd apps/web && pnpm typecheck` -- no type errors
4. `cd apps/web && pnpm build` -- production build succeeds
5. Commit: `feat(web): React frontend first slice with TanStack Router + contact list (Sprint 4)`

## References

- `apps/web/src/main.tsx` — existing React 19 scaffold
- `apps/web/vite.config.ts` — Vite config
- `apps/web/package.json` — dependencies (TanStack, shadcn/ui, Radix)
- `docs/sprint-plan.md` — Sprint 4 frontend slice
