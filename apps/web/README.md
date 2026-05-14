# `apps/web` — leCRM v0 SPA

React 19 + Vite + TanStack Router + TanStack Query + Tailwind + shadcn/ui
scaffold. Builds to `dist/`, then the Go API embeds it via
`//go:embed dist/*` and serves it under `/*` while routing `/v1/*` and
`/auth/*` to REST handlers (per ADR-009 §5.1).

## Local development

```bash
pnpm install
pnpm dev          # http://localhost:5173 (proxies /auth, /v1, /healthz → :8080)
pnpm build        # emits dist/, then `go build` from apps/api picks it up
pnpm test         # vitest
pnpm typecheck
```

The dev server proxies API routes to the Go binary on `:8080` so the
SPA can sit behind the same `Domain=<workspace>.lecrm.fr` cookie scope
in dev as in prod (ADR-009 §5.2).

## shadcn/ui

`components.json` is pre-wired. Add components with the CLI:

```bash
pnpm dlx shadcn@latest add card input dialog
```

`@/` resolves to `src/` (see `tsconfig.app.json` + `vite.config.ts`).
