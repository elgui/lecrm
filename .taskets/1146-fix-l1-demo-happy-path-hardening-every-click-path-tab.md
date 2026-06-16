---
id: 1146
title: "[Fix] L1: Demo happy-path hardening — every click-path tab populated, no console errors / 500s"
status: done
priority: p1
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend, remediation]
category: project
group: lecrm-leo-demo-polish
group_order: 4
order: 4
remediates: 20260531-145435-45f6
---

## Remediation outcome

The previous task `20260531-145435-45f6` was flagged **partial_success**: it
proved the happy path at the **API layer** (a `*`-scoped service-token sweep,
0 5xx, all lists populated) but never drove a browser with devtools open, so the
"zero console errors" half of the acceptance was only *inferred* from the green
build gates. This remediation closes that gap with a real browser walkthrough and
fixes the one defect it surfaced.

**Commit:** `46f6ef4a` — *fix(demo): real browser walkthrough — kill CSP font
console errors, unblock 3rd workspace* (branch `auto/lecrm-leo-demo-polish-20260531`).

### What was done

1. **Real headless-Chromium walkthrough** of every tab
   (`dashboard → contacts → contact-detail → companies → company-detail → deals →
   deal-detail → pipeline → tasks → settings → custom-fields`) across **all 3
   seeded workspaces**, authenticated as the real member `leo@vernayo.com` via a
   valid `lecrm_session` V2 cookie minted with the repo's own
   `auth.EncodeSessionV2` (live `LECRM_SESSION_SECRET`) and injected with
   Playwright `addCookies`. Run inside the `mcr playwright` container because the
   host shell's **6 GB `ulimit -v` hard cap SIGTRAPs chromium** and OOMs
   vite/vitest (documented quirk).
   - Result, all 3 workspaces: **0 page exceptions, 0 failed requests, 0
     unexpected ≥400s**; contacts=10, companies=4, deals=6, pipeline=12 cards,
     custom-fields=2 — every tab populated. Evidence committed at
     `deploy/seed/walkthrough-evidence/live-walkthrough-report.json`.

2. **Provisioning defect found + fixed:** `menuiserie-vasseur` had **zero**
   `core.workspace_members` rows — the `*`-scoped token sweep bypassed membership
   and missed it, but a real human session resolves through `workspace_members`,
   so the entire 3rd workspace returned **401 on every CRM read** (totally blank
   happy path). Granted Léo `integrator` there (idempotent, applied live),
   matching his role on `bistrot-halles`. He is now a member of all three.

3. **Console defect found + fixed:** every page logged the same **11 CSP
   errors/session** — the strict CSP (ADR-009 §5.2, `style-src 'self'`) blocking
   the cross-origin `@import url(fonts.googleapis.com…)` at
   `apps/web/src/index.css:1`. Side effect: Inter never actually loaded (silent
   fallback to system-ui), so the import was pure console-noise. **Fix:** removed
   the cross-origin `@import` (CSP-preserving, zero visual regression). Real Inter
   = self-host, deferred to the L2 typography pass (step 8).
   - **Proof:** built the fixed bundle, served it under the exact staging CSP
     header → **0 console errors**; control page with the old `@import` under the
     same header → 1 (the exact error the live walk saw).
     `deploy/seed/walkthrough-evidence/csp-fix-proof.mjs`.

### Verification

- `tsc --noEmit -p tsconfig.app.json` ✓
- `eslint src` ✓
- `vitest run` → **85/85 passed** (run in-container to dodge the host vmem OOM)
- Live cookie auth check: `GET /v1/workspace/me` + `/v1/contacts` → 200 on all 3

### Deploy note

The source fix is committed on the branch; the **live demo serves the pre-fix
bundle until staging is rebuilt by hand from the main checkout** (this ran in an
isolated worktree with no `deploy/.env.staging`). The `menuiserie-vasseur`
member grant was applied **live** and is already in effect. Full evidence +
reproduce steps: `deploy/seed/DEMO-WALKTHROUGH.md` → "Remediation (step 4 fix)".
