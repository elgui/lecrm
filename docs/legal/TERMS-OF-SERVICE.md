---
title: Terms of Service — leCRM
status: draft (pending publication at gbconsult.me/lecrm/terms)
version: 1.0-draft
created: 2026-05-14
publication-target: gbconsult.me/lecrm/terms (Cloudflare DNS + VPSDeploy static site)
purpose: Google OAuth production review prerequisite — required URL alongside Privacy Policy
---

# Terms of Service — leCRM

**Effective date:** _to be filled when published_
**Operator:** GB Consult (Guillaume Beghin), micro-entrepreneur registered in France — SIRET _TBD_

These Terms of Service ("Terms") govern your use of the leCRM hosted service operated by GB Consult ("we", "us"). By creating an account or accessing the service, you ("you", "Customer") agree to these Terms. If you self-host the source code under the Apache 2.0 license, these Terms do not apply to your deployment — only the license does.

---

## 1. The service

leCRM is a customer-relationship-management application. The hosted service is provided by GB Consult on EU-resident infrastructure under a paid subscription. The source code is available under the Apache 2.0 license; you may self-host instead of subscribing.

### 1.1 What you get
- A workspace with isolated database tenancy (per ADR-001 / ADR-009).
- Access to all features documented at [gbconsult.me/lecrm/docs](https://gbconsult.me/lecrm/docs) as of your subscription start.
- Integration with third-party services (Gmail, etc.) subject to those services' availability.
- Backups, security updates, and operational support per the Service Levels in §6.

### 1.2 What you bring
- A current email address for account communication.
- Valid payment details (subscriptions only).
- Lawful content. You are responsible for the data you store in your workspace.

---

## 2. Acceptable use

You agree not to use leCRM to:

- Violate any applicable law (including but not limited to RGPD, French Loi Informatique et Libertés, anti-spam laws like CAN-SPAM / CASL / French L. 34-5).
- Send unsolicited bulk email or operate an opt-in list without verifiable consent.
- Store data you have no lawful basis to process.
- Probe, scan, or reverse-engineer the service to discover security vulnerabilities outside of our published responsible-disclosure policy.
- Attempt to access another workspace's data.
- Use the service to build a directly competing CRM product (the Apache 2.0 license permits this; the **hosted service** does not — this is a separate contractual restriction on _your subscription_, not the open-source code).
- Exceed the documented API rate limits or attempt to circumvent quotas.

We may suspend or terminate accounts that breach this section after written notice and a 14-day cure period, except for breaches that present immediate harm (security, legal exposure), which may result in immediate suspension.

---

## 3. Third-party integrations (including Gmail)

leCRM offers optional integrations with third-party services. When you connect a third-party account:

- You authorise leCRM to access the third-party data and operations described at the consent screen.
- We process that data under our [Privacy Policy](./PRIVACY-POLICY.md).
- The third party's terms continue to apply to your use of their service; leCRM does not modify them.
- You may revoke access at any time. Revocation does not affect previously-processed data, only future processing.

For Gmail specifically: by connecting Gmail you authorise leCRM to read messages and modify message labels per the scopes documented in the Privacy Policy. Revocation is described in Privacy Policy §7.

---

## 4. Fees, billing, refunds

### 4.1 Subscription
Pricing is displayed at [gbconsult.me/lecrm/pricing](https://gbconsult.me/lecrm/pricing). Fees are billed monthly or annually in advance, in EUR, exclusive of VAT (VAT applied per applicable tax law).

### 4.2 Auto-renewal
Subscriptions auto-renew. You can cancel any time from your billing settings; cancellation takes effect at the end of the paid period.

### 4.3 Refunds
- New customers: 14-day refund window (RGPD distance-selling — French Code de la consommation L. 221-18).
- Annual prepay: pro-rated refund of the unused portion if you cancel due to a material breach by us.
- No refunds for partial months of a monthly plan.

### 4.4 Late payment
Accounts >30 days past due may be suspended. Data is retained for 90 days after suspension; export is available during this period.

---

## 5. Data ownership and portability

- **Your data is yours.** We claim no ownership of any content you store in leCRM.
- You can export all CRM data in JSON or CSV format at any time from Settings → Export.
- On termination, you may export your data for 90 days. After that, data is deleted from primary storage; encrypted backups age out per ADR-006 (35-day rolling window).
- Per RGPD Art. 28, we process your data as a data processor on your instructions; you remain the data controller.

---

## 6. Service levels and availability

### 6.1 Target uptime
**99.5% monthly** measured at the public HTTP entrypoint, excluding (a) scheduled maintenance announced ≥24h in advance, (b) outages caused by third-party services outside our control (e.g. Gmail API outage, OVH datacenter event).

### 6.2 Support
- Email support via support@gbconsult.me — best-effort response within 1 business day for paid plans.
- No phone support during the v0 phase.
- Status page: _TBD_ (Grafana Cloud public dashboard).

### 6.3 Backups
Per ADR-006: WAL-G continuous archive + daily encrypted snapshots, 35-day retention, GPG-encrypted.

### 6.4 Security incidents
We will notify you within **72 hours** of becoming aware of a personal-data breach affecting your workspace (RGPD Art. 33 timing applied to us as your processor).

---

## 7. Intellectual property

### 7.1 leCRM
The leCRM **trademark and logo** are owned by GB Consult. The **source code** is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) and is available at _repo URL TBD_. You may use the source under the license; you may not use the trademark or logo to suggest endorsement of your modified version.

### 7.2 Your content
You retain all rights to data you store in leCRM. By using the service, you grant us a non-exclusive licence solely to operate the service: store, transmit, display your data back to you, and process it through the features you use.

### 7.3 Feedback
If you submit feedback, suggestions, or bug reports, you grant us a perpetual, royalty-free right to use them without attribution, except as required by the Apache 2.0 license for contributions to the open-source repository (governed separately by the project's contribution policy).

---

## 8. Warranties and disclaimers

We provide the service "as is" with the warranties expressly stated in §6 (uptime, support, backups, security-incident notification). To the maximum extent permitted by applicable law (and subject to mandatory French consumer-protection law where you are a consumer), we disclaim all other warranties, express or implied, including merchantability and fitness for a particular purpose.

---

## 9. Limitation of liability

To the maximum extent permitted by law, our aggregate liability arising from or related to the service is limited to the fees you paid us in the 12 months preceding the claim. We are not liable for indirect, consequential, special, or punitive damages, including lost profits or lost data, except where caused by our gross negligence or wilful misconduct.

This section does not limit liability for: (a) personal injury or death caused by negligence, (b) fraud, (c) any liability that cannot be limited under French law.

---

## 10. Indemnification

You will indemnify us against third-party claims arising from your content stored in leCRM violating that party's rights (IP, privacy, or otherwise), provided we notify you promptly and allow you to control the defence.

---

## 11. Termination

- **You** may terminate any time from Settings → Cancel.
- **We** may terminate for: material breach (after 14-day cure notice), non-payment >30 days, immediate-harm scenarios per §2.
- On termination, §5 (data portability) applies.

---

## 12. Governing law and disputes

These Terms are governed by **French law**. Any dispute will be resolved by the courts of **Niort, France** (tribunal compétent), subject to mandatory consumer-protection rules permitting consumers to bring claims in their habitual residence.

For B2B disputes under €5,000, mediation via [CMAP](https://www.cmap.fr/) is required before judicial proceedings.

EU residents may also use the [EU Online Dispute Resolution platform](https://ec.europa.eu/consumers/odr/).

---

## 13. Changes to these Terms

Material changes are communicated by email at least 30 days before they take effect. Continued use after the effective date constitutes acceptance. If you do not accept the new Terms, you may cancel and request a pro-rated refund for the unused portion of a prepaid subscription.

---

## 14. Contact

- **Operator:** GB Consult, Guillaume Beghin — Deux-Sèvres, France
- **General contact:** contact@gbconsult.me
- **Legal notices:** legal@gbconsult.me
- **Postal address:** _TBD_

---

_This document is published under [docs/legal/TERMS-OF-SERVICE.md](https://github.com/_TBD_/leCRM/blob/main/docs/legal/TERMS-OF-SERVICE.md) in the leCRM source repository. Historical versions are available via git history._
