# ADR-007 — Encryption, Secret Management, Audit Logging

**Status:** Accepted
**Date:** 2026-05-10
**Deciders:** Guillaume

---

## Context

GDPR Art. 32 requires "appropriate technical and organisational measures" — vague enough to demand judgement, specific enough that auditors and CNIL inspectors expect documented architecture. leCRM's posture must be defensible without being over-engineered.

This ADR consolidates three related concerns:

1. **Encryption at rest.** What's encrypted where, with what key, against what threat?
2. **Secret management.** Per-tenant Anthropic API keys, OAuth client secrets, DKIM private keys, JWT signing keys, webhook signing secrets — how are they stored, rotated, and accessed?
3. **Audit logging.** What events are logged, what fields, what retention, how is the log made tamper-resistant?

And one cross-cutting compliance question:

4. **Right-to-erasure across encrypted backups** — the genuine GDPR-vs-architecture tension.

The constraints:

- **EU data residency** for any secret store with a managed component.
- **Solo operator.** Vault is appropriate at scale but heavy at v0; sops + age covers v0 cleanly.
- **Twenty's audit log infrastructure** already covers per-object create/update/delete. We extend, not replace.
- **CNIL's published guidance** does not mandate field-level encryption for SMB-tier CRM; documenting LUKS + TLS + access control is defensible.
- **EDPB CEF 2025** (published February 2026) flagged backup-erasure as a supervisory priority; final guidance pending.

The research artefact `docs/research/dr-security.md` §7–§11 covers the technical options.

---

## Decision

### 1. Encryption at rest — layered

| Layer | What | When |
|---|---|---|
| **OS — LUKS** on PostgreSQL data volume | Defends against physical disk theft and provider decommission | v0 onwards |
| **TLS in transit** (PG SSL, all HTTPS endpoints) | Defends against network sniffing | v0 onwards, always-on |
| **Backup — client-side GPG via WAL-G** | Defends against object-storage compromise / subpoena | v0 onwards ([ADR-006](ADR-006-backup-dr.md)) |
| **Field-level (pgcrypto)** | Per-column for ultra-sensitive fields (IBAN, phone) | **Not in v0**; reconsider at v2 if a regulated client requires |

**LUKS configuration:** the PostgreSQL data volume on each Hetzner VM (or shared cluster DB host) is LUKS-encrypted. Key file stored on the root volume so the DB can boot unattended. Documented unlock procedure in the runbook for VPS-replacement scenarios. This protects against physical disk theft and Hetzner decommissioning a drive without secure wipe; **it does not protect against a live root compromise** of the running VPS.

**Twenty CRM has no field-level encryption out of the box.** This is an explicit exposure: a successful root compromise of a running VPS reads all CRM data in plaintext. The accepted v0 posture is documented in client DPAs as "appropriate technical measures: full-volume encryption (LUKS), encryption in transit (TLS), restricted access controls, automated patching, encrypted backups with client-side key management." This is defensible for SMB-tier CRM data per `docs/LEGAL-PLAYBOOK.md` §1 Article 32 checklist.

**v2 reconsideration trigger:** a paying client in a regulated sector (healthcare, legal, finance) explicitly requires column-level encryption, OR a security incident exposes the gap. Adding pgcrypto field-level encryption to specific columns is well-understood Twenty fork work; not built speculatively.

### 2. Secret management

#### v0 (≤10 clients): sops + age

- Each tenant's runtime secrets in `secrets/clients/<slug>/secrets.enc.yaml`, encrypted with sops + age.
- Master age private key stored in a **YubiKey** (primary) and a Bitwarden vault (backup). Two-key custody.
- Decryption at deploy time → environment variables on the target VPS via the Compose service definition.
- Secrets repo is private GitHub (separate from the leCRM source repo); commit history is the audit trail of *when* secrets changed.

**Per-tenant secret manifest (canonical):**

```yaml
# secrets/clients/acme-corp/secrets.enc.yaml (sops-encrypted)
# Per-tenant Anthropic API key (one per workspace; provisioned via LiteLLM team)
anthropic_api_key: ENC[...]

# Per-tenant Brevo (or shared if account-level — see ADR-003 TO RESOLVE)
brevo_api_key: ENC[...]

# Per-tenant DKIM private key (matches DNS record)
dkim_private_key: ENC[...]

# Per-tenant OAuth client secrets for Gmail / Microsoft Graph reply detection
oauth_gmail_client_secret: ENC[...]
oauth_msgraph_client_secret: ENC[...]

# Per-tenant JWT signing key for any tenant-issued tokens
jwt_signing_key: ENC[...]

# Per-tenant webhook signing secret (Brevo, Telegram)
brevo_webhook_secret: ENC[...]
telegram_webhook_secret: ENC[...]

# DB role password
db_role_password: ENC[...]
```

#### v1+ (≥10 clients, production): HashiCorp Vault OSS

- Single Hetzner CX11 (€4/mo) running Vault OSS.
- Per-tenant KV path: `secret/lecrm/clients/<workspace-id>/`.
- **AppRole auth** for each leCRM service (twenty-server, email-service, agent-runtime). Each role grants read on the relevant tenant secrets only.
- **kv-v2 (versioned KV)** — keeps 10 prior versions for rollback if a rotation breaks a deployment.
- Audit backend enabled (writes to `audit/leCRM/vault.log`).
- Auto-unseal via cloud KMS is **not** used in v0 (operational cost). Vault unseal is a documented two-key (Shamir 2-of-3) ceremony performed by Guillaume on each restart. Acceptable because Vault restarts are rare.

Rotation cadence:

| Secret | Rotation | Mechanism |
|---|---|---|
| `jwt_signing_key` | Quarterly | Versioned: new key signs new tokens; old key validates for 24 h overlap |
| `oauth_*_client_secret` | Annually OR on personnel change | Update upstream provider, then update Vault, then rolling-restart services |
| `dkim_private_key` | Annually | Generate new key, update DNS TXT record (with new selector), update Brevo, decommission old selector after 7 days |
| `anthropic_api_key` | On suspected compromise OR billing-period reset | Provision new via Anthropic console, update Vault, deploy |
| `brevo_webhook_secret` | Quarterly | Coordinate with Brevo dashboard rotation |
| `db_role_password` | Annually | Vault dynamic credentials in v2 (out of v0 scope) |

### 3. Audit logging

#### What Twenty captures already

Twenty v0.20+ captures per-object create/update/delete events on CRM objects (contacts, opportunities, tasks). Available in the timeline view. (`docs/research/dr-security.md` §10.)

**TO RESOLVE — verify** the actual event schema and field set in upstream code before relying on it; the marketing claims and the implementation can diverge.

#### What we extend

A NestJS middleware in `gbconsult/audit/` writes a richer event set to a separate `audit_log` table:

```sql
CREATE TABLE audit_log (
  id              BIGSERIAL PRIMARY KEY,
  ts              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  workspace_id    UUID,
  actor_user_id   UUID,
  actor_ip        INET,
  actor_user_agent TEXT,
  event_type      TEXT NOT NULL,
  event_payload   JSONB NOT NULL,
  retention_class TEXT NOT NULL  -- 'auth' | 'data' | 'erasure'
);
CREATE INDEX idx_audit_ws_ts ON audit_log(workspace_id, ts);
CREATE INDEX idx_audit_event_type ON audit_log(event_type);
```

**Event catalogue (with retention):**

| Event | Fields | Retention class | Retention |
|---|---|---|---|
| `auth.login.success` | actor_user_id, actor_ip, actor_user_agent, workspace_id | auth | 1 year |
| `auth.login.failure` | attempted_email, actor_ip, reason | auth | 1 year |
| `auth.logout` | actor_user_id, workspace_id | auth | 1 year |
| `data.export.csv` | actor_user_id, workspace_id, object_type, row_count, query | data | 3 years |
| `data.bulk_download` | actor_user_id, workspace_id, object_type, row_count | data | 3 years |
| `data.record.create` | actor_user_id, workspace_id, object_type, record_id, diff | data | 3 years |
| `data.record.update` | as above | data | 3 years |
| `data.record.delete` | as above | data | 3 years |
| `auth.permission.change` | actor_user_id, target_user_id, old_role, new_role, workspace_id | data | 3 years |
| `auth.api_key.create` | actor_user_id, key_id_last8, scopes, workspace_id | data | 3 years |
| `auth.api_key.revoke` | actor_user_id, key_id_last8, workspace_id | data | 3 years |
| `gdpr.erasure.request` | requestor_email, workspace_id, target_record_id, handler_id | erasure | 5 years |
| `gdpr.erasure.complete` | as above + confirmation, backup_rolloff_date | erasure | 5 years |
| `admin.impersonation.start` | actor_user_id (gbconsult-admin), workspace_id, justification | data | 3 years |
| `admin.impersonation.end` | as above + duration | data | 3 years |

**CNIL retention rationale:** auth events 1 year (legitimate interest, security-defensibility); data events 3 years (typical SaaS standard); erasure events 5 years (proves compliance with the request). Excessive retention is itself a GDPR violation, so a retention cron purges expired rows daily.

**Tamper resistance:** the application's database role has `INSERT, SELECT` only on `audit_log`. No `UPDATE, DELETE` grant. A separate `audit_purge` role used by the retention cron has `DELETE WHERE ts < <retention-cut-off>` permission, isolated by row-level filtering. Not cryptographically tamper-evident, but operationally append-only.

For phase 3 / v2: consider shipping audit events to an external append-only log store (Loki + Grafana for ops, or a write-only S3 bucket with object-lock for tamper-evidence). The TO RESOLVE flags the v2 review.

### 4. Right-to-erasure across encrypted backups

Article 17 GDPR requires erasure "without undue delay." Backups, by design, are not modified after creation. Genuine tension.

**EDPB CEF 2025** (`docs/research/dr-security.md` §11) flagged this as a supervisory priority; **final guidance pending in 2026**.

**leCRM's defensible position (CNIL-compatible, pre-final-EDPB-guidance):**

1. **Erase immediately from production** upon receiving a valid Art. 17 request. Cascade across active tables, soft-delete where Twenty's model uses soft-delete, hard-delete where it doesn't.

2. **Document backup roll-off** in the privacy policy and erasure response letter:
   > "Your data has been deleted from our active systems. Encrypted backup copies may persist for up to 30 days (WAL archives) and 90 days (full base backups), after which they will be automatically destroyed. During this period, your data is encrypted at rest with keys held solely by GB Consult, and is inaccessible to any third party including the storage provider."

3. **Maintain an erasure register.** The `audit_log` `gdpr.erasure.*` events are the primary record. A monthly reconciliation script verifies that no row matching an erasure request appears in any production schema after the request timestamp.

4. **Prevent restoration of deleted data.** Restore runbook ([ADR-006](ADR-006-backup-dr.md)) includes a step: after a base-backup restore, replay the erasure register against the restored data before bringing it into production.

5. **Crypto-shredding (v2 candidate).** Per-workspace data encrypted with a per-workspace key (Vault Transit secret engine) lets us "delete" a workspace by destroying its key. Ciphertext in backups becomes permanently unrecoverable. This is the strongest architectural defence and is generally accepted by DPAs as compliant. **Not implemented in v0** because it requires per-workspace key wrapping at the Postgres layer (pgcrypto) which we deferred. Reconsider when field-level encryption is added.

6. **Track EDPB final guidance.** When published, update privacy policy, erasure response letter, and architecture if the guidance materially changes the requirements.

### 5. Penetration-testing cadence

| Phase | Cadence | Tooling |
|---|---|---|
| Phase 1 (v0) | Self-administered monthly | OWASP ZAP automated scan; `nuclei` CVE scan; manual OWASP Top 10 review on each major release; Fail2ban + rate limiting on auth endpoints |
| Phase 2 (v1) | Annual third-party | EU-based pentest firm, ~€3,000–€8,000. Quarterly ZAP + nuclei automated scans. Lightweight bug bounty (Intigriti — EU-based) once attack surface stabilises |
| Phase 3 (v2) | Semi-annual third-party | Plus ISO 27001 scoping exercise; published vulnerability disclosure policy |

(`docs/research/dr-security.md` §12.)

---

## Consequences

### Positive

- **Layered encryption** with explicit threat-model framing: each layer documents what it defends against. Defensible in DPA negotiations.
- **sops + age in v0** has zero infrastructure cost. Vault arrives at the right scale.
- **Audit log is comprehensive** beyond Twenty's defaults. Auth events, data exports, permission changes, impersonation, erasure — all the events a CNIL inspector would ask for.
- **Per-tenant secret namespacing** scales cleanly to phase 2/3 without restructuring.
- **Erasure response is documented** with a defensible position and a roadmap to crypto-shredding when needed.

### Negative

- **No field-level encryption in v0** is a real exposure: a live root compromise reads all CRM data. Mitigation: documented in DPA, hardening posture (fail2ban, automated patches, restricted SSH, no public DB ports). Not zero-risk; defensible-risk.
- **YubiKey loss in v0 is a recovery scenario.** The Bitwarden backup must be tested; quarterly drill validates this (`docs/research/dr-security.md` §455 item 5 in [ADR-006](ADR-006-backup-dr.md)).
- **Vault adds an operational service in v1+.** Unsealing on each restart is a manual ceremony. Acceptable because restarts are rare; documented in the runbook.
- **Audit log writes synchronously** in the request transaction. Adds 1–3 ms per write to mutating endpoints. Acceptable; alternative async write risks audit gaps on partial-failure.
- **Crypto-shredding deferred.** The strongest erasure-across-backups defence requires field-level encryption work that we explicitly didn't do. v2 reconsideration is a known gap.

### Neutral

- The audit log size grows linearly with active use. At phase 3 (20 clients × 8 active users × 100 events/day) ≈ 16k rows/day, ~6M rows/year. Postgres handles this without strain; partition by `ts` quarterly when row count crosses 10M.
- AGPL §13 footer (per [ADR-002](ADR-002-twenty-fork-management.md)) and the privacy-policy backup-rolloff wording must agree. Coordinated update at v0 launch.

---

## Alternatives Considered

### Alt 1: pgcrypto field-level encryption from day 1

Rejected for v0. (`docs/research/dr-security.md` §7.) Field-level encryption requires deciding which fields, managing per-field keys, handling search/sort encrypted columns (not natively possible), and integrating with Twenty's TypeORM entities. Significant fork work for a hardening posture not required by SMB-tier CNIL guidance. Reconsider at v2 if a regulated client demands it.

### Alt 2: Vault from day 1

Rejected for v0. Adds a service to operate (unsealing, audit log destination, backup of Vault itself) that returns no value at ≤4 clients. sops + age give the same encryption guarantee with zero infrastructure for the v0 timeframe. The migration sops → Vault is a bounded one-time task at the phase 2 transition.

### Alt 3: Doppler / cloud-managed secret manager

Rejected. Doppler is US-headquartered with unclear EU data plane status. Adds a sub-processor for a problem that EU-self-hosted Vault solves. (`docs/research/dr-security.md` §8 secret manager comparison.)

### Alt 4: Twenty's audit log only (no extension)

Rejected. Twenty's audit log covers per-object data events but misses authentication, permission changes, exports, and impersonation — exactly the events a GDPR audit asks for. Extending in `gbconsult/audit/` is straightforward middleware work.

### Alt 5: External SIEM (Loki + Grafana from v0)

Rejected for v0. Adding a SIEM service for ≤4 clients is over-engineered. The Postgres `audit_log` table is queryable, exportable, and tamper-resistant at the application layer. A v2 SIEM migration is feasible by tailing the table to Loki.

### Alt 6: Auto-unseal Vault via Hetzner KMS / cloud KMS

Hetzner doesn't offer KMS as of May 2026. Cloud-KMS auto-unseal would couple us to a third-party KMS (AWS KMS, GCP KMS) — adds a sub-processor with US jurisdictional concerns. Manual unseal on rare restarts is the cleanest posture. Reconsider if a EU-sovereign KMS becomes available.

---

## References

- `docs/research/dr-security.md` (entire document; §7 encryption at rest, §8 backup encryption + secret managers, §9 per-client secret pattern, §10 audit log spec, §11 right-to-erasure across backups, §12 pentest cadence).
- `docs/LEGAL-PLAYBOOK.md` §1 (Article 32 security measures checklist), §2 (sub-processor concerns).
- [PostgreSQL encryption options](https://www.postgresql.org/docs/current/encryption-options.html).
- [sops](https://github.com/getsops/sops).
- [HashiCorp Vault docs](https://developer.hashicorp.com/vault/docs).
- [EDPB CEF 2025 report (right-to-erasure)](https://www.edpb.europa.eu/system/files/2026-02/edpb_cef-report_2025_right-to-erasure_en.pdf).
- [Reed Smith analysis of EDPB CEF 2025](https://www.reedsmith.com/our-insights/blogs/viewpoints/102mm9l/edpb-report-on-the-right-to-erasure-key-takeaways-from-the-2025-coordinated-enfo/).
- [Twenty CRM v0.20 audit logs announcement](https://x.com/twentycrm/status/1802337531272261840).
- [OWASP ZAP](https://www.zaproxy.org/).
- [nuclei (ProjectDiscovery)](https://github.com/projectdiscovery/nuclei).
- Related ADRs: [ADR-002](ADR-002-twenty-fork-management.md) (audit middleware lives in `gbconsult/audit/`), [ADR-005](ADR-005-ai-agent-tenancy.md) (per-tenant Anthropic API keys, OAuth secrets, namespaced here), [ADR-006](ADR-006-backup-dr.md) (GPG keys for backup encryption are managed under this ADR's secret architecture), [ADR-001](ADR-001-tenancy-model.md) (`audit_log` lives in `core` schema in phase 2 — shared, not per-workspace, because GDPR audit visibility is operator-level).

---

## TO RESOLVE

1. **EDPB final guidance on right-to-erasure across backups.** Expected in 2026. Update privacy policy + erasure runbook + this ADR when published. (`docs/research/dr-security.md` §450 item 1.)
2. **Twenty audit log code coverage.** Validate by reading `twentyhq/twenty` source (search for `AuditLog`, `audit_log`) what events Twenty actually captures vs the v0.20 announcement. Document gaps in this ADR's "what Twenty captures already" section. (`docs/research/dr-security.md` §450 item 3.)
3. **Field-level encryption v2 trigger.** Define the exact criterion: "first regulated-sector client signs," "first incident," or "first DPA negotiation that explicitly requires it." Without a trigger, the deferral is open-ended.
4. **YubiKey backup recovery drill.** Annually, simulate YubiKey loss and recover age private key from Bitwarden vault. Verify decryption of a current `secrets.enc.yaml`. Without this drill, the YubiKey + Bitwarden custody is unproven.
5. **Vault unseal-key custody.** Shamir 2-of-3 split: Guillaume holds 2; the 3rd is escrowed. Where? Options: a trusted third-party (lawyer, accountant) with a sealed envelope, or a notarised deposit. Decide before v1+ Vault deployment.
6. **Audit log partition-by-time at scale.** Trigger criterion: `audit_log` row count crosses 10M. At that point, partition by `ts` quarterly and migrate retention purger to drop-partition. Phase 3 task.
7. **Crypto-shredding implementation cost estimate.** When v2 field-level encryption is on the table, scope the per-workspace-key wrapping work. Likely a 2–3 week task: pgcrypto + Vault Transit + workspace-creation hook. Not started.
8. **Privacy policy + DPA wording for the LUKS-only-no-field-level posture.** Legal task tracked in `docs/LEGAL-PLAYBOOK.md`; this ADR commits the architecture to a phrasing the legal posture must defend.
