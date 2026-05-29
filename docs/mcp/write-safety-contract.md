# MCP Write-Safety Contract

**Status:** Active · **Source of truth:** [ADR-012 §6/§7/§8](../adr/ADR-012-mcp-native-capability-layer.md) · Resolves ADR-012 **TO RESOLVE 3 & 4**
**Implementation:** [`apps/api/capability/writesafety.go`](../../apps/api/capability/writesafety.go) · tests in `writesafety_test.go`

This document specifies the **shared safety primitives every MCP write tool reuses**, defined *before* any write tool exists so the next increment (`advance_deal` / `log_interaction` / `capture_lead`, ADR-012 §3) only composes them. It is the MCP counterpart of the OpenAPI + service-token contract tasket (`20260525-1006`): contract first, tools second.

The controls live in the **capability layer** (`apps/api/capability/`), the single source of CRM business logic (ADR-012 §1). They are **mechanism-agnostic** (ADR-012 §7): every decision reads only `Principal` fields, never *how* the principal authenticated. The same gate works whether the `Principal` came from a service token today or an OAuth 2.1 access token later (deferred S4). No control assumes a single trusted machine actor; no control hardcodes `actor_type=mcp_agent` as the only possible writer.

---

## 1. Scope → RBAC role mapping

Each write tool declares the scope it needs. The transport edge stamps a token's granted scopes onto the `Principal`; the capability layer maps that scope set to an effective role and authorizes. **Read-only tokens can never reach a write tool** — this is the primary blast-radius control (ADR-012 §8).

### Write scopes (ADR-009 §4.1 vocabulary)

| Scope | Meaning |
|---|---|
| `crm:read` | Read-only. Canonical read scope; **denied at every write gate.** |
| `crm:write` | The canonical write scope every MCP write tool declares. |
| `*` | Wildcard — full CRM read + write (capped at admin; see below). |

The mapping recognises **any scope containing `write`** (e.g. `deals:write`) or the `*` wildcard as write-granting. This mirrors `rbac.roleFromScopes` (`apps/api/internal/rbac/role.go`) **verbatim in policy**, so a single token authorizes identically against REST and MCP. (The capability layer cannot import `rbac` — that package pulls in `net/http` — so `capability.RoleFromScopes` re-implements the same rule and returns `capability.Role`.)

### Scope → role table

| Token scope set | Effective `Principal` role | Write op outcome |
|---|---|---|
| `crm:read` (or any non-write scope) | `RoleMember` | **Denied** → `ErrReadOnlyScope` |
| _empty / unrecognised_ | `RoleMember` | **Denied** → `ErrReadOnlyScope` |
| `crm:write` (or any `*write*`) | `RoleAdmin` | Allowed (subject to role gate) |
| `*` | `RoleAdmin` | Allowed (subject to role gate) |

**Cap at admin (binding, ADR-012 §6/§7).** A scope set never resolves above `RoleAdmin`. Member management, token administration, and workspace deletion are **owner-only** and must come from a human session — never a delegated, long-lived token. This holds the §7 non-foreclosure checklist: a delegated, lower-trust agent (today a service token, tomorrow an OAuth client) cannot escalate beyond admin.

### The single write gate: `AuthorizeWrite(Principal) error`

Every MCP write tool calls `AuthorizeWrite` first. Two gates, belt-and-suspenders:

1. **Scope gate** (primary, §8) — when the `Principal` carries scopes (token callers), they must resolve to a write-capable role; a read-only scope set yields `ErrReadOnlyScope` *even if `Role` were somehow mis-set*. A **scope-less** `Principal` (a human admin session authorizing purely by membership role) skips this gate.
2. **Role gate** — the resolved `Role` must be `RoleAdmin`+. `RoleNone` → `ErrUnauthenticated`; present-but-insufficient → `ErrForbidden`. Reuses the same `authorize()` the REST write ops use.

Denials are distinct and actionable: `ErrReadOnlyScope` lets the adapter tell the agent "this token is read-only" rather than returning a bare 403. Transport adapters map `ErrReadOnlyScope` and `ErrForbidden` → **403**, `ErrUnauthenticated` → **401**.

---

## 2. Dry-run / preview shape (`dry_run: true`)

Any write tool accepts a `dry_run: true` request flag. On dry-run the tool **plans the would-be effect and returns it without mutating** (no DB write, no idempotency row, no audit-of-mutation).

### Request

```jsonc
{ "dry_run": true, /* …tool-specific intent args… */ }
```

### Preview response

```jsonc
{
  "dry_run": true,
  "operation": "advance_deal",
  "summary": "would advance deal d-123 from \"Proposal\" to \"Negotiation\"",
  "effects": [
    {
      "action": "stage_change",          // create | update | delete | stage_change
      "entity_type": "deal",
      "entity_id": "d-123",
      "before": { "stage": "Proposal" }, // omitted for creates
      "after":  { "stage": "Negotiation" } // omitted for deletes
    }
  ],
  "confirmation_required": false,         // true for destructive/bulk ops
  "confirmation_token": ""                // present only when confirmation_required
}
```

An `Effect` is one would-be change described without mutating — the unit of the preview and the substrate of the confirmation digest. `before`/`after` are the field-level diff; map keys are JSON-marshalled in sorted order, which keeps the confirmation digest deterministic.

---

## 3. Confirmation-token handshake (destructive / bulk ops)

Delete and multi-entity (bulk) tools require an explicit **confirmation token** returned by a prior dry-run and echoed on the real call. No silent bulk mutation.

**Handshake:**

1. Agent calls the destructive op with `dry_run: true`. The Preview comes back with `confirmation_required: true` and a `confirmation_token`.
2. Agent (or a human/parent-agent reviewing the preview) calls the op **for real**, echoing the token in `confirmation_token`.
3. The capability layer re-plans the effect, recomputes the digest, and verifies the token against it. Only an exact match commits.

**Token properties** (`capability.Confirmer`):

- **Stateless, HMAC-signed** — `v1.<exp_unix>.<base64url(hmac_sha256(secret, version␀workspace␀operation␀digest␀exp))>`. No server-side table (the "new, thin" mechanism §6 calls for).
- **Bound to `(workspace, operation, effect-digest)`** — a token from workspace A can't confirm in workspace B; a token for `delete_contact` can't confirm `delete_deal`; **a token for one planned effect can't confirm a different one.** If the underlying rows changed between dry-run and confirm, the digest differs and the confirm fails → forces a fresh dry-run.
- **Time-boxed** — TTL 10 minutes. Long enough to inspect-and-confirm, short enough that a stale token can't authorize a much-later mutation.
- **Constant-time verification** — `subtle.ConstantTimeCompare`, so a failed match leaks no timing signal.
- **Fail-closed secret** — `NewConfirmer` rejects an empty secret; sourced from the same secrets store as other HMAC keys (ADR-007), never hardcoded.

**Errors** (adapters map both to a 409-class "confirmation required/invalid" tool error):

- `ErrConfirmationRequired` — destructive op called for real with no token.
- `ErrConfirmationInvalid` — token malformed, expired, wrong workspace/operation, or no longer matches the planned effect.

---

## 4. Idempotency-key handling

Every write tool accepts or derives an idempotency key; a duplicate call within the key's TTL **returns the cached result** instead of re-applying. Reuses `core.idempotency_keys` (ADR-007/011) — the same store the REST write ops and connectors use (`apps/api/internal/crm/connectors.go`).

- **Supplied** by the caller via `WriteOptions.IdempotencyKey`, **or derived** by the tool (e.g. a hash of the intent args) so retries of the same logical intent collapse.
- **Replay short-circuit:** when a key is set and the cache holds a result, `GuardedWrite.Run` returns it immediately with `Replayed=true`, *before* re-planning or applying. The mutation closure never runs on a hit.
- The cache lookup/store is owned by the tool's `replay`/`apply` closures (which hold the DB tx); `GuardedWrite` owns only the security-relevant ordering.

---

## 5. Audit attribution (fail-closed)

Every mutation emits an audit row inside the apply transaction — **fail-closed**: if the audit write fails, the mutation rolls back (reuses the existing `EmitAudit` pattern, ADR-007/009, `apps/api/internal/crm/connectors.go`).

- `actor_type` is read **from the `Principal`** (`ActorTypeMCPAgent` for an MCP agent today), **never hardcoded** as the sole writer — an OAuth-authenticated end-user client (S4) or a human session writes through the same path with its own `actor_type`. This satisfies the §7 non-foreclosure checklist.
- The token id (when token-authenticated) is recorded alongside, so every mutation is attributable and reversible-by-inspection — the backstop in the §8 prompt-injection posture.

---

## 6. Orchestration: `GuardedWrite` (the shape every write tool composes)

`GuardedWrite.Run` sequences the controls in a fixed, can't-get-it-wrong order. A write tool supplies three closures that isolate the only side-effecting work (DB I/O); `GuardedWrite` owns the ordering so it cannot be reordered per tool.

```
1. authorize  → AuthorizeWrite (scope→role gate). Read-only scope rejected here, before any closure runs.
2. dry-run    → if DryRun: plan() → Preview (+ confirmation token for destructive ops). No mutation, no idempotency write.
3. replay     → if IdempotencyKey set: cache hit returns stored result (Replayed=true), before re-planning/applying.
4. confirm    → if Destructive: re-plan, recompute digest, Verify the supplied token. Committed effect == previewed effect.
5. apply      → run the mutation. apply() emits fail-closed audit + stores the idempotency result inside its own tx.
```

| Closure | Responsibility |
|---|---|
| `replay` | Look up the idempotency cache; `hit=false` when no key/row. `nil` → never replays. |
| `plan` | Compute the would-be `Preview` without mutating (read tx). Drives both dry-run output and the confirmation digest. |
| `apply` | Perform the mutation; emit fail-closed audit + store idempotency result; return the canonical `(status, body)`. |

Non-destructive ops leave `Confirmer` nil. Destructive ops **must** set `Destructive: true` + a `Confirmer`, or `Run` errors (misconfiguration fails closed).

---

## 7. Why this is enough for the deferred OAuth future (§7)

The §6 guardrails — scopes cap blast radius, dry-run, confirmation, fail-closed audit — are *exactly* what an end-user OAuth client needs. Because every control reads only `Principal` fields, adding the MCP OAuth 2.1 authorization server later is a **pure transport-edge addition** that maps the access token onto the *same* `Principal`. **Zero tool changes.** Building these now is the same investment OAuth requires later — aligned, not duplicated.

---

## References

- [ADR-012 §6 (write-safety), §7 (auth-/tool-plane separation), §8 (prompt-injection posture)](../adr/ADR-012-mcp-native-capability-layer.md)
- [ADR-009 §4.1 — service-token scope model](../adr/ADR-009-stack-and-license.md) · `apps/api/internal/auth/service_token.go`
- `apps/api/internal/rbac/role.go` — RBAC roles + `roleFromScopes` (the mirrored policy)
- `apps/api/internal/crm/connectors.go` — existing idempotency + fail-closed audit pattern
- `core.idempotency_keys`, `core.audit_logs` (ADR-007)
