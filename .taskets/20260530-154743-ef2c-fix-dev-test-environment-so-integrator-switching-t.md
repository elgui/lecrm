---
id: 20260530-154743-ef2c
title: Fix dev/test environment so integrator-switching tests can run
status: done
priority: p1
created: 2026-05-30
updated: 2026-05-30
done: 2026-05-30
category: tooling
group: lecrm-integrator-switching
order: 0
plan: true
---

## Context
A `/supervise` run to **deliver tested results (e2e and unit tests)** for the
`lecrm-integrator-switching` group had to PAUSE: the working environment cannot
execute the test suite. This tasket fixes the env so the build can resume.
Clean work should continue on a fresh branch off `main` (the earlier
`feat/integrator-switching-v2` branch carried no commits).

## Blockers to fix (before resuming taskets 1-4)
1. **Docker daemon is DOWN.** Every integration test uses testcontainers:
   `packages/db` migration tests, `apps/admin` tenant tests, and the
   `apps/api` auth cross-tenant fixture (`internal/testfixtures/tenantpair`).
   Start it — likely `sudo systemctl start docker` (ASK before sudo per policy).
2. **pnpm NOT installed** (only node v24). Tasket 4 (frontend workspace
   switcher) needs `apps/web` build + E2E. Install via corepack:
   `corepack enable && corepack prepare pnpm@latest --activate`.
3. **`go` not on default PATH.** It lives at `/usr/local/go/bin/go`
   (go1.25.0). Export it for the session / shell rc.
4. **Hook blocks `git reset --hard`.** The PreToolUse hook
   `security-validator.ts` errors on `git reset --hard` and, when calls are
   batched, cancels the sibling tool calls. Fix the hook or avoid `--hard`
   (use `git checkout <branch>` to restore a clean tree).
5. **Flaky session tool I/O** (delayed / batched / swallowed stdout) already
   caused a code-clobber incident. Run the resume on a stable host and issue
   one tool call at a time.

## Cleanup
- DELETE the abandoned branch `feat/integrator-switching` — it holds TWO BAD
  commits (`87c2f093`, `75ab1dcf`) that clobbered
  `apps/api/internal/rbac/role.go`, `apps/admin/internal/tenant/create.go`,
  and `apps/admin/cmd/lecrm-admin/main.go`. **DO NOT merge it.**
  (`git branch -D feat/integrator-switching` once on another branch.)
- Also delete the empty `feat/integrator-switching-v2` if still present.

## Design notes for the rebuild (already scoped during the paused run)
- Tasket 1: next free migration number is **0018** (0001-0017 exist; 0010 is
  taken). App read role is `lecrm_api`; provisioner is `lecrm_provisioner`.
  `core.workspace_members.role` CHECK is the inline auto-named
  `workspace_members_role_check`. Add `rbac.RoleIntegrator` surgically — keep
  `RoleNone` / `roleFromScopes` / the json struct tags; `PermissionsFor`
  already yields owner-equivalent once integrator outranks owner (it uses
  `AtLeast`). Postgres functional PK is illegal -> use a UNIQUE INDEX on
  `(workspace_id, lower(email))`.
- Tasket 2: the real `tenant.Create` API is
  `Create(ctx, conn, CreateOptions{Slug,AdminEmail,OwnerEmail,...}, stdout)`
  using `callWrapper` (fresh/upsert) and an inline tx (force-recreate) that
  call the SECURITY DEFINER `core.lecrm_provision_workspace_with_registry`.
  Add the grant insert there; `creatorEmail = OwnerEmail || AdminEmail`. The
  CLI (`apps/admin/cmd/lecrm-admin/main.go`) uses urfave/cli v2 with top-level
  `Commands` + `Subcommands` funcs. Test harness: `dbtest.StartPostgres(t)` +
  `dbtest.Connect(t, dsn)` + `migrations.Apply(ctx, dsn)`, package
  `migrations_test`, module `github.com/gbconsult/lecrm/packages/db`.
- Tasket 3: NOTE there IS a `apps/api/internal/members` package (handler.go /
  store.go) — the "exclude integrator from members list" step DOES apply
  (the paused run wrongly assumed it was absent).

## Done When
- [ ] `docker info` succeeds
- [ ] `pnpm -v` works
- [ ] `go version` works without an absolute path
- [ ] `cd packages/db && go test ./migrations/... -count=1` RUNS (not just
      compiles) and is green
- [ ] abandoned `feat/integrator-switching` branch deleted
