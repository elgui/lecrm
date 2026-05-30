# deploy/postgres/gpg/ — WAL-G client-side encryption key material

WAL-G GPG-encrypts every WAL segment and every base-backup chunk before
upload to OVH Object Storage. This directory carries the **public** key
that gets baked into the postgres image; the **private** key never lives
in the repo (YubiKey primary, Bitwarden backup, per ADR-006 §2).

## Files

- `lecrm-backup.pub.asc` — armored public key for
  `lecrm-backup@gbconsult.me`. Committed.
- `lecrm-backup.fingerprint` — SHA-256 fingerprint of the keypair (text,
  for grep-in-CI verification). Committed.

## Bootstrap (one-time, by Guillaume on a clean laptop)

```bash
# 1. Generate the keypair on an air-gapped machine (or YubiKey directly).
gpg --quick-generate-key 'lecrm-backup <lecrm-backup@gbconsult.me>' rsa4096 default 5y

# 2. Export the public key (commit this file).
gpg --armor --export lecrm-backup@gbconsult.me > deploy/postgres/gpg/lecrm-backup.pub.asc

# 3. Capture the fingerprint.
gpg --with-fingerprint --with-colons --keyid-format LONG \
    --list-keys lecrm-backup@gbconsult.me \
  | awk -F: '/^fpr:/ { print $10; exit }' \
  > deploy/postgres/gpg/lecrm-backup.fingerprint

# 4. Transfer the private key to YubiKey (primary).
gpg --edit-key lecrm-backup@gbconsult.me
#   gpg> keytocard   # move the encryption subkey
#   gpg> save

# 5. Export an encrypted private-key backup to Bitwarden (secondary).
gpg --export-secret-keys --armor lecrm-backup@gbconsult.me \
  | gpg --symmetric --cipher-algo AES256 \
  > /tmp/lecrm-backup.priv.gpg
# Upload /tmp/lecrm-backup.priv.gpg to the Bitwarden vault entry
# "leCRM — backup GPG private key (encrypted)". The Bitwarden master
# password is the only credential needed to recover; the symmetric
# passphrase is stored in the same Bitwarden entry's note field but
# encrypted separately (defence-in-depth, never both in clear).

# 6. Verify the air-gapped private key was destroyed:
shred -u /tmp/lecrm-backup.priv.gpg
gpg --delete-secret-keys lecrm-backup@gbconsult.me   # on the air-gapped machine
```

## Verifying the baked-in key matches the canonical fingerprint

```bash
docker run --rm lecrm/postgres:v0 \
  gpg --with-fingerprint --with-colons \
      /etc/postgres/gpg/lecrm-backup.pub.asc \
  | awk -F: '/^fpr:/ { print $10; exit }'
# Compare against deploy/postgres/gpg/lecrm-backup.fingerprint.
```

## Recovery (loss of YubiKey)

Documented in `ops/runbooks/restore.md` §5. Summary: pull the encrypted
private-key blob from Bitwarden, decrypt with the symmetric passphrase
(also in Bitwarden under the same entry), import to a fresh YubiKey or
a dedicated restore host. Annual recovery drill verifies this path.

## Status — staging key generated 2026-05-30

The placeholder has been **replaced with a real rsa4096 public key**
(fingerprint in `lecrm-backup.fingerprint`, encryption subkey present),
generated to unblock staging WAL-G backups on Cloudflare R2.

⚠️ **Staging-grade custody, not the canonical flow above.** This key was
generated **on the staging host** (no air-gapped laptop / YubiKey
available), then the private key was AES256-symmetric-wrapped and stored
in Bitwarden ("leCRM — backup GPG private key (encrypted)"); the on-host
private material was shredded. The YubiKey-primary path (steps 4–6) was
**not** performed. Before signing a real client, regenerate per the
canonical air-gapped + YubiKey bootstrap and rotate this staging key out.

(Originally this repo shipped a placeholder `lecrm-backup.pub.asc` so the
Dockerfile `COPY` succeeds for CI builds — it decoded to literal
`PLACEHOLDER` and was rejected at runtime by `wal-g` encryption.)
