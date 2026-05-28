---
id: 20260525-1008-frontend-complete-embed-csv
title: "Frontend feature-complete + go:embed + CSV export"
status: done
priority: p1
created: 2026-05-25
updated: 2026-05-28
done: 2026-05-28
category: project
group: crm-frontend-rbac-export
group_order: 200
order: 2
plan: true
tags: [frontend, go-embed, csv-export, sprint-9]
---

# Frontend feature-complete + go:embed + CSV export

## Pre-flight: Verify Previous Tasket

Before starting, verify RBAC is implemented:

1. `grep -c 'RequireRole' apps/api/internal/http/` -- RBAC middleware present
2. `cd apps/api && go test -race -count=1 ./...` -- all tests pass
3. `git log --oneline -10 | grep -i "RBAC\|role"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

Sprint 9: all 8 v0 features must be complete in the React frontend, the SPA must be embedded in the Go binary for single-binary deployment, and tenant data export (CSV) ships.

Source of truth: `docs/sprint-plan.md` Sprint 9
Working directory: `/home/gui/Projects/leCRM`

## Steps

1. Complete frontend pages for all 8 features:
   - Contact list + detail + create/edit forms (react-hook-form + zod validation)
   - Company list + detail + create/edit forms
   - Deal list + detail + create/edit forms
   - Pipeline Kanban (from previous tasket — verify it's integrated)
   - Notes inline on entity detail pages (add/edit/delete)
   - Tasks panel (list, create, complete toggle, due date picker)
   - Custom properties editor on contact/deal detail pages
   - Member management under /settings/members (invite, role change, remove)
   - Settings page: workspace name, branding (placeholder for v0)

2. Embed SPA in Go binary:
   - Build: `cd apps/web && pnpm build` → produces `apps/web/dist/`
   - In `apps/api/cmd/lecrm-api/main.go` or equivalent:
     ```go
     //go:embed all:../../apps/web/dist
     var webDist embed.FS
     ```
   - Serve: Chi catch-all route serves embedded files for non-`/v1/` paths
   - SPA fallback: return `index.html` for any unmatched path (client-side routing)

3. Caddy proxy configuration:
   - `/v1/*` → Go API REST handlers
   - `/*` → Go API embedded SPA (single binary, no separate static server)
   - Verify CSP headers still work with embedded SPA

4. CSV export (feature 8):
   - `GET /v1/contacts/export?format=csv` — streams CSV download
   - `GET /v1/companies/export?format=csv`
   - `GET /v1/deals/export?format=csv`
   - Workspace-scoped (only tenant's own data)
   - Include custom properties as additional columns (flatten JSONB)
   - Set `Content-Disposition: attachment; filename="contacts_2026-05-25.csv"`
   - Per-tenant download — sovereignty pitch made concrete

5. Wire Vercel AI SDK 6 (no v0 use):
   - `pnpm add ai @ai-sdk/react` in apps/web
   - Import and configure but don't expose any AI features yet
   - Preserves v2 chat/voice optionality without frontend rewrite

6. Final verification:
   - Build Go binary with embedded SPA: `cd apps/api && go build -o lecrm-api ./cmd/lecrm-api`
   - Run binary: `./lecrm-api`
   - Navigate to `http://localhost:8080` → SPA loads
   - Navigate to all pages → all features work
   - Export CSV → file downloads correctly
   - Binary size check: should be 10-30 MB (Go binary + SPA assets)

## Done When

- [ ] All 8 v0 features have working frontend pages
- [ ] SPA embedded in Go binary via go:embed
- [ ] Single binary serves both API and SPA
- [ ] CSV export works for contacts, companies, deals (with custom properties)
- [ ] Vercel AI SDK 6 installed (unused at v0)
- [ ] `pnpm typecheck` + `pnpm build` pass
- [ ] Go binary builds and serves correctly
- [ ] Binary size < 30 MB

## Completion Verification

1. `cd apps/web && pnpm build` -- SPA builds
2. `cd apps/api && go build -o /tmp/lecrm-api ./cmd/lecrm-api` -- binary builds with embed
3. `ls -lh /tmp/lecrm-api` -- binary size check
4. `cd apps/api && go test -race -count=1 ./...` -- all tests pass
5. Commit: `feat: frontend feature-complete, go:embed single-binary, CSV export (Sprint 9)`

## References

- `apps/web/src/` — existing React scaffold
- `apps/api/cmd/lecrm-api/` — Go binary entry point
- `docs/sprint-plan.md` Sprint 9
- ADR-009 §5.1 — go:embed deployment pattern
