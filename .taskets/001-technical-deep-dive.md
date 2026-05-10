---
id: lecrm-001
title: "leCRM — Technical Deep-Dive: Architecture, Deliverability, Scalability, AI-Native UX"
status: done
priority: p1
created: 2026-05-10
updated: 2026-05-10
tags: [architecture, deliverability, scalability, multi-tenancy, ai-native-ui, sequences, security, disaster-recovery]
category: project
group: lecrm-technical-foundation
order: 1
done: 2026-05-10
project: leCRM
project_root: /home/gui/Projects/leCRM
---

# leCRM — Technical Deep-Dive Session

## Why this tasket exists

The strategic and commercial path for leCRM is now decided (GO verdict, see `docs/STRATEGIC-OVERVIEW.md`). The next step is **hardening the technical architecture** before any v0 build starts. This tasket scopes a fresh, focused session whose only job is to resolve the hardest technical questions and produce concrete artefacts the v0 build can be executed against.

**Deliberately out of scope for this session:** legal, billing, sales, contracts, micro-vs-SASU, apporteur d'affaires, HubSpot partner status, Leo briefing. All administrative concerns are parked. This session is **pure technical**.

## What leCRM is (one paragraph for cold-start context)

A managed-CRM-as-a-service for French/EU SMBs (3-15 users), built on a forked AGPL-3.0 Twenty CRM (`github.com/twentyhq/twenty`, NestJS + PostgreSQL + React, multi-tenant via workspace + subdomain). GB Consult hosts on EU infrastructure, customizes per client, operates end-to-end. Sold via Leo (Vernayo) as discreet introducer. The strategic moat is **complete UI freedom enabled by AGPL source access** — the long-term product is chatbot-driven / voice-driven / AI-agent-driven CRM that HubSpot's API rate-limits make impossible. v0 ships parity with HubSpot Sales Hub Pro for the ICP; v2 layers AI-native interfaces as premium add-ons.

## Goal of this session

Produce two concrete deliverables, written to the project:

1. **`docs/ARCHITECTURE.md`** — system architecture document covering tenancy model, service boundaries, data flow, deployment topology, scaling path
2. **`docs/adr/`** directory with one ADR (Architecture Decision Record) per resolved technical question (1-2 pages each, format: Context / Decision / Consequences / Alternatives Considered)

Plus optional research artefacts in `docs/research/` for any deep dives that warrant their own document (e.g., email-deliverability-deep-dive.md).

## Topics to address (priority-ordered)

### P0 — Must resolve before v0 build starts

#### 1. Email deliverability without spam markers
Sending CRM-generated emails (transactional notifications, sequences, eventually marketing email if a client needs it) without landing in spam. Hardest of the technical problems because failure is invisible until clients complain.

Specific questions to answer:
- **Provider choice:** Brevo (FR) vs Scaleway TEM (FR) vs Mailjet (FR) — pricing, deliverability quality, reply-tracking webhook quality, dedicated-IP option, DKIM/SPF/DMARC management, GDPR posture. Postmark is OFF the table (US-based, no DPF certification — see `LEGAL-PLAYBOOK.md`).
- **Per-client domain authentication:** how do we onboard each client's sending domain (DKIM, SPF, DMARC records in their DNS) with minimal friction? Documented onboarding script.
- **Sender reputation strategy:** shared IP via provider vs dedicated IP per client. At what client volume does dedicated IP make sense?
- **IP warming:** what's the cold-start protocol when we add a new client?
- **Bounce and complaint handling:** webhook → contact suppression in Twenty's data model. Implementation pattern.
- **Content scoring:** Spam Assassin / GlockApps / Mail Tester pre-flight before sequences go out
- **Reply tracking for sequences:** Gmail Pub/Sub vs IMAP IDLE vs Microsoft Graph subscription — implementation comparison, OOO classifier (Haiku), state machine
- **List hygiene:** auto-suppression of hard bounces, complaints, unsubscribes — single source of truth in Twenty
- **Volume planning:** what email throughput do we need at 5 / 10 / 20 clients? Provider tier sizing.

Output: `docs/research/email-deliverability.md` + ADR for provider choice + ADR for sequences/reply-detection architecture.

#### 2. Multi-tenant architecture (the big one)
Twenty supports multi-tenant via shared instance + workspace-per-subdomain (`IS_MULTIWORKSPACE_ENABLED=true`). But: shared DB at scale has known issues; per-client VPS isolates better but costs more.

Specific questions:
- **v0 model: 1 VPS per client (fully isolated)** vs **shared multi-tenant on 1 VPS (Twenty's default)**? Trade-off matrix.
- **Database isolation:** schema-per-tenant (one Postgres, multiple schemas) vs database-per-tenant (one Postgres, multiple databases) vs shared schema with workspace_id discriminator (Twenty's default). Performance, blast radius, backup/restore complexity, GDPR data isolation defensibility.
- **Tenant boundary security:** how do we guarantee no data leak across workspaces in a shared deployment? Twenty's row-level access in shared DB is via app-layer filtering — risk profile?
- **Resource limits per workspace:** prevent one noisy tenant from taking down others. CPU/memory/connection pool quotas.
- **Workspace provisioning:** new client onboarding script (subdomain, DNS, SSL, DB schema/space, default data, OIDC config). 30-min target end-to-end.
- **Backup strategy per client:** can we restore one client without touching others?
- **Upgrade strategy:** when Twenty ships a security patch, do we upgrade all clients simultaneously or staggered? Coordination cost.

Output: `docs/ARCHITECTURE.md` (main doc) + ADR for tenancy model.

#### 3. Twenty fork management
We have a shallow fork (3-5 core files modified for SSO, RLS, audit, enterprise gate stub). Twenty ships every 2 weeks. How do we keep the fork sane?

Specific questions:
- **Branching strategy:** main tracks our deploy; release/X.Y.Z branches per Twenty release we adopt; feature branches for our patches.
- **Rebase cadence:** monthly? per-release? Trigger criteria (security patch = mandatory; minor feature = optional).
- **Patch isolation:** keep our changes in a separate `gbconsult/` directory with hooks, or modify Twenty files in place? Diff hygiene matters for rebase pain.
- **Custom modules architecture:** which features go in:
  - (a) Twenty core fork (e.g., SSO substitution — must touch auth.module.ts)
  - (b) Twenty extension package via twenty-sdk (e.g., custom objects, workflow nodes, UI panels)
  - (c) Separate microservice over the API (e.g., AI agents, chatbot interfaces, email sender)
- **Versioning of our fork:** semver scheme that tracks both upstream Twenty version and our patch version (e.g., `twenty-2.2.0+lecrm-1.4`)
- **AGPL compliance publishing:** which GitHub org? Public repo from day 1? README that explains the fork's purpose without burning the brand?

Output: ADR for fork management + ADR for module-placement decision tree.

#### 4. Service boundaries and AI-native interface stack
The v2 vision (chatbot-as-CRM-UI, voice CRM, autonomous agents, LLM dashboards) requires a clear service architecture. v0 should design for v2 without prematurely building v2.

Specific questions:
- **Reference architecture:** Twenty fork as core ↔ AI-agent layer (separate service) ↔ chatbot interfaces (Telegram/WhatsApp/Slack via OpenClawing/Tele-Claude pattern) ↔ email sender ↔ voice transcription pipeline. Diagram + service contract definition.
- **AI agent tenancy:** per-client agent config (system prompt, allowed actions, model tier). Where stored? Twenty workspace metadata vs separate config DB?
- **State management for multi-turn AI conversations:** session storage, context window management, hand-off between agent and human user
- **Internal API for agents:** Twenty's GraphQL is rate-limited externally — but our agents run inside our infra, do we use the same API or a privileged internal channel? Auth model.
- **Cost control:** per-tenant Anthropic API budget caps. Billing pass-through to clients vs absorbed in MRR.
- **Existing CaaS infra reuse:** OpenClawing (CaaS) and Tele-Claude (Telegram bots) already exist. How does leCRM integrate vs duplicate?

Output: ADR for service architecture + ADR for AI agent tenancy + ADR for internal API model.

### P1 — Must resolve within first 3 months (before scaling past 3 clients)

#### 5. Scalability path 1 → 20 → 50 clients
At what client count does each layer become a bottleneck and what's the migration plan?

Specific questions:
- **Database scaling:** Twenty's shared PostgreSQL — at what workspace count does query performance degrade? Read-replica strategy. Connection pool sizing.
- **Application scaling:** NestJS server CPU/RAM at concurrent user load. Horizontal scaling via load balancer. Sticky sessions for workspace routing.
- **Queue scaling:** BullMQ + Redis — at what throughput does Redis need clustering?
- **Cost-per-client at each tier:** modelled in `STRATEGIC-OVERVIEW.md` Section 5; verify against actual VPS sizing.
- **Multi-region:** stay single-region (FR/DE) or consider EU-multi-region for client-residency preferences? Probably defer.
- **Caching layer:** when does Redis-as-cache become necessary? Probably 20+ clients with v2 AI features.

Output: ADR for scaling phases (Phase 1 / Phase 2 / Phase 3 architecture diffs).

#### 6. Security and GDPR data isolation
Beyond the legal-playbook DPA scope — actual technical implementation.

Specific questions:
- **Encryption at rest:** PostgreSQL pgcrypto or full-disk LUKS? Field-level for sensitive PII?
- **Backup encryption:** S3-server-side or client-side? Key management per client.
- **Secret management:** per-client Anthropic API keys, OAuth client secrets, DKIM private keys. HashiCorp Vault? Doppler? Kubernetes secrets? AWS Secrets Manager?
- **Audit log requirements:** Twenty's audit infrastructure is AGPL — what events to log for GDPR-defensibility (access, export, deletion, permission change)? Retention period.
- **Right-to-erasure implementation:** per-client soft-delete with retention vs hard-delete. Does it cascade to backups? (Common GDPR-defensibility question.)
- **Penetration testing posture:** when do we engage external pentesters? Probably Phase 3.

Output: ADR for encryption + secret management + audit log spec.

#### 7. Disaster recovery and operational playbooks
What's our RPO/RTO commitment and how do we actually meet it?

Specific questions:
- **Backup frequency and retention:** continuous WAL archive vs hourly snapshots vs daily dumps. Retention by client tier.
- **RPO commitment:** target ≤1 hour for v0; ≤15 min for production. Mechanism.
- **RTO commitment:** target ≤4 hours for v0; ≤1 hour for production. Mechanism.
- **Restore drill:** how often do we test restore? Quarterly? What's the runbook?
- **Failover capability:** at v0, single VPS = no failover. At v1+, what's the path? Hot standby? Active-active?
- **Provider failure scenarios:** Hetzner outage, Brevo outage, Anthropic outage — what happens to clients? Fallback paths.

Output: ADR for backup/restore + on-call runbook + DR test schedule.

### P2 — Resolve before v2 (AI-native features)

#### 8. AI-native UX prototypes — proof of concept work
Before promising chatbot-driven CRM as a v2 feature, prototype on the existing CaaS infrastructure to validate.

Specific tasks:
- Connect a Telegram bot to Twenty's GraphQL API — read deals, log a call, update stage, send follow-up. Proof of latency, error handling, multi-user isolation.
- Voice → text → action prototype. Whisper (local or API) + Claude classification + Twenty mutation. Proof of accuracy.
- Autonomous pipeline-watching agent prototype. Watches one workspace, identifies stale deals, drafts re-engagement emails. Proof of utility.
- LLM-driven dashboard ("ask your CRM") prototype. Cube.dev semantic layer + Claude agent. Proof of query quality.

Output: 4 prototype repositories + lessons-learned document.

## Reference documents (read before starting)

All in `/home/gui/Projects/leCRM/docs/`:

- `STRATEGIC-OVERVIEW.md` — synthesis and decision document; the single best 10-15 minute read
- `FEASIBILITY-MEMO.md` — full risk register, build roadmap, GTM positioning, fork architecture
- `ICP-ARCHETYPE.md` — beta client profile (informs which features must work end-to-end first)
- `LEGAL-PLAYBOOK.md` — sub-processor list, GDPR obligations, AGPL §13 implementation (technical implications only — skip the contract/billing parts for this session)
- `HUBSPOT-PARTNER-BILLING-RESEARCH.md` — skip; administrative

External technical anchors:
- Twenty repo: https://github.com/twentyhq/twenty
- Twenty docs: https://docs.twenty.com/
- Twenty multi-workspace setup: https://docs.twenty.com/developers/self-host/capabilities/setup
- Twenty extension SDK: https://www.npmjs.com/package/twenty-sdk
- AGPL §13: https://www.gnu.org/licenses/agpl-3.0.en.html
- Brevo transactional API: https://developers.brevo.com/
- Scaleway TEM: https://www.scaleway.com/en/transactional-email-tem/

## Recommended approach for the new session

This is a multi-stage problem. Don't try to solve everything in one shot.

1. **Read** the strategic overview + the fork-architecture section of the feasibility memo (~15 min)
2. **Plan mode** to break down the architecture work into a clean task list with dependencies (~15 min)
3. **Parallel research dispatch** for deep dives that need external sources:
   - Email deliverability best practices 2026 (Brevo vs Scaleway TEM technical comparison; per-client DKIM patterns)
   - Multi-tenant PostgreSQL patterns at SMB SaaS scale (schema-per-tenant vs shared-schema vs database-per-tenant)
   - AI agent tenancy patterns (how Vercel, Cloudflare AI, OpenAI organizations isolate per-tenant agent state)
4. **Spawn the architect agent** for the core architecture document — it knows how to structure ADRs and system diagrams
5. **Iterate** on individual ADRs as decisions get refined (~1-2 hours per ADR)
6. **Validate** by running through 3 concrete scenarios end-to-end:
   - "Onboard a new wine-distributor client (Persona B)" — provisioning, DNS, SSL, default data, OIDC
   - "Send a 5-step sequence to 100 contacts with reply detection" — email layer, queue, state machine, suppression
   - "Recovery from full VPS failure" — backup integrity, restore steps, communication

## Constraints (binding for any architecture decision)

- **Solo operator with AI-augmented dev velocity** — every choice must be operable by one person with Claude Code as a force multiplier
- **4-6 weeks v0 to first paying Design Partner live** — architecture must support a v0 that's bridge-heavy (Reply.io for sequences, embedded Metabase for dashboards) and evolves to native in v1
- **Tech stack fixed:** NestJS + PostgreSQL + React (Twenty's stack), Hetzner DE or OVH FR for hosting, Brevo or Scaleway TEM for email, Anthropic Claude APIs for AI
- **EU data residency mandatory** — no US sub-processors except where DPF-certified and explicitly justified
- **AGPL compliance** — fork published on GitHub, modifications publishable, footer attribution
- **Cost-per-client at scale** — infra must stay <20% of MRR through Phase 3 (20 clients)
- **No premature optimization** — v0 is allowed to be 1-VPS-per-client; v1+ optimizes

## Done criteria for this session

The session is complete when:

- [ ] `docs/ARCHITECTURE.md` exists and covers tenancy model, service boundaries, data flow, deployment topology, scaling phases — readable cold by a developer who hasn't seen this project
- [ ] At least 6 ADRs exist in `docs/adr/`: tenancy model, fork management, email provider, sequences/reply-detection, AI agent tenancy, backup/DR
- [ ] All P0 questions above have either a documented answer or a clearly scoped sub-tasket (`/vt`) for a follow-up session
- [ ] At least one identified prototype task is queued (one of the 4 P2 prototypes) so the AI-native v2 vision has tangible evidence by the time we sell it
- [ ] No P0 question is silently unanswered — explicit `TO RESOLVE` markers if anything is parked

## What to ignore in this session

- Anything in `LEGAL-PLAYBOOK.md` about contracts, DPA wording, lawyer engagement, micro-entrepreneur tax, SASU, apporteur d'affaires — pure administrative
- Anything in `HUBSPOT-PARTNER-BILLING-RESEARCH.md` — administrative
- Sales positioning, pricing tiers, ICP sourcing — covered in `ICP-ARCHETYPE.md` and out of scope
- Brand naming — administrative

If anything in those files has a *technical* implication (e.g., GDPR sub-processor list affects email provider choice), bring it back; otherwise leave it alone.

---

**Tasket ready. Next session: read `STRATEGIC-OVERVIEW.md` first, then this tasket, then start with P0 #1 (email deliverability) since it has the biggest unknown-unknowns.**
