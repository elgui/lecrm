---
id: 20260614-154815-d8f9
title: "Preflight: suppression + volume caps + throttle + GlockApps scoring"
status: later
priority: p2
created: 2026-06-14
updated: 2026-06-14
tags: [sequences, v1, deliverability, suppression]
category: project
group: lecrm-v1-build
group_order: 300
order: 7
plan: true
---

# Preflight: suppression + volume caps + throttle + GlockApps scoring

## Why
ADR-004 rev 2 §8: before any Brevo send, `sequences.preflight` enforces deliverability discipline — suppression list, per-tenant volume caps, per-recipient throttle, bounce policy. Plus GlockApps pre-flight content scoring (ADR-003 Mitigation 2). These are the deliverability-moat guardrails; a bad send damages shared-account reputation.

## Steps
1. `sequences.preflight(ctx, tx, enrollmentID, stepIndex)` called inside the `send_step` job BEFORE any Brevo API call, sharing the workspace-scoped tx.
2. Enforce: per-tenant `monthly_send_cap` at worker entry; per-recipient throttle (≤1 step / 24h per `contact_id`); `email_suppression` pre-send check → `Transition(→ suppressed)`.
3. Bounce policy: suppress after 3 consecutive soft bounces; hard bounce / complaint → immediate suppression row.
4. GlockApps content scoring on sequence-template save: score <7/10 blocks activation (warning, admin can override with explicit confirmation).
5. v1 default integration is MANUAL via `ops/scripts/glockapps-preflight.sh`; API-automated triggering is an OPEN question (§Q2) — do NOT build the API integration at v1 unless costed/approved.
6. Tests: suppressed contact never re-sends; cap/throttle enforced; soft-bounce-3 suppression.

## Done when
- No send occurs for a suppressed/capped/throttled recipient; the enrollment transitions correctly.
- A template scoring <7/10 is blocked pending admin override.
- GlockApps runs via the documented manual script (no unapproved API integration).

## References
- ADR-004 rev 2 §8 (preflight/suppression), §Q2 (GlockApps integration tier)
- ADR-003 §Mitigations 1–3 (DKIM discipline, pre-flight scoring, list hygiene)
- Depends on order:3
