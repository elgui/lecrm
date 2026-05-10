# leCRM Disaster Recovery & Security Architecture

**Date:** 2026-05-10  
**Scope:** leCRM — managed CRM-as-a-service for French/EU SMBs, Twenty CRM AGPL fork, NestJS + PostgreSQL + React, hosted on Hetzner DE / OVH FR.  
**SLA targets:** RPO ≤1 h (v0) / ≤15 min (production); RTO ≤4 h (v0) / ≤1 h (production).  
**Constraints:** EU data residency, GDPR Art. 32 + Art. 17, solo operator, per-client restore granularity.

---

## 1. PostgreSQL Backup Strategy

### Tool Comparison

| Tool | RPO achievable | Operational weight | Status (May 2026) |
|------|---------------|--------------------|--------------------|
| `pg_dump` daily | ≤24 h | Minimal (cron + rclone) | Stable, logical only |
| `pg_dump` daily + manual WAL copy | ≤1 h | Low | Works but brittle |
| WAL-G (continuous WAL + base backup) | ≤60 s | Low–Medium | Active, Go, recommended |
| pgBackRest (full/diff/incr + archive) | ≤60 s | Medium | **Archived Apr 2026** — funding lost when Crunchy Data was acquired by Snowflake; Percona advises "keep using it" but no new features/bug fixes expected; a coalition-funding model is being explored [Percona, Apr 2026](https://percona.community/blog/2026/04/28/pgbackrest-is-archived-what-now/) |
| Hetzner VM snapshots | ≤1 h (snapshot cadence) | Minimal | Coarse-grained, OS-level |

**Verdict for leCRM:** Use **WAL-G** for continuous WAL archiving to an S3-compatible bucket. WAL-G is a single Go binary, configuration via environment variables, integrates directly with PostgreSQL's `archive_command`, and supports S3-compatible endpoints out of the box — including Hetzner Object Storage and Scaleway. It is the least operationally complex path to RPO ≤15 min.

Hetzner snapshots serve as a coarse safety net (daily or on-demand before major changes) but must not be the primary DR mechanism — they do not provide PITR granularity.

pgBackRest remains usable if already deployed, but new deployments should default to WAL-G given its uncertain governance future.

### WAL-G key configuration (PostgreSQL)

```ini
# postgresql.conf
wal_level = replica
archive_mode = on
archive_command = 'wal-g wal-push %p'
archive_timeout = 60        # Force WAL rotation every 60 s → RPO ceiling ~60 s
restore_command = 'wal-g wal-fetch %f %p'
```

```bash
# Environment (inject via systemd unit or secrets manager)
WALG_S3_PREFIX=s3://lecrm-wal/clientname
AWS_ENDPOINT_URL=https://nbg1.your-objectstorage.com   # Hetzner NBG1
AWS_ACCESS_KEY_ID=<key>
AWS_SECRET_ACCESS_KEY=<secret>
AWS_REGION=eu-central-1
WALG_COMPRESSION_METHOD=brotli
WALG_UPLOAD_CONCURRENCY=4
```

Sources: [WAL-G PostgreSQL docs](https://wal-g.readthedocs.io/PostgreSQL/) · [WAL-G Storages](https://wal-g.readthedocs.io/STORAGES/)

---

## 2. Off-Site Backup Destination

### Pricing Comparison (EU, May 2026)

| Provider | Region | ~100 GB/mo | ~1 TB/mo | Encryption at rest (default) | Customer-managed keys |
|----------|--------|------------|----------|-----------------------------|-----------------------|
| **Hetzner Object Storage** | DE (NBG, FSN) | ~€0.50 (within €4.99 base incl. 1 TB) | ~€4.99 flat | **No** — SSE-C available (you supply key per-upload) | Yes, SSE-C |
| **Scaleway Object Storage** | FR (PAR) | ~€1.10 | ~€11–14 | Yes (server-managed default) | Glacier tier available; KMS add-on |
| **OVH Object Storage** | FR (GRA, SBG) | ~€0.70–1.20 | ~€7–12 | Yes (server-managed default) | Depends on tier |

Sources: [Hetzner Object Storage](https://www.hetzner.com/storage/object-storage/) · [DanubeData EU S3 comparison 2025](https://danubedata.ro/blog/best-s3-compatible-object-storage-europe-2025) · [DanubeData Wasabi alternatives 2026](https://danubedata.ro/blog/wasabi-alternatives-europe-pricing-2026)

**Recommendation:** Hetzner Object Storage is the best price/TB for leCRM's scale. **Critical caveat: Hetzner does NOT encrypt objects at rest by default.** You must either (a) use SSE-C (pass your key on every PUT/GET — awkward for WAL-G), or (b) **encrypt before upload** (preferred: WAL-G's `WALG_COMPRESSION_METHOD` + client-side encryption pipeline, or a dedicated tool like `age`). Client-side encryption before upload is the correct architecture for GDPR Art. 32 compliance and key sovereignty. See Section 5.

For teams wanting a zero-touch server-side encryption default, Scaleway or OVH are reasonable second choices at ~2–3× the cost, though you lose key sovereignty unless you layer client-side encryption on top anyway.

---

## 3. Per-Client Restore Granularity

### v0: VPS-per-client (trivial)

Each client has its own VPS and PostgreSQL cluster. Their WAL archive lives in a dedicated S3 prefix (`s3://lecrm-wal/<client-slug>/`). Restore procedure:

1. Provision a fresh VPS (or stop the existing one).
2. `wal-g backup-fetch /var/lib/postgresql/data LATEST`
3. Set `recovery_target_time = '2026-05-06 15:00:00 UTC'` in `postgresql.conf`.
4. Start PostgreSQL — it replays WAL until target time, then stops.
5. Verify data, then `SELECT pg_wal_replay_resume()` to go live.

Total operation: 30–60 minutes for a typical SMB DB (<20 GB). Well within RTO ≤1 h.

### v1+: Shared PostgreSQL with logical multi-tenancy (hard)

All clients share one PostgreSQL cluster, isolated by a `workspace_id` foreign key (Twenty's model). Surgical restore for one tenant requires:

1. Spin up a **temporary recovery instance** (do NOT restore on production).
2. Restore the shared DB to the target point-in-time on the recovery instance.
3. Run an extraction query for the affected tenant:
   ```sql
   -- Export one tenant's data as INSERT statements
   COPY (
     SELECT * FROM crm_contacts WHERE workspace_id = 'client-uuid'
   ) TO '/tmp/client-contacts-recovery.csv' CSV HEADER;
   ```
4. Identify all tables with `workspace_id`, script the extraction (a one-time migration helper script is worth building in advance).
5. Import the recovered rows into the production DB via `UPSERT` or staging table, conflict-resolving manually.
6. Drop the recovery instance.

**This is inherently slow and error-prone.** Estimate 2–4 hours of skilled DBA time per incident. For v1+, invest in: (a) per-workspace logical backups as a supplement (a daily `pg_dump --schema=... -t workspace_data` export per tenant, stored separately), or (b) row-level versioning / soft-delete + event sourcing so "restore" becomes "replay events to T-minus."

---

## 4. RPO/RTO Mechanics

### Hitting RPO ≤15 min

The mechanism is `archive_timeout = 60` in PostgreSQL + WAL-G `wal-push`. PostgreSQL rotates the current WAL segment every 60 seconds at most; WAL-G archives it immediately. Combined latency (segment rotation + upload) is typically under 90 seconds on a low-traffic SMB instance. WAL segments are 16 MB uncompressed but compress ~70–80% with brotli for typical CRM data.

The practical RPO for a leCRM client with moderate traffic: **1–3 minutes** with this setup.

Note: `archive_timeout` is the correct knob for archive-based RPO. If you need **sub-minute RPO** (not required per SLA), switch to streaming replication to a hot standby — that gives second-scale RPO without archive latency. For v0 (single VPS), streaming replication requires a second VPS; see Section 6.

### Hitting RTO ≤1 h

Two paths:

| Approach | RTO | Cost | Complexity |
|----------|-----|------|------------|
| **Cold restore from WAL archive** | 30–90 min (depends on DB size + WAL replay) | Storage only | Low |
| **Hot standby + streaming replication** | 5–15 min (promote standby) | 2× VPS cost | Medium |

For v0, cold restore is acceptable and hits ≤1 h for databases under ~50 GB with a modern VPS. For production (v1+), hot standby is required to reliably hit ≤1 h.

---

## 5. Restore Drill Runbook (Quarterly)

Run this drill against a **test client** (dummy data) every quarter. Estimated time: 90 minutes.

```bash
#!/usr/bin/env bash
# Quarterly DR Drill — leCRM
# Run on: staging VPS (never production)
# Estimated duration: 90 min
# Pre-requisites: WAL-G installed, env vars set, postgres stopped

set -euo pipefail
RESTORE_TARGET="2026-04-01 14:00:00 UTC"   # Last known-good point
PGDATA=/var/lib/postgresql/16/main
BACKUP_PREFIX=s3://lecrm-wal/test-client

echo "=== STEP 1: Stop PostgreSQL ==="
systemctl stop postgresql

echo "=== STEP 2: Clear data directory ==="
mv $PGDATA ${PGDATA}.bak-$(date +%s)
mkdir -p $PGDATA
chown postgres:postgres $PGDATA
chmod 700 $PGDATA

echo "=== STEP 3: Fetch base backup ==="
sudo -u postgres wal-g backup-fetch $PGDATA LATEST

echo "=== STEP 4: Configure recovery ==="
cat > /etc/postgresql/16/main/postgresql.conf.d/recovery.conf <<EOF
restore_command = 'wal-g wal-fetch %f %p'
recovery_target_time = '${RESTORE_TARGET}'
recovery_target_action = 'promote'
EOF

echo "=== STEP 5: Create recovery signal ==="
touch $PGDATA/recovery.signal
chown postgres:postgres $PGDATA/recovery.signal

echo "=== STEP 6: Start PostgreSQL (WAL replay begins) ==="
systemctl start postgresql

echo "=== STEP 7: Monitor recovery ==="
# Watch until recovery completes
until sudo -u postgres psql -c "SELECT pg_is_in_recovery();" | grep -q 'f'; do
  echo "Still replaying WAL... $(sudo -u postgres psql -tc 'SELECT now()')"
  sleep 10
done

echo "=== STEP 8: Verify data integrity ==="
sudo -u postgres psql -d lecrm -c "SELECT COUNT(*) FROM crm_contacts;"
sudo -u postgres psql -d lecrm -c "SELECT MAX(created_at) FROM crm_contacts;"

echo "=== STEP 9: Document results ==="
echo "Drill completed at $(date). Verify row counts match pre-drill baseline."
echo "Record: backup-fetch duration, WAL replay duration, total elapsed time."
```

**Post-drill checklist:**
- [ ] Total elapsed time documented (target ≤1 h)
- [ ] Row counts match expected baseline
- [ ] WAL-G logs show no errors
- [ ] Backup retention policy verified (`wal-g delete retain FULL 7`)
- [ ] Test client data verified at recovery target time
- [ ] Drill documented in incident log with timestamp

---

## 6. Failover Phases

### v0: Single VPS (acceptable for ≥99.5% SLA over a quarter)

99.5% = 10.9 hours downtime/quarter budget. A cold-restore RTO of 1 h leaves ample headroom for rare hardware failures. **No standby required.**

Provider-managed mitigation: Hetzner Cloud offers auto-restart of VMs on hardware failure. Enable it:
```bash
hcloud server update <server-id> --auto-rescue-enabled
```
Hetzner Floating IPs can be reassigned within seconds — useful if you provision a replacement VPS: point the Floating IP at the new VM after restore completes, avoiding DNS TTL delays.

Hetzner Volume (persistent block storage) is worth evaluating: the PostgreSQL data volume can be detached from a failed VPS and re-attached to a replacement in ~2 minutes, potentially cutting RTO to 20–30 minutes (volume attach + postgres start, no WAL replay needed).

### v1+: Hot Standby on Second VPS

```
Primary VPS (NBG1) ──streaming replication──► Standby VPS (HEL1 or FSN1)
        │                                              │
        └── WAL archive → Hetzner Object Storage ─────┘
                           (safety net)
```

- Streaming replication: `primary_conninfo` in standby's `postgresql.conf`, `wal_keep_size = 1GB`.
- Failover promotion: `pg_ctl promote` or Patroni for automated failover.
- **Patroni** (etcd-backed) is the standard solo-operator HA stack. It manages automatic leader election, standby promotion, and fencing without a full Kubernetes dependency. Runs on 2 VPS nodes + 1 tiny etcd node (or use Hetzner's etcd-as-a-service equivalent).
- RTO with Patroni auto-failover: 30–60 seconds.

### Provider-Failure Scenarios

| Scenario | Immediate action | Customer comms |
|----------|-----------------|----------------|
| **Hetzner outage (full region)** | Activate cross-region WAL restore to OVH FR VPS (pre-provisioned or on-demand). ETA: 45–90 min. | "We are aware of a hosting provider incident. Your data is safe. We are restoring service to an alternate data center. ETA: [X]. We will update every 30 minutes." |
| **Hetzner Object Storage outage** | WAL archiving pauses (PostgreSQL queues WAL files locally, bounded by `wal_keep_size`). Production continues. Restore is blocked until storage recovers. | Not customer-impacting unless DB also fails during outage. |
| **Brevo (email) outage** | CRM email-sending features degrade gracefully. No data loss risk. Queue emails and retry when Brevo recovers. | "Email send features are temporarily delayed. CRM data access is unaffected." |
| **Anthropic API outage** | AI-augmented features (chatbot, suggestions) degrade. CRM core is unaffected. Circuit-breaker pattern: catch 5xx, return graceful "AI features temporarily unavailable" UI message. | No proactive comms needed unless outage exceeds 4 h. |

---

## 7. Encryption at Rest

### OS-Level: LUKS

LUKS full-disk encryption encrypts the entire VPS data volume. It protects against **physical media theft** (e.g., Hetzner decommissioning a drive without secure wipe) and **provider-level access**. Once the VPS is running and the LUKS volume is unlocked, the PostgreSQL process accesses plaintext data normally — a compromised running VPS can still access all data.

Hetzner Cloud VMs boot from images; LUKS on the data volume requires either (a) a key file stored on the root volume (acceptable for the threat model of physical disk theft), or (b) network-unlocking via Clevis/Tang for true headless operation.

**For leCRM v0:** LUKS on the PostgreSQL data volume is a reasonable baseline. Key stored on root volume, documented unlock procedure in runbook.

### Field-Level: pgcrypto

`pgcrypto` encrypts individual columns using symmetric or asymmetric keys. Use cases: DKIM private keys stored in the DB, OAuth client secrets, phone numbers for ultra-sensitive tenants. The overhead is per-query decryption cost and key management complexity. The PostgreSQL docs note that decrypted data and keys exist on the server during query execution — this is not a defense against a root-compromised VPS, only against data-at-rest theft.

Sources: [PostgreSQL Encryption Options](https://www.postgresql.org/docs/current/encryption-options.html)

### Practical Recommendation for leCRM

| Layer | What it protects | Recommended? |
|-------|-----------------|--------------|
| LUKS on data volume | Physical disk theft, provider decommission | Yes — v0 baseline |
| TLS in transit (PG SSL) | Network sniffing | Yes — always on |
| Client-side backup encryption | Backup at rest on object storage | Yes — mandatory (see §8) |
| pgcrypto field-level | Per-column for specific PII (e.g., IBAN, phone) | Optional, add for highest-sensitivity fields only |

**Twenty CRM has no field-level encryption out of the box.** If a VPS is compromised (root access gained), all CRM data is readable in plaintext. LUKS mitigates offline theft but not a live compromise. The practical Article 32 posture: document LUKS + TLS + access controls + regular patching as your "appropriate technical measures" — this is defensible for SMB-scale CRM data.

---

## 8. Backup Encryption & Secret Management

### The Correct Architecture

**Encrypt before upload.** Do not rely on provider-side SSE. This ensures key sovereignty: even if Hetzner or Scaleway is compelled to hand over bucket contents, the data is unreadable without your key.

```
PostgreSQL WAL → WAL-G (with WALG_GPG_KEY_ID or age pipe) → encrypted blob → Hetzner Object Storage
```

WAL-G supports GPG encryption natively (`WALG_PGP_KEY` env var pointing to an armored public key). Alternatively, pipe through `age` as a post-process step. GPG is more widely documented for WAL-G.

### Secret Manager Comparison

| Tool | EU residency | Infra required | Audit logs | Key rotation | Cost | Recommendation |
|------|-------------|----------------|------------|-------------|------|----------------|
| **sops + age** | Keys stored wherever you put them | None (binary + git) | None built-in | Manual (`sops rotate`) | Free | Best for solo operator, config-as-code |
| **HashiCorp Vault OSS** | Self-host on EU VPS | Dedicated VPS or sidecar | Yes (audit backend) | Automated lease renewal | VPS cost (~€5/mo) | Overkill for v0; good for v1+ |
| **Bitwarden Secrets Manager** | Self-hostable (EU) | Docker / Vaultwarden | Limited | Manual | Free OSS / ~$6/mo cloud | Good UI, less mature than Vault |
| **Doppler** | US-company, EU data plane unclear | SaaS | Yes | Yes | ~$12/mo | Avoid — US sub-processor risk |
| **Infisical** | Self-hostable (EU) | Docker | Yes | Yes | Free OSS | Emerging alternative to Vault |

**Recommendation for leCRM:**

- **v0 (solo, ≤10 clients):** `sops` + `age`. Zero infrastructure. Each client's secrets live in `secrets/<client-slug>/secrets.enc.yaml`, encrypted with an `age` key pair. The private key is stored in a hardware token (YubiKey) or at minimum in a password manager (Bitwarden). Secrets are decrypted at deploy time into environment variables. Git history preserves a full audit trail of when secrets changed (though not who accessed them).

- **v1+ (≥10 clients, production):** Add HashiCorp Vault OSS on a dedicated small VPS (Hetzner CX11, €4/mo). Vault provides dynamic secrets, lease-based rotation, and an audit log. Each tenant gets its own KV path: `secret/lecrm/clients/<workspace-id>/`. Use Vault's AppRole auth for the application.

Sources: [sops GitHub](https://github.com/getsops/sops) · [Bitwarden vs Vault comparison](https://bitwarden.com/blog/bitwarden-secrets-manager-hashicorp-vault-alternative/) · [Best secret management tools 2026](https://infisical.com/blog/best-secret-management-tools)

---

## 9. Per-Client Secret Pattern

Each tenant in leCRM has its own set of rotating secrets. Recommended namespacing:

```yaml
# secrets/clients/acme-corp/secrets.enc.yaml (sops-encrypted)
anthropic_api_key: ENC[...]
oauth_client_id: ENC[...]
oauth_client_secret: ENC[...]
dkim_private_key: ENC[...]
jwt_signing_key: ENC[...]
webhook_signing_secret: ENC[...]
```

At runtime, the leCRM API service loads these from environment (in v0, injected by deploy script) or from Vault's KV store (in v1+, fetched at startup via AppRole token).

**Rotation cadence:**
- `anthropic_api_key`: Rotate when billing period resets or on suspected compromise.
- `jwt_signing_key`: Rotate quarterly; implement key versioning (new key signs new tokens, old key still validates for 24h overlap).
- `dkim_private_key`: Rotate annually; update DNS TXT record in sync.
- `oauth_client_secret`: Rotate on any personnel change or annually.

For Vault (v1+), use `kv-v2` (versioned KV) — it keeps 10 prior versions, enabling rollback if a rotation breaks something.

---

## 10. Audit Log Specification (GDPR-Defensibility)

### What Twenty CRM Captures

Twenty v0.20+ added "audit logs for every object" — meaning create/update/delete events on CRM objects (contacts, opportunities, tasks) are timestamped and stored. This is surfaced in the timeline view per object. Sources: [Twenty v0.20 release](https://x.com/twentycrm/status/1802337531272261840)

**What Twenty does NOT capture by default (gaps to fill):**
- Authentication events (login, failed login, logout)
- Data export / bulk download events
- Permission / RBAC changes
- API key creation / revocation
- Erasure request processing log
- Admin impersonation of a workspace

### Required Audit Event Table (CNIL-defensible)

| Event | Fields to log | Retention |
|-------|--------------|-----------|
| User login (success) | user_id, workspace_id, IP, user_agent, timestamp | 1 year |
| User login (failure) | attempted_email, IP, reason, timestamp | 1 year |
| Data export / CSV download | user_id, workspace_id, object_type, row_count, timestamp | 3 years |
| Record create / update / delete | user_id, workspace_id, object_type, record_id, diff, timestamp | 3 years |
| Permission change | actor_id, target_user_id, old_role, new_role, timestamp | 3 years |
| Right-to-erasure request received | requestor_email, workspace_id, timestamp, handler_id | 5 years |
| Right-to-erasure completion | same + confirmation, backup_rolloff_date | 5 years |
| API key create / revoke | user_id, key_id (last 8 chars), timestamp | 3 years |

**CNIL retention guidance:** French data protection regulations and CNIL guidance treat security audit logs as a legitimate interest (Art. 6(1)(f)) justified by security obligations under Art. 32. Typical defensible retention: 1 year for authentication, 3 years for data access, 5 years for erasure records. Do not retain indefinitely — excessive retention is itself a GDPR violation.

**Implementation approach for leCRM:** Instrument the NestJS API layer with a middleware that writes to a separate `audit_log` table (or a dedicated append-only log store). The table must be immutable from the application layer — writes only, no deletes via app. A separate scheduled job handles retention-based purging.

---

## 11. Right-to-Erasure Across Backups

### The Structural Tension

Article 17 GDPR requires erasure "without undue delay." Backups, by design, are not modified after creation. This creates a genuine conflict.

### Regulatory Position (2025–2026)

The EDPB's 2025 Coordinated Enforcement Action (CEF 2025), published February 2026 and involving 32 DPAs including CNIL:

> "Half of the responding DPAs raised concerns that many controllers have no specific procedures for erasure in the backup context, and some controllers do not delete or remove personal data from backups at all, nor do they have processes to prevent previously deleted data from being restored when backups are reinstalled."

This is a **supervisory priority area**. DPAs have explicitly requested EDPB guidance on what "without undue delay" means for backups — guidance is pending.

Sources: [EDPB CEF 2025 report](https://www.edpb.europa.eu/system/files/2026-02/edpb_cef-report_2025_right-to-erasure_en.pdf) · [Reed Smith analysis](https://www.reedsmith.com/our-insights/blogs/viewpoints/102mm9l/edpb-report-on-the-right-to-erasure-key-takeaways-from-the-2025-coordinated-enfo/)

### Defensible Position for leCRM

The currently accepted practice (CNIL-compatible, pre-final-EDPB-guidance):

1. **Erase immediately from production** upon receiving a valid Art. 17 request.
2. **Document** that encrypted backups containing the subject's data will roll off within your retention window (recommend: 30 days for WAL archives, 90 days for base backup sets).
3. **Communicate to the data subject** in your privacy policy and erasure response letter: *"Your data has been deleted from our active systems. Encrypted backup copies may persist for up to [X] days, after which they will be automatically destroyed. During this period, your data is encrypted and inaccessible without keys held solely by leCRM."*
4. **Prevent restoration of deleted data:** When restoring from backup (e.g., after a failure), apply the erasure list before bringing restored data into production. Maintain an erasure register with timestamps.
5. **Key destruction as GDPR-equivalent erasure:** If you encrypt each client's data with a per-client key (at-rest encryption with client-specific keys), destroying the key renders the ciphertext permanently unrecoverable — this is the cryptographic equivalent of deletion and is generally accepted by DPAs as compliant. This is the strongest architectural defense.

### HubSpot / Salesforce Published Positions

Both HubSpot and Salesforce's DPA documents state that personal data deleted from production will be purged from backup within their standard backup retention cycle (typically 30–90 days), and that backup data is not accessible to customers or subject to individual record lookup. This is the industry-standard defensible position — leCRM should adopt equivalent language.

---

## 12. Penetration Testing Cadence

### Phase 1 — v0 (1–10 clients, pre-revenue or early MRR)

Self-administered:
- Run OWASP ZAP automated scan against staging monthly as part of CI/CD.
- Use `nuclei` (ProjectDiscovery) for CVE-based scanning: `nuclei -u https://staging.lecrm.io -t cves/`.
- Review OWASP Top 10 manually on each major feature release (auth, API endpoints, file upload if any).
- Configure Fail2ban and rate limiting on all auth endpoints from day 1.

Sources: [OWASP ZAP](https://www.zaproxy.org/docs/desktop/start/pentest/)

### Phase 2 — v1 (10–25 clients, post-product-market fit)

- Annual third-party penetration test by an EU-based firm. Budget: €3,000–€8,000 for a focused web-app + API test.
- Bug bounty via a lightweight platform (Intigriti — EU-based) once the attack surface stabilizes.
- Quarterly ZAP + nuclei automated scans continue.

### Phase 3 — v2 (25+ clients, Series A / significant MRR)

- Semi-annual external pentest.
- ISO 27001 scoping exercise.
- Formal vulnerability disclosure policy (VDP) published on website.

---

## 13. Concrete Recommendations by Phase

### v0: VPS-per-Client

| Concern | Recommendation |
|---------|---------------|
| Backup tool | WAL-G with `archive_timeout = 60` |
| Backup destination | Hetzner Object Storage (NBG1), client-side encryption via WAL-G GPG before upload |
| Encryption at rest | LUKS on PostgreSQL data volume |
| Secret management | `sops` + `age`; private key on YubiKey or Bitwarden |
| Per-tenant secrets | One `sops`-encrypted YAML per client in `secrets/<slug>/` |
| Failover | Hetzner Floating IP + auto-restart; Hetzner Volume for fast VPS swap |
| Restore drill | Quarterly, using the runbook in §5 |
| Audit logs | Custom NestJS middleware writing to `audit_log` table; gaps listed in §10 |
| Erasure | Immediate production deletion + document 30-day backup rolloff in privacy policy |
| Pentest | Monthly ZAP scan on staging; manual OWASP Top 10 review on major releases |
| RTO/RPO achieved | RPO ~1–3 min, RTO ~30–60 min |

### v1+: Shared Infrastructure, Logical Isolation

| Concern | Recommendation |
|---------|---------------|
| Backup tool | WAL-G (same) + daily per-workspace `pg_dump` as surgical restore supplement |
| Backup destination | Hetzner Object Storage (primary) + Scaleway FR (secondary, cross-region DR) |
| Secret management | HashiCorp Vault OSS on Hetzner CX11; per-workspace KV path; AppRole auth |
| Failover | Patroni (Hetzner primary + standby in second DC) + Hetzner Floating IP |
| Per-tenant restore | Documented surgical extraction script (§3) + per-workspace daily `pg_dump` |
| Audit logs | Expand to append-only log store (e.g., Loki or a dedicated `audit_log` DB) |
| Erasure | Add erasure register; implement crypto-shredding (per-workspace encryption keys) |
| RTO/RPO achieved | RPO ~30–90 s (streaming replication), RTO ~1–5 min (Patroni promote) |

---

## Open Questions / TO RESOLVE

1. **EDPB backup erasure guidance (pending):** Final EDPB guidance on "without undue delay" for backups is expected in 2026. Monitor and update privacy policy + erasure procedures when published. [EDPB tracker](https://www.edpb.europa.eu/our-work-tools/our-documents/other/coordinated-enforcement-action-implementation-right-erasure_en)
2. **pgBackRest governance:** Monitor whether the coalition-funding model for pgBackRest materializes. If it does and Percona backs it, pgBackRest remains a viable alternative to WAL-G for future deployments.
3. **Twenty CRM audit log gaps:** Validate what exactly Twenty's AGPL code logs (vs. what the product page claims). Open a GitHub issue or search the `twentyhq/twenty` repo for `AuditLog` to confirm event schema before relying on it for GDPR defensibility.
4. **Hetzner Object Storage WAL-G SSE-C feasibility:** Test whether WAL-G's GPG encryption can be layered alongside Hetzner SSE-C or whether client-side GPG alone suffices. GPG before upload is simpler and more portable.
5. **Cross-region DR latency:** Quantify how long a full WAL restore to OVH FR takes for a 20 GB database from Hetzner Object Storage NBG1. This determines whether cross-region RTO ≤4 h is realistic.

---

*Research sources: [WAL-G docs](https://wal-g.readthedocs.io/PostgreSQL/) · [pgBackRest archived](https://percona.community/blog/2026/04/28/pgbackrest-is-archived-what-now/) · [PostgreSQL continuous archiving](https://www.postgresql.org/docs/current/continuous-archiving.html) · [Hetzner Object Storage](https://www.hetzner.com/storage/object-storage/) · [Hetzner SSE-C docs](https://docs.hetzner.com/storage/object-storage/howto-protect-objects/encrypt-with-sse-c/) · [EDPB CEF 2025](https://www.edpb.europa.eu/system/files/2026-02/edpb_cef-report_2025_right-to-erasure_en.pdf) · [Reed Smith EDPB analysis](https://www.reedsmith.com/our-insights/blogs/viewpoints/102mm9l/edpb-report-on-the-right-to-erasure-key-takeaways-from-the-2025-coordinated-enfo/) · [PostgreSQL encryption options](https://www.postgresql.org/docs/current/encryption-options.html) · [sops](https://github.com/getsops/sops) · [archive_timeout GUC](https://postgresqlco.nf/doc/en/param/archive_timeout/) · [DanubeData EU S3 2025](https://danubedata.ro/blog/best-s3-compatible-object-storage-europe-2025) · [Twenty v0.20 audit logs](https://x.com/twentycrm/status/1802337531272261840)*
