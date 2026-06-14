---
id: 20260528-142652-8580
title: v1 kickoff signal — unpark lecrm-v1-build/aa6f and update group_order
status: next
priority: p2
created: 2026-05-28
updated: 2026-06-14
tags: [v1-readiness, kickoff, milestone]
category: project
group: lecrm-v1-readiness
group_order: 80
order: 4
plan: true
---

## Why

Final gate of the v1 readiness plan-group. When this tasket flips to `done`, v1 (`lecrm-v1-build`, tasket `aa6f`) becomes the next active build track.

> **2026-06-14 decision — first-client gate DROPPED.** Guillaume chose to
> decouple v1 from any "first paying client" milestone and keep the build train
> moving. The original precondition #4 ("First paying client signed") is removed;
> v1 kicks off as soon as the technical readiness gates below close. (The
> commercial motion proceeds in parallel and is no longer a blocker here.)

**Preconditions to flip `lecrm-v1-build` → `next`** (ALL must hold):

1. Sibling tasket `lecrm-v1-readiness/order:1` (ADR-004 rev. 2) — `done`.
2. Sibling tasket `lecrm-v1-readiness/order:2` (v0 ship-gate verification) — `done`.
3. Sibling tasket `lecrm-v1-readiness/order:3` (Brevo plan tier, tasket `2702`) — `done`. **← the only one still open; now `next`, run it first.**

## Steps when all preconditions hold

1. Update `.taskets/20260510-162158-aa6f-lecrm-v1-native-sequences-engine-track-f-post-firs.md`:
   - `status: later` → `status: next`
   - Bump `updated:` to today.
   - Add an `unparked:` line referencing this readiness tasket's commit.
2. Confirm v1-build's `group_order`. It already sits at `300` (sorts after the now-complete `lecrm-staging-deploy` group at 220, before `lecrm-v2-exploration` at 400) — that ordering is correct, so **no bump needed**. (The earlier "20→90" instruction was stale; aa6f was renumbered to 300 under the current feature-track convention.)
3. Re-read the v1 done criteria in `aa6f`. Reconcile against ADR-004 rev. 2 from sibling 1 — they should now match. If they don't, fix the tasket body before the run.
4. Verify the v1 tasket's open-dependencies section is empty (Brevo + ADR-004 should both be closed by readiness gates 1 & 3).
5. Decide whether v1 needs sub-taskets (likely yes — at minimum: enrollments DDL, river job framework, Brevo inbound handler, Gmail Pub/Sub watch, OOO classifier, GlockApps preflight). If so, run the Tasket `CreatePlanGroup` workflow against ADR-004 rev. 2 to spawn them as a v1-build sub-plan-group.
6. Commit the status flip + group_order bump under a single chore message:
   ```
   chore(taskets): unpark lecrm-v1-build — v1 readiness gates closed
   ```

## Evidence (fill in before flipping `done`)

```
ADR-004 rev. 2 commit:        [hash]
v0 ship-gate commit:          [hash]
Brevo plan-tier decision:     [link to ADR-003 addendum from tasket 2702]
```

## Done when

- `.taskets/20260510-162158-aa6f-*.md` shows `status: next`, `group_order: 300` (unchanged), and an `unparked:` line.
- This tasket's evidence section is filled in with real values.
- A v1-build sub-plan-group exists (or the call was explicitly made not to — record the rationale here).

## Why this needs its own tasket

Not just a checkbox on tasket `aa6f` because the kickoff carries decisions (group_order bump, sub-plan-group creation) that should be auditable as a discrete commit and surfaced to Guillaume as a decision point before v1 work starts. (Earlier draft of this tasket said "reviewable by Léo" — Léo doesn't follow leCRM, see memory `feedback_leo_scope_lecrm`.)
