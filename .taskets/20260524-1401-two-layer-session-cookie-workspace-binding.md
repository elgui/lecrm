---
id: 20260524-1401-two-layer-session-cookie-workspace-binding
title: "Two-layer session cookie with workspace binding"
status: done
priority: p1
created: 2026-05-24
updated: 2026-05-25
done: 2026-05-25
category: project
group: council-architecture-hardening
group_order: 40
order: 2
plan: true
tags: [security, auth, session, multi-tenant, cookie]
---

# Two-layer session cookie with workspace binding

## Pre-flight: Verify Previous Tasket

Before starting, verify Tasket 1 ("Slug tombstoning + reserved blocklist") completed:

1. `ls packages/db/migrations/0005_slug_tombstoning.sql` -- migration exists
2. `grep -c 'tombstoned_at' packages/db/queries/workspaces.sql` -- query updated
3. `git log --oneline -10 | grep -i "tombston"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

The current session implementation uses a single HMAC-SHA256 token with a shared signing key across all workspaces. The council identified two attack vectors:
1. **Cross-tenant replay:** If the HMAC key is shared, a stolen cookie from workspace A could theoretically be manipulated for workspace B
2. **Subdomain takeover:** A cookie issued for `acme.lecrm.fr` is only protected by cookie domain scoping, not by cryptographic binding to the tenant

The fix is a two-layer cookie design where the workspace slug is cryptographically bound into the token structure, and the workspace middleware validates the binding before any business logic executes.

Source of truth: `docs/council-architecture-review-2026-05-24.md`
Working directory: `/home/gui/Projects/leCRM`

## Approach

Replace the current `EncodeSession`/`DecodeSession` with a two-layer design:

**Outer layer (HMAC-SHA256):**
- Contains: workspace_slug + expiry + inner_payload_hash
- Purpose: proves the cookie was issued by us AND binds it to a specific workspace
- Verifiable without decrypting the inner payload

**Inner layer (AES-GCM encrypted):**
- Contains: user_id (UUID) + workspace_id (UUID) + issued_at + role grants
- Encrypted with a key derived from LECRM_SESSION_SECRET + workspace_slug (domain separation)
- Purpose: carries the actual session data

**Validation flow (workspace middleware):**
1. Extract workspace slug from Host header (existing behavior)
2. Verify outer HMAC — reject immediately if invalid
3. Check outer workspace_slug matches resolved workspace — reject if mismatch (cross-tenant replay blocked)
4. Decrypt inner payload — reject if decryption fails
5. Inject session context for downstream handlers

## Steps

1. Create `apps/api/internal/auth/session_v2.go`:
   - Define `SessionV2` struct (outer claims + encrypted inner)
   - `EncodeSessionV2(session, workspaceSlug, secret)` — produces two-layer token
   - `DecodeSessionV2(token, workspaceSlug, secret)` — verifies outer, decrypts inner
   - Key derivation: `HKDF(secret, salt=workspaceSlug)` for per-workspace encryption key
2. Update `apps/api/internal/auth/cookie.go`:
   - `SetSessionCookie` uses V2 encoding
   - Cookie domain scoping unchanged (still per-workspace subdomain)
3. Update `apps/api/internal/workspace/middleware.go`:
   - After resolving workspace from Host header, validate outer workspace claim matches
   - Reject with 401 if workspace binding fails (not 403 — don't leak tenant existence)
4. Update `apps/api/internal/auth/handlers.go`:
   - `Callback` handler uses V2 encoding with resolved workspace slug
   - `Me` handler uses V2 decoding
5. Migration compatibility: support decoding V1 tokens during rollout (grace period)
   - If V2 decode fails, attempt V1 decode + log deprecation warning
   - Re-issue as V2 on next successful V1 decode (transparent upgrade)
6. Update `apps/api/internal/auth/session_test.go`:
   - Test: V2 token for workspace A cannot be decoded with workspace B slug
   - Test: Tampered outer HMAC is rejected
   - Test: V1 → V2 transparent upgrade works
   - Test: Expired tokens are rejected
7. Update `apps/api/internal/auth/e2e_test.go`:
   - Test: Full OIDC flow produces V2 cookie bound to correct workspace
   - Test: Cookie from workspace A rejected on workspace B endpoint

## Done When

- [ ] Session tokens cryptographically bind workspace slug (outer HMAC includes slug)
- [ ] Inner payload encrypted with per-workspace derived key (HKDF)
- [ ] Workspace middleware rejects tokens where outer slug doesn't match resolved workspace
- [ ] V1 → V2 transparent migration works (decode V1, re-issue V2)
- [ ] Cross-tenant replay test: cookie from workspace A returns 401 on workspace B
- [ ] Existing auth e2e tests pass (no regression)
- [ ] `go vet` and `golangci-lint` clean

## Completion Verification

1. `ls apps/api/internal/auth/session_v2.go` -- new file exists
2. `grep -c 'workspaceSlug' apps/api/internal/auth/session_v2.go` -- workspace binding present
3. `grep -c 'HKDF' apps/api/internal/auth/session_v2.go` -- key derivation present
4. `cd apps/api && go test -race -count=1 ./internal/auth/...` -- all auth tests pass
5. `cd apps/api && go test -race -count=1 ./internal/workspace/...` -- middleware tests pass
6. Commit: `feat(auth): two-layer session cookie with workspace binding`

## References

- `apps/api/internal/auth/session.go` — current V1 implementation
- `apps/api/internal/auth/cookie.go` — cookie management
- `apps/api/internal/workspace/middleware.go` — workspace resolution
- `docs/council-architecture-review-2026-05-24.md` — council review
- golang.org/x/crypto/hkdf — HKDF key derivation
