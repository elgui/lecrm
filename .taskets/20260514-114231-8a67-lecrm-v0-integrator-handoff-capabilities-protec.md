---
id: 20260514-114231-8a67
title: "leCRM v0 — Integrator-handoff capabilities (protects the Distribution moat)"
status: later
priority: p1
created: 2026-05-14
updated: 2026-05-14
category: engineering
group: lecrm-v0-execution-discipline
order: 5
---

## Read this cold — full context inline

The PRD Executive Summary names "Distribution — Léo's accumulated HubSpot-buyer market intelligence and qualified pipeline" as the 4th moat — the ONLY moat component that cannot be reproduced by a capitalized competitor in 18 months. If Léo is the channel, the codebase must protect HIS unit of work. Three concrete capabilities are load-bearing for this moat — none of them are in v0 scope as written, and all three need to ship before the first Vernayo client migration.

## Why this exists

From PRD step-02 round-2 (Winston, 2026-05-14, on Distribution as 4th moat):

> "If Léo is the channel, the codebase must protect his unit of work: the integrator handoff. Concretely: (a) tenant provisioning has to be a one-command operation, not a checklist; (b) the 'acquisition channels → pipeline stages → stage properties → automations' methodology needs to be expressible as a versioned config artifact Léo can author, diff, and replay across clients; (c) audit/observability on per-tenant changes so Léo can debug what he shipped. None of this is in v0 scope as written. Flag it."

Reinforced by Mary (round 1): Léo's distribution is the relationship asset; if the codebase makes integrator-handoff painful, the channel stalls.

## Prerequisite (DOR)

- Tenant model committed — schema-per-tenant scaffolding live (post-`20260510-202450-b844` Part B).
- ADR-010 (tasket `20260514-114217-3c84`) committed — the methodology config storage shape depends on the metadata-engine pattern.
- Léo's methodology reference accessible at `${PAI_DATA_DIR}/35_CLIENTS/Leo/README.md` (5-element schema: acquisition channels → pipeline stages → stage properties → automations → color coding).

## Approach

### Capability 1 — One-command tenant provisioning

Build (or formalize from scaffolding) a CLI/script that takes minimal input — tenant slug, admin email, Léo-as-creator metadata — and provisions:

- Postgres schema per ADR-001
- Default roles for multi-user RBAC
- Default pipeline structure (configurable later)
- Audit logging enabled
- OAuth client placeholder
- Default methodology config (see capability 2)

Target: `lecrm tenant create <slug> --admin <email> [--template <name>]` (or equivalent), ≤30 seconds end-to-end. NOT a Notion checklist Léo follows by hand.

### Capability 2 — Versioned methodology config artifact

Design + implement a per-tenant config schema that captures Léo's 5-element methodology:

- **Acquisition channels** — list, with attribution metadata
- **Pipeline stages** — ordered, with stage-properties + entry/exit conditions
- **Stage properties** — typed fields required to advance a deal between stages
- **Automations** — triggers + actions on stage transitions
- **Color coding** — referenced for UI; spec at `${PAI_DATA_DIR}/35_CLIENTS/Leo/...`

Config must be:

- **Stored versioned** — git-tracked YAML or DB rows with version history; either is fine. Pick whichever aligns with ADR-010.
- **Authorable by Léo without engineer involvement** — CLI is fine for v0; admin UI lands v1+.
- **Diffable** — `lecrm config diff <client1> <client2>` shows divergence.
- **Replayable** — apply config from client1 onto a new client2 tenant in one step.

### Capability 3 — Per-tenant audit + observability surface

Léo needs to debug what he shipped without escalating to Guillaume:

- Per-tenant audit log (who/what/when for every config change + every record-level write)
- Per-tenant query surface (Léo answers "did the automation fire?" without DB access)
- Minimum for v0: a `/admin/audit?tenant=X` endpoint Léo authenticates against. Richer dashboard is v1+.

## Done When

- [ ] Tenant provisioning is a single command (documented in repo README), demoed end-to-end in ≤30s
- [ ] Methodology config schema committed; one example config (Léo's standard CRM-integrator template) checked in to the repo
- [ ] Config diff + replay verified by exercising "clone client X's config to new client Y" against a test tenant
- [ ] Per-tenant audit log captures config changes; Léo-facing query endpoint or admin view exists
- [ ] Léo (or proxy) does a dry-run handoff on a test tenant and confirms the workflow is usable without engineer support

## References

- `{output_folder}/planning-artifacts/prd.md` — Exec Summary §What Makes This Special #4 (Distribution moat)
- `${PAI_DATA_DIR}/35_CLIENTS/Leo/README.md` — Léo's profile + integrator methodology reference (5-element schema)
- `docs/adr/ADR-001-tenancy-model.md` — schema-per-tenant baseline
- Tasket `20260514-114217-3c84` — ADR-010 (metadata pattern affects how the methodology config is stored)
- Tasket `20260510-202450-b844` Part B — Week-1 scaffolding (tenant model source)
