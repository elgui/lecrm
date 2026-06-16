---
id: 20260531-145435-dca9
title: "L3: Per-record 'Assistant IA' docked placeholder panel (honest, branded, no input box)"
status: done
priority: p2
created: 2026-05-31
updated: 2026-05-31
done: 2026-05-31
tags: [lecrm, demo, ux, leo, frontend]
category: project
group: lecrm-leo-demo-polish
order: 9
plan: true
---

> Part of group **lecrm-leo-demo-polish** — the Léo integrator demo polish, sequenced.
> North Star: a demo to Léo (integrator, **desktop**) that is pleasant to use and to the eyes.
> Source review: `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/UX-REVIEW-FINDINGS.md` + screenshots in `/home/gui/Projects/leCRM/.taskets/ux-review-screenshots/` (01–19).
> Frontend lives in `apps/web` (React 19 + TanStack Router/Query, Tailwind + shadcn).
> Read before Write; scope `git add` to files you change (no `git add -A` — ambient drift in this tree).

## Why — the one differentiator moment
leCRM's moat is AI-native UX enabled by AGPL source ownership. The demo currently has **zero visible AI surface**. The BMAD party converged on locating the chatbot's seat at the **per-record detail level** (contact + deal). User decision: ship it as an **HONEST PLACEHOLDER** — not live, not faked — selling "this seat is yours to wire" with zero risk of collapsing under a follow-up question.

## What to do (in `apps/web`, on contact + deal detail)
- Add a slim **docked right-rail "Assistant IA" affordance** (sparkle icon) on the contact and deal detail pages.
- Clicking it opens a branded panel with copy like: **"Assistant IA — bientôt disponible. Connectez votre propre modèle — le code est à vous (AGPL)."**
- **NO open text input** — it's a reserved, branded seat, not an interactive bot. (Prevents the "ask a second question, illusion collapses" failure mode.)
- Match the calm visual system; make it feel intentional, not a TODO.

## Future-wiring contract (document in code comments / an ADR note for when it goes LIVE — Winston's guardrail)
- A single **SSE streaming endpoint** that builds the prompt **server-side from the authenticated workspace only** (no cross-tenant data in prompts).
- Route the LLM call through an **EU-region model** so the AI surface inherits the same data-residency/tenant guarantees as any other read — otherwise the first AI feature undercuts the EU-residency pitch.
- `/v1/ai/chat` is already reserved and `ai.ts` has `aiEnabled=false` — keep consistent with that.

## Acceptance
- Contact + deal detail show a branded, docked "Assistant IA" placeholder panel, no input box, on-brand copy.
- Future-wiring contract captured in code/ADR.
- `tsc`, `eslint`, `vitest` green in `apps/web`.
