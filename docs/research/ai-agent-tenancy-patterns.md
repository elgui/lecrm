# AI-Agent Tenancy and Service-Boundary Architecture for leCRM

**Date:** 2026-05-10
**Scope:** v2 AI-agent layer — per-client conversational CRM, voice-to-CRM, autonomous pipeline agents, LLM dashboards
**Constraint surface:** Twenty AGPL fork + Anthropic Claude + existing Tele-Claude / OpenClawing infra

---

## 1. Per-Tenant Agent State Isolation

### The Abstract Pattern

Every major platform that runs multi-tenant agents converges on the same three-layer isolation model:

**Layer 1 — Identity envelope.** Every inbound request carries a signed token that encodes `tenant_id`, `user_id`, roles, and permitted tool scopes. The agent runtime resolves this before touching any data or calling any model. The LLM itself never sees raw `tenant_id` strings in free-form positions — it receives a pre-filtered context constructed by a trusted layer.

**Layer 2 — Namespaced storage.** All persistent state (conversation history, retrieved documents, agent memory) is keyed on `workspace_id`. For logical isolation (acceptable for leCRM's SMB tier), this means `WHERE workspace_id = $1` enforced at the ORM/query layer via PostgreSQL Row-Level Security (RLS) policies. The agent service connects as a role that activates RLS on every session — a forgotten filter cannot leak cross-tenant.

**Layer 3 — Config registry.** Per-tenant behavioral configuration (system prompt, allowed tools, model tier, cost cap) lives in a separate, authoritative store — not embedded in the conversation history. The agent loads this config at the start of each conversation and stamps it as the immutable context for that session.

### Reference implementations

- **Vercel AI SDK v5** (released July 2025) makes the separation explicit: `UIMessage` (what the user sees) vs `ModelMessage` (what goes to the LLM). Persistence hooks (`onFinish`) deliver both types cleanly so you can store them in different tables with different retention policies. The `useChat` generic type parameter lets the host app encode `tenantId` into the message type for full-stack type safety. ([AI SDK 5 announcement](https://vercel.com/blog/ai-sdk-5))

- **Anthropic Workspaces + per-workspace API keys.** Anthropic's own Admin API (`/v1/organizations/usage_report/messages`) lets you filter and group by `workspace_id` and `api_key_id`. Mapping one Anthropic Workspace per leCRM tenant is viable for a handful of clients but does not scale to 50+ tenants without organizational overhead. The practical pattern: one shared Anthropic org, tag usage via metadata attached to API calls (see Section 5).

- **AWS multi-tenant GenAI** recommends "silo" (dedicated resources per tenant) for high-security or high-spend tenants and "pool" (shared resources, logical isolation) for standard SMB tenants. leCRM starts in pool mode; silo is an upsell for enterprise clients. ([AWS blog](https://aws.amazon.com/blogs/machine-learning/build-a-multi-tenant-generative-ai-environment-for-your-enterprise-on-aws/))

- **Blaxel's isolation guide** adds the principle of **segregated OpenTelemetry pipelines**: each tenant's traces flow into a PII-redacted sink so observability data is also isolated. For EU/GDPR compliance this matters. ([Blaxel blog](https://blaxel.ai/blog/multi-tenant-isolation-ai-agents))

---

## 2. Where to Store Per-Tenant Agent Config

### Options and trade-offs

| Store | What lives there | Pros | Cons |
|---|---|---|---|
| **Twenty workspace metadata** (extend schema via Metadata API) | system_prompt, allowed_tools[], model_tier, cost_cap_eur | Single source of truth; admin UI "free"; follows workspace lifecycle | Requires Twenty schema migration per config change; AGPL fork drift; pollutes the CRM domain model |
| **Separate agent-config Postgres table** (in agent-runtime DB) | Same fields + cache_seed, last_prompt_hash | Clean domain separation; agent service owns its config; easy to version | Second DB to operate; config sync needed if Twenty workspace is deleted |
| **Redis with `ws:{workspace_id}:config` key** | Loaded config + TTL | Sub-millisecond load at conversation start; natural TTL invalidation | Not the system of record; needs a backing store; data loss on Redis failure |

### Recommendation: Separate agent-config Postgres table + Redis cache

Store the authoritative config in the agent-runtime's own PostgreSQL database (table: `tenant_agent_config`). On each conversation start, the agent service checks a Redis key `ws:{workspace_id}:agent_cfg`; on miss it reads Postgres and caches for 5 minutes. Config changes invalidate the Redis key immediately via a webhook from the leCRM admin UI.

This keeps the Twenty fork clean. The Metadata API is better reserved for CRM schema extensions (custom fields, objects) that need to be visible in the Twenty UI — not for agent infrastructure config.

Schema skeleton:
```sql
CREATE TABLE tenant_agent_config (
  workspace_id       UUID PRIMARY KEY,
  system_prompt      TEXT NOT NULL,
  allowed_tools      TEXT[]  NOT NULL DEFAULT '{}',
  model_tier         TEXT NOT NULL DEFAULT 'claude-haiku-4-5',
  monthly_cap_eur    NUMERIC(10,2),
  soft_warn_pct      SMALLINT DEFAULT 80,
  cache_seed_hash    TEXT,   -- SHA256 of system_prompt for cache tracking
  updated_at         TIMESTAMPTZ DEFAULT NOW()
);
```

---

## 3. Multi-Turn Conversation State Management

### Storage model

The Redis + PostgreSQL hybrid is the industry standard for production LLM chatbots in 2025–2026:

- **Redis** (`conv:{conversation_id}:messages`): Holds the last N turns (recommended: last 20 turns or last 8,000 tokens, whichever is smaller) as a JSON list. Sub-100ms reads; eviction after session TTL (e.g., 24h for Telegram, 30min for voice).
- **PostgreSQL** (`conversation_messages` table): Full durable transcript. Written asynchronously (every turn, or batched every 5 turns). Indexed on `(workspace_id, conversation_id, created_at)`. This is the source of truth for compliance, billing reconciliation, and conversation resumption after Redis eviction.

Telegram/WhatsApp chatbot sessions can span hours or days. Voice sessions are short-burst (< 5min) and can live entirely in Redis with a PostgreSQL flush on session end.

### Context-window management strategies

**Sliding window with anchored facts (recommended for leCRM).** Keep the last N turns in the active window. Before the window, maintain a "facts block" — a static summarised section extracted at conversation start (CRM record context: account name, open deals, last interaction) — that is always included and marked for caching (see Section 6). The sliding window sits after the facts block, so older turns naturally fall out as new turns arrive without losing the anchored CRM context.

**Summarisation at threshold.** When the conversation reaches N turns (e.g., 30), call the LLM with a dedicated summarisation prompt to compress turns 1..20 into a paragraph, store it in Postgres (`conversation_summaries`), and resume with turns 21..30 plus the new summary as the facts block. This mirrors LangChain's `ConversationSummaryBufferMemory` pattern. ([LangChain memory docs](https://jetthoughts.com/blog/langchain-memory-systems-conversational-ai/))

**Hand-off suspend/resume.** When a human agent takes over, the agent runtime writes a `handoff_state` record to Postgres containing the full context hash and a `suspended_at` timestamp. On resume, it rehydrates from Postgres, re-populates Redis, and continues. The voice pipeline always uses suspend/resume since voice sessions can pause mid-sentence if the user is interrupted.

### Atomic writes

Avoid dual-write inconsistency (Redis succeeds, Postgres fails) by writing to Postgres first (synchronously), then updating Redis. Use a Postgres NOTIFY trigger to invalidate the Redis cache on direct DB writes by other services. ([State management patterns](https://dev.to/inboryn_99399f96579fcd705/state-management-patterns-for-long-running-ai-agents-redis-vs-statefulsets-vs-external-databases-39c5))

---

## 4. Internal API for Agents — Three Options Evaluated

The agent-runtime service needs to read and write CRM data on behalf of tenants. Three architectural choices:

### Option A: Public GraphQL + privileged service token

The agent calls Twenty's `/graphql/` endpoint with a long-lived API key scoped to the target workspace. Twenty's built-in role system limits what the key can read/write.

- **Blast radius:** Twenty's existing authorization layer contains damage. A compromised key affects one workspace (if properly scoped).
- **Auditability:** Twenty logs every GraphQL operation. Full audit trail without extra code.
- **Complexity:** Minimal. Reuses Twenty's existing rate-limiting and auth middleware.
- **Downside:** Subject to Twenty's public rate limits. Bulk pipeline-watching agents (e.g., "check all open deals every hour") will hit limits for tenants with large datasets. Prompt-injection via CRM data fields is a real risk — sanitize all CRM content before including in the system prompt.

### Option B: Internal NestJS service (bypass rate limits)

A thin internal service (`agent-data-service`) sits in the same Docker network as Twenty. It exposes a purpose-built REST/gRPC API for the specific operations agents need (get_contact, update_deal_stage, add_note). Internally it talks to Twenty's Postgres directly or via private GraphQL, bypassing public rate limits.

- **Blast radius:** Larger — the internal service has broad DB access. A bug in it could corrupt any tenant's data.
- **Auditability:** You own the audit log; write it explicitly. More work but also more flexible (can emit to Langfuse traces).
- **Complexity:** Medium. Requires building and maintaining an extra service.
- **Upside:** No rate-limit headaches; can batch-load CRM context efficiently; can implement the "privileged internal channel" without touching Twenty's code.

### Option C: Direct Postgres access from agent-runtime

The agent service connects directly to Twenty's Postgres with a read-write role.

- **Blast radius:** Maximum. Bypasses all application-level authorization.
- **Auditability:** Only at the Postgres WAL level; hard to attribute to specific agent actions.
- **Complexity:** Low to build, high to maintain safely (schema changes in Twenty break agent queries).
- **Verdict:** Strongly discouraged. Only acceptable for read-only analytics queries behind a separate read replica.

### Recommendation: Option A for v1, migrate to Option B for v2

Start with the public GraphQL + workspace-scoped service token. It is safe, auditable, and requires zero extra infrastructure. Monitor for rate-limit friction. If bulk pipeline-watching agents or batch CRM-sync operations hit limits (likely at >10 active tenants), introduce the internal NestJS service as a "CRM data adapter" layer that the agent-runtime calls instead. The Twenty MCP Server (already AGPL, community-maintained) can be vendored as the skeleton for Option A without writing a GraphQL client from scratch. ([Twenty MCP Server](https://github.com/mhenry3164/twenty-crm-mcp-server))

**Prompt-injection mitigation:** All CRM data retrieved by the agent (contact notes, email bodies, deal descriptions) must pass through a sanitization step before being concatenated into the LLM context. Strip or escape any content that begins with "Ignore previous instructions" or similar patterns. Never use raw CRM text as the `system` prompt content.

---

## 5. Cost Control Mechanics

### What Anthropic provides natively

The Anthropic Admin API (`/v1/organizations/usage_report/messages`) supports filtering and grouping by `api_key_id` and `workspace_id`. This is your primary attribution mechanism. Usage data appears within 5 minutes of request completion; the API supports polling once per minute. ([Anthropic Usage & Cost API](https://platform.claude.com/docs/en/api/usage-cost-api))

The limitation: Anthropic has no native per-tenant hard-cap mechanism. Workspaces have rate limits and monthly spend caps, but these operate at the Anthropic account level, not at leCRM's per-client level.

### Implementation pattern for per-tenant cost caps

**Pre-flight token estimation + post-flight reconciliation + circuit breaker:**

```
Request arrives for workspace W
  → Load config: monthly_cap_eur, soft_warn_pct for W
  → Load current_month_spend(W) from cost_ledger table (updated by reconciler)
  → If current_spend >= monthly_cap_eur → reject with 402 (hard cap)
  → If current_spend >= monthly_cap_eur * soft_warn_pct / 100 → proceed + flag admin
  → Estimate tokens: count(system_prompt) + count(conversation_context) + count(user_message)
  → If estimated_cost + current_spend > monthly_cap_eur → reject (pre-flight block)
  → Call Anthropic API
  → On response: read usage.input_tokens, usage.output_tokens, cache hits from response
  → Calculate actual cost; write to cost_ledger(workspace_id, amount_eur, timestamp)
```

The `cost_ledger` table is the per-tenant meter. A background reconciler (runs every 5 minutes) calls the Anthropic Admin API and cross-checks — if the local ledger diverges by more than 5%, log an alert. For billing pass-through, the reconciler also writes monthly summaries to a `tenant_billing_summary` table that feeds the invoice generator.

### Observability tooling options

| Tool | Self-hosted? | Per-tenant cost tracking | Circuit breaker |
|---|---|---|---|
| **Langfuse** | Yes (MIT, Docker) | Yes — per `userId` and per `project`; Metrics API for downstream billing | No native cap enforcement; must build around Metrics API |
| **OpenLLMetry** (Traceloop) | Yes (OpenTelemetry-based) | Yes — via OTel resource attributes; Prometheus metrics exportable | No |
| **Helicone** | Partial (cloud-first) | Yes — per `user` header, per `property` | Soft rate limiting via gateway mode |
| **LiteLLM proxy** | Yes | Yes — per `user` or `team` in LiteLLM DB; native budget caps and hard-fail | Yes — built-in `max_budget` per team/user |

**LiteLLM proxy is the strongest fit** for leCRM's requirement. Deploy it between the agent-runtime and Anthropic. Configure one LiteLLM "team" per leCRM workspace. Set `max_budget` per team in USD. LiteLLM enforces the cap server-side (returns 429 when exceeded), logs all usage, and exposes a `/team/info` endpoint for the reconciler. It also handles model routing (Haiku → Sonnet → Opus per tenant tier) transparently. ([LiteLLM docs](https://docs.litellm.ai/))

For billing pass-through: charge clients at cost + margin (e.g., cost × 1.4). For absorbed MRR: build the cap into the tier price, with usage overage notifications to the leCRM admin before the hard stop.

---

## 6. Anthropic Prompt Caching for Per-Tenant System Prompts

### How it works

Anthropic caches up to the `cache_control: {type: "ephemeral"}` marker. Cache hits cost **0.1×** the base input price; writes cost **1.25×** (5-min TTL) or **2.0×** (1-hour TTL). Minimum cacheable size: 1,024 tokens for Sonnet 4.5/4; 2,048 for Sonnet 4.6; 4,096 for Opus 4.x and Haiku 4.5. ([Anthropic Prompt Caching](https://platform.claude.com/docs/en/docs/build-with-claude/prompt-caching))

The break-even point: a 5-min TTL write pays off after approximately **1.25 calls** with the same cached prefix within the 5-minute window. For a busy Telegram bot, this is trivially exceeded. For a voice session (one burst), the 1-hour TTL is better — the write cost is 2× but the session completes before the 5-min TTL would expire.

### Shared prefix + per-tenant suffix pattern

The critical ordering rule: **static content must come before dynamic content**. For leCRM, structure every system prompt in two blocks:

**Block 1 (cached, static across all tenants):**
```
You are a CRM assistant for a French SMB. Your role is to help users
manage their sales pipeline, log interactions, and retrieve contact
information. Always respond in the user's language (French or English).
You have access to the following tools: [tool definitions].
[CACHE BREAKPOINT]
```
This block (~1,500–3,000 tokens including tool definitions) is identical across ALL tenants using the same model tier. One cache entry serves every workspace. Cache hit rate approaches 100% for this block after the first request per 5-min window.

**Block 2 (not cached, per-tenant, dynamic):**
```
Workspace context: {company_name}, {industry}, {custom_instructions}.
Current user: {user_name}, role: {role}.
Open deals count: {n}. Last sync: {timestamp}.
```
This block changes per tenant and per session. It is not cached — that is intentional and correct. Trying to cache it would require a separate cache entry per tenant and would invalidate on every CRM data change.

**Block 3 (conversation history — automatic multi-turn caching):**
Anthropic's automatic caching advances the cache breakpoint with each turn. With the static prefix already cached, the conversation history accumulates cache hits automatically as the conversation grows. Each new turn only incurs full input cost for the new message tokens.

### Realistic cost savings at leCRM's scale

Assume: 10 active tenants, 50 messages/day/tenant, Sonnet 4.6 at $3/MTok input. Average system prompt = 2,000 tokens. Without caching: 10 × 50 × 2,000 = 1,000,000 tokens/day = $3/day. With caching (assuming 80% hit rate on the static block): 200,000 × 0.1 + 800,000 × 1.25 × write_fraction ≈ ~$0.50–0.80/day. Rough saving: 70–80%. At 50 tenants, this becomes material (~$100+/month saved).

Pre-warm caches at agent-service startup using `max_tokens: 0` calls to avoid cold-start latency on the first user message of the day.

---

## 7. Service Boundary Diagram: v0 → v1 → v2

### v0: Twenty core (now)

```
[Browser/Admin UI]
       |
  [Twenty Web (NestJS)]
       |
  [Twenty Postgres]   [Redis (Twenty internal)]
```
Single service. No AI. Workspace isolation via schema-per-tenant in Postgres.

### v1: + Email sender + Reply.io bridge (next 3 months)

```
[Browser/Admin UI]
       |
  [Twenty Web (NestJS)]  ←→  [Email-Sender Service]
       |                            |
  [Twenty Postgres]           [Reply.io API]
  [Redis (Twenty)]
```
Email-sender is a small NestJS/Node service. Calls Twenty's GraphQL for contact data. Calls Reply.io API for sequence management. State: none beyond Twenty. Authentication: workspace-scoped Twenty API key.

### v2: + Agent runtime + Chatbot gateways + Voice pipeline

```
                        [leCRM Admin UI]
                              |
              ┌───────────────┴────────────────┐
              |                                |
     [Twenty Web (NestJS)]         [Agent Config API]
              |                                |
     [Twenty Postgres]            [Agent-Config Postgres]
     [Twenty Metadata API]        [Redis: config cache]
              |
     [Twenty GraphQL — public, workspace-scoped service tokens]
              |
    ┌─────────┴──────────┐
    |                    |
[Agent Runtime]   [Email-Sender]
(NestJS/Python)
    |         |
    |    [LiteLLM Proxy]
    |         |
    |    [Anthropic API]  ←── prompt caching, per-model routing
    |
    ├── [Conv. History Postgres]  (durable, per-workspace RLS)
    ├── [Redis: active session cache]  (last 20 turns, TTL 24h)
    ├── [Langfuse self-hosted]  (traces, costs, evals)
    |
    ├── [Telegram Gateway]  (Tele-Claude vendored/forked — see §8)
    ├── [WhatsApp Gateway]  (future, same interface)
    └── [Voice Pipeline]    (OpenClawing fork — see §8)
              |
        [STT / TTS services]
```

**State ownership summary:**
- CRM records: Twenty Postgres (via GraphQL calls from Agent Runtime)
- Agent config: Agent-Config Postgres + Redis cache
- Conversation history: Conv. History Postgres (durable) + Redis (hot session)
- Cost ledger: Agent-Config Postgres (reconciled against Anthropic Admin API)
- Observability traces: Langfuse (self-hosted, one project per leCRM tenant)
- Model routing: LiteLLM (budget caps enforced per team = per workspace)

**Communication:**
- Agent Runtime → Twenty: public GraphQL with workspace-scoped service token
- Agent Runtime → LiteLLM: internal HTTP (same Docker network)
- LiteLLM → Anthropic: public HTTPS
- Gateways → Agent Runtime: internal REST/WebSocket
- Agent Runtime → Langfuse: SDK (async, non-blocking)

---

## 8. Reuse vs. Build: Tele-Claude and OpenClawing

### Current state

- **Tele-Claude** (51.77.146.49): A Telegram bot runtime already deployed. Handles webhook delivery, session management, Telegram API integration.
- **OpenClawing**: Voice pipeline infrastructure (STT/TTS + Claude integration pattern).

### Options

**Option A: Share the existing services.** leCRM chatbot gateways connect to the live Tele-Claude and OpenClawing services as shared infrastructure.

- Pro: No duplication; improvements flow to both products.
- Con: Release coupling. A leCRM-specific change (e.g., CRM-specific tool routing) requires coordinating across the shared service. Multi-tenancy for leCRM's clients mixes with other products' tenants. Operational blast radius: a Tele-Claude outage takes down all products simultaneously.

**Option B: Fork-and-vendor into leCRM's stack.** Copy the relevant gateway logic into the leCRM agent-runtime repo. Maintain independently.

- Pro: Full control over the interface contract; leCRM-specific features (e.g., per-tenant Telegram bot tokens, CRM tool routing) can be added cleanly without affecting other products.
- Con: Code duplication; bug fixes must be cherry-picked back.

**Option C: Extract into a shared gateway library/service with a stable interface contract.**

- Pro: Best of both worlds. The gateway service exposes a `TenantAgentGateway` interface; leCRM and other products implement it. Each product gets its own gateway deployment, consuming a shared library.
- Con: Requires upfront interface design effort.

### Recommendation: Option B (fork-and-vendor) for v1, migrate to Option C for v2

In the short term, copy the Telegram gateway logic into leCRM's codebase. This removes release coupling at the cost of some duplication — acceptable while leCRM's agent API is still evolving. Once the `AgentMessage` interface stabilises (after 3–5 active tenants), extract a shared `@leCRM/gateway-core` package that both products consume. Voice pipeline: same pattern — fork OpenClawing's session logic into a `voice-agent-service` within leCRM, sharing the audio processing library via package.

Per-tenant Telegram bot tokens: each leCRM tenant gets a dedicated Telegram bot (registered via BotFather). The gateway stores `{workspace_id → bot_token}` in Agent-Config Postgres. This gives clients a branded bot handle (e.g., `@AcmeCRMBot`) and isolates conversation routing cleanly.

---

## Recommendations Summary

| Concern | Decision |
|---|---|
| **Agent config storage** | Separate agent-config Postgres table + Redis read cache. Do not extend Twenty's schema for infrastructure config. |
| **Tenant isolation mechanism** | Postgres Row-Level Security on `workspace_id` for all agent-side tables. One Anthropic Workspace per environment (not per tenant), API key per service. |
| **Conversation state** | Postgres for durable transcript + Redis for hot last-20-turn session. Write Postgres first, then Redis. Sliding-window context with anchored CRM facts block. |
| **Internal API for agents** | Option A (public GraphQL + workspace service token) for v1. Introduce NestJS CRM data adapter for v2 if rate limits become friction. Vendor the Twenty MCP Server as GraphQL client skeleton. |
| **Cost control** | LiteLLM proxy with per-team budget caps (= per leCRM workspace). Supplement with local `cost_ledger` table and 5-minute reconciler against Anthropic Admin API. Soft warn at 80% cap, hard block at 100%. |
| **Prompt caching** | Two-block system prompt: shared static prefix (cached, ~2,000 tokens, one cache entry serves all tenants on same model) + per-tenant dynamic suffix (not cached). Use 1-hour TTL for Haiku/voice sessions; 5-min for Sonnet chatbot sessions. Pre-warm on agent-service startup. |
| **Observability** | Langfuse self-hosted. One Langfuse project per leCRM workspace for per-tenant trace isolation and cost reporting. Feed Langfuse cost data into billing pipeline. |
| **Gateways** | Fork Tele-Claude and OpenClawing into leCRM repo for v1. Per-tenant Telegram bot tokens stored in agent-config DB. Plan shared library extraction at v2. |

---

## Sources

- [Anthropic Prompt Caching — platform.claude.com](https://platform.claude.com/docs/en/docs/build-with-claude/prompt-caching)
- [Anthropic Usage & Cost Admin API](https://platform.claude.com/docs/en/api/usage-cost-api)
- [Multi-tenant isolation for AI agents — Blaxel](https://blaxel.ai/blog/multi-tenant-isolation-ai-agents)
- [Build multi-tenant GenAI on AWS](https://aws.amazon.com/blogs/machine-learning/build-a-multi-tenant-generative-ai-environment-for-your-enterprise-on-aws/)
- [State management patterns for long-running AI agents — DEV Community](https://dev.to/inboryn_99399f96579fcd705/state-management-patterns-for-long-running-ai-agents-redis-vs-statefulsets-vs-external-databases-39c5)
- [Vercel AI SDK 5 announcement](https://vercel.com/blog/ai-sdk-5)
- [Vercel AI SDK persistence DB reference](https://github.com/vercel-labs/ai-sdk-persistence-db)
- [LangSmith cost tracking](https://docs.langchain.com/langsmith/cost-tracking)
- [Langfuse token and cost tracking](https://langfuse.com/docs/observability/features/token-and-cost-tracking)
- [Langfuse self-hosting](https://langfuse.com/self-hosting)
- [Twenty CRM APIs documentation](https://docs.twenty.com/developers/extend/capabilities/apis)
- [Twenty MCP Server (community)](https://github.com/mhenry3164/twenty-crm-mcp-server)
- [Redis context window management for LLM apps](https://redis.io/blog/context-window-management-llm-apps-developer-guide/)
- [Multi-tenant AI infrastructure — Medium (Isuru)](https://isuruig.medium.com/multi-tenant-ai-infrastructure-the-5-isolation-layers-that-determine-whether-your-customers-data-340aaeef4922)
- [Anthropic API pricing 2026 — Finout](https://www.finout.io/blog/anthropic-api-pricing)
- [Prompt caching cost savings case study — DEV Community](https://dev.to/stella_lin_82914c71e25769/anthropic-prompt-caching-cut-our-rca-cost-by-90-5gmb)
