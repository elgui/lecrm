---
id: 20260514-204706-731a
title: "leCRM v0 — Integrator handoff Phase 2: versioned methodology config artifact (Sprint 9)"
status: later
priority: p1
created: 2026-05-14
updated: 2026-05-14
tags: [integrator-handoff, distribution-moat, methodology-config, sprint-9]
category: engineering
group: lecrm-v0-sprint-9
group_order: 9
order: 1
plan: true
---

## Read this cold — full context inline

Split-out **Phase 2** of the integrator-handoff Distribution-moat work. Originally bundled in tasket `20260514-114231-8a67` (now superseded). Per sprint plan §Sprint 9 (Wk 9), this is the second capability — depends on Phase 1's tenant CLI (`20260514-204646-dc1b`).

## Why this exists

PRD step-02 round-2 (Winston, 2026-05-14): "The 'acquisition channels → pipeline stages → stage properties → automations' methodology needs to be expressible as a versioned config artifact Léo can author, diff, and replay across clients."

Without this, every new client is a hand-tweaked copy of the previous and divergence is invisible. With this, Léo's 5-element methodology (per `${PAI_DATA_DIR}/35_CLIENTS/Leo/README.md`) becomes a portable, diffable, replayable asset.

## Prerequisite (DOR)

- Phase 1 done — `lecrm tenant create` CLI shipped (`20260514-204646-dc1b`).
- ADR-010 committed — methodology config storage shape depends on metadata-engine pattern (DDL-primary vs JSONB-primary chosen at `20260514-114217-3c84`, Sprint 4).
- Multi-user RBAC live (Sprint 8 sibling — feature 7).
- Pipeline Kanban view feature-complete (Sprint 8 sibling — feature 2).

## Approach — Capability 2: Versioned methodology config artifact

Design + implement a per-tenant config schema that captures Léo's 5-element methodology:

- **Acquisition channels** — list, with attribution metadata
- **Pipeline stages** — ordered, with stage-properties + entry/exit conditions
- **Stage properties** — typed fields required to advance a deal between stages
- **Automations** — triggers + actions on stage transitions
- **Color coding** — referenced for UI; spec at `${PAI_DATA_DIR}/35_CLIENTS/Leo/...`

Config must be:

- **Stored versioned** — git-tracked YAML or DB rows with version history; whichever aligns with ADR-010's chosen pattern.
- **Authorable by Léo without engineer involvement** — CLI is fine for v0; admin UI lands v1+.
- **Diffable** — `lecrm config diff <client1> <client2>` shows divergence.
- **Replayable** — apply config from client1 onto a new client2 tenant in one step.

One **example config** (Léo's standard CRM-integrator template) committed to the repo as the reference implementation.

## Done When

- [ ] Config schema implemented per ADR-010's pattern (DDL or JSONB)
- [ ] `lecrm config show <tenant>` / `lecrm config diff <t1> <t2>` / `lecrm config replay <src> <dst>` CLI verbs working
- [ ] One example config (Léo's standard template) checked into repo
- [ ] Replay tested end-to-end: clone client X's config onto a fresh client Y tenant, verify identical state
- [ ] Cross-link: Phase 3 (Sprint 11 — audit surface) captures config changes from this layer

## References

- Supersedes Capability 2 of `20260514-114231-8a67`
- Sibling Phase 1 (`20260514-204646-dc1b`), Phase 3 (Sprint 11 — audit surface)
- `docs/sprint-plan.md` §Sprint 9 (Wk 9)
- `${PAI_DATA_DIR}/35_CLIENTS/Leo/README.md` — Léo's integrator methodology (5-element schema)
- Tasket `20260514-114217-3c84` — ADR-010 (storage pattern this consumes)
