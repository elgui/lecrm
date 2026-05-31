# scripts/

Dev / CI helper scripts.

| Script | Purpose |
|---|---|
| `smoke-test-provision.sh` | Acceptance gate for `packages/db/migrations/`. Boots an isolated Postgres cluster on a non-privileged port, applies all migrations in order, exercises `core.lecrm_provision_workspace(uuid)` against ADR-009 §2.1's idempotency + lateral-expansion-mitigation assertions, then exercises `core.users` (issuer, sub) uniqueness from 0002. Default cluster is PG15 from `/usr/lib/postgresql/15/bin`; override `PGBIN` to point elsewhere. Production target per ADR-009 §2 is PG17; the function body uses only portable features that work on PG13+. |
| `authentik-provision-oidc-client.py` | Provisions the `lecrm` OIDC client (provider + application + redirect URI regex + admin API token) in a freshly-booted Authentik. Idempotent; reading the existing provider if one is already there. Invoked as `docker exec lecrm-authentik-worker ak shell -c "exec(open('/path/to/script').read())"`. The script prints `CLIENT_ID`, `CLIENT_SECRET`, and an admin `API_TOKEN_KEY` on stdout for the caller to capture into `deploy/.env.dev`. |
| `authentik-provision-test-user.py` | Provisions the `guillaume-e2e` internal user (idempotent) for the OIDC e2e test. Same `ak shell` invocation pattern. |
| `authentik-brand-lecrm.py` | Brands the Authentik login screen for leCRM: rewrites every Brand's `branding_title`/`branding_logo`/`branding_favicon`/`branding_default_flow_background` and the `default-authentication-flow` title, removing the stock orange "authentik" wordmark + "Welcome to authentik!". Idempotent, version-defensive (only sets fields the running Authentik has), and self-contained — assets are inline SVGs base64-encoded into `data:` URIs at runtime, so there is no media file to copy and nothing to lose on the Hetzner migration. Same `ak shell` invocation pattern. |
