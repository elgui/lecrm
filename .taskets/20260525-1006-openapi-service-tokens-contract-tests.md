---
id: 20260525-1006-openapi-service-tokens-contract-tests
title: "OpenAPI 3.1 generation + service tokens + contract tests"
status: done
priority: p1
created: 2026-05-25
updated: 2026-05-28
done: 2026-05-28
category: project
group: crm-crud-complete
group_order: 60
order: 4
plan: true
tags: [api, openapi, service-tokens, testing, sprint-7]
---

# OpenAPI 3.1 generation + service tokens + contract tests

## Pre-flight: Verify Previous Taskets

Before starting, verify REST handlers are complete:

1. `ls apps/api/internal/http/contacts.go apps/api/internal/http/deals.go` -- handlers exist
2. `cd apps/api && go test -race -count=1 ./internal/http/...` -- handler tests pass

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

Sprint 7 deliverables: OpenAPI spec generation, workspace-scoped service tokens (needed for the chatboting connector from ADR-011 and MCP adapter), and contract tests validating the REST surface.

Source of truth: `docs/sprint-plan.md` Sprint 7
Working directory: `/home/gui/Projects/leCRM`

## Steps

1. OpenAPI 3.1 spec generation:
   - Evaluate: `ogen` (generates server from spec) vs `oapi-codegen` (generates from spec) vs hand-written spec + validation
   - Recommended: hand-write `docs/openapi.yaml` and generate TypeScript types for frontend via `@hey-api/openapi-ts`
   - Spec covers all entity endpoints, pagination model, error responses
   - CI job: validate spec + generate types on every PR

2. Service tokens (ADR-009 §4.1):
   - Migration: add `service_tokens` table to workspace schema:
     ```sql
     CREATE TABLE service_tokens (
       id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
       name text NOT NULL,
       token_hash text NOT NULL,  -- argon2id
       actor_type text NOT NULL CHECK (actor_type IN ('human_api','mcp_agent','internal_service','connector')),
       scopes jsonb NOT NULL DEFAULT '["*"]',
       expires_at timestamptz,
       last_used_at timestamptz,
       created_at timestamptz NOT NULL DEFAULT now()
     );
     ```
   - Endpoints:
     - `POST /v1/workspace/tokens` — create token, return plaintext once (never stored)
     - `GET /v1/workspace/tokens` — list tokens (name, actor_type, scopes, expires_at, last_used_at — never hash)
     - `DELETE /v1/workspace/tokens/:id` — revoke token
   - Middleware update: accept `Authorization: Bearer <token>` alongside session cookies
     - Hash incoming token with argon2id, look up in service_tokens
     - Set actor_type from token record
     - Update last_used_at
   - Token format: `lecrm_<workspace_slug>_<random_32bytes_base64url>`

3. Contract tests:
   - For each endpoint in the OpenAPI spec, verify:
     - Response matches spec schema (status codes, field types, required fields)
     - Error responses match error schema
     - Pagination contract (cursor format, has_more semantics)
   - Use `go test` HTTP table tests against a real test server (testcontainers Postgres)
   - Verify service token auth works alongside session cookie auth

4. Frontend type generation:
   - `@hey-api/openapi-ts` generates TypeScript types from `docs/openapi.yaml`
   - TanStack Query hooks use generated types
   - CI: regenerate on spec change, fail if types are stale

## Done When

- [ ] OpenAPI 3.1 spec covers all entity endpoints
- [ ] Service tokens: create, list, delete, revoke working
- [ ] Bearer token auth works in middleware alongside session cookies
- [ ] `actor_type` from service token flows into audit log entries
- [ ] Contract tests validate all endpoints against spec
- [ ] TypeScript types generated from spec
- [ ] CI validates spec + types freshness

## Completion Verification

1. `ls docs/openapi.yaml` -- spec exists
2. `grep -c 'service_tokens' packages/db/queries/*.sql` -- token queries exist
3. `cd apps/api && go test -race -count=1 ./...` -- all tests pass including contract tests
4. Commit: `feat(api): OpenAPI 3.1 spec, service tokens, contract test suite (Sprint 7)`

## References

- `docs/sprint-plan.md` Sprint 7
- ADR-009 §4 — Idempotency-Key, pagination, service tokens
- ADR-009 §4.1 — token format, argon2id hashing, actor_type claim
- ADR-011 §6 — service tokens for connector auth
