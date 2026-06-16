# secrets/oauth/ — per-user OAuth grant manifests

SOPS-encrypted OAuth refresh-token manifests, one file per granting user.
Distinct from `secrets/clients/` (per-tenant runtime secrets) and
`secrets/operator/` (Tier-0 platform secrets) because an OAuth grant is
scoped to a single **(workspace, user)** pair — the rep who authorised
leCRM to watch their mailbox.

Introduced for Gmail reply detection (ADR-004 rev2 §4, tasket
`20260614-154815-5078`). Gmail is the only provider at v1 (ADR-009 §9);
`gmail/` is therefore the only subtree today. Microsoft Graph / IMAP
grants, if ever added, get sibling subtrees under `secrets/oauth/`.

## Layout

```
secrets/oauth/
└── gmail/
    ├── _template/
    │   └── secrets.yaml.template        # field schema, plaintext, committed
    └── <workspace_id>/
        └── <user_id>.enc.yaml           # per-user grant, sops-encrypted, committed
```

Only `*.enc.yaml` and the `_template/*.template` are committed. The
plaintext intermediate (`<user_id>.yaml`, between template-copy and
sops-encrypt) is gitignored — see the `secrets/oauth/` block in the
repository `.gitignore`. ADR-004 rev2 §4 writes the path as
`<user_id>.yaml`; that shorthand maps to the committed `.enc.yaml` form
under the secrets baseline (ADR-007 §2).

## Setup

End-to-end procedure (one-time GCP project setup + per-user OAuth grant +
Pub/Sub topic & push subscription): **`ops/runbooks/gmail-oauth-pubsub-setup.md`**.

## See also

- `ops/runbooks/gmail-oauth-pubsub-setup.md` — full setup runbook.
- `ops/secrets/.sops.yaml` — encryption policy (creation rule for this subtree).
- `ops/runbooks/secret-rotation.md` — rotation cadences and procedures.
- `secrets/README.md` — the parent secrets layout and ADR-007 §2 rationale.
- `docs/adr/ADR-004-rev2-sequences-architecture.md` §4 (Gmail reply detection).
- `docs/adr/ADR-007-encryption-secrets-audit.md` §2 (sops + age baseline).
