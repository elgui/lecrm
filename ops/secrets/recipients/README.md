# ops/secrets/recipients/

Holds the **public** age recipient files for SOPS encryption. Public-key
material only — never private keys. Anything that decrypts data lives on
Guillaume's YubiKey (primary) or in the Bitwarden vault (backup), per
ADR-007 §2.

## Files

- `guillaume.age.pub` — Guillaume's age public key. Created by
  `ops/secrets/bootstrap.sh` from the operator workstation. Committed
  to git as a public artefact (it's a public key; safe).
- `ci-deploy.age.pub` — *(planned, v0.5)* CI deploy public key. Lets
  GitHub Actions decrypt at release time without operator
  interaction. Tracked as TO RESOLVE-1 in
  `ops/runbooks/secret-rotation.md`.

## Adding a new recipient

1. Generate the new keypair on the machine that will hold it.
2. Copy the **public** half (`age1…`) here as `<owner>.age.pub`.
3. Append the public-key line to the `age:` field of every relevant
   block in `ops/secrets/.sops.yaml` (multiple recipients are
   comma-separated).
4. Re-encrypt every existing manifest so the new recipient can read
   them: `sops updatekeys secrets/clients/<slug>/secrets.enc.yaml`
   (repeat for each file). The runbook walks through this.
5. Commit `.sops.yaml`, the public-key file, and the re-keyed
   manifests in one PR.
