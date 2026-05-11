---
id: 20260511-160906-2fc8
title: "Next-session priming — orient on Path D state, then pick: (a) docs cleanup, (b) resume PRD, or (c) Week-1 scaffolding"
status: done
priority: p0
created: 2026-05-11
updated: 2026-05-11
category: tooling
group: lecrm-session-priming
order: 1
done: 2026-05-11
---

# Next-Session Priming

Read this first. This tasket exists so you know where things stand at the end of the 2026-05-10 / 2026-05-11 session and which thread to pick up.

## TL;DR — pick exactly one of three paths

**(a) Finish the documentation cleanup** (~1 hour). Two docs still describe the dead fork path: `docs/STRATEGIC-OVERVIEW.md` §4 (moat) and `docs/FEASIBILITY-MEMO.md` §2-3 (license posture + build roadmap). Both have banners flagging the supersession; both need a focused rewrite. Lowest cognitive load, satisfying close-out.

**(b) Resume the PRD workflow** (~half-day). PRD is paused at `step-02-discovery` — see `{output_folder}/planning-artifacts/prd.md` frontmatter `pauseState` block for the verbatim resume protocol (re-discover inputs, re-confirm classification under the Path D + ADR-009 frame, then advance to `step-02b-vision.md`). Heavier; produces a real PRD artefact.

**(c) Start Week-1 scaffolding execution** (open-ended; council estimate is 1-2 weeks). Tasket `20260510-202450-b844` Part B is queued for this. Caveat: tasket `20260510-202450-a5d3` (Week-2 Go ramp checkpoint, binding decision) follows, and is the first hard schedule gate from ADR-009 §Schedule. Don't start (c) unless you have at least 2 weeks of focused capacity.

Recommended order: (a) → (b) → (c). The doc cleanup unblocks the PRD discovery step; the PRD orients the execution work.

## Where things stand (2026-05-11)

### Strategic posture (locked)
- **Moat:** ownership + Leo's distribution + tailorization speed + transparent pricing. NOT AI-native UX. AI is v2 upside.
- **Pitch:** *"transparent, honest pricing with any kind of tailorization."*
- **ICP locked** (`docs/ICP-ARCHETYPE.md`): Marc / Anne / Pierre archetypes; French/EU SMBs 3-15 users who rejected HubSpot on price/sovereignty/customization.

### Technical foundation (locked)
- **Path D — clean-room reimplementation** (`docs/adr/ADR-008-clean-room-reimplementation.md`). NOT a Twenty fork. Twenty source may be read as architectural reference; no code copied.
- **Stack locked** (`docs/adr/ADR-009-stack-and-license.md`):
  - Backend: **Go 1.23** (net/http + Chi + sqlc + Atlas + river + zitadel/oidc). TypeScript+Hono is the documented fallback if the week-2 Go-ramp checkpoint fails.
  - Database: **PostgreSQL 17 schema-per-tenant** via per-workspace Postgres role + `ALTER ROLE search_path`, provisioned via a single `SECURITY DEFINER` function (`lecrm_provision_workspace`).
  - API: **REST + thin MCP adapter** in a separate `cmd/lecrm-mcp/` binary. GraphQL deferred to v2 only if a Design Partner demands it.
  - Frontend: **React 19 + Vite + TanStack Router/Query + shadcn/ui + Radix UI**, embedded in the Go binary via `//go:embed dist/*` for single-binary Phase-1 deploy.
  - Auth: **Authentik 2025.10 self-hosted at v0 → Zitadel Cloud EU/CH at v1+**.
  - Observability: **LGTM Compose stack at v0 → Grafana Cloud EU at v1+**.
  - Background jobs: **river** (Postgres-native, no Redis at v1).
  - License: **Apache 2.0**, with FSL-2.0-Apache-2.0 as a credible upgrade path if competitor protection becomes a real concern post-launch.
  - Hosting: **OVH-first** (French HQ, Strasbourg/Gravelines DCs, no US sub-processors). Ubicloud-on-Hetzner DE documented as Phase-2 fallback.
- **Honest v0 timeline: 11-13 weeks** to first paying Design Partner. **Council rating: P50 achievable, P80 not achievable at current scope.**
- **Four binding schedule gates** (per ADR-009 §Schedule):
  - **Week 2** — Go ramp litmus test (three concrete tasks; switch to TypeScript+Hono if any blocks > 4h). Tasket `20260510-202450-a5d3`.
  - **Week 5** — ADR-010 metadata-engine pattern decision (per-tenant DDL primary, JSONB fallback documented). Not yet a tasket.
  - **Week 6** — metadata-engine scope gate (fall back to JSONB if DDL pattern hits complexity ceiling). Bundled with week-5 ADR-010 decision.
  - **Week 5-6** — Google OAuth app review initiated (4-6 week external blocker for production Gmail scopes). If not started by end of week 6, week 11-12 deploy slips. Not yet a tasket — flag as a v0 prerequisite tasket the next session should queue.

### Documentation status

| Doc | Status | Action |
|---|---|---|
| `docs/adr/ADR-001` (tenancy) | Live, amended inline by ADR-009 §2 | none |
| `docs/adr/ADR-002` (Twenty fork) | **SUPERSEDED** by ADR-008 | preserve as historical record |
| `docs/adr/ADR-003` (Brevo) | Live | none |
| `docs/adr/ADR-004` (sequences) | Substantive intent survives; implementation re-scoped to Go + river — see tasket `aa6f` | flagged for future re-issue |
| `docs/adr/ADR-005` (AI agent tenancy) | Implementation-language + Redis/GraphQL assumptions contradicted by ADR-009 — see ADR-009 §Consequences/Negative + TO RESOLVE-12 | flagged for re-read after stack settles |
| `docs/adr/ADR-006` (backup/DR) | Live | none |
| `docs/adr/ADR-007` (encryption/secrets/audit) | Live | none |
| `docs/adr/ADR-008` (clean-room) | **The Path D decision record** | none |
| `docs/adr/ADR-009` (stack + license) | **The locked stack record** | none |
| `docs/research/stack-selection.md` | Live, 5-researcher dossier + §11 council validation | none |
| `docs/STRATEGIC-OVERVIEW.md` | §2 (Technical) **revised**; §4 (Moat) **pending revision** per its own banner | **path (a)** |
| `docs/FEASIBILITY-MEMO.md` | §2-3 describe the fork path; ADR-008 TO RESOLVE-5 captures the rewrite obligation | **path (a)** |
| `docs/ARCHITECTURE.md` | PENDING REWRITE banner; reference-only until stack-aware rewrite | bigger job; defer until after Week-1 scaffolding gives real architectural feedback |
| `{output_folder}/planning-artifacts/prd.md` | **PAUSED** at step-02; frontmatter `pauseState` has the resume protocol | **path (b)** |

### Tasket state

**Open / active:**
- `20260510-202450-b844` (status=later, no review flag) — week-1 scaffolding housekeeping. Part A complete (`6f490e0`). **Part B is the actual week-1 scaffolding work** — pick this for path (c).
- `20260510-202450-a5d3` (status=later) — week-2 Go ramp checkpoint, binding schedule decision.
- 6 rescoped v0-build / v1 / v2 taskets (status=later, review flags cleared 2026-05-11): `1023` (secrets baseline), `29dc` (Metabase reporting bridge), `499c` (Brevo transactional), `aa6f` (v1 native sequences), `d1ba` (backup baseline), `11e5` (v2 Telegram bot prototype). All bodies fully re-aligned to Go+PG+Apache 2.0 per ADR-009. **Downstream of Week-1 scaffolding** — do not pick up before tasket `b844` Part B is committed.

**Closed:**
- 4 deleted-tombstone taskets (`6149`, `8550`, `9466`, `c458`) — status=deleted with audit-trail review messages. Twenty-fork-specific work with no Path D analogue.
- 2 parent taskets (`001` technical deep-dive, `002` v0 build kickoff) — status=done.
- 1 stack-research tasket (`0c44`) — status=done; produced ADR-009 + `docs/research/stack-selection.md`.

### Branch / worktree state

Main branch tip: see `git log --oneline -1` (will be after this tasket's housekeeping commit if the session committed before closing).

**Parallel worktree exists:** `/home/gui/Projects/.worktrees/home_gui_Projects_leCRM/auto--lecrm-v0-scaffolding-20260510` on branch `auto/lecrm-v0-scaffolding-20260510`. The branch is **1 commit ahead of main** — `d55b697 docs: automation run report — ga-20260510-d4a8ff`. The report is a 135-line honest 0/3 failure verdict for an automation run that hit a content filter on b844, found c458 already deleted, and correctly skipped a5d3 as a future checkpoint. Status as of session close: pending the user's call on whether to (i) cherry-pick the report to main and delete the worktree+branch, (ii) leave the worktree quiescent, or (iii) discard the report. **Recommended: option (i)** — the report is useful audit history; preserving it on main and dropping the worktree closes the session cleaner.

## Specific resumption protocols

### Path (a) — Finish docs cleanup

1. **STRATEGIC-OVERVIEW.md §4 (Moat) rewrite** — replace the AI-native-interface-freedom framing with the new moat structure (ownership + Leo's distribution + tailorization speed + transparent pricing). Drop the v1/v2 stack chronology framing in favour of "v1 sells on ownership + tailorization; v2 monetises AI-native UX as a category if it materialises." Cross-reference Mary's R3 council validation that "sovereign codebase + tailorization depth" is the durable core, Leo is a GTM accelerant. Update the banner at the top of the doc to remove "§4 still requires revision" once done.
2. **FEASIBILITY-MEMO.md §2-3 rewrite** — replace the AGPL-fork license sizing with the Apache 2.0 clean-room frame; replace the 4-track parallel build with the actual Week-1-through-Week-12 plan from ADR-009 §Schedule. The §2 verdict line ("GO on the technical path") survives; the technical sizing underneath does not.
3. Commit as `docs: strategic-overview §4 + feasibility-memo §2-3 — Path D alignment` or split into two commits.
4. After completion: the entire docs/ tree is consistent with Path D + ADR-009. The next session can resume the PRD.

### Path (b) — Resume the PRD

Follow the verbatim protocol in the `pauseState` block at `{output_folder}/planning-artifacts/prd.md`. Summary:
1. Re-discover inputs (drop `docs/research/fork-management.md` from active inputs; add `docs/adr/ADR-008`, `docs/adr/ADR-009`, `docs/research/stack-selection.md`).
2. Re-confirm classification: `saas_b2b` / `general` / **medium** / **greenfield** / flags `[phased-architecture, api-contract-load-bearing, schedule-risk-gates]`.
3. Advance to `step-02b-vision.md`.
4. Continue through the 11-step PRD workflow at user's pace.

### Path (c) — Week-1 scaffolding

Open tasket `20260510-202450-b844` Part B. Caveats:
- Tasket originally hit a content filter on the automation harness (see `automation-report-ga-20260510-d4a8ff.md` on the auto-worktree branch). Manual or interactive execution recommended; do not relaunch via the automator until the filter trigger is understood.
- Go 1.23 must be installed before starting (the automation run failed in part because Go was absent).
- Week-1 scaffolding output feeds directly into the week-2 Go ramp litmus test (tasket `20260510-202450-a5d3`). The litmus test is a binding decision — if Go ramp blocks at >4h on any of three tasks, the project switches to TypeScript+Hono. Plan for the worst case in your week-2 calendar.

## Out of scope for this tasket

This tasket is *orientation only*. It produces no code, no doc, no decision. It exists to make the next session land in the right place fast.

## Acceptance criteria

- [ ] Next-session executor (human or agent) has read this tasket end-to-end.
- [ ] Path chosen (a / b / c) and a focused tasket either picked or created.
- [ ] This tasket marked `done` after the next session's first commit confirms forward motion.
