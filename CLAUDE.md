# leCRM — working agreement

Project-specific guardrails for agents. Read before editing or deploying.
High-level infra/CI/deploy map: `docs/INFRASTRUCTURE.md` (authoritative).

## ⚠️ Deploy target moved to Netcup (cutover 2026-06-14)

The **live staging + public demo** (`https://demo.lecrm.gbconsult.me`) now runs on
the **Netcup box `152.53.143.175`** (`lecrm-staging`, arm64) — the wildcard
`*.lecrm.gbconsult.me` DNS points there. Compose project lives at `/opt/lecrm` (a
**non-git** file tree), Docker runs as **root** (no `sg` shim). Authoritative map:
`docs/INFRASTRUCTURE.md`.

The OVH box `51.77.146.49` (`vps-25b8e3b3`, this `/home/gui/Projects/leCRM`
checkout) is the **OLD** staging host, superseded at the cutover (decommission
pending). **Rebuilding here does NOT affect the public demo.** Always `hostname`
to know which box you're on.

Deploy-from-working-tree still bites on **whichever box you rebuild**: `docker
compose ... up -d --build` compiles the image from the working tree, not a
committed ref. So:
- **Commit your change before (or as part of) deploying it** — never ship a dirty
  tree; pin `LECRM_IMAGE_TAG` or commit first so the image is reproducible.
- On Netcup, `/opt/lecrm` has **no `.git`** — update it by syncing tracked source
  from a clean checkout (e.g. `git archive <ref> apps packages deploy/compose
  deploy/Dockerfile | ssh root@152.53.143.175 tar -xf - -C /opt/lecrm`), then
  rebuild. `git pull` does not apply there.

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

Full runbook: `deploy/README.md` → "Staging"; authoritative map:
`docs/INFRASTRUCTURE.md`. **Live host: Netcup `152.53.143.175`**
(`ssh root@152.53.143.175 -i ~/.ssh/lecrm_netcup_ed25519`), compose project at
`/opt/lecrm`, Docker as root. Public demo `https://demo.lecrm.gbconsult.me`
(health: `/healthz` → 200). Rebuild + restart the API (it embeds the React SPA)
from `/opt/lecrm`:

```bash
docker compose --env-file deploy/.env.staging \
  -f deploy/compose/postgres.yml -f deploy/compose/api.yml up -d --build api
```

DB migrations + per-workspace River setup run via `lecrm-migrate apply` then
`lecrm-migrate river-setup --all` (superuser DSN on loopback `127.0.0.1:54320`).
Gmail reply detection is gated by `LECRM_GMAIL_*` in `deploy/.env.staging` (unset
→ push route unmounted, runtime not started).

CI (`.github/workflows/ci.yml`) **builds/tests only — it does not deploy.**
Staging is updated by hand on the host, so a green `main` does not imply the
live image matches `main`.
