# Council Architecture Review — leCRM

**Date:** 2026-05-24
**Council Members:** Architect (Serena Blackwood), Engineer (Marcus Webb), Security (Rook Blackburn), Researcher (Ava Chen)
**Format:** 3-round structured debate
**Architecture Rating:** 7/10

---

## Executive Summary

The council reviewed leCRM's full architecture: Go 1.25 + Chi v5 + sqlc + pgx v5, React 19 embedded via go:embed, PostgreSQL 17 schema-per-tenant with per-workspace roles, OIDC auth, Caddy wildcard TLS, Docker Compose, and LGTM observability.

**Verdict:** Unusually well-disciplined for a solo-dev project. Strong foundations (schema isolation, typed SQL, atomic provisioning, defense-in-depth). Three security gaps prevent production readiness for public clients but are each fixable in under a sprint. Architecture is viable for trusted Design Partners today.

---

## Areas of Convergence (all 4 members agree)

1. **Schema-per-tenant is correct** at 1-50 workspaces — ceiling is 500-1000 schemas
2. **LGTM self-hosted is premature** — defer to structured slog + Grafana Cloud free tier
3. **Slug tombstoning is non-negotiable** — highest-priority fix, ship this week
4. **Session needs workspace binding** — two-layer cookie (outer HMAC + workspace slug, inner encrypted payload)
5. **River is acceptable** if wrapped behind an interface (40-line seam buys swap optionality)
6. **Keep apps/migrate separate** — privilege boundary justifies go.work overhead

---

## Remaining Disagreements

| Tension | Position A | Position B |
|---------|-----------|-----------|
| Statement cache fix | Serena/Ava: PgBouncer transaction mode | Marcus: PgBouncer doesn't fully solve schema-qualified queries |
| Revocation mechanism | Rook: Bloom filter in front of revocation table | Marcus: Short-TTL JWT + Redis |
| Shared state layer | Marcus: Add Redis now | Serena: Redis adds operational surface for solo dev |

---

## Recommended Actions (Priority Order)

| # | Action | Timeline | Rationale |
|---|--------|----------|-----------|
| 1 | Slug tombstoning + reserved blocklist | This week | Only gap with no retroactive fix; closes phishing + replay |
| 2 | Two-layer session cookie with workspace binding | Sprint 1 | Closes cross-tenant replay; enables future revocation |
| 3 | SECURITY DEFINER audit — replace with INVOKER where possible | Sprint 1 | Shortest exploitation timeline of all findings |
| 4 | Drop LGTM — ship structured slog to stdout, Grafana Cloud free | Sprint 2 | Frees ~1GB RAM, reduces operational surface |
| 5 | Session revocation (short-TTL JWT or bloom filter) | Sprint 3 | Acceptable risk at v0 with trusted partners |
| 6 | PgBouncer + connection pool strategy | Pre-10 clients | Statement cache and connection ceiling not urgent at 3-5 tenants |
| 7 | River advisory locks for schema-switch safety | Pre-10 clients | Edge case under concurrent load, not v0 critical |

---

## Round 1: Initial Positions

### Architect (Serena Blackwood)

- Schema-per-tenant with SECURITY DEFINER provisioning is textbook tenant isolation. At 50 tenants, Postgres handles 50 schemas trivially. Real ceiling is 500-1000 schemas where pg_catalog bloat and connection-pool fragmentation bite.
- go:embed is a yellow flag: dev-loop friction at 50+ frontend components. Recommend dev-mode proxy to Vite's dev server, with go:embed reserved for production builds only.
- Go workspace with 3 modules maps cleanly to deployment boundaries, but MCP needs to become a 4th workspace module soon.
- Missing AI-native seam: metadata JSONB is write-optimized (GIN index on containment) but AI agents need semantic search. The MCP binary must NOT share the same pgxpool as the API.
- Weakest link: workspace resolver has no cache, and per-schema SQL defeats prepared-statement sharing (50x statement cache memory at 50 workspaces).

### Engineer (Marcus Webb)

- Stateless session is a liability before Sprint 7 ships revocation. Only way to invalidate = rotate LECRM_SESSION_SECRET, killing all sessions for all workspaces simultaneously.
- LGTM stack is premature. Five observability containers + Postgres + Caddy + Authentik + API on a single VPS is cramped. Authentik alone pulls 500+ MB under warm-up.
- JSONB metadata engine re-queries custom_property_definitions on every Set() with no caching. Will bite during bulk imports (500 contacts x 20 custom fields = 500 serial round-trips).
- go.work monorepo is the operational risk: CI runs 3 separate test+lint passes. When a change spans apps/api and packages/db, you track module boundaries that don't buy anything at this scale.
- CI smoke test teardown is fragile: inline psql heredoc drops schemas with string-interpolated UUID-derived names.

### Security (Rook Blackburn)

- Biggest concern: cookie theft via subdomain takeover. If any subdomain can be pointed at an attacker-controlled server (lapsed CNAME), a valid session cookie provides full tenant access. HMAC proves issuance, not destination.
- Critical question: is workspace ID cryptographically bound in the session, or is the HMAC key shared across all workspaces? If shared, cross-tenant cookie replay is possible.
- OIDC post-callback race condition window: between id_token verification and cookie issuance is where logout/session fixation bugs live.
- SECURITY DEFINER function is the highest-risk single point. Input validation weakness = provisioner-level access.
- Solo-dev cognitive load causing subtle workspace role misconfiguration is the operational security risk.

### Researcher (Ava Chen)

- Stack sound at target scale. Schema-per-tenant precedented (Pennylane uses similar PG isolation for French fintech).
- River library youth is primary technical risk — not yet battle-tested vs Sidekiq/Celery equivalents.
- 11-13 week timeline is aggressive but achievable for Go + Claude Code pairing.
- French CRM alternatives (Sellsy, noCRM.io) succeeded with sovereignty positioning but had full teams. Solo-dev is the novel variable.

---

## Round 2: Responses & Challenges

### Architect (Serena)

- Keep migrate separate: different lifecycle, elevated schema-creation privileges, should never share API's runtime permission set. Merging = privilege escalation surface.
- Cookie needs two-layer design: outer HMAC with workspace slug, inner encrypted payload with role grants. Definitively answers cross-tenant replay.
- River wrappable behind an interface (40 lines today, swap to Temporal if needed).
- Concede: LGTM is premature. Structured logging to stdout + Dokku's log aggregation until 20 workspaces.

### Engineer (Marcus)

- Prepared-statement cache pollution is urgent: PgBouncer transaction mode is incompatible with schema-qualified prepared statements. Design implicitly assumes session mode, capping at 100-150 concurrent connections on a 12GB VPS.
- SECURITY DEFINER > subdomain takeover in exploitability timeline. A misconfigured provisioning script ships on day one; subdomain takeover requires external attacker finding a lapsed CNAME.
- River + schema switching = ambiguous connection state on mid-execution failure. Need advisory locks around schema transitions in job workers.
- Workspace resolver cache + session revocation = same underlying problem (no shared in-process state). Fix one, fix both.

### Security (Rook)

- Session revocation table is a DoS amplifier: every authenticated request becomes a DB read on the hot path. Need bloom filter in front, refreshed every 30s.
- 50x plan cache bloat = inter-tenant DoS channel. Adversarial tenant can issue carefully crafted queries to pollute plan cache. Documented attack pattern against shared Postgres.
- Wildcard cert + churned subdomain = trivially convincing phishing. Browsers show padlock, users see no warning. Fix: slug tombstoning — never release a workspace subdomain back to the pool.

### Researcher (Ava)

- PgBouncer transaction mode with DEALLOCATE ALL is the documented approach for pgx v5.
- Pennylane shipped Day-1 observability but used Datadog SaaS, not self-hosted. Self-hosting tax is real for solo dev.
- RFC 9700 (OAuth 2.0 Security BCP) specifically warns about subdomain takeover for multi-tenant OAuth. Slug tombstoning is industry standard.

---

## Round 3: Final Synthesis

### Architect (Serena)

Strong consensus on priorities. Disagreements are implementation-timing disputes, not directional conflicts. Top 3 actions: (1) PgBouncer transaction-mode config as blocking prerequisite, (2) Two-layer cookie, (3) Slug tombstoning. Actions 2-3 parallelizable.

### Engineer (Marcus)

Ship THIS WEEK: slug tombstoning with reserved-slug blocklist. One migration, one middleware change. It's the only security gap that cannot be patched retroactively. Everything else has a recovery path. A recycled slug that gets weaponized does not. Punch list: slug tombstoning → Redis for cache+revocation → search_path middleware → River load test → go:embed audit.

### Security (Rook)

Not secure enough for public paying clients in current state. Not a damning verdict — three issues fixable in under a sprint each. v0 with trusted Design Partners: acceptable risk (attack surface requires motivated adversary who knows system exists). Public launch without fixes: unacceptable (slug reuse and SECURITY DEFINER vulns appear in first 30 minutes of basic external assessment). Fix three items + one day of external review = production approval.

### Researcher (Ava)

Architecture rated 7/10 — above median for solo-dev at this maturity stage. JWT with short TTL + refresh rotation is industry standard (Auth0, Clerk, Supabase pattern). go.work with privilege-separated modules matches Grafana Labs multi-binary pattern. Three fixable gaps prevent 8+.

---

## Key Findings Summary

### Strengths
- Schema-per-tenant with per-workspace PG roles (textbook isolation)
- Typed SQL via sqlc (eliminates SQL injection class)
- Atomic SECURITY DEFINER provisioning (single function, idempotent)
- Privilege-separated binaries (API, admin, migrate)
- Defense-in-depth search_path verification in RunWorkspaceJob
- Single-binary deployment (operational simplicity)
- Well-documented ADR chain (10 ADRs)

### Weaknesses (Fixable)
- No slug tombstoning (phishing + replay vector)
- Session cookie not workspace-bound (cross-tenant replay possible)
- SECURITY DEFINER functions not audited for INVOKER conversion
- No session revocation mechanism until Sprint 7
- LGTM stack consuming ~1GB RAM unnecessarily
- No workspace resolver cache (DB hit per request)
- Prepared-statement cache bloat at scale (50x per tenant)

### Risks (Acceptable at v0)
- River library immaturity (mitigated by interface wrapper)
- PgBouncer absence (not critical at 3-5 tenants)
- Metadata definition cache miss on bulk operations
- go:embed dev-loop friction (proxy pattern mitigates)
