---
id: 1131
title: "[Fix] Prompt-injection / confused-deputy hardening for write-capable MCP"
status: done
priority: p2
created: 2026-05-29
updated: 2026-05-29
done: 2026-05-29
tags: [mcp, security, prompt-injection, confused-deputy, remediation, adr-005, adr-012]
category: project
group: mcp-native-capability-layer
group_order: 210
order: 6
remediates: 20260529-1005-mcp-write-injection-hardening
---

## Remediation outcome

The previous task `20260529-1005-mcp-write-injection-hardening` was flagged
`partial_success`: the recovery note claimed the documentation commit "was
never made — the terminal output ends mid-session before `git commit` was
executed."

Verification shows that conclusion was **incorrect** — the work was in fact
committed before the prior session ended:

- Commit `252b8d61 feat(mcp): document trust boundary + adversarial
  scope-containment tests (ADR-012 §8)` lands the full scope:
  - `docs/mcp/trust-boundary.md` (104 lines) — defense-in-depth layers
    (scope containment, dry-run + confirmation, fail-closed audit) and the
    sanitization boundary drawn **explicitly**: §2 states the adapter does
    NOT sanitize CRM content and §5 assigns that to the agent-runtime Tier 2
    (ADR-005 §4).
  - `docs/mcp/write-safety-contract.md` — links the trust boundary.
  - `apps/api/capability/injection_test.go` (142 lines) — proves injected
    record/field content cannot escalate a read-only token to write, cross
    tenants, or one-shot a destructive op (confirmation handshake required).
  - `apps/mcp/internal/mcpserver/injection_test.go` (119 lines) — end-to-end
    through JSON-RPC dispatch: read-only token stays read-only regardless of
    arg content; hostile content forwarded verbatim as opaque data.
  - ADR-012 §8 + TO RESOLVE 6 marked resolved; ADR-005 §4 cross-referenced.

This remediation tasket therefore confirms the committed work is solid and
reconciles the dashboard state files that were left modified in the working
tree (`34ee8c2c chore(taskets): sync dashboard status metadata ...`).

### Build / test sanity

- `go test ./capability/ -run Injection` — `ok github.com/gbconsult/lecrm/apps/api/capability`
- `go test ./internal/mcpserver/ -run Injection` — `ok github.com/gbconsult/lecrm/apps/mcp/internal/mcpserver`
- `docs/mcp/trust-boundary.md` and `docs/mcp/write-safety-contract.md` present and committed.

Remediates: `20260529-1005-mcp-write-injection-hardening`
