---
id: 20260511-164048-6e3d
title: Next-session priming — path (a) docs cleanup done; pick (b) resume PRD or (c) Week-1 scaffolding
status: done
priority: p0
created: 2026-05-11
updated: 2026-05-14
tags: [priming, orientation, session-handoff]
category: tooling
done: 2026-05-14
---

Read this first. The 2026-05-11 session executed path (a) of the previous priming (`20260511-160906-2fc8`) — docs cleanup — and committed it in `fee1c54`. Strategic and technical posture are now consistent across `docs/`. Pick the next thread below.

## TL;DR — pick exactly one of two paths

**(b) Resume the PRD workflow** (~half-day). PRD is paused at `step-02-discovery` — see `{output_folder}/planning-artifacts/prd.md` frontmatter `pauseState` block for the verbatim resume protocol (re-discover inputs under Path D + ADR-009 frame, advance to `step-02b-vision`). Inputs to drop: `docs/research/fork-management.md`. Inputs to add: `docs/adr/ADR-008`, `docs/adr/ADR-009`, `docs/research/stack-selection.md`, plus the now-revised `docs/STRATEGIC-OVERVIEW.md` §4 and `docs/FEASIBILITY-MEMO.md` §2-3. Produces a real PRD artefact; heavier than (a), lighter than (c).

**(c) Start Week-1 scaffolding execution** (open-ended; ADR-009 estimate is 1-2 weeks for the scaffolding alone, 11-13 weeks to first paying Design Partner). Tasket `20260510-202450-b844` Part B is queued for this. Caveat: tasket `20260510-202450-a5d3` (Week-2 Go ramp checkpoint, binding decision per ADR-009 §1.1) follows and is the first hard schedule gate. Don't start (c) unless you have at least 2 weeks of focused capacity AND Go 1.23 installed.

Recommended order: (b) → (c). The PRD orients the execution; without it, Week-1 scaffolding is unconstrained and may build the wrong shape.

## What changed in the 2026-05-11 session

### Doc rewrites (commit `fee1c54`)
- `docs/STRATEGIC-OVERVIEW.md` §4 rewritten around the four moat components: sovereign codebase + Leo's distribution + tailorization speed + transparent pricing. AI-native UX moved to v2 strategic optionality, not the v1 bet. Banner updated. Cross-referenced Mary's R3 council validation that "sovereign codebase + tailorization depth" is the durable core; Leo is the GTM accelerant.
- `docs/FEASIBILITY-MEMO.md` §2 (License) rewritten for the Apache 2.0 clean-room frame: no AGPL §13, no CLA-ratchet, no fork rebase tax. §3 (Build Roadmap) rewritten as the 11-13 week Week-1-through-12 v0 plan with the four binding schedule gates (Wk 2 Go ramp; Wk 5 ADR-010; Wk 6 metadata-engine scope; Wk 5-6 Google OAuth review kickoff). TL;DR's license bullet, timeline, and recommended path realigned. v3 banner flags which sections are canonical and which are pending light reissue.
- Tasket `20260511-160906-2fc8` (previous priming) marked done in commit `b106c21`.

### Doc consistency snapshot

| Doc | Status |
|---|---|
| `docs/STRATEGIC-OVERVIEW.md` | §2 + §4 aligned to Path D. §1 executive summary and §8 risk register still describe the fork frame in places — flagged in banner; pending light reissue. |
| `docs/FEASIBILITY-MEMO.md` | §2-3 canonical (Path D, ADR-009). §1 (Strategic Frame), §4 (HubSpot moat sizing), §5-§10 still describe the v2 fork frame in places — flagged in banner; pending light reissue. Where the doc is internally inconsistent, §2-3 are canonical. |
| `docs/ARCHITECTURE.md` | PENDING REWRITE banner still in place. Reference-only until stack-aware rewrite. **Defer** until Week-1 scaffolding produces real architectural feedback (per the previous priming's guidance). |
| `docs/adr/ADR-001` through `ADR-009` | All canonical and current. ADR-002 superseded by ADR-008 (preserved as historical record). |
| `docs/research/stack-selection.md` | Live, 5-researcher dossier + §11 council validation. Canonical for ADR-009. |

### Housekeeping pass (2026-05-11)
Tasket inventory audited per the Tasket Housekeep workflow. Result: clean.
- 8 active taskets, all correctly classified, no stale review flags except `b844`'s intentional Part-A/Part-B mid-triage flag.
- 6 terminal taskets (4 deleted-tombstone for dead Twenty-fork work, 2 meta-parents done).
- 1 done (the previous priming).
- No frozen-group-straggler signals (`lecrm-v0-build` group at 3/8 terminal, below the 0.7 threshold).

## Open active taskets — state at 2026-05-11

**Scaffolding group (`lecrm-v0-scaffolding`):**
- `20260510-202450-b844` Part B (Week-1 scaffolding) — status=later, p0. Part A complete (`6f490e0`); Part B is the actual code-writing work. Picked up by path (c).
- `20260510-202450-a5d3` (Week-2 Go ramp checkpoint) — status=later, p0. First hard schedule gate. Three concrete litmus tests per ADR-009 §1.1; if any blocks > 4h, switch to TypeScript+Hono (irrevocable). Downstream of `b844`.

**V0 build group (`lecrm-v0-build`):**
- `20260510-162158-1023` — secrets baseline (sops + age), p1, order 4. Bodies aligned to Go+PG+Apache 2.0.
- `20260510-162158-29dc` — Metabase reporting bridge, p2, order 2. Interim dashboard until v1 native dashboards land.
- `20260510-162158-499c` — Brevo transactional integration, p1, order 1. Aligned to ADR-003.
- `20260510-162158-aa6f` — v1 native sequences (Track F, post-first-client), p2, order 5. Deferred to v1 per ADR-009 §9.
- `20260510-162158-d1ba` — backup baseline (WAL-G + GPG + Hetzner Object Storage), p1, order 3. Aligned to ADR-006.

**Standalone:**
- `20260510-155549-11e5` — v2 Telegram bot prototype (chatbot-as-CRM-UI proof of concept), p2. Strategic optionality per STRATEGIC-OVERVIEW §4; do not start before first 5 clients live.

**Downstream of Week-1 scaffolding.** None of the v0-build taskets should be picked up before `b844` Part B commits — they assume the scaffolding (monorepo, sqlc, river schemas, Caddy, etc.) exists.

## v0 schedule gates (from ADR-009 §9)

| Gate | Week | Decision | If failed |
|---|---|---|---|
| **G1 — Go ramp litmus** | End of week 2 | 3 concrete tests per ADR-009 §1.1 pass within 4 h each | Switch stack to TypeScript+Hono (irrevocable) |
| **G2 — ADR-010 authored** | End of week 5 | Metadata-engine pattern recorded (per-tenant DDL primary; JSONB fallback) | Decision deferred to G3 — adds week-6 risk |
| **G3 — Metadata-engine scope** | End of week 6 | Cumulative metadata work ≤ 5 days | Fall back to JSONB `data` column on generic `objects` table per workspace schema |
| **G4 — Google OAuth review** | End of week 6 at latest | Application submitted to Google for production Gmail scopes | Week 11-12 deploy slips by 4-6 weeks |

Tasket exists for G1 (`a5d3`). G2 / G3 / G4 are not yet taskets; the next session should queue them when path (c) begins.

## Branch / worktree state at session close

Main branch tip: `b106c21 tasket: mark 2fc8 priming done — path (a) executed via fee1c54`. Previous commits this session: `fee1c54 docs: strategic-overview §4 + feasibility-memo §2-3 — Path D alignment`. Working tree clean.

No active worktrees. The auto-worktree from 2026-05-10 was already cleaned up before this session started (its automation report is on main as `210f535`).

## Specific resumption protocols

### Path (b) — Resume the PRD

Follow the verbatim protocol in the `pauseState` block at `{output_folder}/planning-artifacts/prd.md`. Summary:
1. Re-discover inputs: drop `docs/research/fork-management.md`; add `docs/adr/ADR-008`, `docs/adr/ADR-009`, `docs/research/stack-selection.md`, `docs/STRATEGIC-OVERVIEW.md` (§4 revised 2026-05-11), `docs/FEASIBILITY-MEMO.md` (§2-3 revised 2026-05-11).
2. Re-confirm classification: `saas_b2b` / `general` / **medium** / **greenfield** / flags `[phased-architecture, api-contract-load-bearing, schedule-risk-gates]`.
3. Advance to `step-02b-vision.md`.
4. Continue through the 11-step PRD workflow at user's pace.

### Path (c) — Week-1 scaffolding

Open tasket `20260510-202450-b844` Part B. Caveats:
- The earlier automated run hit a content filter on the automation harness (see `automation-report-ga-20260510-d4a8ff.md` content on main at `210f535`). Manual or interactive execution recommended; do not relaunch via the automator until the filter trigger is understood.
- Go 1.23 must be installed before starting.
- Week-1 scaffolding output feeds directly into the Week-2 Go ramp litmus test (`a5d3`). The litmus test is a binding, irrevocable decision — if Go blocks at >4h on any of three tests, switch the project to TypeScript+Hono.
- After `b844` Part B commits, queue follow-up taskets for G2 (Wk 5 ADR-010), G3 (Wk 6 metadata-engine scope), and G4 (Wk 5-6 Google OAuth review).

## Out of scope for this tasket

This tasket is *orientation only*. It produces no code, no doc, no decision. It exists to make the next session land in the right place fast.

## Acceptance criteria

- [ ] Next-session executor (human or agent) has read this tasket end-to-end.
- [ ] Path chosen (b or c) and a focused tasket either picked or created.
- [ ] This tasket marked `done` after the next session's first commit confirms forward motion.
