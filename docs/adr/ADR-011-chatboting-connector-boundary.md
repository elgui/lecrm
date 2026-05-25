# ADR-011 — First Connector Boundary: chatboting Prospection Pipeline

**Status:** Proposed
**Date:** 2026-05-25
**Deciders:** Guillaume
**Related:** [ADR-010](ADR-010-metadata-engine.md) (metadata engine pattern — amended by §3 of this ADR to add `json` property type). [ADR-009 §5](ADR-009-stack-and-license.md) (REST + thin MCP adapter). [ADR-001](ADR-001-tenancy-model.md) (schema-per-tenant preserved). Prospection Ritual Map (`chatboting/docs/prospection-ritual-map.md`).

---

## Context

leCRM is designed to work with connectors. The first connector candidate is chatboting's prospection pipeline — a mature, production system (38+ API endpoints, 8+ database tables, 105+ migrations) that runs daily outreach campaigns discovering, scoring, and inviting French SMB prospects to claim AI chatbots.

Today, chatboting's prospection pipeline lives entirely within chatboting: search discovery (Brave API), candidate enrichment and scoring (multi-axis Bayesian), email harvesting, invitation lifecycle, A/B experiment tracking, and coverage analytics. The daily operator ritual (`/prospection-target next`) interleaves strategic decisions (who to target, when, why) with operational execution (provision tenants, render emails, send invitations).

A Prospection Ritual Map (`chatboting/docs/prospection-ritual-map.md`, 2026-05-25) analyzed all 13 operator-visible steps of the daily ritual against four candidate boundary positions. The map found that **7 out of 13 steps cross the system boundary** under the naive "harvest is the handoff" split — failing the council's criterion of ≤2 synchronous boundary crosses.

The map surfaced a pragmatic middle ground (Option D) where only 2 boundary crosses remain, both reads, neither synchronous-blocking on the operator. This ADR records Option D as the first connector design.

### Design principles

1. **leCRM owns connectors, not integrations.** A connector is a declared interface with event contracts and read endpoints. An integration is tightly coupled code sharing databases. leCRM will never share a database with chatboting.
2. **No runtime coupling at v0.** Chatboting's widget, wizard, RAG, and billing pipelines must never depend on leCRM being available. leCRM is additive — its absence degrades campaign intelligence, not campaign execution.
3. **The daily ritual is the test.** Any boundary design that makes the `/prospection-target next` ritual slower, more brittle, or harder to debug than today is rejected.

---

## Decision

### 1. Boundary position: Option D — "Registry + campaign-history moves; execution stays"

**leCRM owns (strategic layer):**
- Tier registry (prospect tier classification: T1 high-value → T3 long-tail)
- Jurisdiction registry (geo + regulatory constraints)
- Vertical-fit registry (category × geo scoring criteria and research history)
- Campaign history (which cells were targeted, when, by whom, with what outcome)
- Prospect relationship timeline (the full journey from discovery to conversion)
- Post-send lifecycle dashboard (conversion funnel, claim rates, experiment learnings)

**Chatboting owns (operational layer):**
- `vertical_packs` — consumed at runtime by widget and wizard
- `email_variants` — consumed by invitation render pipeline
- `invitations` — tenant lifecycle trigger, email delivery
- `batch_candidates` / `batch_searches` — harvest and scoring execution
- Tenant provisioning (wizard, AaaS, RAG, knowledge base)
- Email sending (Resend SMTP pipeline)

**The orchestrating skill** (`/prospection-target next`) queries BOTH systems: leCRM for "what cell to target today" (steps 1-3 of the ritual), chatboting for "is content ready for this cell" (step 4).

### 2. Boundary crosses (exactly 2)

| Direction | When | Nature | Blocking? |
|-----------|------|--------|-----------|
| **leCRM reads from chatboting** | Step 4 — verify pack/variant readiness | `GET /api/v1/platform/packs/:category/readiness?language=fr` → `{packReady, variantReady, missingItems[]}` | No — informational. If chatboting is down, skill shows "readiness unknown, proceed with caution" |
| **Chatboting pushes events to leCRM** | Step 13 — lifecycle tracking | `POST /api/v1/connectors/chatboting/events` → `{event, contactRef, dealRef, payload}` | No — async fire-and-forget. If leCRM is down, chatboting retries with exponential backoff; daily ritual is unaffected |

Both crosses are non-blocking. The daily ritual completes even if either system is unavailable — degraded intelligence, not degraded execution.

### 3. Spec amendments to existing ADRs

#### 3a. ADR-010 §4 — add `json` property type

The `property_type` CHECK constraint in `custom_property_definitions` (currently: `'string' | 'number' | 'boolean' | 'enum' | 'date'`) is amended to include `'json'`.

**Why:** Chatboting's `scoring_breakdown` is a structured JSONB object with 8+ fields (`pageCountFactor`, `companySizeBonus`, `cmsBonus`, `chatbotPenalty`, `urlPatternPenalty`, `categoryMultiplier`, `effectiveClaimRate`, `rawScore`, `finalScore`). Flattening this into individual `number` properties loses the semantic grouping. A `json` property type accepts pre-validated JSONB blobs from connectors.

**Validation rule for `json` type:** The application layer validates that the value is valid JSON and optionally checks against a JSON Schema stored in `allowed_values` (if provided). If `allowed_values` is NULL, any valid JSON is accepted. Fail-closed: invalid JSON → 400.

**Migration:** A future migration (post-0006) will `ALTER TABLE ... DROP CONSTRAINT` and recreate the CHECK with the added value. The SECURITY DEFINER provisioning function will be updated to include `'json'` in the CHECK for new workspaces.

#### 3b. ADR-010 §3 — `parent_type` extended for registries

The `parent_type` CHECK on `custom_property_definitions` (currently: `'contact' | 'deal'`) is extended to accept additional parent types as leCRM's entity model grows. For the chatboting connector specifically:

- `'contact'` — a prospect/candidate discovered via chatboting (or any other source)
- `'deal'` — a campaign cell execution (targeting a specific geo×category)
- Future: `'campaign'`, `'company'` (when those entities are built)

The `objects` table's `parent_type` is already a free-text field with no CHECK — no change needed there.

#### 3c. Audit log — add `connector` actor type and event namespace

The audit log's `actor_type` field (currently accommodating `'user'` and `'agent'` per ADR-009 §7.2 design) is extended with `'connector'`. The event namespace for connector-originated mutations is `connector.<source>.<event>`:

- `connector.chatboting.contact.created` — new prospect synced from chatboting
- `connector.chatboting.deal.stage_changed` — invitation lifecycle event updated a deal
- `connector.chatboting.activity.recorded` — lifecycle event stored as activity

### 4. Connector event contract

Chatboting pushes events to leCRM via a single webhook endpoint. Events are idempotent (re-delivery is safe) and carry enough context for leCRM to create/update entities without calling back to chatboting.

#### Event envelope

```json
{
  "event": "invitation.claimed",
  "source": "chatboting",
  "timestamp": "2026-05-25T14:30:00Z",
  "idempotency_key": "inv_<uuid>_claimed_<timestamp>",
  "workspace": "gbconsult",
  "payload": {
    "candidate": {
      "url": "https://restaurant-chez-paul.fr",
      "business_name": "Restaurant Chez Paul",
      "contact_email": "paul@chez-paul.fr",
      "category_primary": "restaurant",
      "geo_country": "FR",
      "geo_region": "Île-de-France",
      "geo_city": "Paris",
      "candidate_score": 78.5,
      "scoring_breakdown": { ... }
    },
    "invitation": {
      "id": "<uuid>",
      "status": "claimed",
      "variant_id": "<uuid>",
      "experiment_id": "<uuid>",
      "sent_at": "2026-05-20T10:00:00Z",
      "opened_at": "2026-05-21T08:30:00Z",
      "claimed_at": "2026-05-25T14:30:00Z"
    },
    "tenant": {
      "id": "<uuid>",
      "slug": "chez-paul"
    }
  }
}
```

#### Event types (initial set)

| Event | When | leCRM action |
|-------|------|-------------|
| `candidate.enriched` | Batch search completes enrichment | Create/update Contact with custom properties (score, CMS, geo, category) |
| `invitation.created` | Invitation drafted | Create Deal at "Discovery" stage, link to Contact |
| `invitation.sent` | Email dispatched | Move Deal to "Proposal Sent", record Activity |
| `invitation.opened` | Recipient opened email | Record Activity on Deal |
| `invitation.claimed` | Prospect claimed chatbot | Move Deal to "Closed-Won", record Activity with tenant link |
| `invitation.expired` | Token expired unclaimed | Move Deal to "Closed-Lost" with reason "expired", record Activity |
| `invitation.reply_positive` | Prospect replied positively | Record Activity, flag Deal for follow-up |

### 5. Registry data model

Registries are stored as `objects` rows in leCRM's metadata engine, using `object_type` to distinguish registry types. This validates ADR-010's design for non-record-bound custom objects (§4, last paragraph: "lets the same table host non-record-bound custom objects later").

| Registry | `object_type` | `parent_type` | `data` shape |
|----------|--------------|--------------|-------------|
| Tier | `registry.tier` | NULL | `{tier: "T1", label: "High-value", criteria: "...", updated_at: "..."}` |
| Jurisdiction | `registry.jurisdiction` | NULL | `{country: "FR", region: "Île-de-France", constraints: [...], updated_at: "..."}` |
| Vertical-fit | `registry.vertical_fit` | NULL | `{category: "restaurant", geo: "FR/Paris", fit_score: 85, last_researched: "...", research_notes: "..."}` |
| Campaign brief | `registry.campaign` | NULL | `{cell: "restaurant×Paris", targeted_at: "2026-05-25", candidate_count: 12, sent_count: 10, claimed_count: 2, ...}` |

This avoids new tables — registries use the existing `objects` infrastructure, indexed by the GIN index on `data`.

### 6. Authentication

Chatboting authenticates to leCRM's connector endpoint using a **workspace-scoped service token** (planned for Sprint 7 per the sprint plan). The token identifies both the workspace and the source system:

- Header: `Authorization: Bearer <service_token>`
- Token payload includes: `workspace_id`, `source: "chatboting"`, `scopes: ["connector.push_events"]`

leCRM authenticates to chatboting's readiness endpoint using chatboting's existing platform API key (already exists, used by the platform admin panel).

### 7. Timeline alignment

| leCRM milestone | Sprint | Connector readiness |
|----------------|--------|-------------------|
| Contact/Deal tables exist | 6-7 | Events can create entities |
| Service tokens | 7 | Chatboting can authenticate |
| Activity log wired | 7 | Events can record activities |
| Pipeline Kanban | 8 | Campaign funnel visible in UI |
| MCP adapter skeleton | 9 | Claude agent can query campaign state |
| Gmail sync seam | 10 | External-system-sync pattern validated (connector uses same seam) |

The connector event endpoint can be built in Sprint 7 alongside service tokens — it's a natural first consumer. Registry import (from chatboting's current markdown files) can happen any time after the metadata engine is live (Sprint 5).

---

## Consequences

### Positive

- **leCRM gets a real user from Sprint 7.** Not synthetic test data — real prospects, real campaigns, real conversion metrics flowing through Contact/Deal/Activity/Custom Properties.
- **Metadata engine validated by a demanding consumer.** Chatboting's scoring breakdown (8-field JSONB), category taxonomy, and multi-axis geo data exercise the GIN index, `json` property type, and non-record-bound object types.
- **Connector pattern established.** The chatboting connector becomes the reference implementation for future connectors (Gmail, Brevo, HubSpot import, etc.). Event envelope, service token auth, and idempotency patterns are reusable.
- **Daily ritual improves.** The operator skill queries leCRM for strategic context (faster than reading markdown files) and chatboting for operational readiness (unchanged). Two systems, two concerns, clean separation.
- **No runtime coupling.** Chatboting's widget/wizard/billing never depends on leCRM. Event delivery is async with retry. The daily ritual degrades gracefully if either system is unavailable.

### Negative

- **Event delivery reliability.** If chatboting's event push fails and retries exhaust, leCRM's pipeline view becomes stale. Mitigation: periodic reconciliation job (leCRM queries chatboting's invitation list endpoint to detect drift). Acceptable at v0 scale (<100 invitations/week).
- **Two sources of truth for prospect data.** Chatboting's `batch_candidates` and leCRM's Contacts are separate records. They sync via events but can diverge if events are lost. Mitigation: idempotency keys + reconciliation. The `candidate.url` field is the natural dedup key.
- **Registry migration from markdown.** Today's registries are git-tracked markdown files in chatboting's `docs/marketing/`. Moving them to leCRM's JSONB objects means the operator edits them in leCRM's UI (or via MCP) instead of a text editor + git commit. This is a workflow change. Mitigation: import script + leCRM UI must be at least as fast as editing markdown.

### Neutral

- **ADR-010 schema shape unchanged.** The `objects` table already supports `object_type` for non-record-bound entities (registries). The only schema change is adding `'json'` to the `property_type` CHECK constraint.
- **ADR-001 tenancy model unchanged.** The connector endpoint lives in the workspace-scoped route group. Events are tenant-isolated by the service token's workspace binding.
- **Sprint plan not disrupted.** The connector endpoint is a natural addition to Sprint 7 (alongside service tokens and activity log). Registry import is a Sprint 5-6 bonus, not critical path.

---

## Alternatives Considered

### Option A — "Harvest is the handoff" (leCRM owns steps 1-7)

leCRM owns the full decision phase including candidate harvest. **Rejected:** 7 out of 13 ritual steps cross the boundary. Steps 4 and 6 require chatboting data in the upstream phase, creating a dependency loop. Fails the council's ≤2-crosses criterion.

### Option B — "Proposal is the handoff" (leCRM owns steps 1-5)

leCRM owns decision through proposal. **Rejected:** Step 4 still crosses (pack/variant verification). Step 6 (variant authoring) is editorial/marketing territory that arguably belongs in the CRM layer, but its output (SQL applied to chatboting's `email_variants` table) is tightly coupled to chatboting's runtime. Moving it creates a write dependency that violates the "no runtime coupling" principle.

### Option C — "Nothing moves yet" (leCRM is read-only observer)

Everything stays in chatboting. leCRM receives events and builds a timeline. **Considered viable but suboptimal:** Zero boundary crosses, but leCRM doesn't get meaningful ownership. The "who to target and when" intelligence stays in markdown files + Claude skill context, never becoming structured CRM data. Registries remain unqueryable.

### Full migration (chatboting's prospection moves entirely to leCRM)

Replace chatboting's 38+ prospection endpoints with leCRM equivalents. **Rejected:** 6+ months of work for marginal benefit. Chatboting's prospection system is mature and working. The value is in the strategic layer (registries, campaign history, relationship timeline), not in re-implementing Brave search + candidate scoring + invitation rendering.

---

## References

- `chatboting/docs/prospection-ritual-map.md` — 13-step ritual analysis with boundary-cross matrix
- [ADR-010](ADR-010-metadata-engine.md) — metadata engine pattern (amended by this ADR: `json` property type)
- [ADR-009 §5](ADR-009-stack-and-license.md) — REST + MCP adapter design
- [ADR-009 §7.2](ADR-009-stack-and-license.md) — audit log design (`actor_type` extended by this ADR)
- `docs/council-architecture-review-2026-05-24.md` — council review (7/10 rating, connector validation noted)
- chatboting API: `apps/api/src/modules/prospecting/prospecting.routes.ts` (12 endpoints)
- chatboting API: `apps/api/src/modules/invitations/invitation.routes.ts` (19 endpoints)
- chatboting API: `apps/api/src/modules/experiments/experiment.routes.ts` (7 endpoints)

---

## TO RESOLVE

1. **ADR-010 migration for `json` property type.** Schedule a migration (post-0006) to add `'json'` to the `property_type` CHECK on `custom_property_definitions` and update the provisioning function. Track as a Sprint 5 tasket.
2. **Audit log `actor_type` extension.** Add `'connector'` to the audit log's actor type enum/check. Track alongside the activity log wiring in Sprint 7.
3. **Connector event endpoint implementation.** `POST /api/v1/connectors/:source/events` with service token auth, idempotency key dedup, and workspace-scoped entity creation. Sprint 7.
4. **Readiness endpoint on chatboting side.** `GET /api/v1/platform/packs/:category/readiness` — new lightweight endpoint returning pack + variant readiness for a category/language pair. Low effort (one query against existing tables).
5. **Registry import script.** One-time script to parse chatboting's `docs/marketing/` markdown registries into leCRM `objects` rows. Sprint 5-6.
6. **Reconciliation job design.** Periodic (daily) job where leCRM queries chatboting's `GET /platform/invitations?status=claimed` to detect events that were lost in transit. Sprint 8+.
7. **60-day stability gate.** Per the ritual map's recommendation: no operational coupling (leCRM commanding chatboting to provision/send) until chatboting's prospection schema has been stable for 60 days. Current last schema change: migration 057 (geo_city_district). Gate opens ~late July 2026.
