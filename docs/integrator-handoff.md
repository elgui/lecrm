# Integrator Handoff (Phase 1) — `gb-tenant` Operator Guide

**Audience:** Léo (Vernayo, GB Consult integrator) and any future integrator-partner.
**Scope:** Story 8.1 (Sprint 8). Phase 2 (versioned methodology) and Phase 3
(per-tenant audit surface) ship in later sprints; this document grows with
them.

## What you get in Phase 1

One command, on your laptop, no Notion checklist, no Postgres credentials,
no partial-provisioning failure mode:

```bash
gb-tenant create --slug chauvet79 --admin-email leo@vernayo.com
```

That single invocation provisions, in **one Postgres transaction**:

- `core.workspaces` row (registry, with `admin_email`, `creator_email`,
  `provisioning_features_applied`)
- Workspace role `workspace_<uuid_no_hyphens>` (LOGIN, scoped, 30 s
  statement timeout)
- Workspace schema `workspace_<uuid_no_hyphens>` + metadata-engine tables
- River queue schema `river_<uuid_no_hyphens>`
- `core.audit_log` row, `event = 'workspace.provisioned'`
- Default 5-stage sales pipeline (Discovery → Closed-Won/Lost)

All six writes commit together or **nothing** commits. The first Design
Partner demo doesn't end with a half-provisioned tenant. (Story 8.1 AC-T1.)

## One-time setup (Léo's laptop)

### 1. SSH access to the production Dokku host

You need SSH-key access (no password) to the integrator user on the main
Dokku VPS:

```bash
ssh dokku@54.37.157.49 apps:list
```

If that returns the list of Dokku apps, you're good. If it asks for a
password or "Permission denied (publickey)" — send Guillaume your public
key (`~/.ssh/id_*.pub`) and have him add it via
`ssh-copy-id` or directly to `~dokku/.ssh/authorized_keys` on the host.

### 2. Add the `gb-tenant` alias to your shell

Append to `~/.zshrc` (or `~/.bashrc`):

```bash
alias gb-tenant='ssh dokku@54.37.157.49 run lecrm-admin tenant'
```

`dokku run lecrm-admin` already invokes the image's ENTRYPOINT (`/app/lecrm-admin`),
so the alias must NOT repeat the binary path — passing it again makes urfave/cli
treat `/app/lecrm-admin` as a subcommand and exit with `No help topic for ...`.

Open a new terminal. Verify:

```bash
gb-tenant list
```

You should see the current tenant registry (empty if 8.1 just shipped).

## Tuesday-morning command shape

When a new client is ready to be onboarded:

```bash
# 1. Provision (default template, 5-stage gbconsult pipeline)
gb-tenant create --slug chauvet79 --admin-email contact@chauvet79.fr \
    --owner-email leo@vernayo.com

# 2. Verify all 14 invariants are green
gb-tenant verify --slug chauvet79

# 3. Hand off the slug to the API team (or to your own configuration)
gb-tenant get --slug chauvet79
```

**That's the whole flow.** No SQL, no Postgres credentials on your laptop,
no manual schema creation.

## Slug rules (AC-V1)

A slug is the URL-safe tenant identifier and lives forever in
`core.workspaces.slug`. Pick carefully:

- Regex: `^[a-z][a-z0-9-]{2,31}$`
- Lowercase ASCII first character (no digit start)
- Alphanumeric + hyphens, length 3–32
- Examples that work: `chauvet79`, `chauvet-79`, `acme-001`
- Examples that **don't**: `chauvé-79` (non-ASCII), `Chauvet79`
  (uppercase), `79chauvet` (digit start), `ch` (too short)

The CLI runs this regex before connecting to Postgres — an invalid slug
never even touches the database.

## Idempotency flags

| Flag | Behavior | When to use |
|---|---|---|
| (none) | Fails loud if slug exists (AC-F2 verbatim error). | Default — first provisioning. |
| `--upsert` | Silent no-op if slug exists; provisions fresh otherwise. | Safe re-runs from scripts / CI smoke tests. |
| `--force-recreate` | **Destroys** the existing tenant (role + schemas + River queue + registry row + audit markers) and recreates from scratch. Atomic. | Demo resets only. **Never on a paying tenant.** |

## The verify subcommand

```bash
gb-tenant verify --slug chauvet79
```

Runs 14 invariants AC-I-01..AC-I-14 sequentially. Each prints one line:

```
[OK] INV-01 Tenant role exists
[OK] INV-02 Tenant schema + River queue schema exist
...
[OK] INV-12 Default pipeline contains exactly 5 stages of gbconsult-default
[OK] INV-13 Stdout includes [PROVISION] RBAC seeding: skipped (covered by apps/admin/internal/tenant/create_test.go)
[OK] INV-14 Migration cold-clean against vanilla postgres:16 (deferred to sibling story)
```

Exit 0 if all pass; non-zero on the first `[FAIL]`. Pass `--all-failures`
to scan every invariant even after a failure (useful for triage).

## Integrator workspace access (grant / revoke / list)

GB Consult's integrator (you, Léo) administrates a client workspace as a
**distinct, non-billable principal** — hidden from the client's member list
and tagged in `core.audit_log`. Whether you can switch into a workspace is
governed by `core.integrator_grants`, an email-keyed pending-grant table.
The grant is recorded against your **email**, not a user row, so it exists
*before you have ever logged into that tenant* — first login materializes the
real membership from the grant.

### Auto-grant on provision (the common case)

When you provision a tenant with `--owner-email`, the same flow that creates
the workspace also writes a matching integrator grant for that email — **in
the same Postgres transaction** as provisioning (or on the same connection,
immediately after, for the non-destructive paths). So a freshly-provisioned
tenant is switch-able the moment `create` returns:

```bash
gb-tenant create --slug chauvet79 --admin-email contact@chauvet79.fr \
    --owner-email leo@vernayo.com
# ... provisioning output ...
# [PROVISION] integrator grant: leo@vernayo.com
```

Notes:

- The grant is written **only** for `--owner-email` (the integrator). The
  client's `--admin-email` is a normal owner and is **never** auto-granted
  integrator access. Omit `--owner-email` and no grant is created.
- It is idempotent: re-running `create --upsert` does not duplicate the
  grant (`ON CONFLICT DO NOTHING` on the case-insensitive
  `(workspace_id, lower(email))` index).
- `granted_by` is attributed to `$LECRM_OPERATOR_EMAIL` (set this in your
  shell so the audit trail shows who created the grant).

### Explicit grant / revoke / list (tenants you did not provision)

For tenants that already exist — or to grant a second integrator — use the
`integrator` subcommands:

```bash
# Grant integrator access (idempotent)
gb-tenant integrator grant --slug chauvet79 --email leo@vernayo.com

# Revoke it (idempotent — "nothing to revoke" if absent, still exits 0)
gb-tenant integrator revoke --slug chauvet79 --email leo@vernayo.com
# → integrator access revoked: leo@vernayo.com on chauvet79 (grant removed, 1 live membership(s) cleared)

# List grants for one tenant ...
gb-tenant integrator list --slug chauvet79

# ... or across every workspace
gb-tenant integrator list
```

`list` prints a table joined to `core.workspaces.slug`:

```
SLUG       EMAIL             GRANTED_BY          GRANTED_AT
chauvet79  leo@vernayo.com   ops@gbconsult.me    2026-05-30T16:18:42Z
```

Flags:

| Command  | Flag       | Meaning                                                   |
|----------|------------|-----------------------------------------------------------|
| `grant`  | `--slug`   | Tenant slug (required)                                     |
| `grant`  | `--email`  | Integrator email to grant (required)                      |
| `grant`  | `--granted-by` | Operator attribution (defaults to `$LECRM_OPERATOR_EMAIL`) |
| `revoke` | `--slug`   | Tenant slug (required)                                     |
| `revoke` | `--email`  | Integrator email to revoke (required)                     |
| `list`   | `--slug`   | Filter to one tenant (optional; omit to list all)         |

An unknown or tombstoned slug fails loud with a `tenant_not_found`
structured error rather than silently doing nothing.

`revoke` is a complete off-switch: it deletes the pending grant **and** any
already-materialized `role='integrator'` membership row for that email,
within a single transaction. Removing the live membership matters because
once you have logged into a tenant, login-time elevation has written a real
`workspace_members` row that is independent of the grant — deleting the grant
alone would stop *future* elevation but leave your current owner-equivalent
access intact. Access ends effective immediately: the rbac middleware
re-resolves your role from `workspace_members` on every request, so the next
request after revoke carries no integrator principal. (A genuine non-integrator
membership that happens to share the email — e.g. you are also a real owner
somewhere — is never touched: the delete is scoped to `role='integrator'`.)

> **Login-time elevation and the workspace switcher UI now ship alongside
> this.** The grant is the precondition the login flow reads to materialize an
> `integrator` membership row; `GET /auth/workspaces` and the
> `WorkspaceSwitcher` component surface the workspaces you can switch into.

## Phase 2 — Versioned Methodology Config (Sprint 9)

Phase 2 makes Léo's 5-element methodology a portable, diffable, replayable
config artifact stored as versioned JSONB rows in each tenant's `objects`
table (per ADR-010 §3).

### What you get in Phase 2

```bash
# Apply the standard methodology template to a tenant
gb-tenant config apply --slug chauvet79

# View the current methodology config
gb-tenant config show --slug chauvet79

# View a specific version
gb-tenant config show --slug chauvet79 --version 1

# Compare methodology configs between two clients
gb-tenant config diff --slug-a chauvet79 --slug-b expressionist

# Clone one client's methodology onto another
gb-tenant config replay --src chauvet79 --dst new-client
```

### The 5-element methodology

Each config captures:

1. **Acquisition channels** — lead sources with UTM-style attribution
   metadata (source + medium)
2. **Pipeline stages** — ordered stages with entry/exit conditions that
   gate deal progression
3. **Stage properties** — typed fields required at each stage (string,
   number, boolean, enum, date)
4. **Automations** — triggers (stage transitions) and actions (email
   notifications, field auto-sets)
5. **Color coding** — hex colors per stage for Kanban UI rendering

### Versioning

Configs are **append-only** — each write creates a new row with an
incremented `version_seq`. The CLI reads the latest by
`MAX(version_seq)`. History is preserved; use `--version N` to inspect
prior versions.

Storage: `<workspace>.objects` table, `object_type = 'methodology_config'`,
`version_seq` inside the `data` JSONB payload.

### Templates

The `gbconsult-default` template ships as the reference implementation
(see `docs/templates/gbconsult-default-methodology.json`). Phase 2
supports a single template; multi-template registry lands in a future
sprint.

### Replay workflow (new client onboarding)

```bash
# 1. Provision the tenant (Phase 1)
gb-tenant create --slug new-client --admin-email new@client.fr \
    --owner-email leo@vernayo.com

# 2. Clone methodology from your best reference client
gb-tenant config replay --src chauvet79 --dst new-client

# 3. Verify identical config
gb-tenant config diff --slug-a chauvet79 --slug-b new-client
# → "methodology configs are identical."
```

### What's NOT in Phase 2

- **Admin UI for config editing** — CLI-only in v0; admin UI lands v1+.
- **Config-driven automation execution** — automations are declared but
  not yet wired to the stage-transition engine. Phase 3 or Sprint 10.
- **Multi-template registry** — single `gbconsult-default` template.
  Custom templates require manual JSON authoring for now.
- **Config validation against `custom_property_definitions`** — the
  property types in the config are informational; runtime enforcement
  via ADR-010 §4 lands when the API consumes this config.

---

## Phase 3 — Per-tenant audit + observability surface (Sprint 11)

When Léo ships a config or wonders "did the welcome email actually
fire on tenant `chauvet79` yesterday?", he no longer needs to ping
Guillaume or pry open psql. Two equivalent surfaces sit on top of the
`core.audit_log` table that has been collecting events since Sprint 7:

### CLI: `lecrm-admin audit query`

Same SSH path as the rest of the CLI, so the muscle memory carries over.

```bash
# Everything for a tenant in the last 24 hours
gb-tenant audit query --tenant chauvet79 --since 24h

# Did the welcome-email automation fire?
gb-tenant audit query \
  --tenant chauvet79 \
  --event email.send.success \
  --since 7d

# Just config-replays attributed to a human
gb-tenant audit query \
  --tenant chauvet79 \
  --event config.template.replayed \
  --actor human_api
```

Flags:

| Flag        | Meaning                                                          |
|-------------|------------------------------------------------------------------|
| `--tenant`  | Tenant slug (required)                                           |
| `--since`   | Lower bound: RFC3339 (`2026-05-27T00:00:00Z`) or relative (`24h`, `7d`) |
| `--until`   | Upper bound, same formats                                        |
| `--event`   | Exact event name (e.g. `email.send.success`, `config.template.applied`) |
| `--actor`   | `human_api` \| `mcp_agent` \| `internal_service` \| `system`     |
| `--limit`   | Page size (default 100, cap 500)                                 |
| `--format`  | `table` (default) or `json`                                      |

### REST: `GET /admin/audit`

For dashboards, scripts, or the future v1+ admin UI.

```bash
curl -H "Authorization: Bearer $LECRM_ADMIN_TOKEN" \
  "https://api.lecrm.fr/admin/audit?tenant=chauvet79&since=24h&event=email.send.success"
```

Query params mirror the CLI flags. The endpoint lives outside
workspace-subdomain routing (Léo crosses tenants by passing
`?tenant=`), authenticates via constant-time bearer-token compare
against `LECRM_ADMIN_TOKEN`, and returns:

- `200` `{ "tenant": "...", "count": N, "entries": [...] }`
- `401` if the bearer token is missing or wrong
- `404` if the tenant slug is unknown
- `400` for malformed `since`/`until`/`limit`
- `503` if `LECRM_ADMIN_TOKEN` is unset on the server (fail-closed)

### What Phase 3 wires that Phase 2 didn't

Phase 2's `config apply` and `config replay` now emit:

- `config.template.applied` (actor_type=`human_api`)
- `config.template.replayed` (actor_type=`human_api`)

with `payload.operator_email` populated from `LECRM_OPERATOR_EMAIL` (set
this in Léo's shell so the audit trail attributes mutations correctly).

### What's NOT in Phase 3

- **Dashboard UI** — JSON + table output only; richer UI is v1+.
- **OIDC admin auth** — single shared bearer token at v0; OIDC claims
  with admin scope land when the admin-UI ships.
- **Tail mode / live streaming** — point-in-time queries only.
- **Cross-tenant aggregation** — one tenant per query; bulk audit
  export is operator-side (psql) for now.

---

## Operations notes (for Guillaume)

### Production Dokku deployment (AC-D1..D3, AC-D6)

> **Scope note:** "production Dokku host `54.37.157.49`" below refers to the
> `lecrm-admin` **CLI** only (one-shot, `Running: false`, invoked via
> `dokku run`). The leCRM **API/web app does not run on Dokku** — it runs as
> a Compose stack on `51.77.146.49` (staging). See
> [`docs/INFRASTRUCTURE.md`](INFRASTRUCTURE.md) for the full environment map.

When 8.1 lands, run these on the production Dokku host (`54.37.157.49`):

```bash
# 1. Create the app (CLI-only — no HTTP listener)
dokku apps:create lecrm-admin
dokku proxy:disable lecrm-admin

# 2. Wire the DSN. lecrm-admin connects as lecrm_provisioner (Tier-0 secret
#    per ADR-007). Use the same DSN that lecrm-api's migration job uses,
#    NOT the application DSN.
dokku config:set lecrm-admin \
  DATABASE_URL=postgres://lecrm_provisioner:...@DB_HOST:5432/lecrm \
  LECRM_PROVISIONER_DSN=postgres://lecrm_provisioner:...@DB_HOST:5432/lecrm \
  LECRM_LOG_LEVEL=info

# 3. Push the image (CI does this on main merges; manual is fine too)
git push dokku-prod main
```

**Dokku-specific gotchas observed during the first deploy on 54.37.157.49:**

- The repo root has no `Dockerfile`; the admin image lives at
  `apps/admin/Dockerfile`. Tell Dokku where to find it:
  `dokku builder-dockerfile:set lecrm-admin dockerfile-path apps/admin/Dockerfile`.
- lecrm-admin is a one-shot CLI, not a long-running service. Dokku's
  default `web` process tries to keep it alive and marks the deploy
  failed when it exits. Disable the web process so the image is built
  but never auto-started: `dokku ps:scale lecrm-admin web=0`.
  Operator invocations via `dokku run lecrm-admin tenant ...` spin up
  fresh one-shot containers.

The image refuses to start if any `LECRM_API_*` env var is present
(AC-D5 — Winston's R2 condition for same-image binary co-location).

### What's NOT in Phase 1

Deferred to later sprints / stories:

- **RBAC role seeding** — stdout prints `[PROVISION] RBAC seeding: skipped
  (not implemented in v0)` on every provision. Flips to `ok (3 roles
  applied)` when the RBAC sibling story lands.
- **OAuth client registration** — sibling Sprint 8 story.
- **Versioned methodology config** — Phase 2 (Sprint 9), now shipped.
  See "Phase 2" section above.
- **`tenant update` / `tenant delete`** — out of scope. Use
  `--force-recreate` for the demo-reset use case.
- **Vault-backed credential rotation** — v1.

## References

- Story 8.1: `{output_folder}/implementation-artifacts/8-1-lecrm-admin-tenant-create.md`
- Parent tasket: `.taskets/20260514-204646-dc1b-lecrm-v0-integrator-handoff-phase-1-one-command-te.md`
- Phase 2 (Sprint 9 methodology config): `.taskets/20260514-204706-731a-...`
- Phase 3 (Sprint 11 audit surface): `.taskets/20260514-204724-fa6b-...`
- ADR-009 §2.1 — `core.lecrm_provision_workspace` SECURITY DEFINER contract
- ADR-007 — audit log shape
- ADR-010 — metadata-engine pattern (JSONB-primary on `objects` table)
- Reference template: `docs/templates/gbconsult-default-methodology.json`
