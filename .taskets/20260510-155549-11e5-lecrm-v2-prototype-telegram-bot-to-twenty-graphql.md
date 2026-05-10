---
id: 20260510-155549-11e5
title: leCRM v2 prototype — Telegram bot to Twenty GraphQL (chatbot-as-CRM-UI proof of concept)
status: later
priority: p2
created: 2026-05-10
updated: 2026-05-10
category: tooling
---

P2 follow-up from docs/ARCHITECTURE.md §10 ADR-005 and tasket 001-technical-deep-dive (P2 prototype #1).

## Goal
Prove the v2 chatbot-driven CRM premise on existing CaaS infra (Tele-Claude pattern) before selling it.

## Build
A Telegram bot that connects to Twentys public GraphQL API with a service token, supporting:
- Read deals
- Log a call
- Update deal stage
- Send a follow-up email via the email service

## Validate
- Latency (<2s p95)
- Error handling
- Multi-user isolation (chat_id -> workspace_id routing per ADR-005)

## Output
- Prototype repo (separate or in /prototypes/telegram-crm-ui/)
- Lessons-learned document at docs/research/prototype-telegram-crm-ui-lessons.md

## Constraints
- Dev VPS only, 1 test workspace, 1 test bot
- No production data
- Time-boxed: 2 days max

## Acceptance
- End-to-end demo recorded showing all 4 actions working
- Lessons doc covers latency observations, GraphQL rate-limit notes, and v2 viability verdict

## References
- docs/adr/ADR-005-ai-agent-tenancy.md (shared bot routing pattern)
- docs/STRATEGIC-OVERVIEW.md §4 (UI-freedom moat)
- docs/ARCHITECTURE.md §6 (AI agent layer)
