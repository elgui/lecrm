# leCRM

A managed CRM-as-a-service for French and EU SMBs, operated by **GB Consult SARL**.

This repository is the public source of leCRM as required by the GNU Affero General Public License v3.0 §13. It contains GB Consult's modifications on top of [Twenty CRM](https://github.com/twentyhq/twenty), the upstream project.

## Status

**Pre-launch.** Repository initialised on 2026-05-10; upstream Twenty source vendored at tag `v2.2.0` on 2026-05-10. The architecture and implementation plan are documented in a separate (private) project. The first paying client is expected in 2026 Q3.

## License & Attribution

leCRM is a modified fork of [Twenty CRM](https://github.com/twentyhq/twenty), licensed under the GNU Affero General Public License v3.0 (AGPL-3.0). Source code for all modifications is available at this repository.

- See [`LICENSE`](./LICENSE) for the full AGPL-3.0 license text.
- See [`NOTICE`](./NOTICE) for upstream attributions and trademark notice.
- See [`CHANGES.md`](./CHANGES.md) for the running list of modifications relative to upstream.

## Repository layout

```
.
├── LICENSE                AGPL-3.0 (with @license Enterprise carve-outs from upstream)
├── NOTICE                 Upstream attribution and trademark notice
├── CHANGES.md             Running list of modifications vs. upstream
├── README.md              This file
├── UPSTREAM-README.md     Twenty's README, preserved verbatim
├── ops/                   Per-client provisioning template + scripts (leCRM addition)
├── packages/              Twenty's monorepo (vendored from upstream)
│   └── twenty-server/
│       └── src/
│           └── engine/
│               └── gbconsult/   GB Consult's patch directory
│                                (NestJS DI overrides — OIDC, enterprise gate)
└── (rest of upstream tree, vendored from twentyhq/twenty v2.2.0)
```

All leCRM customisations live under `gbconsult/` (the patch directory) plus `ops/` (provisioning templates). Exactly **one** file in the upstream tree is touched: `packages/twenty-server/src/app.module.ts`, which imports the override module last so it shadows core providers. See `docs/adr/ADR-002` (in the private architecture project) for the rationale.

## Source-build correspondence (AGPL §13)

When leCRM is operated as SaaS, every page served over the network displays a footer link to the **exact commit** running in production. Find the running commit by inspecting the page footer or by querying `https://<your-tenant>.lecrm.fr/api/version` (returns the upstream Twenty version and the leCRM patch version, e.g. `twenty-2.2.0+lecrm.4`).

Each tagged release in this repository corresponds to a deployment. To inspect or rebuild the exact code running for any deployment, check out the matching tag.

## Versioning

Releases follow [Forgejo-style](https://forgejo.org/docs/next/user/versions/) versioning that pins both the upstream Twenty version and our patch increment:

```
twenty-<UPSTREAM>+lecrm.<PATCH>
```

Example: `twenty-2.2.0+lecrm.4` is the 4th leCRM revision atop Twenty 2.2.0.

## Contributing

This repository exists to satisfy AGPL-3.0 §13 source-availability obligations. We are not currently accepting external contributions to leCRM-specific code. Bug reports and security findings are welcome via GitHub Issues. Improvements relevant to upstream Twenty should be opened directly against [twentyhq/twenty](https://github.com/twentyhq/twenty).

## Security

Report security vulnerabilities privately to **security@gbconsult.me** rather than via public Issues. Please allow up to 30 days for triage before public disclosure.

## Contact

- Operator: GB Consult SARL (Paris, France)
- Web: <https://gbconsult.me>
- Email: <hello@gbconsult.me>

---

*"Powered by Twenty CRM (AGPL-3.0) — source: github.com/elgui/lecrm"*
