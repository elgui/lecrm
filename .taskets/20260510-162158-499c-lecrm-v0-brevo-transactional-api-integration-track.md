---
id: 20260510-162158-499c
title: leCRM v0 — Brevo Transactional API integration (Track B)
status: later
priority: p1
created: 2026-05-10
updated: 2026-05-10
tags: [email, brevo, integration, v0]
category: project
group: lecrm-v0-build
order: 1
plan: true
---

# leCRM v0 — Brevo Transactional API integration (Track B)

## Why this tasket exists
The v0 spine ships with Brevo SMTP wired into the per-client docker-compose template (`ops/templates/docker-compose.template.yml`). That covers transactional email via plain SMTP, but the Brevo *Transactional API* path (`POST /v3/smtp/email`) is what unlocks the v1 sequences architecture: per-message `messageId` for downstream reply correlation, webhook-driven event handling (`delivered`, `hardBounce`, `softBounce`, `blocked`, `spam`, `unsubscribed`, `inboundEmailProcessed`), and per-client domain authentication automation.

Reference: ADR-003 (Brevo provider decision, `docs/adr/ADR-003-email-provider-brevo.md`).

## Done criteria
- [ ] Brevo account created; sender domain authenticated (DKIM + SPF + DMARC) for the first Design Partner.
- [ ] `gbconsult/email/` module added with a `BrevoTransactionalEmailService` that wraps `@getbrevo/brevo` v5.
- [ ] Twenty's `EmailModule` substituted via DI override (same pattern as the EnterprisePlanService stub).
- [ ] BullMQ queue `email-event` consuming Brevo webhooks (`/api/email/events`), with HMAC signature verification and a DLQ.
- [ ] Webhook event handler writes hard-bounces and complaints into a new `email_suppression` table; pre-flight check against the table on every send.
- [ ] Bounce-rate alarm: per-workspace, 7-day rolling window, complaint-rate >0.1% auto-pauses sequences and tags admin.
- [ ] Brevo sales email (`docs/research/brevo-sales-email-draft.md`) sent (Guillaume task; log here when done).

## Open dependencies
- Brevo plan tier confirmation (Starter → Standard → Business) per ADR-003 §Plan tier.
- Per-client domain DNS API integration (Cloudflare / OVH / Gandi) is in scope for the v1 sequences sub-tasket (F), not this one.
