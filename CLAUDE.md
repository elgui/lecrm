# leCRM — working agreement

Project-specific guardrails for agents. Read before editing or deploying.
High-level infra/CI/deploy map: `docs/INFRASTRUCTURE.md` (authoritative).

## ⚠️ This checkout may BE the live deploy source

On host `51.77.146.49` (`vps-25b8e3b3`) the path `/home/gui/Projects/leCRM`
is the **staging checkout that deploys build from** — `docker compose ... up
-d --build` compiles the running image from the **working tree, not from a
committed ref**. Consequences every session must respect:

- **Check where you are before deploying:** `hostname` / `hostname -I`. If you
  are on `vps-25b8e3b3`, your uncommitted edits go live the moment someone
  rebuilds. Treat the working tree as production input.
- **Commit your change before (or as part of) deploying it.** Never rely on a
  dirty tree as the artifact — pin via `LECRM_IMAGE_TAG` or commit first so the
  running image is reproducible from history.

## Avoid working-tree drift (the #1 recurring problem)

This tree chronically accumulates uncommitted edits to unrelated Go/test files
(see INFRASTRUCTURE.md "Known divergences" #3). To stop it growing:

- **Scope every `git add` to the files YOU changed.** Never `git add -A` /
  `git add .` here — it would sweep in unrelated pre-existing drift. List paths
  explicitly (e.g. `git add apps/web/`).
- **Inspect `git status` first** and mentally separate *your* changes from the
  ambient drift. Commit only the former, with a narrowly-scoped message.
- **Don't "tidy up" or revert drift you didn't create** — it may be another
  agent's or Guillaume's in-flight work. Leave it; surface it if it blocks you.
- Branch off `main` for non-trivial work; don't pile onto a dirty `main`.

## Read before Write — never reconstruct a file from memory

Always `Read` a file's current contents immediately before `Edit`/`Write`, and
**never put the Read and the Write of the same file in one tool block** (the
Write composes from assumptions because the Read result returns only after the
block). A prior session clobbered ~738 lines this way; another clobbered the
three list routes. Prefer surgical `Edit`. Many routes are folders
(`routes/contacts/index.tsx`) using TanStack Router + react-query hooks
(`useContacts`, `data.data`) with inline create forms — verify, don't assume.

## Environment quirks

- Docker needs the group shim: `sg docker -c "docker ..."` (user `gui` is in
  the `docker` group but the login shell isn't refreshed).
- Go is at `/usr/local/go/bin/go` (not always on PATH).
- Web toolchain: `pnpm` (standalone) / `bun`; from `apps/web` use
  `node_modules/.bin/{tsc,vitest,vite,eslint}` directly. Gate changes on
  `tsc --noEmit -p tsconfig.app.json`, `eslint src`, and `vitest run`.
- A `security-validator` PreToolUse Bash hook intermittently errors and
  cancels the whole parallel tool batch — run Bash calls one per block,
  separate from file edits. It also blocks `git reset --hard`; use
  `git checkout -- <path>` to restore.
- Test Postgres must bind `127.0.0.1` only (a prior exposed test DB was
  crypto-mined). Never `docker run -p 5432:5432 postgres`.

## Deploy (staging) — quick reference

Full runbook: `deploy/README.md` → "Staging". Public demo:
`https://demo.lecrm.gbconsult.me` (health: `/healthz` → 200). Rebuild + restart
the API (it embeds the React SPA) from the host checkout:

```bash
sg docker -c "docker compose --env-file deploy/.env.staging \
  -f deploy/compose/postgres.yml -f deploy/compose/api.yml up -d --build api"
```

CI (`.github/workflows/ci.yml`) **builds/tests only — it does not deploy.**
Staging is updated by hand on the host, so a green `main` does not imply the
live image matches `main`.
