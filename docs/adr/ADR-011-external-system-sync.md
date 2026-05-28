# ADR-011 — External-System Sync Abstraction

**Status:** Accepted
**Date:** 2026-05-27 (Proposed and Accepted)
**Deciders:** Guillaume
**Related:** [ADR-007](ADR-007-encryption-secrets-audit.md) (per-tenant credential storage). [ADR-009](ADR-009-stack-and-license.md) (stack selection; River jobs; schema-per-tenant). [ADR-010](ADR-010-metadata-engine.md) (JSONB metadata engine — extended by this ADR's connector use case; supersedes the planned "chatboting-connector-boundary" ADR-011 reference in ADR-010 §4a). Tasket `20260510-162158-1023` (secrets baseline). Tasket `20260514-114238-bf09` (Google OAuth submission). Tasket `20260514-114210-9b41` (test strategy — integration-boundary section).

---

## Context

The PRD executive summary lists Gmail-sync as a v0 feature and connector-ready architecture as a differentiator ("Tailorization speed" — §What Makes This Special #2). The naive path is to code Gmail-sync as a one-off integration. The risk, identified in round-2 council framing (Winston, 2026-05-14):

> "If v0 hardcodes Gmail-sync as a one-off rather than as the first instance of an 'external-system sync' pattern, every future connector is a rewrite."

Reinforced by Murat: connectors are an integration boundary that needs its own test architecture — contract tests, webhook reliability, rate-limit handling.

The metadata engine (ADR-010) and the background-job framework (River adapter in `apps/api/internal/jobs/`) already provide the extension points connectors need. This ADR defines the abstraction layer that sits between those primitives and any specific external system.

### Scope constraints

- **v0:** Gmail only. Read-only inbound import (thread → contact association). Write-back deferred to v1+.
- **v0 scale:** 1–10 workspaces, 3–15 users per workspace, <50 threads/sync per user.
- **Second connector (Shopify)** validated on paper only (§7) — no implementation until a client needs it.

---

## Decision

### 1. Connector seam: the `sync.Provider` interface

Every external system implements `sync.Provider` (defined in `apps/api/internal/sync/provider.go`):

```go
type Provider interface {
    ID() ProviderID
    Direction() SyncDirection
    Pull(ctx context.Context, conn *Connection) (*PullResult, error)
    Match(ctx context.Context, conn *Connection, rec InboundRecord) (*EntityMatch, error)
    ValidateCredentials(ctx context.Context, conn *Connection) error
}
```

**Design choices:**

- **Provider returns normalized data, not writes directly.** Providers produce `InboundRecord` structs (external ID, entity type, normalized fields, match hint). The sync engine routes these to entity-specific DB writers. This keeps providers decoupled from the CRM data layer.
- **Opaque cursors.** The sync cursor (`PullResult.Cursor`) is `json.RawMessage` — the engine stores and replays it, but only the provider interprets it. Gmail uses a history ID; Shopify would use a page cursor or webhook event ID. No framework changes needed per provider.
- **Match hints, not match logic in the engine.** The provider attaches a `MatchHint` (strategy + value) to each record. The engine delegates back to `Provider.Match` for resolution. This allows provider-specific matching (e.g., Gmail matches by email, Shopify by customer email or order reference).

### 2. Sync direction model

```go
type SyncDirection int
const (
    Inbound  SyncDirection = iota // external → leCRM
    Outbound                       // leCRM → external
    Bidir                          // both
)
```

v0 Gmail declares `Inbound`. The interface supports all three directions for future connectors. Outbound and Bidir add a `Push(ctx, conn, records) error` method to Provider — deferred to v1 to avoid speculative interface surface.

### 3. Connection lifecycle (`sync_connections` table)

Per-workspace table tracking provider connection state:

```sql
sync_connections (
    id           uuid PK,
    provider_id  text NOT NULL UNIQUE,  -- one connection per provider per workspace
    status       text NOT NULL CHECK (...),
    settings     jsonb,                 -- provider-specific config
    sync_cursor  jsonb,                 -- opaque cursor from last Pull
    last_sync_at timestamptz,
    last_error   text,
    created_at   timestamptz,
    updated_at   timestamptz
)
```

**Status lifecycle:**

```
pending → active → paused → active (user toggle)
                 → error  → active (retry succeeds)
                          → revoked (token permanently invalid)
active  → disconnected (user removes connection)
```

Credentials are NOT stored in this table. They live in the per-tenant secret store (SOPS v0; Vault v1+ per ADR-007). The sync engine resolves credentials at job runtime via `jobs.CredentialResolver`.

### 4. Entity ID mapping (`external_entity_mappings` table)

Dedicated per-workspace table for fast external ID ↔ leCRM entity lookups:

```sql
external_entity_mappings (
    id             uuid PK,
    provider_id    text NOT NULL,
    external_id    text NOT NULL,
    entity_type    text NOT NULL,
    entity_id      uuid NOT NULL,
    last_synced_at timestamptz,
    meta           jsonb,    -- provider-specific metadata about the link
    created_at     timestamptz,
    UNIQUE (provider_id, external_id)
)
```

**Why a dedicated table (not the metadata engine):**

- JSONB containment queries (`data @> '{"gmail_thread_id": "..."}'`) on the `objects` table are O(GIN-scan), not O(btree). At sync volume (hundreds of lookups per cycle), the dedicated UNIQUE btree index on `(provider_id, external_id)` is required.
- The mapping has a fixed, known schema — it's not "custom" data. JSONB flexibility adds complexity without benefit.
- FK-like semantics (entity_id references contacts/deals/companies) are explicit in the schema rather than buried in JSONB conventions.

A secondary index on `(entity_type, entity_id)` supports the reverse lookup (given a contact, find all external IDs linked to it — needed for the UI timeline).

### 5. Conflict resolution policy

**v0 (read-only import):** No conflicts possible. External system is the sole source of truth for imported data. leCRM stores a read-only copy.

**v1+ (write-back / bidirectional):** Per-connection configurable policy:

| Policy | Behavior |
|--------|----------|
| `external_wins` | External system overwrites leCRM changes. Simplest; suitable for canonical-source providers. |
| `internal_wins` | leCRM changes take precedence. Suitable for CRM-is-source-of-truth workflows. |
| `surface_conflict` | Both sides' changes are preserved; user resolves in the UI. Most correct; most complex. |

Default: `external_wins`. Stored in `sync_connections.settings` as `{"conflict_policy": "external_wins"}`.

### 6. Rate limiting and retry

**Rate limiting:** Per-provider configuration, not framework-level. Gmail's quota (250 units/sec/user) and Shopify's (2 req/sec for REST, 50 points/sec for GraphQL) have different shapes. The provider is responsible for respecting its own limits.

The engine provides:
- Context with deadline (configurable per connection, default 5 minutes).
- A `Pull` contract that may return partial results + cursor (the engine stores the cursor and schedules a continuation job).

**Retry policy (engine-level):**
- On transient error (5xx, timeout): exponential backoff — 1 min, 5 min, 30 min. Max 3 retries per sync cycle.
- On auth error (401, 403): mark connection as `error`, do not retry until credentials refreshed.
- On permanent error (provider returns explicit "not found"): skip record, log, continue.

### 7. Trigger mechanism: poll (v0), webhook (v1+)

**v0 (poll):** River periodic job per active connection. Default interval: 15 minutes. Configurable per connection in `settings.poll_interval_minutes`.

**v1+ (webhook):** Provider-specific webhook endpoint at `/v1/webhooks/{provider_id}`. The webhook handler verifies the signature, looks up the connection, and enqueues an immediate sync job. Falls back to polling if webhooks are unavailable (e.g., development environments).

### 8. Failure modes and observability

| Failure | Detection | Response |
|---------|-----------|----------|
| Token expired | `ValidateCredentials` returns error | Attempt refresh; if fails → status=`revoked` |
| Rate limited (429) | Provider receives 429 from external API | Backoff + partial cursor save |
| External API down | Pull returns network error | Exponential backoff; status=`error` after 3 failures |
| Entity mismatch | Match returns confidence < 0.8 | Skip; log warning; surface in sync dashboard (v1) |
| Schema drift | External system changes API response shape | Provider-level defensive parsing; unknown fields ignored |
| Cross-tenant leakage | Advisory lock prevents concurrent mutation | search_path verified pre- and post-job (existing pattern from `jobs.withSafeExec`) |

**Structured logging (slog):**
- `sync.start`: connection_id, provider, workspace_id
- `sync.pull.completed`: records_count, cursor_advanced
- `sync.record.matched`: external_id, entity_id, confidence
- `sync.record.created`: external_id, new_entity_id
- `sync.record.skipped`: external_id, reason
- `sync.completed`: connection_id, records_processed, duration
- `sync.error`: connection_id, error, will_retry

---

## Shopify Paper Exercise (Second Connector Validation)

To validate the abstraction, here is a complete walkthrough of implementing a hypothetical Shopify connector using the same seam. **No abstraction changes required.**

### Shopify provider implementation

```go
package shopify

type Provider struct{ client *shopify.Client }

func (p *Provider) ID() sync.ProviderID   { return sync.ProviderShopify }
func (p *Provider) Direction() sync.SyncDirection { return sync.Inbound }
```

### Pull

- **Cursor format:** `{"since_id": "5678901234", "created_at_min": "2026-05-01T00:00:00Z"}`
- **API call:** `GET /admin/api/2024-01/orders.json?since_id={cursor.since_id}&limit=50`
- **Normalization:** Each Shopify order → `InboundRecord{ExternalID: order.ID, EntityType: "deal", Fields: {title, amount, currency, customer_email}, MatchHint: {Strategy: "email", Value: order.customer.email}}`
- **Cursor advance:** Last order ID becomes new `since_id`.

### Match

- Gmail matches threads to contacts by participant email → **same MatchHint strategy ("email")**.
- Shopify matches orders to contacts by customer email → **same MatchHint strategy ("email")**.
- Shopify could also match by customer ID to an external_entity_mapping from a prior sync → **uses existing LookupByExternalID**.

### Entity mapping

| Shopify entity | leCRM entity | Mapping |
|----------------|-------------|---------|
| Customer | Contact | email match or create |
| Order | Deal | new deal per order |
| Product | (no v0 entity) | stored as metadata object via ADR-010 |

### Credentials

- OAuth 2.0 access token stored in per-tenant secrets (same slot shape as Gmail: `oauth_shopify_access_token`).
- `ValidateCredentials`: `GET /admin/api/2024-01/shop.json` — 200 = valid, 401 = revoked.

### Rate limiting

- Shopify REST: 2 requests/second (bucket with 40 request pool). Provider implements token-bucket internally.
- `Pull` returns partial results + cursor when approaching limit.

### What changes to the abstraction?

**None.** The Provider interface, Connection lifecycle, EntityMapping table, Engine orchestration, and rate-limit-via-cursor pattern all fit Shopify without modification. The only new code is the `shopify.Provider` implementation.

---

## Schema migration

Migration `0011_external_sync.sql` extends `core.lecrm_provision_workspace` with steps 11–12:
- Step 11: `sync_connections` table (per-workspace, UNIQUE on provider_id)
- Step 12: `external_entity_mappings` table (per-workspace, UNIQUE on (provider_id, external_id), indexed by (entity_type, entity_id))

Both tables use `CREATE TABLE IF NOT EXISTS` for idempotent re-provisioning.

---

## Go package layout

```
apps/api/internal/sync/
├── provider.go      Provider interface, core types (ProviderID, InboundRecord, PullResult, etc.)
├── connection.go    Connection struct, ConnectionStatus, ConnectionStore/MappingStore interfaces
├── registry.go      ProviderRegistry (Register/Get/List)
├── engine.go        SyncEngine (orchestrates pull → match → map → apply; stub implementation)
└── gmail/
    └── gmail.go     Gmail provider (implements Provider; panics on invocation — stub)
```

---

## Integration-boundary test architecture (cross-reference)

Per Murat's council input and tasket `9b41` (test strategy), the sync boundary requires:

- **Contract tests:** Provider implementations tested against recorded HTTP fixtures (httptest + golden files). Verifies normalization of external API responses into InboundRecords.
- **Mapping tests:** ConnectionStore and MappingStore tested with testcontainers-go against real Postgres (per existing pattern in `testfixtures/tenantpair`).
- **Engine integration tests:** Full pull → match → map → apply cycle with a mock Provider and real DB.
- **Webhook reliability tests (v1+):** Idempotency (same webhook delivered twice), ordering (out-of-order delivery), and signature verification.

These are commitments for the test-strategy scope doc, not implementations in this ADR.

---

## Consequences

**Positive:**
- Gmail-sync ships through the abstraction, not around it. Future connectors (Shopify, Outlook, chatboting) are additive — new Provider implementation + credential slot, no framework changes.
- The dedicated mapping table provides O(1) external-ID lookups that JSONB cannot match at sync volume.
- Opaque cursors and provider-owned rate limiting mean the framework never needs to understand provider-specific pagination or quota models.

**Negative:**
- More indirection for a single connector than a direct Gmail API integration. Justified by the PRD's explicit connector-ready requirement and the round-2 council's "cheap insurance" framing.
- Two new tables per workspace increase provisioning surface. Mitigated by `CREATE TABLE IF NOT EXISTS` idempotency and the existing Atlas sweep model.

**Neutral:**
- No credentials stored in the database — the seam delegates to the existing secret-management architecture (ADR-007). This is a constraint, not a feature or a cost.
