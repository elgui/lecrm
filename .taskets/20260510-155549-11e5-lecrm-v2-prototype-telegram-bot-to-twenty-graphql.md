---
id: 20260510-155549-11e5
title: leCRM v2 prototype — Telegram bot to Twenty GraphQL (chatbot-as-CRM-UI proof of concept)
status: later
priority: p2
created: 2026-05-10
updated: 2026-05-10
category: tooling
review: "rescoped-by: docs/adr/ADR-009-stack-and-license.md on 2026-05-10. Body re-aligned to Go + Postgres 17 + REST + thin MCP adapter + Apache 2.0 + OVH-first hosting. See body for the re-scoped done-criteria. Original Twenty-fork-shaped plan superseded; intent preserved."
---

# leCRM v2 prototype — Telegram bot → leCRM REST + MCP (chatbot-as-CRM-UI proof of concept)

## Why this tasket exists

P2 follow-up from ADR-005 (AI agent tenancy) and the moat-reframe in STRATEGIC-OVERVIEW §4. v2 is when leCRM monetises AI-native UX as a premium add-on (~€100-200/mo per client) — but the premise needs proving on existing CaaS infra (Tele-Claude pattern) before being sold.

Re-scoped 2026-05-10: original target was Twenty's public GraphQL with a service token. Under the locked Go + REST + MCP stack ([ADR-009](docs/adr/ADR-009-stack-and-license.md)), the target is **leCRM REST API + thin MCP adapter** (the `mark3labs/mcp-go` server in `apps/mcp/cmd/lecrm-mcp/`), not Twenty GraphQL.

## Goal

Prove the v2 chatbot-driven CRM premise on existing CaaS infra (Tele-Claude pattern) using leCRM's own REST+MCP surface, before selling it.

## Build

A Telegram bot that connects to leCRM via:
- **Primary path:** the MCP adapter (Streamable HTTP transport, workspace-scoped Bearer service token from ADR-009 §4.1; `actor_type = mcp_agent`).
- Optionally also the bare REST API for paths the MCP adapter doesn't yet expose.

Supported actions:
- Read deals
- Log a call activity
- Update deal stage
- Send a follow-up email via the email service (tasket 499c — gates this dependency)

## Validate

- Latency (<2s p95 end-to-end including LLM tool-use turns).
- Error handling (workspace-scoped token revoked mid-session; rate limit hit; CRM-side validation failure).
- Multi-user isolation (chat_id → workspace_id routing per ADR-005 §7).

## Output

- Prototype repo under `prototypes/telegram-crm-ui/` (separate Go module in the same workspace).
- Lessons-learned document at `docs/research/prototype-telegram-crm-ui-lessons.md` covering: latency observations, MCP rate-limit / scope-friction notes, recommendations for the v2 productisation.

## Constraints

- Dev VPS only, 1 test workspace, 1 test bot.
- No production data.
- Time-boxed: 2 days max.

## Acceptance

- End-to-end demo recorded showing all 4 actions working through the MCP adapter.
- Lessons doc covers latency, MCP rate-limit notes, scope-design observations, and v2 viability verdict.
- Recommendations for the production v2 design fed back into ADR-005 / ADR-009 follow-up.

## References

- [ADR-005](docs/adr/ADR-005-ai-agent-tenancy.md) (shared Telegram bot, chat_id routing pattern; agent runtime separate microservice).
- [ADR-009](docs/adr/ADR-009-stack-and-license.md) §4.1 (service tokens with `actor_type=mcp_agent`), §4.2 (MCP adapter location), §4.3 (AI SDK ↔ MCP framing).
- [STRATEGIC-OVERVIEW §2 + §4](docs/STRATEGIC-OVERVIEW.md) (AI-native UX as v2 monetisation, not v1 moat).
