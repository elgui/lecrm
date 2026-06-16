---
id: 20260614-154815-a81e
title: OOO classifier (rules + Haiku)
status: done
priority: p2
created: 2026-06-14
updated: 2026-06-15
tags: [sequences, v1, ooo, classifier]
category: project
group: lecrm-v1-build
group_order: 300
order: 7
plan: true
done: 2026-06-15
---

# OOO classifier (rules + Haiku)

## Why
ADR-004 rev 2 §5: a two-stage out-of-office classifier decides whether a detected reply is a genuine human reply (`reply_received`) or an auto-responder (`ooo_detected`, reschedule after the return date). Rules catch ~95% at zero cost; Haiku handles the ambiguous tail. The interface is deliberately swappable (the most-likely-rewritten module in v2).

## Steps
1. `apps/api/internal/sequences/ooo/rules.go` — FR + English regex set; freeze a ~120-sample anonymised fixture from the research dataset for unit tests (~95% precision target).
2. Haiku fallback for ambiguous cases — `claude-haiku-4-5-20251001` (ADR-005), prompt-cached; cost ceiling ~$0.50/mo at 10k replies.
3. `ooo/dateparse.go` — return-date extraction ("de retour le 15 mai"); unparseable → reschedule `+5 business days`.
4. Expose the one-method interface `Classify(ctx, ReplyBody) (Category, Confidence, OOOReturnDate, error)` — swappable without touching the state machine.
5. Wire into the `poll_reply` path so a match routes to `reply_received` vs `ooo_detected` (sets `ooo_returns_at`).

## Done when
- Rules fixture passes at the precision target; ambiguous samples fall through to Haiku.
- Return dates parse (or default `+5 business days`); `ooo_detected` sets `ooo_returns_at` and reschedules.
- `Classify` is the only coupling between the classifier and the state machine.

## References
- ADR-004 rev 2 §5 (classifier), §Q1 (rules-vs-ML trigger)
- ADR-005 (model selection)
- Depends on order:4
