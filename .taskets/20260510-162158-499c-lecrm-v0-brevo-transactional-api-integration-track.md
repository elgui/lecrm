---
id: 20260510-162158-499c
title: leCRM v0 — Brevo Transactional API integration (Track B)
status: later
priority: p1
created: 2026-05-10
updated: 2026-05-10
tags: [email, brevo, integration, v0]
category: project
review: "rescoped-by: docs/adr/ADR-009-stack-and-license.md on 2026-05-10. Body re-aligned to Go + Postgres 17 + REST + thin MCP adapter + Apache 2.0 + OVH-first hosting. See body for the re-scoped done-criteria. Original Twenty-fork-shaped plan superseded; intent preserved."
group: lecrm-v0-build
order: 1
plan: true
---

# leCRM v0 — Brevo Transactional API integration (Go HTTP client)

## Why this tasket exists

Per [ADR-003](docs/adr/ADR-003-email-provider-brevo.md), Brevo is the EU-resident email provider. Under the clean-room Go stack (ADR-009), integration is via a hand-rolled Go HTTP client wrapping Brevo's REST API — not a NestJS service wrapping `@getbrevo/brevo` v5 as in the original Twenty-fork plan.

**This tasket is downstream of [b844](20260510-202450-b844-lecrm-v0-twenty-fork-tasket-housekeeping-week-1-sc.md) (scaffolding) — start after the scaffold is up. The Go ramp checkpoint (a5d3) must have outcome=CONTINUE Go before this proceeds; if SWITCH was decided, re-write this tasket against `node-mailers/brevo-sdk` instead.**

## Re-scoped done criteria

- [ ] Brevo account created; sender domain authenticated (DKIM + SPF + DMARC) for the first Design Partner.
- [ ] `apps/api/internal/email/brevo/` Go package: idiomatic HTTP client wrapping `POST /v3/smtp/email`, the inbound webhook receivers (`delivered`, `hardBounce`, `softBounce`, `blocked`, `spam`, `unsubscribed`), and HMAC signature verification.
- [ ] river background-job handlers for `email.send.requested` (mutation path) and `email.event.received` (webhook ingest). Per-workspace `river_<workspace_base36>` schema per ADR-009 §8.3.
- [ ] `email_suppression` table per workspace schema; pre-flight check against the table on every send.
- [ ] Bounce-rate alarm: per-workspace, 7-day rolling window, complaint-rate >0.1% emits a `security.email_bounce_rate_high` event and pauses pending sends for that workspace.
- [ ] Audit-log emission per ADR-007 §3 on every `email.send.*` event with `actor_type` claim from the service token (`human_api` or `internal_service`).
- [ ] OpenAPI 3.1 surface for the email send endpoint (`POST /v1/workspaces/{id}/emails`) wired into `apps/api`.

## Out of scope

- Native sequences engine with reply detection (separate tasket aa6f — v1, post-first-paying-client).
- Per-client domain DNS API integration for automated DKIM provisioning (Cloudflare / OVH / Gandi) — v1 work.

## References

- [ADR-003](docs/adr/ADR-003-email-provider-brevo.md) (Brevo provider decision).
- [ADR-009](docs/adr/ADR-009-stack-and-license.md) §4 (REST), §4.1 (service tokens with `actor_type`), §8.3 (river job tenancy).
- [ADR-007](docs/adr/ADR-007-encryption-secrets-audit.md) §3 (audit-log catalogue).
