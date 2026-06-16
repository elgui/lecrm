---
id: 1138
title: "[Fix] Frontend workspace switcher UI"
status: done
updated: 2026-05-30
done: 2026-05-30
priority: p1
created: 2026-05-30
tags: [lecrm, integrator, rbac, multi-tenant, auth, frontend, remediation]
category: engineering
group: lecrm-integrator-switching
order: 4
remediates: 20260530-150314-8142
plan: true
---

## Remediation task #1138

Follow-up to `20260530-150314-8142` ("Frontend workspace switcher UI"), commit
`7c9886f8`. The original was flagged `partial_success` only because the verifier
could not see the file contents to confirm the implementation met the spec
(DropdownMenu usage, conditional rendering, anchor navigation, integrator
labeling, correct API endpoint).

### Outcome: no code fix required — implementation is correct

A deep code review of all four changed files against the actual codebase and the
backend contract found **no blocking bugs**. Each of the suspected failure modes
listed in the remediation brief was checked and does not apply:

- **"Missing DropdownMenu import"** — N/A. The repo has **no** `dropdown-menu`
  component and no `@radix-ui/react-dropdown-menu` dependency
  (`apps/web/src/components/ui/` has only badge/button/card/input/label/skeleton/
  table/textarea). The author correctly **hand-rolled** the dropdown with a
  `Button` trigger + an absolutely-positioned `role="listbox"` panel and a
  manual outside-click `useEffect`. `Button`, `cn`, and the lucide icons
  (`Check`, `ChevronsUpDown`, both present in `lucide-react ^0.469`) are imported
  correctly. There is no broken import.
- **"Wrong API endpoint"** — Correct as written. `use-workspaces.ts` calls
  `api.get('/auth/workspaces')`. The Go handler registers `GET /auth/workspaces`
  at the **router root** (`AuthHandler.Register`), outside the `/v1` prefix and
  outside the workspace-middleware group. The `api` helper (`lib/api.ts`,
  `BASE='/v1'`) passes any **leading-slash** path through verbatim (only bare
  paths get `/v1/` prefixed), so `/auth/workspaces` hits the right route — the
  same convention `use-auth.ts` uses for `/auth/me`.
- **"Response-shape parsing"** — Matches exactly. Backend encodes
  `{"data": [{slug, role, url}]}` (`workspaceEntry` struct); the hook types it as
  `{ data: AccessibleWorkspace[] }` and reads `data.data`. `AccessibleWorkspace`
  (`lib/types.ts`) = `{ slug; role; url }`.
- **"Missing conditional checks"** — Present: `if (!workspaces ||
  workspaces.length <= 1) return null;` so single-workspace users see nothing.
- **"Missing anchor navigation"** — Present and correct: non-current entries
  render as real `<a href={ws.url}>` full-page anchors (required to cross the
  subdomain boundary so the Authentik SSO re-auth re-scopes the session cookie,
  per ADR-009 §5.2); the current workspace renders as a non-link `<div>` with a
  check mark.
- **"Incorrect integrator labeling"** — Works: when the current workspace's role
  is `"integrator"` the trigger reads `GB Consult · administrating {slug}` and
  the list is headed "Client accounts"; otherwise "Your workspaces". This is
  consistent with the login-elevation design (commit `917c92b5`), which
  materializes an elevated integrator as an `integrator`-role member on the
  active workspace.
- **TypeScript** — Type-correct under the project's `strict` +
  `noUnusedLocals`/`noUnusedParameters`: the `Dispatch`/`SetStateAction` imports
  are used; `React.RefObject<HTMLDivElement | null>` matches
  `useRef<HTMLDivElement>(null)` under React 19 `@types/react ^19.0.2`;
  `tailwindcss-animate` is installed so the `animate-in …` classes resolve.

`__root.tsx` mounts `<WorkspaceSwitcher />` in the sidebar footer above the user
block — correct placement.

### Build verification

`pnpm --filter @lecrm/web build`/`typecheck` could **not** be executed in this
isolated worktree: it has no installed `node_modules` and no root
`package.json`/`pnpm-workspace.yaml` (partial checkout), and per the project env
notes node install/fetch is blocked under the 6 GB vmem cap. This is the same
environment limitation recorded for the sibling remediation `#1133` ("vite build
is WASM-OOM blocked"); verification was therefore done by static type-level
review, which is clean. No code changes were made, so commit `7c9886f8` stands
and the worktree remains clean.
