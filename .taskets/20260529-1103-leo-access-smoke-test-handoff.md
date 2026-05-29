---
id: 20260529-1103-leo-access-smoke-test-handoff
title: "Staging: create Léo's login, end-to-end smoke test, and access handoff note"
status: todo
priority: p1
created: 2026-05-29
category: project
group: lecrm-staging-deploy
group_order: 220
order: 4
plan: true
tags: [deploy, staging, access, oidc, smoke-test, handoff, leo-test]
---

# Staging: create Léo's login, end-to-end smoke test, and access handoff note

## Pre-flight: Verify Previous Tasket

1. `curl -sS -o /dev/null -w '%{http_code}\n' https://demo.lecrm.gbconsult.me/healthz` -- 200 (order:3)
2. `curl -sS -D - -o /dev/null https://demo.lecrm.gbconsult.me/auth/login | grep -i location` -- 302 to Authentik
3. Demo workspace + seed data present (order:3)

**If any check fails, STOP and report. Do not proceed.**

> **Human-in-the-loop + outward-facing:** This creates a real account for Léo and produces material for an external person. **Do NOT send anything to Léo without Guillaume's explicit review/approval.** Keep the handoff product-facing only — see scope note below.

## Léo scope (binding — memory `feedback_leo_scope_lecrm`)

Léo's role here is **product/customer-signal tester only**. He gets a URL + login and a short "what to try" note. **Never** loop him into infra, hosting, stack, Brevo, or code — and never ask him deployment/architecture questions. The handoff note must contain zero infra detail.

## Context

The staging app is live and populated. Final step: give Léo a working login, prove the critical path end-to-end, and hand over clean access instructions.

Auth is OIDC-only via Authentik (ADR-009 §7.1; no password-login path in the app).

> **Council ruling (2026-05-29): use a LOCAL Authentik account for Léo** — unanimous. Google-upstream OIDC adds an external trust boundary / lockout risk (a Google suspicious-activity challenge at 11pm before a demo locks Léo out of staging entirely) that isn't worth it for a single tester. A local account is one credential lifecycle we fully control (revoke/rotate on demand). **Document the Google upstream connector as a follow-on** for when there's real multi-user onboarding to validate — not now.

Users are keyed on `(issuer, sub)` (tasket `20260515-192005-dd81`), so this choice is durable.

Working directory: `/home/gui/Projects/leCRM`.

## Steps

1. **Create Léo's user** in Authentik as a **local account** (council ruling — no Google upstream). Reference `scripts/authentik-provision-test-user.py` as a template; set a strong password and deliver it to Léo over a secure channel.
2. **Grant workspace membership + role.** Insert into `core.workspace_members` linking Léo's user to the `demo` workspace. **Role = `admin`** so he can exercise writes (create contacts, move deals) — not `owner` (owner can delete the workspace / manage tokens; not needed for testing). Confirm against `apps/api/internal/rbac/role.go`.
3. **End-to-end login smoke test.** Run the build-tagged OIDC flow test adapted to the live host, OR do it manually: open `https://demo.lecrm.gbconsult.me`, log in as Léo, and confirm — session cookie `Domain=demo.lecrm.gbconsult.me` (not a parent-domain wildcard, ADR-009 §5.2); `GET /auth/me` returns populated `{user_id, workspace_id}`; the user row is keyed on `(issuer, sub)`.
4. **Critical-path verification in the browser** (the things Léo will actually do): see seeded data → create a contact → edit it → move a deal across a stage (Kanban) → add a note/activity → CSV export. Fix anything broken before handoff.
5. **Write the access handoff note** — `docs/handoff/leo-test-access.md` (product-facing, infra-free):
   - the URL (`https://demo.lecrm.gbconsult.me`) and how to log in;
   - 5–6 concrete things to try (matching step 4);
   - what kind of feedback is most useful (usability, missing fields, confusing flows — i.e. customer signal);
   - a contact line for issues (Guillaume).
   Keep it short and non-technical. **Hold for Guillaume's review before it reaches Léo.**

## Done When

- [ ] Léo's Authentik user exists; identity method recorded.
- [ ] `core.workspace_members` links Léo to `demo` as `admin`.
- [ ] End-to-end login verified live (cookie scoping + `/auth/me` populated + `(issuer,sub)` key).
- [ ] Browser critical path green: view → create → edit → move stage → note → CSV export.
- [ ] `docs/handoff/leo-test-access.md` written, product-facing, infra-free.
- [ ] Handoff explicitly flagged as **pending Guillaume's review** before sending to Léo (not auto-sent).

## Completion Verification

1. `psql "$LECRM_DATABASE_URL" -c "SELECT u.email, m.role FROM core.workspace_members m JOIN core.users u ON u.id=m.user_id JOIN core.workspaces w ON w.id=m.workspace_id WHERE w.slug='demo';"` -- Léo present as admin
2. Manual: login as Léo at `https://demo.lecrm.gbconsult.me`, complete the step-4 path -- all succeed
3. `test -f docs/handoff/leo-test-access.md && ! grep -iE 'ssh|docker|postgres|caddy|ovh|dokku|sops|compose' docs/handoff/leo-test-access.md` -- note exists and is infra-free
4. Commit: `docs(handoff): Léo staging access note + access provisioned (pending review before send)`

## References

- `scripts/authentik-provision-test-user.py` — user provisioning template
- `apps/api/internal/rbac/role.go` — roles (admin vs owner)
- `deploy/README.md` Day-3 — end-to-end OIDC flow test + asserted properties
- ADR-009 §5.2 / §7.1 — cookie scoping, Authentik, `(issuer,sub)` keying
- memory `feedback_leo_scope_lecrm` — Léo = product/customer signal only; never infra
