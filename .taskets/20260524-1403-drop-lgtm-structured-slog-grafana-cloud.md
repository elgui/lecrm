---
id: 20260524-1403-drop-lgtm-structured-slog-grafana-cloud
title: "Drop LGTM stack — structured slog to Grafana Cloud free"
status: todo
priority: p2
created: 2026-05-24
category: project
group: council-architecture-hardening
group_order: 40
order: 4
plan: true
tags: [observability, infrastructure, resource-optimization]
---

# Drop LGTM stack — structured slog to Grafana Cloud free

## Pre-flight: Verify Previous Tasket

Before starting, verify Tasket 3 ("SECURITY DEFINER audit") completed:

1. `ls packages/db/migrations/0006_security_definer_hardening.sql` -- migration exists
2. `cd apps/admin && go test -race -count=1 ./internal/tenant/...` -- tests pass
3. `git log --oneline -10 | grep -i "SECURITY DEFINER\|definer"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

The council unanimously agreed that self-hosted LGTM (Loki, Grafana, Tempo, Prometheus + OTel Collector) is premature at v0 with <5 clients. It consumes ~1.1GB RAM (optimistic estimate — Authentik alone pulls 500+ MB under warm-up). On a single VPS also running Postgres, Caddy, Authentik, and the API binary, this is unjustifiable overhead.

Pennylane (French fintech, similar scale at launch) shipped Day-1 observability but used Datadog SaaS, not self-hosted. The correct v0 posture is structured logging to stdout + Grafana Cloud free tier (50GB logs/month, 10k series metrics).

Source of truth: `docs/council-architecture-review-2026-05-24.md`
Working directory: `/home/gui/Projects/leCRM`

## Approach

1. Keep `deploy/compose/lgtm.yml` for optional local deep-debugging (don't delete)
2. Remove LGTM from the default compose profile (it shouldn't auto-start)
3. Ensure apps/api already uses `log/slog` for structured logging (verify and enhance)
4. Add `tenant_id` / `workspace_slug` to every log line via slog context
5. Configure Grafana Cloud free tier as the log destination (via Promtail or Alloy lightweight agent)
6. Document the observability posture in a brief ops note

## Steps

1. Audit current compose setup:
   ```bash
   grep -r 'lgtm\|loki\|grafana\|tempo\|prometheus\|otel' deploy/compose/
   ```
2. If LGTM is included in a default `docker-compose.yml` or referenced in other compose files, remove the dependency (keep the file, remove the `include` or `depends_on`)
3. Verify `apps/api` uses `log/slog`:
   ```bash
   grep -r 'log/slog\|slog\.' apps/api/
   ```
4. If slog is not yet pervasive, add structured logging middleware to Chi router:
   - Request logging: method, path, status, duration, workspace_slug, user_id
   - Error logging: include request_id and workspace context
5. Add workspace context to slog in workspace middleware:
   - After resolving workspace, inject `slog.String("workspace", slug)` into the request's logger
6. Create `deploy/compose/observability-lite.yml`:
   - Grafana Alloy (lightweight agent, ~50MB RAM) shipping logs to Grafana Cloud
   - Or simply document that `docker logs lecrm-api | promtail` is sufficient at v0
7. Update `deploy/README.md` or create `ops/observability.md`:
   - Document: v0 = slog stdout + Grafana Cloud free. v1 = self-hosted LGTM when >20 workspaces
   - Include Grafana Cloud setup steps (API key, Loki endpoint)
8. Run the API and verify structured JSON logs include tenant context

## Done When

- [ ] LGTM stack is NOT auto-started in default compose profile
- [ ] `deploy/compose/lgtm.yml` still exists (optional, for local debugging)
- [ ] All API log lines include `workspace` field when in workspace context
- [ ] All API log lines include `request_id` for correlation
- [ ] Structured JSON output from slog (not plain text)
- [ ] Observability posture documented (v0 approach + v1 upgrade path)
- [ ] VPS memory footprint reduced by ~1GB (LGTM not running)

## Completion Verification

1. `grep -c 'slog' apps/api/internal/http/server.go` -- structured logging present
2. `grep -l 'workspace' apps/api/internal/workspace/middleware.go` -- workspace in logs
3. `ls ops/observability.md || ls deploy/README.md` -- documentation exists
4. `cd apps/api && go build ./...` -- builds clean
5. Commit: `chore(infra): defer LGTM stack, ship structured slog for v0 observability`

## References

- `deploy/compose/lgtm.yml` — current LGTM compose
- `apps/api/internal/http/server.go` — Chi router + middleware assembly
- `apps/api/internal/workspace/middleware.go` — workspace resolution (add log context here)
- `docs/council-architecture-review-2026-05-24.md` — council review
- Grafana Cloud free tier: 50GB logs/month, 10k series metrics
