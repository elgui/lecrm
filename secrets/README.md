# secrets/ — SOPS-encrypted secret manifests

Per ADR-007 §2 (v0 secrets baseline: sops + age). The encryption policy
lives at `ops/secrets/.sops.yaml`; the rotation runbook lives at
`ops/runbooks/secret-rotation.md`; the bootstrap procedure lives at
`ops/secrets/README.md`.

## Layout

```
secrets/
├── clients/
│   ├── _template/
│   │   └── secrets.yaml.template      # field schema, plaintext, committed
│   └── <slug>/
│       └── secrets.enc.yaml           # per-tenant, sops-encrypted, committed
└── operator/
    ├── secrets.yaml.template          # operator field schema, plaintext, committed
    └── secrets.enc.yaml               # platform Tier-0 secrets, sops-encrypted, committed
```

Only `*.enc.yaml` and `*.template` files are committed. The plaintext
intermediate forms (`secrets.yaml` without the `.enc`) are gitignored —
see the secrets-section of the repository `.gitignore`.

## ADR-007 §2 vs this repo

ADR-007 §2 notes that the secrets repo *may* be a separate private
GitHub repo. For v0 we keep encrypted manifests in the main `leCRM`
repo because (a) Guillaume is the sole operator and the access list is
identical to the source-tree access list, and (b) co-located encrypted
secrets reduce the cognitive load of "which repo holds what." If the
operator pool grows or a security review demands separation, the move
is a single `git filter-repo` away — paths are unchanged.

## See also

- `ops/secrets/README.md` — bootstrap procedure (age key, YubiKey, Bitwarden).
- `ops/secrets/.sops.yaml` — encryption policy.
- `ops/provision-client.sh` — render encrypted manifest → Compose `.env`.
- `ops/runbooks/secret-rotation.md` — rotation cadences and procedures.
- `docs/adr/ADR-007-encryption-secrets-audit.md` §2.
- `docs/adr/ADR-009-stack-and-license.md` §2.1, §4.1, §7.1, §9.
