---
id: 20260514-204724-fa6b
title: "leCRM v0 — Integrator handoff Phase 3: per-tenant audit + observability surface (Sprint 11)"
status: done
priority: p1
created: 2026-05-14
updated: 2026-05-28
done: 2026-05-28
tags: [integrator-handoff, distribution-moat, audit, observability, sprint-11]
category: engineering
group: lecrm-v0-sprint-11
group_order: 11
order: 3
plan: true
---

## Read this cold — full context inline

Split-out **Phase 3** of the integrator-handoff Distribution-moat work. Originally bundled in tasket `20260514-114231-8a67` (now superseded). Per sprint plan §Sprint 11 (Wk 11), this is the third capability — depends on Phase 1 + Phase 2 having shipped.

## Why this exists

PRD step-02 round-2 (Winston, 2026-05-14): "Audit/observability on per-tenant changes so Léo can debug what he shipped." Without this, every "did the automation fire?" question escalates to Guillaume; with it, Léo self-serves and the channel scales.

## Prerequisite (DOR)

- Phase 1 done — tenant CLI (`20260514-204646-dc1b`).
- Phase 2 done — methodology config artifact (`20260514-204706-731a`).
- Audit log infrastructure live (Sprint 7 work — ADR-007 catalogue with `actor_type` claim, `security.workspace_id_mismatch` event, mutation-path fail-closed).

## Approach — Capability 3: Per-tenant audit + observability surface

Léo needs to debug what he shipped without escalating to Guillaume:

- **Per-tenant audit log** — captures config changes (Phase 2 origin) + every record-level write
- **Per-tenant query surface** — Léo answers "did the automation fire?" without DB access
- **Minimum for v0**: a `/admin/audit?tenant=X` endpoint Léo authenticates against. Richer dashboard is v1+.

The audit log infrastructure already exists from Sprint 7 (`actor_type`, append-only, fail-closed on mutations). This phase wires a Léo-facing query view on top: a CLI verb, an admin endpoint, or both.

## Done When

- [ ] `/admin/audit?tenant=X&since=...&actor=...` REST endpoint live with workspace-scoped auth
- [ ] Léo can answer "did automation A fire on tenant X yesterday?" from the surface alone — no DB access required
- [ ] Config changes from Phase 2 emit audit entries with `actor_type=human_api` and identifiable Léo-as-creator metadata
- [ ] Léo (or proxy) does a dry-run handoff debugging exercise on a test tenant and confirms the workflow is usable

## References

- Supersedes Capability 3 of `20260514-114231-8a67`
- Sibling Phase 1 (`20260514-204646-dc1b`), Phase 2 (`20260514-204706-731a`)
- `docs/sprint-plan.md` §Sprint 11 (Wk 11) — Brevo + backup + observability + integrator audit surface
- `docs/adr/ADR-007-encryption-secrets-audit.md` — audit log catalogue (this consumes)
- `docs/adr/ADR-009-stack-and-license.md` §4.1 — service tokens + `actor_type` claim (audit attribution depends on this)
