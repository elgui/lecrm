# ADR-012 — MCP-Native Capability Layer & Read-Write Agent Interface

**Status:** Accepted
**Date:** 2026-05-29 (Proposed and Accepted)
**Deciders:** Guillaume
**Related:** [ADR-009](ADR-009-stack-and-license.md) §4 (REST + thin MCP adapter; service-token scopes incl. `mcp_enabled`/write; `actor_type=mcp_agent`; separate-binary binding §4.2; planned `packages/crm-adapter/` §8.1; recursion-depth bound; per-(workspace,token) rate limit). [ADR-011](ADR-011-external-system-sync.md) (connector-event mutation pattern — the *other* write path AI already uses). [ADR-010](ADR-010-metadata-engine.md) (custom-property definitions — the substrate for a self-describing MCP schema). [ADR-005](ADR-005-ai-agent-tenancy.md) (agent-runtime tiering; prompt-injection sanitization responsibility). [ADR-007](ADR-007-encryption-secrets-audit.md) (audit fail-closed; `idempotency_keys`). [ADR-001](ADR-001-tenancy-model.md) (schema-per-tenant; per-workspace Postgres role).

---

## Context

leCRM's strategic differentiator is not integration count or price — it is **AI-native interfaces enabled by source access**: chatbot, voice, and agent-driven UX that a closed CRM cannot match. The clearest near-term expression of that thesis is a **conversational interface delivered to clients** — an LLM that does not merely *answer questions about* the CRM but *operates* it on the user's behalf.

Today there are three ways into the CRM, and they pull in different directions:

1. **REST API** (`apps/api/internal/crm/handlers.go`) — the exhaustive, deterministic, HubSpot-style contract. Business logic lives here, coupled to HTTP handlers.
2. **Connector-event endpoint** (`POST /v1/connectors/{source}/events`, ADR-011) — the **mutation** path AI systems use *today* (chatboting), but only by speaking leCRM's fixed event vocabulary (`candidate.enriched`, `invitation.created`…). Idempotent, audited, scoped.
3. **MCP adapter** (`apps/mcp/`, ADR-009 §4.2) — **read-only**, a *separate* Go binary with its own read implementation (`apps/mcp/internal/store/`), hitting Postgres through the `workspace_<id>_ro` role. Read-only is enforced *at the database level*.

The asymmetry is the problem this ADR resolves: **an LLM can read via MCP but, to write, must either speak the rigid connector-event schema or drive the REST API.** Neither is the "soft, intelligent interface" the client chatbot needs. A chatbot told "mark the Acme deal won and log that they signed today" should not have to know whether that maps to `PATCH /v1/deals/{id}/stage` + `POST /v1/contacts/{id}/activities`, nor be restricted to a closed set of pre-baked events.

Two architectural risks, identified now while the MCP surface is 6 tools and not 60:

- **Logic divergence.** The MCP binary's `internal/store/` duplicates "list contacts" separately from the API. ADR-009 §8.1 planned a shared `packages/crm-adapter/` precisely to avoid this; reads currently diverge from it. The moment MCP writes, a *second* divergent implementation of every mutation (with its own RBAC, idempotency, audit) is unacceptable.
- **Foreclosing end-user OAuth.** Service tokens (ADR-009 §4.1) are machine-to-machine. "Any LLM connects easily" — a user wiring *their* workspace into Claude Desktop / ChatGPT / claude.ai via a consent screen — requires MCP's OAuth 2.1 authorization. We are **not building that now** (PoC owns the whole chain end-to-end), but no decision in this ADR may make it a rewrite later.

### Scope constraints

- **This ADR sets direction and resolves the read-write design.** It is not a build ticket. The first concrete increment (§10) is deliberately small.
- **PoC posture:** GB Consult owns the full chain (our agents, our chatbot). End-user OAuth clients are a **deliberately-not-foreclosed future**, not a v0 deliverable.
- The separate-binary decision (ADR-009 §4.2) is **binding and survives** — this ADR changes *what the binary links*, not *that it is separate*.

---

## Decision

### 1. One capability layer is the single source of CRM business logic (binding)

Realize the planned `packages/crm-adapter/` (ADR-009 §8.1) as a **protocol-agnostic capability layer**. It owns every CRM operation — reads *and* writes — and depends only on the store + domain types, never on `net/http`, JSON-RPC, or MCP. Each operation:

- takes a resolved **`Principal`** (workspace + role + scopes + `actor_type`), never a transport object;
- enforces RBAC, idempotency (via `core.idempotency_keys`, ADR-007), and fail-closed audit **inside** the capability call — not in the adapter;
- returns domain results, not wire formats.

**REST handlers, MCP tools, and connector-event handlers all become thin projections over this layer.** No business logic in any protocol adapter.

```
                 ┌─────────────────────────────────────────┐
   REST  ───────▶│                                          │
   (apps/api)    │   packages/crm-adapter (capability layer)│──▶ store / domain
                 │   • Principal-based authz (RBAC)          │    (schema-per-tenant)
   Connector ───▶│   • idempotency + fail-closed audit       │
   events        │   • metadata-engine aware (ADR-010)       │
   (apps/api)    │                                          │
                 │                                          │
   MCP    ───────▶│                                          │
   (apps/mcp)    └─────────────────────────────────────────┘
```

**Consolidation requirement:** `apps/mcp/internal/store/` is folded into / replaced by the capability layer's read operations. The MCP binary stops carrying a divergent read implementation. This is the precondition for everything below — write tools that re-implement mutations are explicitly out of bounds.

> The separate-binary topology (ADR-009 §4.2) is unaffected: `apps/mcp` *links* `packages/crm-adapter` as a library, exactly as `apps/api` does. Shared types, one `go test ./...`, independent deploy/crash isolation — all preserved.

### 2. MCP is the *intelligent* projection — intent-shaped, not a CRUD mirror (binding)

REST and MCP are **two surfaces over one core, with different design philosophies. They are not the same endpoints mechanically translated.**

| | REST API | MCP interface |
|---|---|---|
| Audience | Integrators, the SPA, deterministic clients | LLMs, the client chatbot, agents |
| Optimizes for | Completeness, stable contract, determinism | LLM ergonomics, intent capture, token efficiency, safety-by-default |
| Granularity | Fine-grained CRUD, exhaustive | Primitives **+ intent-shaped composites** |
| Schema | OpenAPI 3.1, fixed | **Self-describing per workspace** (§5), rich tool descriptions |
| Error style | HTTP status + machine codes | Natural-language `isError` results the model can recover from |
| Stability | URL-versioned `/v1`, strict | Versioned tool catalog; descriptions tuned like prompts |

A CRUD-mirrored MCP ("create_contact", "update_deal", …) is the trap: it forces the LLM to orchestrate multi-step workflows and re-derive intent every call. The "soft interface" is **intent tools** that encapsulate the workflow once and do the right thing.

### 3. Tool taxonomy: primitives + intent composites (direction)

Two layers, both projecting the §1 capability layer:

- **Primitives** — thin, predictable, for when the model needs precision: `read_contact`, `list_deals`, `search_contacts` (the existing 6, plus `update_contact_field`, `move_deal_stage` as the minimal write primitives).
- **Intent composites** — the differentiator; each encapsulates a real user story (§4) as one safe, idempotent, auditable call:
  - `log_interaction(contact_or_company, summary, outcome?)` → upserts contact if needed + appends an activity.
  - `advance_deal(deal, to_stage, note?, mark_closed_at?)` → stage transition + activity, with stage-name fuzzy match.
  - `capture_lead(name, email?, company?, source)` → contact (dedup by email) + optional deal in first stage. (Mirrors what the connector-event path does for chatboting — same capability call, different door.)
  - `set_followup(entity, when, note)` → flags/schedules follow-up.

The intent set grows from observed chatbot transcripts, not speculation. Primitives are the escape hatch when no composite fits.

### 4. User stories that justify write capability (the value articulation)

Write tools earn their place only against concrete stories. These are the bar:

- **S1 — Client chatbot operates the CRM (the headline).** *"Mark the Acme deal as won, they signed today."* → `advance_deal(Acme, "Closed-Won", mark_closed_at=today)`. Read-only MCP cannot do this; the connector-event schema would need a bespoke event per phrasing. Intent tools make it natural.
- **S2 — Conversational lead capture.** A site/voice chatbot turns a conversation into `capture_lead(...)` + `log_interaction(...)`. Today this only works if the lead arrives through chatboting's fixed `candidate.enriched` event; the MCP path generalizes it to *any* LLM frontend we build.
- **S3 — Internal ops agent triage.** Our own agent reads new contacts, enriches, advances deals overnight — same tools, `actor_type=mcp_agent`, full audit trail.
- **S4 — (deferred, not foreclosed) End-user LLM.** A client connects their workspace to Claude Desktop and says "tidy up my pipeline." Needs OAuth (§7) — out of scope now, but §1/§6/§7 ensure it's an additive edge, not a rewrite.

**Conclusion on the open question:** full read-write MCP is justified — but *as intent tools backed by the shared capability layer*, not as a CRUD clone of REST. The value is owning a conversational write surface that any future LLM frontend (ours first, clients' later) plugs into without bespoke event schemas.

### 5. Self-describing workspace schema via the metadata engine (differentiator)

A generic CRM MCP exposes a fixed schema. leCRM has per-workspace custom properties (ADR-010 `custom_property_definitions`). The MCP interface **exposes the connecting workspace's actual schema** so the LLM uses real fields correctly:

- Expose `custom_property_definitions` as an **MCP Resource** (`lecrm://workspace/schema`) the client reads as context.
- Reflect custom properties into write-tool input schemas (or accept a typed `custom_properties` map validated against definitions in the capability layer).

This is a capability a closed CRM structurally cannot offer, and it is half-built already. It is the most leCRM-specific reason to invest in MCP depth.

### 6. Write-safety model (binding for any write tool)

Write tools ship only with all of:

| Control | Mechanism | Reuse |
|---|---|---|
| **Scope → RBAC** | Each write tool declares a scope; capability layer maps token scope → `Principal` role and authorizes. Read-only tokens cannot reach write tools. | ADR-009 §4.1 scopes; existing RBAC `Principal` |
| **DB-role strategy by scope** | Read-only token → `workspace_<id>_ro` connection (**DB-level read-only preserved**). Write token → read-write role; safety is app-level (this row). | Existing ro role; per-workspace role (ADR-001) |
| **Idempotency** | Every write tool accepts/derives an idempotency key; duplicate calls return the cached result. | `core.idempotency_keys` (ADR-007/011) |
| **Dry-run / preview** | Destructive or composite tools accept `dry_run: true` → return the *would-be* effect (a diff), no mutation. | new, thin |
| **Confirmation for destructive/bulk** | Delete and multi-entity tools require an explicit confirmation token returned by a prior dry-run; no silent bulk mutation. | new, thin |
| **Audit attribution** | `actor_type=mcp_agent` (+ token id) on every mutation; fail-closed. | ADR-007/009 — already emitted |
| **Rate limit + recursion bound** | per-(workspace, token) limit; tool-call depth ≤ 5. | ADR-009 §4.2 — already enforced |

### 7. Auth-plane / tool-plane separation — keep end-user OAuth open (binding)

This is the decision that protects the deferred S4 future:

- **Tools authorize against `Principal`, never against an auth mechanism.** A tool knows the caller's workspace/role/scopes; it does not know or care whether those came from a service token or an OAuth access token.
- **Authentication is a pluggable transport-edge concern.** Today: service-token verification (ADR-009 §4.1). Later: **MCP OAuth 2.1** authorization server issuing workspace-scoped, RBAC-mapped access tokens — added at the edge, mapping onto the *same* `Principal`. No tool changes.
- **Design tools as if the caller may be a delegated, lower-trust agent now.** The §6 guardrails (scopes cap blast radius, dry-run, confirmation, audit) are exactly what an end-user OAuth client needs. Building them now is the *same* investment OAuth requires later — aligned, not duplicated.

**Non-foreclosure checklist (binding):** no write tool may assume a single trusted machine actor; no tool may grant authority beyond its declared scope; no schema may hardcode `actor_type=mcp_agent` as the only writer. Violating any of these re-couples the tool plane to the service-token mechanism and breaks S4.

### 8. Trust boundary & prompt-injection posture for write-capable MCP

ADR-009 §4 flagged that external MCP clients receive *unsanitized* data and pushed sanitization to the client. **Writes raise the stakes** (confused-deputy: injected content in a contact note steering the LLM to mutate). Posture:

- **Scopes are the primary blast-radius control** — an injection cannot exceed the token's granted scope.
- **Dry-run + confirmation** on destructive/bulk ops give a human/parent-agent an interception point.
- **Audit is the backstop** — every mutation is attributable and reversible-by-inspection.
- **Sanitization of CRM content fed back to the model** remains the agent-runtime's job (ADR-005 Tier-2), not the thin adapter's — unchanged, but now explicitly load-bearing. **Resolved & documented**: see [`docs/mcp/trust-boundary.md`](../mcp/trust-boundary.md), which draws the line explicitly (the adapter treats every record field as opaque, untrusted data; it never parses content as an instruction) and is backed by adversarial scope-containment tests. (TO RESOLVE 6, tasket `20260529-1005`.)

### 9. MCP primitives: use all three, deliberately (direction)

Today only **Tools** are used. A well-designed surface uses:

- **Tools** — actions (reads, writes, searches). §3.
- **Resources** — addressable, token-efficient read context the client pulls without a tool round-trip: `lecrm://workspace/schema` (§5), `lecrm://contact/{id}`, `lecrm://deal/{id}`, `lecrm://pipeline`. Cuts tokens and latency vs forcing a `read_*` call for context.
- **Prompts** — reusable CRM workflow templates ("triage my inbox", "prep brief for this deal") that encode good multi-tool sequences once.

---

## 10. Increment plan (scope discipline)

ADR-012 sets direction; the *next* increment is intentionally small, given v0 read-only MCP just shipped:

**Increment 1 (this ADR's concrete output):**
1. Extract/realize `packages/crm-adapter` capability layer; route REST CRM handlers through it (refactor, no behavior change).
2. Repoint `apps/mcp` reads at the capability layer; delete the divergent `internal/store` read logic.
3. Ship **3 intent write tools** behind a write scope: `advance_deal`, `log_interaction`, `capture_lead` — with §6 controls (scope, idempotency, dry-run, audit).
4. Expose `lecrm://workspace/schema` Resource (§5).

**Deferred (not foreclosed):** OAuth authorization server (§7), Prompts (§9), full intent catalog (§3), Resources beyond schema. Each is additive given Increments above.

---

## Consequences

**Positive:**
- One capability layer ends logic divergence before writes multiply it; REST/MCP/connector-events become thin and consistent. The strongest answer to an acquirer's CTO: "every door enforces the same RBAC/idempotency/audit, in one place."
- The client-chatbot vision (S1/S2) gets a real conversational write surface — the embodiment of the AI-native moat — without bespoke event schemas per phrasing.
- End-user OAuth (S4) is preserved as a pure edge-addition by §7; the safety work it needs is built now anyway (§6).
- Self-describing schema (§5) is a structural advantage over closed CRMs, reusing ADR-010.

**Negative:**
- The capability-layer extraction is a refactor with no immediate user-visible payoff; it is insurance, justified the way ADR-011's abstraction was ("cheap insurance" vs every-future-thing-is-a-rewrite).
- Write-capable MCP weakens the DB-level read-only guarantee *for write-scoped tokens* (it becomes app-level). Mitigated by §6 (ro tokens keep the DB guarantee) and the single-enforcement-point design.
- Larger trust-boundary surface (§8) than a read-only adapter; mitigated by scopes + dry-run + audit, but real.
- Intent-tool design is an ongoing, transcript-driven effort — descriptions are tuned like prompts, not written once.

**Neutral:**
- Separate-binary topology (ADR-009 §4.2) and `mark3labs/mcp-go` choice are unchanged.
- `actor_type=mcp_agent`, per-(workspace,token) rate limit, recursion bound — already in place, now load-bearing for writes.

---

## Alternatives Considered

- **MCP-in-API process (fold the binary in).** Simplest path to writes (one process, shared everything). Rejected: overturns ADR-009 §4.2 binding (crash isolation, independent scaling of sticky agent sessions). The shared-capability-package approach gets the same logic-unification *without* collapsing the process boundary.
- **MCP binary calls REST over HTTP for writes.** Keeps isolation, reuses handlers. Rejected: double hop + serialization, and it would make MCP a mechanical CRUD mirror (violates §2) — the REST shape would leak into the LLM surface. Library linkage to the capability layer is cleaner and faster.
- **Keep MCP read-only; all writes stay connector-events (ADR-011).** Safest. Rejected as the *primary* surface: forces every LLM frontend to learn a fixed event vocabulary, which is the opposite of the "soft, intelligent interface" and cannot serve S1 phrasings generally. (Connector-events remain correct for *system-to-system* push from chatboting — the two coexist; intent tools and events are two doors onto the same capability calls.)
- **CRUD-mirrored MCP tools.** Rejected per §2 — pushes workflow orchestration and intent-rederivation onto the model every call.
- **Build OAuth now.** Rejected: PoC owns the whole chain; §7 keeps it a cheap future addition. Building it now is speculative surface (cf. ADR-011's deferral of `Push`/webhooks).

---

## References

- [ADR-009 §4](ADR-009-stack-and-license.md) — REST + thin MCP adapter; service-token scopes; separate-binary binding; planned `packages/crm-adapter/`; rate limit; recursion bound; `actor_type=mcp_agent`.
- [ADR-011](ADR-011-external-system-sync.md) — connector-event mutation pattern (the system-to-system write door that coexists with MCP intent tools).
- [ADR-010](ADR-010-metadata-engine.md) — `custom_property_definitions`, the substrate for the self-describing MCP schema (§5).
- [ADR-005](ADR-005-ai-agent-tenancy.md) — agent-runtime tiering; prompt-injection sanitization ownership (§8).
- [ADR-007](ADR-007-encryption-secrets-audit.md) — `idempotency_keys`; fail-closed audit (§6).
- [MCP specification — authorization (OAuth 2.1)](https://modelcontextprotocol.io/specification) — the §7 deferred end-user path.

---

## TO RESOLVE

1. **`packages/crm-adapter` capability-layer extraction** — interface shape (per-operation `Principal` arg), and the REST-handler refactor sequence. Increment 1.1.
2. **MCP read consolidation** — delete `apps/mcp/internal/store/` read logic; repoint at capability layer; confirm `_ro` connection still used for read-only-scoped tokens. Increment 1.2.
3. ~~**Intent write-tool scope mapping** — define the write scope(s) and the scope→RBAC role mapping for `advance_deal`/`log_interaction`/`capture_lead`. Increment 1.3.~~ **RESOLVED** (Increment 1.3, tasket `20260529-1002`): `crm:write`/`*` → `RoleAdmin` (capped), read-only → denied; gate is `capability.AuthorizeWrite`. See [`docs/mcp/write-safety-contract.md`](../mcp/write-safety-contract.md) §1.
4. ~~**Dry-run / confirmation contract** — the wire shape of a `dry_run` preview and the confirmation-token handshake for destructive/bulk tools (§6).~~ **RESOLVED** (tasket `20260529-1002`): `Preview` wire shape + HMAC-bound, effect-digest, time-boxed confirmation token (`capability.GuardedWrite`/`Confirmer`). See [`docs/mcp/write-safety-contract.md`](../mcp/write-safety-contract.md) §2–§3.
5. **`lecrm://workspace/schema` Resource shape** — how `custom_property_definitions` is serialized for LLM consumption (§5). Increment 1.4.
6. ~~**Prompt-injection / confused-deputy hardening for writes (§8)** — confirm agent-runtime (ADR-005 Tier-2) sanitization covers the write-driven case; document the thin-adapter's non-responsibility explicitly.~~ **RESOLVED** (tasket `20260529-1005`): trust boundary documented in [`docs/mcp/trust-boundary.md`](../mcp/trust-boundary.md) — the adapter explicitly does **not** sanitize CRM content (agent-runtime Tier-2 owns that, ADR-005 §4); its content-independent guarantees are scope-containment + dry-run/confirmation + fail-closed audit. Adversarial tests prove injected record/field content cannot escalate beyond token scope, cross tenants, or one-shot a destructive op (`capability/injection_test.go`, `apps/mcp/internal/mcpserver/injection_test.go`).
7. **Deferred — MCP OAuth 2.1 authorization server (§7)** — design only when the first end-user-LLM client (S4) is real. Verify §7 non-foreclosure checklist holds at each increment until then.
8. **Intent-tool catalog growth process** — wire chatbot transcripts → new intent tools (§3); avoid speculative tool sprawl.
