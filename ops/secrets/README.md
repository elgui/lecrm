# ops/secrets/ — SOPS + age bootstrap

Per ADR-007 §2: sops + age is leCRM's v0 secret-management baseline.
Vault is the v1+ target; this directory carries the v0 policy, the age
recipient artefacts, and the bootstrap procedure.

## Files in this directory

- `.sops.yaml` — SOPS creation rules. Governs which paths get encrypted
  with which age recipient(s). Committed.
- `bootstrap.sh` — one-shot script that generates Guillaume's age
  keypair, writes the public-key recipient into `.sops.yaml` and into
  `recipients/guillaume.age.pub`, and prints the YubiKey + Bitwarden
  custody steps. Committed.
- `recipients/` — committed **public** age keys. See its README.

## Prerequisites

- `age` v1.1+ (`sudo apt install age` on Debian, `brew install age` on
  macOS, or fetch a release binary from
  <https://github.com/FiloSottile/age/releases>).
- `sops` v3.9+ (`brew install sops`, `apt install sops`, or
  <https://github.com/getsops/sops/releases>).
- A YubiKey for primary private-key custody.
- A Bitwarden vault (Guillaume's existing personal account is fine) for
  the backup copy.

## Bootstrap (first-time, once per operator)

```bash
cd $(git rev-parse --show-toplevel)
ops/secrets/bootstrap.sh
```

The script will:

1. Refuse to run if `~/.config/sops/age/keys.txt` already contains a
   leCRM-tagged key (idempotent guard).
2. Run `age-keygen` and write the private key to
   `~/.config/sops/age/keys.txt` with mode 0600.
3. Extract the matching public key and write it to
   `ops/secrets/recipients/guillaume.age.pub`.
4. Replace every `REPLACE_WITH_AGE_PUBLIC_KEY` token in
   `ops/secrets/.sops.yaml` with that public key.
5. Print the YubiKey + Bitwarden custody checklist (manual steps; the
   script cannot drive your YubiKey or Bitwarden GUI).

After the script finishes, commit the resulting `.sops.yaml` diff and
the new `recipients/guillaume.age.pub` file.

## Per-secret workflow (after bootstrap)

```bash
# New per-tenant manifest
cp secrets/clients/_template/secrets.yaml.template \
   secrets/clients/acme-corp/secrets.yaml
$EDITOR secrets/clients/acme-corp/secrets.yaml         # fill values
sops --config ops/secrets/.sops.yaml \
     --encrypt --in-place secrets/clients/acme-corp/secrets.yaml
mv secrets/clients/acme-corp/secrets.yaml \
   secrets/clients/acme-corp/secrets.enc.yaml
git add secrets/clients/acme-corp/secrets.enc.yaml

# Edit an existing encrypted manifest in place
sops --config ops/secrets/.sops.yaml \
     secrets/clients/acme-corp/secrets.enc.yaml
```

## Deploying secrets to a tenant VPS

See `ops/provision-client.sh`. The flow is:

1. Decrypt `secrets/clients/<slug>/secrets.enc.yaml` to a temporary
   plaintext YAML on the operator workstation (sops uses the local
   age private key — YubiKey-touch required if your key file points
   at the YubiKey identity).
2. Render the YAML to a `KEY=value`-per-line `deploy/.env.<slug>` file.
3. `scp` the `.env.<slug>` to the deploy host, mode 0600, owner root.
4. `docker compose --env-file deploy/.env.<slug> up -d`.

## Custody (ADR-007 §2)

- **Primary copy of the private age key — YubiKey.** Either via
  `age-plugin-yubikey` (preferred; the key never leaves the YubiKey),
  or as an OpenPGP-on-YubiKey AES-encrypted backup of `keys.txt`.
  Document the YubiKey serial + slot in your personal records.
- **Backup copy — Bitwarden.** Paste the entire content of
  `~/.config/sops/age/keys.txt` into a Bitwarden secure note tagged
  `leCRM / age / 2026`. Encrypted at rest by Bitwarden's vault key.
- **Recovery drill (ADR-007 TO RESOLVE-4) — annual.** Simulate YubiKey
  loss: on a clean machine, restore the key from Bitwarden and
  successfully decrypt a current `secrets.enc.yaml`. Record the date
  in `ops/runbooks/secret-rotation.md`.
