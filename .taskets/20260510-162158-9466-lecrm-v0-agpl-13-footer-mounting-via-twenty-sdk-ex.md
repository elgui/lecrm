---
id: 20260510-162158-9466
title: leCRM v0 — AGPL §13 footer mounting via twenty-sdk extension
status: deleted
priority: p1
created: 2026-05-10
updated: 2026-05-10
tags: [agpl, compliance, frontend, v0]
category: project
review: "superseded-by: docs/adr/ADR-008-clean-room-reimplementation.md + docs/adr/ADR-009-stack-and-license.md. Twenty-fork-specific work; no analogue under clean-room Apache 2.0 build. Cleared by housekeeping in tasket 20260510-202450-b844 Part A on 2026-05-10."
group: lecrm-v0-build
order: 7
plan: true
---

# leCRM v0 — AGPL §13 footer mounting

## Why this tasket exists
The v0 spine includes the `AGPLFooter.tsx` component scaffold in `packages/twenty-front/src/lecrm/`, but the component is not yet mounted. AGPL §13 ("Appropriate Legal Notices") requires that the source-link notice render on every page served. Before the first paying client onboards, the footer must be live.

Reference: ADR-002 §5, `packages/twenty-front/src/lecrm/README.md`.

## Done criteria
- [ ] Author a `twenty-sdk-lecrm` extension package that mounts `AGPLFooter` via Twenty's app-extension hooks (preferred path).
- [ ] If the extension API does not yet expose a global-footer hook, fall back to a single-line touch on Twenty's top-level layout component (cost: +1 file in the upstream-rebase conflict surface).
- [ ] Footer renders on every page (login, app, settings, error). Verified via Playwright smoke test in CI.
- [ ] Footer copy reviewed against `docs/LEGAL-PLAYBOOK.md` §7 (AGPL §13 wording approval — ADR-002 TO RESOLVE item 5).
- [ ] Footer text resolved at runtime from `/api/version` so the displayed revision tracks the deployed tag.
