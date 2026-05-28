# Backup bootstrap — one-time WAL-G + OVH Object Storage setup per client

Run this when provisioning a new client VPS (Phase 1). It is the
companion to [restore.md](restore.md): without these steps the restore
runbook has nothing to read.

References:
- [ADR-006](../../docs/adr/ADR-006-backup-dr.md) §1–2.
- [STRATEGIC-OVERVIEW.md](../../docs/STRATEGIC-OVERVIEW.md) §2 (OVH-first).

---

## 1. Create the OVH Object Storage bucket (one-time, platform-wide)

A single bucket `lecrm-wal` carries every client's archive under their
own prefix `<client-slug>/`. Per-client IAM credentials are scoped to
that prefix only. This is cheaper and operationally simpler than
one-bucket-per-client without weakening isolation (the prefix
restriction is enforced server-side by OVH IAM).

```bash
# Via OVH CLI (ovhai / s3cmd / aws cli — pick one). aws-cli example:
export OVH_REGION=gra                                  # Gravelines, FR
export AWS_ENDPOINT=https://s3.${OVH_REGION}.io.cloud.ovh.net
export AWS_ACCESS_KEY_ID=<ovh-admin-key>
export AWS_SECRET_ACCESS_KEY=<ovh-admin-secret>

aws --endpoint-url "${AWS_ENDPOINT}" --region "${OVH_REGION}" \
    s3api create-bucket --bucket lecrm-wal

# Verify.
aws --endpoint-url "${AWS_ENDPOINT}" --region "${OVH_REGION}" \
    s3 ls s3://lecrm-wal/
```

OVH-specific quirks (caught during scaffolding):

- **Path-style addressing only** for High-Perf S3 at the GRA/SBG
  endpoints — wal-g defaults to virtual-host style. Set
  `AWS_S3_FORCE_PATH_STYLE=true` in `walg.env`.
- **Region label must be lowercase** (`gra`, not `GRA`). Sig v4 fails
  on case mismatch.
- **Endpoint hostnames differ** between High-Perf and Cloud Archive
  tiers. We want High-Perf (`s3.<region>.io.cloud.ovh.net`); Cloud
  Archive is glacier-class and unsuitable for WAL.

---

## 2. Provision per-client IAM credentials (one-time per client)

```bash
CLIENT_SLUG=acme-corp

# Create user.
aws --endpoint-url "${AWS_ENDPOINT}" --region "${OVH_REGION}" \
    iam create-user --user-name "lecrm-walg-${CLIENT_SLUG}"

# Attach a prefix-restricted policy: read+write to
# s3://lecrm-wal/<client-slug>/*, nothing else.
cat > /tmp/policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ListPrefix",
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": ["arn:aws:s3:::lecrm-wal"],
      "Condition": {
        "StringLike": {"s3:prefix": ["${CLIENT_SLUG}/*"]}
      }
    },
    {
      "Sid": "RWPrefixObjects",
      "Effect": "Allow",
      "Action": ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"],
      "Resource": ["arn:aws:s3:::lecrm-wal/${CLIENT_SLUG}/*"]
    }
  ]
}
EOF

aws --endpoint-url "${AWS_ENDPOINT}" --region "${OVH_REGION}" \
    iam put-user-policy --user-name "lecrm-walg-${CLIENT_SLUG}" \
      --policy-name walg-prefix --policy-document file:///tmp/policy.json

# Issue an access key — capture both halves; the secret is shown ONCE.
aws --endpoint-url "${AWS_ENDPOINT}" --region "${OVH_REGION}" \
    iam create-access-key --user-name "lecrm-walg-${CLIENT_SLUG}"
```

The returned access key and secret go into the client's SOPS-encrypted
`secrets/clients/<slug>/secrets.enc.yaml` under
`walg.access_key_id` / `walg.secret_access_key`, mirroring the rotation
cadence in `ops/runbooks/secret-rotation.md`.

---

## 3. Populate `deploy/postgres/walg.env` on the client VPS

```bash
# On the VPS (or locally before pushing).
cp deploy/postgres/walg.env.example deploy/postgres/walg.env
# Edit:
#   WALG_S3_PREFIX=s3://lecrm-wal/${CLIENT_SLUG}
#   AWS_ACCESS_KEY_ID=...   (from step 2)
#   AWS_SECRET_ACCESS_KEY=...
#   AWS_ENDPOINT=https://s3.${OVH_REGION}.io.cloud.ovh.net
#   AWS_REGION=${OVH_REGION}

# Encrypt with SOPS so it can be committed.
sops -e -i deploy/postgres/walg.env
# Decryption happens on the VPS via the operator's age key, mounted via
# the existing ops/secrets bootstrap.
```

---

## 4. Confirm WAL-G S3 compatibility against OVH (smoke test)

Run once on the live VPS — confirms the credentials, endpoint, and
signing path before letting Postgres start archiving for real.

```bash
docker compose -f deploy/compose/postgres.yml run --rm postgres bash -c '
  set -eu
  echo "smoke" > /tmp/smoke
  /usr/local/bin/wal-g wal-push /tmp/smoke || true
  /usr/local/bin/wal-g backup-list
'
# Expected: backup-list returns "no backups found" but the request is
# SIGNED CORRECTLY (no SignatureDoesNotMatch). The wal-push attempt is
# allowed to fail with "not a valid WAL file" — we only care that the
# transport handshake works.
```

If you see `SignatureDoesNotMatch`, re-check:
- `AWS_REGION` exactly matches OVH's region code (lowercase).
- `AWS_S3_FORCE_PATH_STYLE=true` is set.
- `AWS_ENDPOINT` points at the High-Perf endpoint, not Cloud Archive.

---

## 5. Trigger the first base backup

```bash
docker exec lecrm-walg-backup su postgres -c \
  /usr/local/bin/lecrm/backup-push.sh
```

After this completes, `wal-g backup-list` should show one entry. From
this moment forward the client's RPO is ≤60 s and `restore.md` §1 is
recoverable.

---

## 6. Hand-off to operator

Tick off in the client onboarding checklist:

- [ ] Bucket `lecrm-wal/${CLIENT_SLUG}/` shows base backup + WAL
      segments uploaded.
- [ ] `wal-g wal-verify timeline integrity` exits 0.
- [ ] First quarterly drill scheduled in the operator calendar.
- [ ] Client onboarding doc references this restore runbook.
