---
title: Privacy Policy — leCRM
status: draft (pending publication at gbconsult.me/lecrm/privacy)
version: 1.0-draft
created: 2026-05-14
publication-target: gbconsult.me/lecrm/privacy (Cloudflare DNS + VPSDeploy static site)
purpose: Google OAuth production review prerequisite — covers Gmail sensitive scopes (gmail.readonly, gmail.modify) per §11 Google API Services User Data Policy
---

# Privacy Policy — leCRM

**Effective date:** _to be filled when published_
**Data controller:** GB Consult (Guillaume Beghin), 79 Deux-Sèvres, France — SIRET _TBD_

This Privacy Policy describes how the leCRM service ("leCRM", "we", "us") processes personal data of its users ("you", "your"). leCRM is a sovereign customer-relationship-management tool operated by GB Consult on EU-resident infrastructure. The source code is available under the Apache 2.0 license at _TBD_ (self-host option).

---

## 1. Data we process

### 1.1 Account & workspace data
When you sign up we process: your email address, name, workspace name, and authentication identifiers issued by our identity provider (Authentik). We retain this data for the lifetime of your workspace plus 90 days after deletion (for backup-recovery and audit purposes).

### 1.2 CRM content
We process the records you create or import into leCRM: contacts, deals, companies, notes, tasks, custom-object data. This data is yours; we are a processor for it under RGPD Art. 28. Retention is controlled by you.

### 1.3 Gmail data (if you connect a Google account)
If you authorise leCRM to access your Gmail account, we process:

- **Message metadata** — From/To/Subject headers, Message-ID, thread IDs, labels, timestamps — to surface threads alongside the CRM records they relate to.
- **Message bodies** — to display the conversation in the leCRM thread viewer and let you link a message to a deal, contact, or company.
- **Modifications you initiate** — applying labels (e.g. `leCRM/Deal-Linked`), starring, or archiving — performed against the Gmail API in response to your in-app actions.

**Scopes requested:**

| Google scope | Why leCRM needs it |
|--------------|---------------------|
| `https://www.googleapis.com/auth/gmail.readonly` | Read message metadata + bodies so threads can be surfaced inside CRM records. Without this, the email-thread feature cannot function. |
| `https://www.googleapis.com/auth/gmail.modify` | Apply CRM labels and link state to Gmail threads (e.g. mark a thread as linked to deal X) so the thread→record relationship survives outside leCRM. We do NOT send mail; the `gmail.send` scope is not requested. |

**Where Gmail data lives:**
- Message bodies and attachments are **not persisted** to the leCRM database. They are fetched on-demand from the Gmail API and cached in volatile memory only for the duration of the request.
- Message **metadata** (Message-ID, thread-ID, From/Subject/Date headers, label state, and the CRM record link) is persisted to your workspace's Postgres schema so the integration can index threads and survive Gmail API rate limits.
- Tokens (OAuth refresh + access tokens) are encrypted at rest using the workspace data-encryption key (see ADR-007).

**Data minimisation:** We do not request `gmail.send`, `gmail.compose`, or full-mailbox export. We do not train any AI model on your Gmail data. We do not transfer your Gmail data to any third party except as listed in §4 below.

### 1.4 Operational telemetry
We collect minimal telemetry to operate the service: request logs (URL path, status, latency — no body content), error reports (Sentry self-hosted or Grafana Cloud EU), and aggregate usage metrics. Retention is 30 days for logs, 90 days for error reports.

### 1.5 What we do NOT collect
- We do not use third-party analytics that profile you (no Google Analytics, no Meta Pixel).
- We do not sell, rent, or trade your data.
- We do not use your data — Gmail or CRM — to train AI models.

---

## 2. Legal bases (RGPD Art. 6)

| Processing | Legal basis |
|------------|-------------|
| Account creation, billing, service delivery | Contract performance (Art. 6.1.b) |
| Gmail integration | Your explicit consent at the OAuth screen (Art. 6.1.a); withdrawable any time |
| Security logs, fraud prevention | Legitimate interest (Art. 6.1.f) |
| Legal compliance (tax records, court orders) | Legal obligation (Art. 6.1.c) |

---

## 3. How we use Gmail data — Google's Limited Use clarification

leCRM's use of information received from Google APIs adheres to the [Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy), including the Limited Use requirements. Specifically:

- We only use Gmail data to provide and improve the user-facing email-thread features of leCRM.
- We do not transfer Gmail data to others except to provide or improve these features, or as required by law.
- We do not use Gmail data for serving advertisements, including retargeted or interest-based ads.
- We do not allow humans to read Gmail data except (a) with your specific consent, (b) for security investigations, (c) to comply with legal process, or (d) as part of an aggregated, anonymised debugging dataset.

---

## 4. Subprocessors

We rely on the following EU-resident or EU-data-residency-guaranteed subprocessors:

| Subprocessor | Role | Location | DPA |
|-------------|------|----------|------|
| Ubicloud (managed Postgres on Hetzner) | Database hosting | Germany (FRA / FSN) | [link TBD] |
| OVH | VPS hosting (web/API) | France (GRA / RBX / SBG) | [ovhcloud.com/dpa](https://www.ovhcloud.com/) |
| Cloudflare | DNS + edge caching | Global (EU edge for EU users) | [cloudflare.com/dpa](https://www.cloudflare.com/cloudflare-customer-dpa/) |
| Brevo | Transactional email | France | [brevo.com/legal/privacypolicy](https://www.brevo.com/legal/privacypolicy/) |
| Grafana Cloud (EU) | Observability | Netherlands | [grafana.com/legal/dpa](https://grafana.com/legal/dpa/) |

We do **not** use any US-substrate AI or analytics vendor that would require Standard Contractual Clauses.

---

## 5. International transfers

Personal data does not leave the European Economic Area in the ordinary course of operation. Cloudflare edge caching may transit data through non-EU points-of-presence for performance; this is covered by Cloudflare's published SCCs.

---

## 6. Your rights (RGPD Art. 12-22)

You can exercise the following rights by emailing **privacy@gbconsult.me**:

- **Access** — request a copy of all personal data we hold about you (Art. 15).
- **Rectification** — correct inaccurate data (Art. 16).
- **Erasure** — request deletion ("right to be forgotten", Art. 17). For Gmail-derived data, you can revoke our access at any time via [myaccount.google.com/permissions](https://myaccount.google.com/permissions); this immediately stops new fetches and deletes locally cached metadata within 30 days.
- **Portability** — receive your CRM data in a machine-readable format (Art. 20).
- **Object** — to processing based on legitimate interest (Art. 21).
- **Withdraw consent** — without affecting prior-lawful processing (Art. 7.3).

Response time: 30 days (Art. 12.3), extendable by 60 days for complex requests.

You also have the right to lodge a complaint with **CNIL** (the French data-protection supervisory authority): [cnil.fr/en/plaintes](https://www.cnil.fr/en/plaintes).

---

## 7. Revoking Gmail access

You can revoke leCRM's Gmail access in three ways:

1. **From inside leCRM** — Settings → Integrations → Gmail → Disconnect.
2. **From your Google account** — [myaccount.google.com/permissions](https://myaccount.google.com/permissions) → leCRM → Remove access.
3. **By emailing privacy@gbconsult.me** — we will revoke the token within 1 business day.

Revocation triggers deletion of cached Gmail metadata within 30 days. Tokens are invalidated immediately on receipt of the revocation event from Google.

---

## 8. Security

- All data in transit is encrypted via TLS 1.3.
- Data at rest is encrypted at the disk level (LUKS) and at the application layer for sensitive fields (Gmail tokens, secrets).
- Per ADR-007: tenancy isolation is enforced at the database schema level; cross-tenant access is structurally impossible without an admin-level credential.
- Backups are encrypted (GPG) and retained per ADR-006.
- Security events relevant to your account (e.g. login from a new device) are logged and surfaced in your settings.

---

## 9. Children

leCRM is a B2B service and is not directed at children under 16. We do not knowingly collect data from minors.

---

## 10. Changes to this policy

Material changes are communicated by email at least 30 days before they take effect. Non-material clarifications (typos, formatting) are versioned in the public source repository.

---

## 11. Contact

- **Data controller:** GB Consult, Guillaume Beghin — Deux-Sèvres, France
- **Privacy email:** privacy@gbconsult.me
- **General contact:** contact@gbconsult.me
- **Postal address:** _TBD_

---

_This policy is published under [docs/legal/PRIVACY-POLICY.md](https://github.com/_TBD_/leCRM/blob/main/docs/legal/PRIVACY-POLICY.md) in the leCRM source repository and reflects the state of the service as of the effective date above. Historical versions are available via git history._
