---
id: 20260524-1404-session-revocation-mechanism
title: "Session revocation mechanism"
status: todo
priority: p2
created: 2026-05-24
category: project
group: council-architecture-hardening
group_order: 40
order: 5
plan: true
tags: [security, auth, session, revocation]
---

# Session revocation mechanism

## Pre-flight: Verify Previous Tasket

Before starting, verify Tasket 4 ("Drop LGTM stack") completed:

1. `grep -c 'slog' apps/api/internal/http/server.go` -- structured logging present
2. `git log --oneline -10 | grep -i "slog\|observability\|LGTM"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

Currently the only way to invalidate a session is rotating `LECRM_SESSION_SECRET`, which kills ALL active sessions for ALL workspaces simultaneously. For a compromised account at 5+ clients, this is an unacceptable blast radius.

The council debated two approaches and could not reach consensus:

**Approach A (Marcus's preference):** Short-TTL JWT (15 min) + refresh token rotation + Redis-backed revocation list. Revocation is O(1) in Redis. Cache miss rate negligible if tokens expire in 15 minutes.

**Approach B (Rook's preference):** Revocation table in Postgres + bloom filter in application layer refreshed every 30s. Keeps hot path off the database for 99.9% of requests. No Redis dependency (important for solo dev operational surface).

**Decision for implementor:** Choose based on whether Redis is already in the stack at implementation time. If you added Redis for workspace caching (earlier tasket), use Approach A. If no Redis, use Approach B. Both are valid.

**Timeline:** Acceptable risk to defer past v0 Design Partners (they're trusted, attack surface requires motivated adversary who knows system exists). MUST exist before public launch.

Source of truth: `docs/council-architecture-review-2026-05-24.md`
Working directory: `/home/gui/Projects/leCRM`

## Approach A: Short-TTL JWT + Redis

1. Replace session cookie content with a signed JWT (15 min TTL)
2. Add a refresh token (longer-lived, stored in HttpOnly cookie, rotated on use)
3. On logout or account compromise: add token JTI to Redis revocation set with TTL matching token expiry
4. Middleware checks Redis SISMEMBER before accepting a token
5. Token refresh endpoint: verify refresh token, issue new JWT, rotate refresh token

## Approach B: Postgres revocation table + bloom filter

1. Create `core.session_revocations (jti UUID PRIMARY KEY, revoked_at TIMESTAMPTZ, expires_at TIMESTAMPTZ)`
2. On logout/compromise: INSERT into revocation table
3. Application-layer bloom filter (in-memory, ~1KB per 1000 revocations)
4. Bloom filter refreshed from Postgres every 30s via background goroutine
5. Hot path: check bloom filter (O(1), in-memory). If positive: verify against DB. If negative: proceed.
6. Periodic cleanup: DELETE FROM session_revocations WHERE expires_at < NOW() (cron or River job)

## Steps (choose A or B based on stack state)

### Common steps (both approaches):
1. Add `jti` (JWT ID / session ID) field to the session token (V2 cookie from tasket 2)
2. Add `POST /auth/revoke` endpoint — revokes a specific session by JTI
3. Add `POST /auth/revoke-all` endpoint — revokes all sessions for a user
4. Update `POST /auth/logout` to revoke the current session's JTI
5. Add admin CLI command: `lecrm-admin session revoke --user-id <uuid>` (for emergency)
6. Update workspace middleware: check revocation before processing request

### Approach A specific:
7. Add Redis connection pool to API server config
8. On revoke: `SETEX revoked:<jti> <remaining_ttl> 1`
9. Middleware: `EXISTS revoked:<jti>` before accepting token
10. Add `/auth/refresh` endpoint for token rotation

### Approach B specific:
7. Create migration: `core.session_revocations` table
8. Implement bloom filter (use `bits` package or `github.com/bits-and-blooms/bloom`)
9. Background goroutine: refresh bloom filter every 30s from DB
10. Middleware: check bloom → if hit, verify against DB → reject if confirmed
11. River job: clean expired revocations daily

## Done When

- [ ] Individual sessions can be revoked without affecting other sessions
- [ ] All sessions for a user can be revoked (account compromise scenario)
- [ ] Logout revokes the current session
- [ ] Revoked sessions are rejected within 30s (Approach B) or immediately (Approach A)
- [ ] Admin CLI can emergency-revoke sessions by user ID
- [ ] No performance regression on the auth hot path (< 1ms added latency)
- [ ] Existing auth tests pass
- [ ] New tests: revoke session → subsequent request with that token returns 401

## Completion Verification

1. `grep -c 'revoke\|revocation' apps/api/internal/auth/` -- revocation logic present
2. `cd apps/api && go test -race -count=1 ./internal/auth/...` -- all tests pass
3. `cd apps/api && go test -race -count=1 ./internal/workspace/...` -- middleware tests pass
4. Commit: `feat(auth): add session revocation mechanism`

## References

- `apps/api/internal/auth/session_v2.go` — V2 session (from tasket 2)
- `apps/api/internal/auth/handlers.go` — auth endpoints
- `apps/api/internal/workspace/middleware.go` — request validation
- `docs/council-architecture-review-2026-05-24.md` — council review
- Auth0 JWT revocation pattern: short-TTL + refresh rotation
- github.com/bits-and-blooms/bloom — Go bloom filter library
