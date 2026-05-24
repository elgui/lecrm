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
alias gb-tenant='ssh dokku@54.37.157.49 run lecrm-admin /app/lecrm-admin tenant'
```

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

## Operations notes (for Guillaume)

### Production Dokku deployment (AC-D1..D3, AC-D6)

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

The image refuses to start if any `LECRM_API_*` env var is present
(AC-D5 — Winston's R2 condition for same-image binary co-location).

### What's NOT in Phase 1

Deferred to later sprints / stories:

- **RBAC role seeding** — stdout prints `[PROVISION] RBAC seeding: skipped
  (not implemented in v0)` on every provision. Flips to `ok (3 roles
  applied)` when the RBAC sibling story lands.
- **OAuth client registration** — sibling Sprint 8 story.
- **Multi-template registry** — Sprint 9 / ADR-010 (Phase 2 of integrator
  handoff). For now the only template is `gbconsult-default`.
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
- ADR-010 — metadata-engine destination for `gbconsult-default`
