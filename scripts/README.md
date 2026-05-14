# scripts/

Dev / CI helper scripts.

| Script | Purpose |
|---|---|
| `smoke-test-provision.sh` | Acceptance gate for `packages/db/migrations/0001_init.sql`. Boots an isolated Postgres cluster on a non-privileged port, applies the migration, exercises `core.lecrm_provision_workspace(uuid)` against ADR-009 §2.1's idempotency + lateral-expansion-mitigation assertions, and tears down the cluster. Default cluster is PG15 from `/usr/lib/postgresql/15/bin`; override `PGBIN` to point elsewhere. Production target per ADR-009 §2 is PG17; the function body uses only portable features that work on PG13+. |
