---
id: 20260528-142628-2702
title: Confirm Brevo plan tier for v1 inbound parse webhook
status: parked
priority: p2
created: 2026-05-28
updated: 2026-05-28
tags: [brevo, v1-readiness, external-dep, sales]
category: project
group: lecrm-v1-readiness
group_order: 80
order: 3
plan: true
---

## Why

The v1 sequences engine depends on Brevo's **inbound parse webhook** (`apps/api/internal/email/brevo/inbound.go` — not yet implemented). Inbound parse is plan-tier-gated on Brevo: Starter does NOT include it, Standard/Business does. We need a confirmed plan path before v1 code is written, otherwise the engine has no reply-detection input.

**Status `parked` until sales confirmation lands.**

## What needs answering

1. **Current Brevo plan** — read from the account dashboard or last invoice.
2. **Required plan for inbound parse** — confirm from current Brevo pricing page (not from cached docs — pricing changes).
3. **Upgrade timing** — when do we move tiers? Tied to first paying client revenue per the v1 strategy (post-first-paying-client, target 2026 Q4).
4. **Cost delta** — monthly delta and any per-volume overage clauses.
5. **Inbound endpoint shape** — confirm Brevo's actual inbound webhook payload (sender, recipient, in-reply-to, message-id, headers, parsed body) matches what ADR-004 rev. 2 will assume.

## Steps

1. Check current Brevo plan tier from `https://app.brevo.com/settings/plans` (manual — paste the plan name and renewal date into the evidence section below).
2. From Brevo docs, capture the specific feature requirement for inbound parse. Source link goes in the evidence section.
3. Loop in Léo (`leo@vernayo.com`) — he holds the GB Consult / Vernayo budget for the email stack. Document the decision (upgrade now / upgrade on first-client / stay on Starter and use a different inbound path).
4. If the answer is "different inbound path," identify it (Postmark inbound? Mailgun routes? SES + S3?). Update ADR-003 (Brevo provider decision) accordingly — that ADR may itself need a partial revision.
5. Record the decision as an addendum at the bottom of `docs/adr/ADR-003-email-provider-brevo.md`.

## Source hygiene

The Brevo pricing page and feature matrix MUST be linked in the addendum (per CLAUDE.md Source Hygiene rule). Do not assert pricing or feature gating without a URL next to the claim.

## Done when

- `docs/adr/ADR-003-email-provider-brevo.md` ends with an addendum dated 2026-XX-XX recording:
  - Current plan + renewal date.
  - Plan required for inbound parse (with source URL).
  - Upgrade trigger (e.g., "on first paying client signature").
  - Inbound webhook payload shape link.
- Léo has confirmed the decision in writing (email / Slack thread linked in the addendum).

## Evidence (fill in as you go)

```
Current plan:        [paste from Brevo dashboard]
Renewal date:        [paste]
Required plan:       [paste with source URL]
Monthly cost delta:  [paste]
Upgrade trigger:     [paste decision]
Inbound payload doc: [paste URL]
Léo confirmation:    [paste link]
```

## References

- `docs/adr/ADR-003-email-provider-brevo.md` (current Brevo decision)
- `docs/adr/ADR-004-sequences-architecture.md` (open dep: inbound webhook contract — see sibling tasket `lecrm-v1-readiness/order:1`)
- LeoCollab skill — Léo owns the email stack budget conversation
