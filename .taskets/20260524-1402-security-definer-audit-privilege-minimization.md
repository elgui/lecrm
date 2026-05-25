---
id: 20260524-1402-security-definer-audit-privilege-minimization
title: "SECURITY DEFINER audit and privilege minimization"
status: todo
priority: p1
created: 2026-05-24
category: project
group: council-architecture-hardening
group_order: 40
order: 3
plan: true
tags: [security, postgresql, multi-tenant, privilege-escalation]
---

# SECURITY DEFINER audit and privilege minimization

## Pre-flight: Verify Previous Tasket

Before starting, verify Tasket 2 ("Two-layer session cookie") completed:

1. `ls apps/api/internal/auth/session_v2.go` -- V2 session file exists
2. `cd apps/api && go test -race -count=1 ./internal/auth/...` -- auth tests pass
3. `git log --oneline -10 | grep -i "session cookie"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

The council's security voice (Rook) identified SECURITY DEFINER functions as the highest-risk single point in the architecture. Every function marked SECURITY DEFINER runs with the definer's privileges (lecrm_provisioner), not the caller's. A single exploitable input validation weakness in any DEFINER function grants arbitrary DDL execution across all tenant schemas.

Marcus (Engineer) ranked this above subdomain takeover in exploitability timeline: "A misconfigured SECURITY DEFINER migration script ships on day one. Subdomain takeover requires an external attacker to find a lapsed CNAME."

The current SECURITY DEFINER functions live in:
- `packages/db/migrations/0001_init.sql` — `lecrm_provision_workspace(UUID)`
- `packages/db/migrations/0004_workspaces_admin_email_registry.sql` — `lecrm_provision_workspace_with_registry(UUID, slug, admin_email, creator_email, template)`

Source of truth: `docs/council-architecture-review-2026-05-24.md`
Working directory: `/home/gui/Projects/leCRM`

## Approach

For each SECURITY DEFINER function:
1. **Evaluate if SECURITY INVOKER is sufficient** — if the caller already has the needed privileges, DEFINER is unnecessary
2. **Add explicit schema qualification** — every table/function reference uses `core.tablename`, never relies on search_path
3. **Pin search_path within function body** — `SET search_path = core, pg_catalog` at function start
4. **Input validation hardening** — validate all parameters before any EXECUTE
5. **Minimize dynamic SQL surface** — reduce `format()` + `EXECUTE` to the minimum required operations

## Steps

1. Read and catalog all SECURITY DEFINER functions:
   ```bash
   grep -n 'SECURITY DEFINER' packages/db/migrations/*.sql
   ```
2. For `lecrm_provision_workspace(UUID)` in 0001_init.sql:
   - Add `SET search_path = core, pg_catalog` as first line in function body
   - Validate UUID format before constructing role name (reject if NULL or zero UUID)
   - Ensure `v_role_name` construction cannot produce SQL injection via UUID manipulation
   - Add explicit `IF EXISTS` checks before CREATE ROLE to prevent error on re-run
   - Add `REVOKE ALL ON FUNCTION core.lecrm_provision_workspace(UUID) FROM PUBLIC`
3. For `lecrm_provision_workspace_with_registry(UUID, slug, admin_email, creator_email, template)` in 0004:
   - Add `SET search_path = core, pg_catalog`
   - Validate slug format: `slug ~ '^[a-z][a-z0-9-]{2,31}$'` (match admin CLI validation)
   - Validate email format (basic check: contains @, non-empty)
   - Validate template is in allowed set (or NULL)
   - All schema-qualified: `core.workspaces`, `core.audit_log`, etc.
   - Add `REVOKE ALL ON FUNCTION ... FROM PUBLIC`
4. Create migration `0006_security_definer_hardening.sql`:
   - `CREATE OR REPLACE FUNCTION` for both functions with hardened versions
   - Add REVOKE/GRANT statements
   - Add comment explaining the security rationale
5. Verify the admin CLI's `tenant verify` invariants still pass with hardened functions
6. Run existing integration tests (provisioning tests use these functions)
7. Add a new test: call provisioning function with malformed inputs → expect clean error, not partial execution

## Done When

- [ ] All SECURITY DEFINER functions have `SET search_path = core, pg_catalog` pinned
- [ ] All table references are explicitly schema-qualified (`core.workspaces`, not `workspaces`)
- [ ] UUID and slug inputs are validated before any EXECUTE statement
- [ ] `REVOKE ALL ON FUNCTION ... FROM PUBLIC` applied to all DEFINER functions
- [ ] Only `lecrm_provisioner` role can call the provisioning functions
- [ ] Existing `tenant create` and `tenant verify` integration tests pass
- [ ] New test: malformed UUID → clean error (not partial provisioning)
- [ ] New test: invalid slug format → rejected before any DDL executes
- [ ] Atlas migration lint passes on 0006

## Completion Verification

1. `ls packages/db/migrations/0006_security_definer_hardening.sql` -- migration exists
2. `grep -c 'SET search_path' packages/db/migrations/0006_security_definer_hardening.sql` -- search_path pinned
3. `grep -c 'REVOKE ALL ON FUNCTION' packages/db/migrations/0006_security_definer_hardening.sql` -- revocations present
4. `cd apps/admin && go test -race -count=1 ./internal/tenant/...` -- tenant tests pass
5. `cd apps/migrate && go test -race -count=1 ./...` -- migration tests pass
6. Commit: `fix(security): harden SECURITY DEFINER functions with input validation and search_path pinning`

## References

- `packages/db/migrations/0001_init.sql` — original provisioning function
- `packages/db/migrations/0004_workspaces_admin_email_registry.sql` — extended provisioning
- `apps/admin/internal/tenant/verify.go` — 14-invariant verification suite
- `apps/admin/internal/tenant/create.go` — tenant creation (calls provisioning)
- `docs/council-architecture-review-2026-05-24.md` — council review
- PostgreSQL docs: SECURITY DEFINER vs SECURITY INVOKER
