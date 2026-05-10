---
id: lecrm-002
title: "leCRM — v0 Build Kickoff: shallow fork, Brevo wiring, ops baseline, first Design Partner"
status: open
priority: p1
category: project
created: 2026-05-10
project: leCRM
project_root: /home/gui/Projects/leCRM
tags:
  - v0-build
  - shallow-fork
  - twenty-fork
  - brevo
  - ops-baseline
  - to-resolve-followups
group: lecrm-technical-foundation
order: 2
depends-on:
  - lecrm-001
---

# leCRM — v0 Build Kickoff

## Why this tasket exists

The technical foundation session (lecrm-001) is closed. The architecture is documented in `docs/ARCHITECTURE.md` + 7 ADRs in `docs/adr/`, the public AGPL-3.0 source repository is live at **https://github.com/elgui/lecrm**, and 5 deep-dive research artefacts are in `docs/research/`. The decisions that mattered most are now binding: pure tenancy consolidation by Phase 2, Brevo throughout, shared multi-tenant Telegram bot, public-GraphQL-with-service-token for AI agents.

The next session moves from architecture to **execution**. Per `FEASIBILITY-MEMO.md` §3, v0 to first paying Design Partner is 4-6 weeks of parallel-track build. One session can't do all of it — but it can lay down the spine (shallow fork import + DI override scaffold), wire one external dependency end-to-end (Brevo transactional), set up the per-client Docker template, and queue the rest into discrete sub-taskets so subsequent sessions can pick up specific tracks.

**Out of scope for this session:** sales discovery, brand naming, lawyer engagement, Leo briefing, billing structure migration. The Phase-0 commercial gate runs in parallel under `docs/STRATEGIC-OVERVIEW.md` §9.

## What's already in place (cold-start context)

- **Architecture documents** — `docs/ARCHITECTURE.md` (master, 5,354 words, 4 Mermaid diagrams) + 7 ADRs covering tenancy, fork management, email provider, sequences, AI agent tenancy, backup/DR, encryption/secrets/audit
- **Research artefacts** — 5 docs in `docs/research/` totalling ~17.7k words with cited sources; reusable reference material
- **Public fork repository** — https://github.com/elgui/lecrm (seed commit only: LICENSE, NOTICE, README, CHANGES.md, empty `gbconsult/` patch directory). Twenty's source has not yet been imported.
- **Strategic + commercial documents** — `STRATEGIC-OVERVIEW.md`, `FEASIBILITY-MEMO.md`, `ICP-ARCHETYPE.md`, `LEGAL-PLAYBOOK.md`, `HUBSPOT-PARTNER-BILLING-RESEARCH.md`. Read STRATEGIC-OVERVIEW first if cold.
- **Brevo sales email draft** — `docs/research/brevo-sales-email-draft.md` (FR + EN versions). Guillaume to send to confirm inbound parse plan tier (blocking v1 sequences architecture commitment).

## Goal of this session

Two concrete deliverables:

1. **A working v0 spine on local Docker** — Twenty imported as the upstream tree, `gbconsult/` patch directory populated with the OIDC strategy override + enterprise gate stub via NestJS DI provider override, `docker-compose.yml` for one-tenant local run, OIDC login flow tested against Google Workspace, version endpoint returning `twenty-X.Y.Z+lecrm.0`, AGPL §13 footer rendered.
2. **Discrete sub-taskets** for the remaining v0 tracks (B email layer wiring, C ops baseline, D embedded Metabase, E sales/legal track), each scoped to ~1 week of agent work, queued under the `lecrm-v0-build` group.

Plus the smaller follow-ups inherited from session 001 (see "Topics to address" below).

## Topics to address (priority-ordered)

### P0 — This session

#### 1. Vendor Twenty's source into the fork (Track A start)

The `elgui/lecrm` repo is currently a seed only. We need Twenty's actual source tree imported, with the soft-fork pattern from ADR-002:

- Clone latest stable Twenty release (target `twenty-2.2.0` or whatever's stable as of session date) into the working tree
- Configure the upstream remote (`twentyhq/twenty`) so future rebases work
- Tag the import commit as `twenty-2.2.0+lecrm.0`
- Verify Twenty builds and runs locally with default config (Postgres + Redis via docker-compose)
- Update CHANGES.md with the import event

**Reference:** ADR-002 §1 (rolling rebase pattern), ADR-002 §5 (file layout).

#### 2. Implement the DI override pattern for OIDC + enterprise gate (Track A core)

The patch directory `gbconsult/` is empty. Populate it per ADR-002 §2 — NestJS DI provider override pattern, single override module:

- `gbconsult/auth/oidc-strategy.ts` — Passport OIDC strategy (replaces the Enterprise-licensed upstream strategy). Use `openid-client` (already a Twenty dep).
- `gbconsult/enterprise/plan-service-stub.ts` — `EnterprisePlanService` always-valid stub.
- `gbconsult/auth/auth.module.override.ts` — single override module supplying both providers.
- Modify `app.module.ts` to import the override module **after** Twenty's `AuthModule` (this is the only upstream file edited; ADR-002 acknowledges this trade-off).
- Test: OIDC login flow against Google Workspace test tenant works end-to-end; `EnterprisePlanService.isFeatureEnabled('sso')` returns true unconditionally.

**Reference:** ADR-002 §2-3, FEASIBILITY-MEMO §2.

#### 3. AGPL §13 attribution surface

Per ADR-002 §5:
- UI footer: render `Powered by Twenty CRM (AGPL-3.0) — source: github.com/elgui/lecrm` on every page. Implement as a Twenty extension package (`twenty-sdk`), not a core file edit.
- Version endpoint: `/api/version` returns the running upstream + leCRM revision pair.
- Update README in the fork repo to mention the running version is at the footer link.

**Open item:** ADR-002 §5 currently references `github.com/gbconsult/lecrm`. The repo is actually at `elgui/lecrm` (org permissions blocked the gbconsult creation in session 001). Two paths — see P0 #6.

#### 4. One-tenant Docker compose template (Track C start)

Per ADR-001 (Phase 1 = VPS-per-client), each client gets a dedicated VM running their own Docker compose stack. Build the template:

- `ops/docker-compose.template.yml` parameterized by client (workspace name, subdomain, secrets path, Brevo API key, OIDC client config)
- `ops/provision-client.sh` — script that takes a client name + subdomain and produces a configured compose stack
- Hetzner CX22 sizing assumed (4GB RAM); validate Twenty + Postgres + Redis fits comfortably

**Reference:** ADR-001 §Phase 1, ARCHITECTURE.md §6.1, FEASIBILITY-MEMO §3 Track C.

#### 5. Send the Brevo sales email (low priority — non-blocking)

The draft is at `docs/research/brevo-sales-email-draft.md` (FR + EN). Useful to send when convenient — clarifies inbound parse plan tier and sub-account economics — but **does not block any v0/v1 commitment**. Per the updated [ADR-003 §TO RESOLVE item 1](../docs/adr/ADR-003-email-provider-brevo.md) and [ADR-004 §TO RESOLVE item 2](../docs/adr/ADR-004-sequences-architecture.md), inbound parse is the secondary catch-all reply path; the primary path (Gmail Pub/Sub + Microsoft Graph OAuth) covers >95% of EU SMB accounts on its own. Worst-case answer (inbound parse Enterprise-only) just means we drop the secondary path or fall back to self-hosted Postfix or Mailjet inbound. Five minutes of copy-paste when convenient.

#### 6. Repository-ownership decision

ADR-002 §5 specifies `github.com/gbconsult/lecrm`. The repo is currently at `github.com/elgui/lecrm` because `elgui` lacks `admin:org` scope on the `gbconsult` GitHub org. Two paths:

- (a) Transfer ownership to `gbconsult` org once admin permissions are sorted. Requires gh auth refresh with `admin:org` scope OR a different account with org admin rights. Update README + NOTICE + CHANGES.md to reference the new URL. Update the AGPL §13 footer string.
- (b) Keep `elgui/lecrm` as the canonical location. Update ADR-002 §5 to reference the actual URL. Simpler.

**Recommendation:** (b) for now. Transfer to `gbconsult` org when first paying client signs (puts the asset under the operator entity that takes the revenue). Update ADRs at that point.

### P1 — Queue as sub-taskets after this session

These don't fit in one session but should each become a discrete tasket after P0 #1-4 land:

#### B. Email layer — Brevo wiring
Brevo account setup, sender domain authentication template, transactional email integration (Twenty's `EmailModule` substitution via DI), bounce/complaint webhook → Twenty contact suppression, list hygiene unit. ADR-003.

#### D. Embedded Metabase reporting
Self-hosted Metabase pointed at Twenty's Postgres with workspace-scoped SQL queries, embed via iframe extension. v0 bridge; Cube.dev replacement is v1+. FEASIBILITY-MEMO §3 Track D.

#### F. Native sequences (v1, post-first-client)
Brevo inbound parse webhook + Gmail Pub/Sub Watch + Microsoft Graph subscriptions + IMAP IDLE fallback + BullMQ state machine + OOO classifier. ADR-004. Wait for Brevo sales reply before committing to inbound parse webhook architecture.

#### G. Backup baseline
WAL-G + GPG client-side encryption + Hetzner Object Storage. `archive_timeout=60`. Quarterly restore drill protocol. ADR-006.

#### H. Secret management baseline
sops+age repository for v0 secrets. Per-tenant secret namespacing for Anthropic API keys, OAuth client secrets, DKIM private keys. ADR-007.

#### Staging migration test (action 3 from session 001)

Before any phase-1→phase-2 client move, validate the schema-per-tenant migration on a staging shared cluster with two test workspaces. ADR-001 phase-1→phase-2 migration runbook is the script. **Trigger:** before the 5th paying client signs (Phase 2 trigger). Not urgent now (zero paying clients), but the runbook must be tested at least once before being run on real client data.

### P2 — TO RESOLVE rollup from session 001 ADRs

The architecture session surfaced ~40 TO RESOLVE items across the 7 ADRs. Most are research-y or sales-confirmable rather than code work; surface them here so they don't get lost. Work them as they become blocking.

**Blocking before v0 ships:**
- ADR-001: Wildcard SSL on `*.lecrm.fr` (or chosen apex). Required for multi-tenant routing.
- ADR-002: `@license Enterprise` file inventory + pre-commit hook to prevent accidental redistribution. Required before fork is operationally live.
- ADR-002: AGPL §13 footer wording approval (validate against `LEGAL-PLAYBOOK.md` §7).
- ADR-007: Twenty audit log code coverage validation — does the AGPL audit infrastructure log the events the GDPR-defensibility spec requires?

**Blocking before Phase 2:**
- ADR-001: PgBouncer 1.20+ availability on Hetzner Ubuntu image (need `track_extra_parameters`).
- ADR-001: Twenty `database:migrate` per-schema iteration audit (does it migrate all `workspace_*` schemas?).
- ADR-005: Anthropic EU endpoint + DPF certification status (data-residency compliance for AI features).

**Blocking before Phase 3:**
- ADR-006: Patroni etcd quorum sizing for streaming replication.
- ADR-006: Cross-region restore latency baseline (Hetzner NBG1 → OVH FR).
- ADR-001: `max_connections` phase-3 sizing reconfirm.

**Watch list (not blocking but track):**
- EDPB final guidance on backup erasure (ADR-007).
- Twenty CLA / license-ratchet monitoring (ADR-002, OpenTofu freeze playbook activates if triggered).
- pgBackRest coalition status — currently archived; if a maintained fork emerges, reconsider WAL-G choice (ADR-006).

Full TO RESOLVE lists live in each ADR's terminal section.

## Reference documents (read before starting)

In order of priority for cold start:

1. `docs/ARCHITECTURE.md` — read all of it (~30 min). The cold-start orientation.
2. `docs/adr/ADR-002-twenty-fork-management.md` — the operational manual for this session's primary work.
3. `docs/adr/ADR-001-tenancy-model.md` — informs the Docker compose template (Track C).
4. `docs/adr/ADR-003-email-provider-brevo.md` — informs Track B sub-tasket.
5. `docs/adr/ADR-005-ai-agent-tenancy.md` — out of scope for this session but read briefly for forward-compatibility awareness.
6. `docs/research/fork-management.md` — the deep-dive that informs ADR-002. Read if any ambiguity arises during the DI override implementation.
7. `docs/STRATEGIC-OVERVIEW.md` §9 — the Phase-0 → Phase-3 sequencing context.
8. `docs/FEASIBILITY-MEMO.md` §3 — the parallel-track build roadmap.

External technical anchors:
- Twenty repo + docs: https://github.com/twentyhq/twenty , https://docs.twenty.com/
- NestJS custom providers: https://docs.nestjs.com/fundamentals/custom-providers
- Forgejo soft-fork doc: https://forgejo.org/2024-02-forking-forward/
- AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.en.html

## Recommended approach for the next session

1. **Read** ADR-002 + ADR-001 + the architecture overview (~25 min).
2. **Plan mode** to break the v0-spine work into ordered tasks with explicit dependencies (~10 min). Surface anything that needs decisions to the user upfront.
3. **Track A execution** — vendor Twenty source, set up upstream remote, tag `twenty-X.Y.Z+lecrm.0`, verify upstream builds locally. ~30 min.
4. **DI override implementation** — write the three files in `gbconsult/`, modify `app.module.ts` to load the override, test OIDC login + EnterprisePlanService stub. ~2-3 hours.
5. **AGPL §13 attribution surface** — UI footer extension package, version endpoint. ~1 hour.
6. **Track C start** — Docker compose template + provisioning script. ~1 hour.
7. **Test a one-tenant local stack** end-to-end: docker-compose up, OIDC login, footer visible, version endpoint correct. ~30 min.
8. **Create sub-taskets** for B, D, F, G, H + the staging-migration-test gate. ~30 min.
9. **Commit, tag, push** to `elgui/lecrm`. ~10 min.

Realistic session length: 5-8 hours of agent work with periodic checkpoints. If running long, defer Track C to its own sub-tasket and end this session at "DI override + AGPL §13 surface tested locally."

## Constraints (binding for any decision)

- **Solo operator with AI-augmented dev velocity.** Every choice operable by one person.
- **Public AGPL-3.0 fork from day 1.** All commits land on `elgui/lecrm`. No private staging branches that don't roll back to public.
- **No upstream file edits except `app.module.ts` (the override loader).** All custom code lives in `gbconsult/`. ADR-002 §2 is the contract.
- **No sleeping on TO RESOLVE items.** If a P0 question can't be answered from public docs and blocks code, write it as an explicit blocker in this tasket, not buried in a comment.
- **EU data residency.** No US sub-processors except where DPF-certified. (Twenty itself is FR-based; OK.)
- **Cost-per-client at scale stays <20% of MRR through Phase 3.**

## Done criteria for this session

- [ ] `elgui/lecrm` contains imported Twenty source at a tagged release (e.g. `twenty-2.2.0+lecrm.0`)
- [ ] `gbconsult/` patch directory populated with at least: OIDC strategy, enterprise gate stub, override module
- [ ] `app.module.ts` modified to load the override module; this is the only upstream file edit
- [ ] OIDC login flow against Google Workspace tested end-to-end on local Docker
- [ ] AGPL §13 footer rendered on every page; `/api/version` endpoint returning the upstream + lecrm revision pair
- [ ] `ops/docker-compose.template.yml` + `ops/provision-client.sh` exist and produce a working one-tenant stack
- [ ] Sub-taskets created for tracks B, D, F, G, H + staging-migration-test
- [ ] Brevo sales email sent (Guillaume task, log in this tasket when done)
- [ ] CHANGES.md updated with the import event + override module additions
- [ ] All commits pushed to `elgui/lecrm` main, release tag pushed
- [ ] No P0 question silently unanswered — `TO RESOLVE` markers in tasket and / or new sub-taskets if anything is parked

## What to ignore in this session

- Sales positioning, ICP sourcing, pricing tiers — covered in `STRATEGIC-OVERVIEW.md` and `ICP-ARCHETYPE.md`; out of scope here.
- Brand naming — administrative; the `lecrm` repo name is fine even if the customer-facing brand differs later.
- Legal track (DPA, CGV, SLA, beta agreement, apporteur d'affaires) — covered in `LEGAL-PLAYBOOK.md`; runs in parallel under Phase-0.
- Repository ownership transfer to `gbconsult` org — defer until first paying client signs (P0 #6 recommendation b).
- Anything in `HUBSPOT-PARTNER-BILLING-RESEARCH.md` — administrative.

If anything in those files has a *technical* implication (e.g. wildcard SSL on `*.lecrm.fr` requires a domain registration, which is administrative but blocks Phase 2 multi-tenant routing), surface it back; otherwise leave it alone.

---

**Tasket ready. Next session: read ARCHITECTURE.md + ADR-002 first, then plan-mode the v0-spine work, then start with Track A (vendor Twenty source).**
