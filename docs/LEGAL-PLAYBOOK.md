# Legal Playbook — GB Consult Managed CRM SaaS
**Operator:** Guillaume / GB Consult, micro-entrepreneur, régime BNC services
**Service:** Managed SaaS hosting of Twenty CRM (AGPL-3.0 fork) on EU infrastructure
**Date:** May 2026 | **Version:** 1.0

> **How to use this document:** Sections marked **[LAWYER REQUIRED]** must be drafted or reviewed by a French avocat before client signature. Sections marked **[SELF-MANAGEABLE]** can be produced by Guillaume using the guidance below. The final two sections are the actionable Phase-0 checklists.

---

## Table of Contents

1. GDPR / DPA Structure
2. Sub-Processor Audit — Postmark (US) Flag
3. CGV / CGU for B2B SaaS in France
4. SLA Template
5. Beta vs Production Agreement Differences
6. Apporteur d'Affaires Contract — Leo / Vernayo
7. AGPL-3.0 Compliance — Practical SaaS Implementation
8. Insurance (RC Pro + Cyber)
9. Trademark / Brand Identity
10. HubSpot Solutions Partner Risk — Leo's Position
11. Micro-Entrepreneur Revenue Cap and Incorporation
12. EU VAT Cross-Border B2B SaaS
13. DPIA (Article 35 GDPR)
14. Phase-0 Lawyer Brief (one-page scope)
15. Self-Managed Checklist

---

## 1. GDPR / DPA Structure

**[LAWYER REQUIRED — core template; self-manageable once template exists]**

### Guillaume's Role

In this service model, the client (SMB) is the **data controller** — they determine the purpose and means of CRM data processing. GB Consult is the **data processor** — it operates the infrastructure on behalf of the controller. This triggers Article 28 RGPD (Regulation EU 2016/679), which mandates a written Data Processing Agreement (DPA) between controller and processor before any personal data is processed.

Source: [Article 28 RGPD — EUR-Lex](https://eur-lex.europa.eu/legal-content/FR/TXT/?uri=CELEX%3A32016R0679#d1e3602-1-1)

### Article 28(3) Mandatory Clauses Checklist

Every client DPA must include these eight elements (Article 28(3)(a)-(h)):

- [ ] **Instructions documented:** Process personal data only on documented instructions of the controller, including transfers outside the EU.
- [ ] **Confidentiality:** Persons authorised to process data are under a statutory or contractual obligation of confidentiality.
- [ ] **Security measures:** Implement all measures required under Article 32 (see below).
- [ ] **Sub-processor conditions:** Do not engage a sub-processor without prior specific or general written authorisation of the controller; include flow-down obligations.
- [ ] **Rights assistance:** Assist the controller in responding to data subject requests (access, rectification, erasure, portability, objection).
- [ ] **Compliance assistance:** Assist the controller with breach notifications (Articles 33-34), DPIAs (Article 35), and prior consultations (Article 36).
- [ ] **End-of-contract:** Delete or return all personal data at the controller's choice upon termination, unless EU/member-state law requires retention.
- [ ] **Audit rights:** Make available all information necessary to demonstrate compliance; allow audits and inspections by or on behalf of the controller.

### Article 32 Security Measures (practical SaaS list)

Document these in a DPA annex or Technical and Organisational Measures (TOMs) schedule:

- Encryption in transit (TLS 1.2+) and at rest (AES-256) for all client CRM data
- Separate namespaces/workspaces per client (no cross-client data access)
- Access control: MFA enforced for administrative access; principle of least privilege
- Automated daily backups with off-site replication; retention policy documented
- Vulnerability patching schedule (critical: 48h; high: 7 days; medium: 30 days)
- Incident response process: detection → containment → notification within 72h to controller
- Subprocessor vetting documented

### Article 30 Records of Processing Activities (ROPA)

As a processor, GB Consult must maintain a ROPA under Article 30(2). Minimum entries per client:

| Field | Example |
|-------|---------|
| Controller name & contact | Client SMB SARL, DPO if any |
| Categories of personal data | Contact data (name, email, phone), commercial history |
| Categories of data subjects | Client's own customers and prospects |
| Recipients / sub-processors | Hetzner DE (hosting), Postmark (transactional email) |
| International transfers | US transfer via SCCs (Postmark) |
| Retention period | Duration of contract + 1 year for backups |

Template: [CNIL — Guide du sous-traitant](https://www.cnil.fr/fr/les-outils-de-la-conformite/les-guides-de-la-cnil/guide-pour-les-sous-traitants)

### Sub-Processor Disclosure

The DPA must list sub-processors or grant a right of general authorisation with notification. Recommended approach for a small operator: **general authorisation** clause allowing Guillaume to add/change sub-processors with 30 days notice, and right for the client to object on reasonable grounds. Append a sub-processor schedule to the DPA.

### CNIL Resources

- [Outil de gestion des registres CNIL](https://www.cnil.fr/fr/les-outils-de-la-conformite/outil-de-pilotage-de-votre-conformite) — free ROPA template
- [Modèle de DPA CNIL](https://www.cnil.fr/fr/les-outils-de-la-conformite/les-guides-de-la-cnil/guide-pour-les-sous-traitants) — reference template for Article 28 clauses

---

## 2. Sub-Processor Audit — Postmark (US) Flag

**[SELF-MANAGEABLE with guidance below; flag for lawyer if client demands SCC review]**

### Current Status (verified May 2026)

**Postmark is NOT certified under the EU-US Data Privacy Framework (DPF).** Postmark (operated by Wildbit, now part of ActiveCampaign) uses Standard Contractual Clauses (SCCs) under Commission Decision 2021/914 as its sole legal mechanism for EU-to-US personal data transfers. Infrastructure is hosted in Chicago (Deft) and AWS US-East — no EU data centres exist, and Postmark has publicly stated no plans to add EU servers.

Source: [Postmark EU Privacy page](https://postmarkapp.com/eu-privacy); [Postmark DPA](https://postmarkapp.com/dpa)

### Why This Is a Flag

Transactional CRM emails (e.g., automated welcome emails, deal notifications) sent through Postmark will contain the email addresses of the client's contacts — personal data under RGPD. Routing that data through a US processor without DPF certification requires SCCs.

### Required Additional DPA Clauses if You Keep Postmark

1. Reference Postmark's DPA in your sub-processor schedule
2. In your client DPA annex, document: "Transactional email delivery: Wildbit LLC t/a Postmark, US, transfer mechanism: SCCs (Decision 2021/914, Module 3 — processor to sub-processor)"
3. Inform clients proactively that a US sub-processor is used for email delivery
4. Perform and document a Transfer Impact Assessment (TIA) — brief document assessing US FISA 702 / EO 14086 safeguards

SCC template: [European Commission — SCCs for controllers and processors](https://commission.europa.eu/publications/standard-contractual-clauses-controllers-and-processors-eueea_en)

### Recommended All-EU Alternatives

| Provider | Jurisdiction | DPF Certified | Transactional Email | Pricing (indicative) |
|----------|-------------|---------------|---------------------|---------------------|
| **Brevo (ex-Sendinblue)** | FR | Yes | Yes | Free to €25/mo (40k emails) |
| **Mailjet** | FR (Mailgun group) | Check current | Yes | Free to €15/mo |
| **Scaleway TEM** | FR | N/A (EU only) | Yes | ~€0.35/1000 emails |
| **Infobip** | EU | Yes | Yes | Volume pricing |

**Recommendation:** Switch to **Brevo** or **Scaleway TEM** to eliminate the US transfer entirely. This simplifies client DPAs and reduces CNIL exposure. Brevo offers a GDPR-native DPA, French support, and IS headquartered in Paris.

---

## 3. CGV / CGU for B2B SaaS in France

**[LAWYER REQUIRED — initial draft; self-manageable updates once template exists]**

### Legal Basis

B2B CGV are governed by Articles L441-1 to L441-17 of the Code de commerce. For SaaS, the CGV function as the master service agreement and must be communicated to any professional client who requests them (L441-1 II). Failure to include mandatory late-payment clauses exposes you to a €75,000 fine (L441-6).

Source: [Légifrance — Code de commerce L441-1](https://www.legifrance.gouv.fr/codes/article_lc/LEGIARTI000038567268)

### Mandatory Clause Checklist for SaaS CGV

**1. Identité du prestataire**
Mentions légales: full name (GB Consult / Guillaume [nom]), statut micro-entrepreneur, SIRET, URSSAF registration number, address, email. Required by [Loi pour la Confiance dans l'Économie Numérique (LCEN)](https://www.legifrance.gouv.fr/loda/id/JORFTEXT000000801164).

**2. Description du service**
What is included: managed CRM workspace, subdomain, hosting on EU infrastructure (Hetzner DE or OVH FR), setup, number of users, storage quota. Explicitly exclude what is not included (custom development, data migration labour, third-party API costs).

**3. Tarification et facturation**
- Monthly recurring fee (€99–€450 depending on plan)
- Setup fee (€0–€3,500, non-refundable upon service activation)
- Invoice issued at start of each billing period
- Payment terms: 30 days net from invoice date (Article L441-6, default for B2B)
- Accepted payment methods

**4. Pénalités de retard (MANDATORY — Article L441-6)**
Must appear in CGV and on every invoice:
- Rate: minimum 3× the legal interest rate (taux légal), or the ECB refinancing rate + 10 percentage points — whichever is higher. In practice, most B2B CGV use "3× le taux d'intérêt légal en vigueur"
- Due automatically on the day after the payment due date, without prior notice
- **Indemnité forfaitaire de recouvrement: €40** per late invoice — mandatory, cannot be waived (Article D441-5)
- Additional reimbursement of actual recovery costs if >€40, upon justification

**5. Durée, renouvellement, résiliation**
- Initial term: monthly or annual (state clearly)
- Auto-renewal: specify notice period (recommended 30 days)
- Termination for cause (material breach): 15-day cure period, then right to terminate
- Data export upon termination: provide 30-day export window, then secure deletion

**6. Propriété des données**
State explicitly: all CRM data remains the property of the client. GB Consult acts as processor only. GB Consult will not use client data for its own purposes. See DPA.

**7. Réversibilité**
Provide client data in a standard format (CSV, JSON export from Twenty CRM) within 30 days of contract end. This is increasingly expected for RGPD Article 20 (data portability) compliance.

**8. Responsabilité et limitation**
- Guillaume's liability capped at 12 months of fees paid in the 12 months preceding the claim
- Exclude indirect damages (loss of profit, loss of data beyond backup SLA) — standard in B2B SaaS
- No liability for force majeure, client-caused incidents, or sub-processor outages beyond contractual SLA credits

**9. Force majeure**
Reference Article 1218 of the Code civil. Define triggering events (natural disaster, cyberattack on infrastructure provider, government action, pandemic). Obligation to notify within 48h; service credits suspended during force majeure.

**10. Loi applicable et juridiction**
- Governing law: French law
- Jurisdiction: Tribunal de Commerce de [city] (your principal place of business)
- For B2B only — consumer jurisdiction rules do not apply

**11. Sous-traitance**
Reserve the right to use sub-processors; reference the DPA sub-processor list.

---

## 4. SLA Template

**[SELF-MANAGEABLE — use template below; lawyer review optional]**

### Realistic Uptime Commitment for Solo Operator

Do NOT promise 99.9% (8.7h downtime/year) without redundant infrastructure and on-call monitoring. Realistic commitments for a solo operator with Hetzner/OVH:

- **Standard SLA:** 99.5% monthly uptime (3.6h/month allowable downtime)
- **Enhanced SLA (negotiate per client):** 99.0% (7.2h/month)

### Uptime Calculation

> `Uptime % = ((Total minutes - Downtime minutes) / Total minutes) × 100`
> Scheduled maintenance windows (notified 48h in advance) excluded from downtime calculation.

### Service Credit Structure

| Monthly Uptime | Credit Applied to Next Invoice |
|----------------|-------------------------------|
| 99.0% – 99.5% | 5% of monthly fee |
| 95.0% – 99.0% | 10% of monthly fee |
| < 95.0% | 25% of monthly fee |

Credits are the sole remedy for downtime. No cash refund. Credits expire at contract end.

### Exclusions (Downtime Not Counted)

- Scheduled maintenance (notified 48h in advance; maximum 4h/month)
- Force majeure events
- Client-caused incidents (misconfiguration, credential compromise)
- Third-party sub-processor outages (hosting provider, transactional email)
- DNS propagation delays outside GB Consult control
- Client's own internet connectivity

### Response Times by Severity

| Severity | Definition | Initial Response | Target Resolution |
|----------|-----------|-----------------|-------------------|
| P1 — Critical | Full service down; all users affected | 2 hours (business hours) | 4 hours |
| P2 — Major | Significant functionality impaired; >50% users affected | 4 hours (business hours) | 8 hours |
| P3 — Minor | Limited functionality impaired; workaround available | 1 business day | 5 business days |
| P4 — Cosmetic | Minor issue, no operational impact | 3 business days | Best effort |

Business hours: 09:00–18:00 CET/CEST, Monday–Friday, excluding French public holidays.

### RPO / RTO Commitments

- **Recovery Point Objective (RPO):** 24 hours (daily backups)
- **Recovery Time Objective (RTO):** 4 hours for complete service restoration from backup
- Clients should maintain their own copies of critical data exports

### Data Export on Termination

Within 30 days of contract end, GB Consult will provide a full export of client CRM data in CSV/JSON format. After 30 days, data is securely deleted (certificate available on request).

---

## 5. Beta vs Production Agreement Differences

**[SELF-MANAGEABLE — use two-version approach below]**

### Why Two Versions?

Beta clients accept more risk in exchange for lower price and direct product influence. Using the production CGV with a beta client creates mis-aligned expectations and potential liability on SLA.

### Design Partner Agreement (1-2 clients, €99-149/mo, 6-month term)

| Clause | Design Partner Version |
|--------|----------------------|
| SLA | No uptime guarantee; best-effort only |
| Liability cap | Fees paid in the previous 3 months |
| Term | 6 months fixed; no early exit |
| Feedback | Minimum monthly 1h call; written feedback within 5 business days of request |
| Reference rights | GB Consult may use company name and logo on website/sales materials upon client approval |
| Case study | Client grants right to publish anonymised case study; named version requires written approval |
| Testimonial | Client agrees to provide one written testimonial within 6 months |
| Feedback IP | All feature requests and feedback submitted become GB Consult property (for roadmap use) |
| Conversion | At month 6, auto-converts to production pricing unless client opts out 30 days in advance |
| Explicit beta notice | "This is a beta service. Features may change without notice. Data should be backed up by the client independently." |

### Paying Beta Agreement (3-4 clients, €250-450/mo, 30-day cancel)

| Clause | Paying Beta Version |
|--------|-------------------|
| SLA | Reduced — 99.0% monthly uptime (vs 99.5% production) |
| Service credits | Capped at 10% of monthly fee (vs 25%) |
| Liability cap | Fees paid in previous 6 months |
| Cancellation | 30-day written notice, any time |
| Beta notice | Include explicit beta disclosure; feature set subject to change |
| Feedback | Encouraged but not contractually required |
| Reference | Subject to separate approval |

### Key Difference: No SLA Claims During Beta

Add explicit waiver clause: "During the Beta period, Client acknowledges the Service is in development. Client waives any right to service credits for outages attributable to development or deployment activities."

---

## 6. Apporteur d'Affaires Contract — Leo / Vernayo to GB Consult

**[LAWYER REQUIRED — this contract has significant legal and tax implications]**

### Critical Distinction: Apporteur d'Affaires vs Agent Commercial

This is the most important structural decision. Under French law:

**Agent commercial (Articles L134-1 to L134-17 Code de commerce):** Permanently mandated to negotiate and conclude contracts on behalf of the principal. Triggers mandatory indemnité de clientèle upon termination (typically 2 years of average annual commission). Registration at the Registre Spécial des Agents Commerciaux required.

**Apporteur d'affaires:** No statutory framework. Puts two parties in contact; does not negotiate or conclude contracts in the client's name. Intervention is punctual. NO termination indemnity. NOT required to register.

Source: [Légifrance — L134-1 Code de commerce](https://www.legifrance.gouv.fr/codes/article_lc/LEGIARTI000044056333); [Bpifrance — Intermédiaires du commerce](https://bpifrance-creation.fr/encyclopedie/structures-juridiques/statuts-particuliers/intermediaires-du-commerce-agent-commercial)

**For Leo: structure as apporteur d'affaires.** Leo introduces the prospect to Guillaume. Guillaume negotiates, signs, and invoices the client directly. Leo has no authority to commit GB Consult contractually. This avoids the agent commercial statute and its mandatory indemnification.

**Risk of misclassification:** If Leo regularly negotiates deal terms, accompanies the full sales cycle, and represents GB Consult externally, a court could reclassify the relationship as agent commercial — triggering the L134-12 termination indemnity. The contract and actual practice must be consistent.

### Required Contract Clauses

**Mission**
Leo (Vernayo, SIREN 932 848 815) introduces prospective clients to GB Consult. Mission is strictly limited to identification and initial introduction. Leo does not negotiate terms, sign contracts, or represent GB Consult.

**Commission Rate**
~30% of monthly recurring revenue (MRR) generated by introduced clients, payable monthly, 30 days after GB Consult receives payment from the client. Specify: commission ceases when client churns; no commission on setup fees (optional — document the choice).

**Valid Introduction Definition**
A "valid introduction" is one where: (a) Leo has provided the prospect's contact details and company name in writing before first contact by Guillaume, (b) the prospect becomes a paying client within 90 days of introduction, and (c) the prospect was not already known to GB Consult (define "known" — e.g., not in GB Consult's CRM at date of introduction).

**Exclusivity**
Non-exclusive: Leo may introduce clients to other service providers; GB Consult may use other introducers. This is appropriate given Leo's HubSpot role (see Section 10).

**Duration and Termination**
- Initial term: 12 months, auto-renewing
- 30-day written notice to terminate for convenience
- Immediate termination for cause (material breach, fraud, conflict of interest not disclosed)
- Commission tail: Leo receives commissions on introduced clients for 12 months after termination

**Confidentiality**
Leo must keep client identities, GB Consult pricing, and deal terms confidential. NDA provisions included. Survival: 3 years post-termination.

**Non-circumvention**
GB Consult will not directly contact Leo's own clients (those Leo introduced) to offer services without Leo's written consent for 12 months post-termination.

**Conflict of Interest**
Leo must disclose if an introduced prospect is also a client where Leo provides HubSpot services. GB Consult and Leo agree to manage such conflicts in good faith (e.g., Leo recuses from the negotiation phase).

**Intellectual Property**
Any sales materials created jointly remain the property of GB Consult. Leo may use anonymised case studies only with written approval.

**Tax Handling**
Leo (Vernayo, separate legal entity) invoices GB Consult for commissions. Leo's invoices must include:
- Vernayo SIREN/SIRET
- Leo's TVA number (if applicable — check Vernayo's TVA status)
- 20% TVA if Leo is subject to TVA (verify against Vernayo's franchise threshold)
- Nature of service: "Commission d'apport d'affaires — clients [period]"
GB Consult deducts commissions as a charge (note: micro-entrepreneur regime does not allow deduction of charges — commissions paid to Leo reduce GB Consult's gross revenue for accounting purposes but the micro-entrepreneur regime taxes Guillaume on gross receipts, not net).

**Leo's Name Does Not Appear on Client Bills**
Confirmed: client invoices are issued by GB Consult only. The apporteur d'affaires relationship is confidential from the client unless both parties agree to disclose.

---

## 7. AGPL-3.0 Compliance — Practical SaaS Implementation

**[SELF-MANAGEABLE with technical implementation; lawyer review for proprietary module strategy]**

### What AGPL-3.0 Requires for a SaaS Operator

The GNU Affero General Public License v3 adds a "network use" trigger (Section 13) to the standard GPL:

> "If you modify the Program, your modified version must prominently offer all users interacting with it remotely through a computer network an opportunity to receive the Corresponding Source."

Source: [GNU AGPL-3.0 full text](https://www.gnu.org/licenses/agpl-3.0.en.html)

**Key nuance:** Section 13 is triggered only by **modifications**. If you run Twenty CRM unmodified, strictly speaking the obligation is less clear — but the FSF's intent is that network users should have access to the source. In practice, **always publish your fork** to avoid any ambiguity and to build trust.

### Practical Compliance Checklist

**1. Maintain a public GitHub fork**
- Fork `github.com/twentyhq/twenty` under your own GitHub account (e.g., `github.com/gbconsult/twenty`)
- Push all modifications to this public repository
- Keep it up to date with upstream releases you deploy

**2. Source disclosure mechanism**
- Add a footer link on every page of the hosted CRM: "Powered by [Twenty CRM](https://twenty.com) — [Source code](https://github.com/gbconsult/twenty)"
- This satisfies the "prominent offer" requirement
- The link must be accessible without login (or include it in the login page)

**3. Modification publishing**
- Commit-by-commit is safest and aligns with FSF guidance
- At minimum: publish a snapshot matching each deployed version with a git tag (e.g., `v0.30.0-gbconsult-2026-05`)
- Include a CHANGES.md describing your modifications

**4. Attribution and copyright headers**
- Preserve all existing copyright headers in Twenty source files
- Add your own header for files you create: `// Copyright 2026 GB Consult — Licensed under AGPL-3.0`
- Do not remove Twenty HQ copyright notices

**5. Proprietary module separation (the "API strategy")**
- AGPL does not propagate to software that merely **uses** the application via its API (REST, GraphQL)
- You can build proprietary add-ons (billing portal, client dashboard, custom widgets) as separate services that call the Twenty API
- These add-ons do NOT need to be AGPL-licensed if they are not linked into the Twenty codebase
- Document this separation clearly; the line is: separate process, API calls only, no internal function calls to Twenty code

**6. "Powered by Twenty" trademark**
- "Twenty" is a trademark of Twenty HQ (twentyhq)
- The footer attribution is expected by Twenty HQ and consistent with the AGPL's attribution requirements
- Avoid implying you are Twenty HQ or that your service is the official Twenty cloud
- Use phrasing like "Managed CRM powered by Twenty" rather than "Twenty CRM" as your product name

**7. Real-world references**
- Plausible Analytics (AGPL): publishes source at github.com/plausible/analytics, footer link on hosted version
- Mastodon hosts: each instance runs AGPL code; operators publish forks or upstream link
- Mautic (GPL2): managed hosts provide source link in settings panel

### What You CANNOT Do

- Use Twenty's proprietary cloud features without a commercial licence (check Twenty's cloud offering terms)
- Remove the AGPL licence from any file you distribute
- Relicense Twenty code under a proprietary licence
- Use an AGPL "commercial exception" clause (Twenty has not granted one)

---

## 8. Insurance (RC Pro + Cyber)

**[SELF-MANAGEABLE — get quotes and purchase; no lawyer needed]**

### RC Professionnelle (Required)

As a micro-entrepreneur providing IT services, RC Pro is strongly recommended and will be demanded by some B2B clients in their procurement checklists.

**Recommended coverage:**
- Minimum: €500k per claim / €1M annual aggregate
- Target for SaaS with client data: €1M per claim / €2M annual aggregate
- Include: erreurs et omissions (E&O), RC exploitation, dommages immatériels (data loss, service interruption)

**French insurers for digital services micro-entrepreneurs (2026 indicative pricing):**

| Insurer | Entry Price (RC Pro) | Notes |
|---------|---------------------|-------|
| **Stello** | ~€15/month | Digital-native, fast online quote, good for IT |
| **Hiscox** | ~€175/year | Strong E&O coverage for tech |
| **Orus** | ~€12-20/month | Online, covers digital services |
| **AXA Pro** | ~€200-400/year | Traditional, some B2B clients prefer named insurer |
| **MMA** | ~€200-350/year | Traditional, good for professional services |

Source: [Stello RC Pro comparatif](https://www.stello.eu/articles/comparatif-assurances-rc-pro); [Hiscox via Coover](https://www.coover.fr/responsabilite-civile-pro/assureurs/hiscox)

### Cyber Insurance

Recommended once you have 3+ paying clients with live CRM data. Cyber covers:
- Data breach response costs (notification, forensics, credit monitoring)
- Ransomware / business interruption
- Third-party liability for client data breach caused by your infrastructure

**Pricing:** Cyber-specific add-on typically +10–30% on RC Pro premium, or ~€100–300/year standalone at micro-entrepreneur scale.

**Recommended approach:** Start with Stello or Hiscox bundled RC Pro + cyber option. Upgrade standalone cyber policy when annual revenue > €50k.

### Contract Clause Expectation from B2B Clients

Large SMB clients may ask you to prove RC Pro coverage. Add to your CGV: "GB Consult maintains professional liability insurance of at least €[1M] per claim. Certificate available upon written request."

---

## 9. Trademark / Brand Identity

**[SELF-MANAGEABLE — process is straightforward; defer filing until post-beta]**

### INPI Registration Process (2026)

1. Choose your mark (text, logo, or combined)
2. Select Nice Classification classes relevant to your service:
   - **Class 42:** Software as a service (SaaS), cloud computing, CRM software services
   - Optional Class 35: Business management software, CRM consulting
3. File online at [inpi.fr](https://www.inpi.fr) — electronic filing only
4. Pay fees:
   - **€190 for the first class** (2026 rate — electronic filing)
   - **+€40 per additional class**
   - Renouvellement (10 years): €290 first class + €40/class
5. Examination: INPI examines for absolute grounds (descriptiveness, genericness); publication for opposition (2-month window)
6. If no opposition: registration ~6 months after filing

Source: [INPI — Tarifs des procédures](https://www.inpi.fr/ressources/propriete-intellectuelle/tarifs-procedures-et-prestations-de-linpi); [INPI — Le déposant et le coût](https://www.inpi.fr/realiser-demarches/propriete-intellectuelle/deposant-et-cout-dune-marque)

### Cost-Benefit at Beta Scale

**Defer until post-beta (month 6+).** At Phase-0 with 1-2 clients, brand investment should be minimal. File when:
- You have a confirmed brand name that differentiates from "Twenty"
- You have 3+ clients and recurring revenue >€1k/month
- You have a logo worth protecting

**One-class filing (Class 42):** €190. Do this yourself — it is straightforward.

### Twenty Trademark Risk

The word mark "Twenty" belongs to Twenty HQ. Do not name your product "Twenty" or "Twenty [anything]". Choose a distinct product name for your managed service. "Powered by Twenty" in attribution is acceptable; using "Twenty" as your primary brand is not.

### Domain Strategy

- `.fr` domain: preferred for French B2B clients (trust signal)
- `.com`: good for international reach
- Recommendation: register both once brand name is confirmed (~€10-15/year each)

---

## 10. HubSpot Solutions Partner Risk — Leo's Position

**[SELF-MANAGEABLE — document the separation; no lawyer required unless HubSpot acts]**

### What the HubSpot Solutions Partner Agreement Actually Says (2026)

Based on the current HubSpot Solutions Partner Program Agreement (legal.hubspot.com/solutions-partner-program-agreement), the agreement includes an **explicit non-exclusivity clause:**

> "This Agreement does not create an exclusive agreement between you and us. Both you and we will have the right to recommend similar products and services of third parties and to work with other parties in connection with the design, sale, installation, implementation and use of similar services and products of third parties."

**There is no explicit clause prohibiting partners from introducing clients to competing CRM products.** The restrictions found in the 2026 agreement relate to: (a) not commercialising HubSpot program benefits, (b) not copying HubSpot's core features in their own products, and (c) non-solicitation of HubSpot employees.

Source: [HubSpot Solutions Partner Program Agreement](https://legal.hubspot.com/solutions-partner-program-agreement); [February 2026 Legal Update](https://community.hubspot.com/t5/HubSpot-Legal-Stuff/February-25-2026-Legal-Update-HubSpot-Solutions-Partner-Program/ba-p/1251747)

### Risk Assessment

**Contractual risk: LOW** based on current published terms. The non-exclusivity clause specifically permits working with competitors.

**Relationship risk: MEDIUM.** HubSpot account managers may pressure Leo informally if they perceive him as actively steering clients away from HubSpot. This is a business risk, not a legal one.

**Misuse risk: LOW** as long as Leo is documented as an introduceur only, not as representing HubSpot in any GB Consult context.

### Protective Measures

1. **Document Leo's role precisely** in the apporteur d'affaires contract: "Introducer introduces prospects to GB Consult independently of any other commercial relationships Vernayo may maintain."
2. **Leo should not pitch Twenty CRM during HubSpot engagements.** Keep the two commercial activities separate.
3. **No co-branded materials** mixing HubSpot and GB Consult / Twenty CRM.
4. **Periodic review:** HubSpot updates its partner terms periodically. Leo should review the agreement at each renewal. Next notable change: July 15, 2026 (new program unification and €400/month membership fee requirement).

---

## 11. Micro-Entrepreneur Revenue Cap and Incorporation

**[SELF-MANAGEABLE for monitoring; accountant or lawyer for incorporation decision]**

### 2026 Thresholds (Services BNC)

| Threshold | Amount | Consequence |
|-----------|--------|------------|
| **Plafond micro-entrepreneur (BNC services)** | **€83,600/year** | Exceeding for 2 consecutive years exits micro-entrepreneur regime |
| **TVA franchise base (services)** | **€37,500/year** | Must charge TVA from 1st day of month following breach |
| **TVA seuil de tolérance (services)** | **€41,250/year** | Must charge TVA immediately if breached |

Sources: [URSSAF — seuils 2026](https://www.autoentrepreneur.urssaf.fr/portail/accueil/sinformer-sur-le-statut/toutes-les-actualites/2026--modification-des-seuils-de.html); [TVA auto-entrepreneur 2026](https://www.portail-autoentrepreneur.fr/academie/statut-auto-entrepreneur/tva)

Note: The LFI 2026 proposed unifying TVA thresholds was ultimately **not adopted** — 2026 thresholds remain unchanged from 2025.

### Critical MRR Trigger Points

| MRR | Annual Run Rate | Action |
|-----|----------------|--------|
| €3,125/month | €37,500 | TVA franchise base threshold — start tracking carefully |
| €3,438/month | €41,250 | TVA seuil de tolérance — charge TVA immediately if breached |
| €6,967/month | €83,600 | Micro-entrepreneur ceiling — plan incorporation |

**At €99-450/month per client:** TVA threshold is reached at ~8-11 clients (at mid-price). This is realistic within 12-18 months. Plan for it now.

### When to Switch Legal Structure

Trigger: when revenue consistently exceeds €60k/year (2 years in succession exits micro-entrepreneur automatically) OR when TVA threshold is breached.

| Structure | Best for | Key advantage |
|-----------|----------|---------------|
| **EI (Entreprise Individuelle)** | Solo, simple | Slightly more than micro but still simple |
| **SASU** | Solo, growth planned | Limited liability, investor-ready, salary deductible |
| **EURL** | Solo, B2B services | Limited liability, gérant social status |

**Recommendation:** Plan a SASU incorporation once MRR reaches €4k/month. Engage an expert-comptable early (budget €1,200-2,000/year for a small SASU).

### Commission Deductibility Note

**IMPORTANT:** Under the micro-entrepreneur/BNC regime, Guillaume pays social charges on **gross receipts**, not net. The ~30% commission paid to Leo is NOT deductible under the micro regime. This erodes effective margin significantly. This is another argument for incorporating (SASU/EURL) once revenue grows — where charges (including Leo's commissions) are fully deductible expenses.

---

## 12. EU VAT Cross-Border B2B SaaS

**[SELF-MANAGEABLE once TVA is triggered — consult accountant for OSS setup]**

### Domestic French Clients (Most Common)

Until TVA threshold is breached: no TVA charged (mention "TVA non applicable, article 293 B du CGI" on invoices).

Once TVA threshold breached: charge 20% TVA on all services; collect and remit quarterly to DGFiP.

### EU B2B Clients Outside France (Reverse Charge)

For B2B clients registered for VAT in another EU member state:
- Do NOT charge French TVA
- Invoice mentions: "Autoliquidation — article 196 Directive TVA 2006/112/CE"
- Client self-accounts for local VAT (reverse charge / autoliquidation)
- This works only if client provides their valid EU VAT number (verify at [VIES](https://ec.europa.eu/taxation_customs/vies/))

### OSS (One Stop Shop)

OSS registration is relevant for **B2C** (consumer) cross-border services — not your use case (B2B only). For B2B SaaS with EU clients, the reverse charge mechanism applies. No OSS needed.

### Non-EU Clients (UK, CH)

- UK (post-Brexit): UK client applies UK VAT rules (likely reverse charge under UK rules if B2B); you invoice without French TVA; mention "outside EU VAT scope"
- Switzerland: Not in EU VAT area; reverse charge applies; invoice without TVA; client self-accounts per Swiss rules

---

## 13. DPIA (Article 35 GDPR)

**[SELF-MANAGEABLE assessment; CNIL consultation if DPIA confirms high risk]**

### When a DPIA Is Required

Article 35 RGPD requires a DPIA when processing is "likely to result in a high risk." The CNIL has published a list of processing operations requiring mandatory DPIA (délibération CNIL no. 2018-327).

For a managed B2B CRM:
- **Contact database of SMB clients' own customers:** Generally NOT high risk (standard B2B contact management)
- **If client processes sensitive data** (health information, political opinions, criminal data of their contacts): DPIA required for that client's workspace
- **Large-scale processing:** A 3-15 user CRM for an SMB is not "large scale"

**Practical conclusion:** A standard CRM deployment for French SMBs does NOT require a DPIA at your current scale. Document this assessment in writing.

**DPIA becomes relevant when:**
- A client processes health or financial data of their own consumers
- Number of data subjects processed exceeds hundreds of thousands
- Automated decision-making or profiling is implemented

Source: [CNIL — Analyse d'impact relative à la protection des données](https://www.cnil.fr/fr/cnil-direct/question/lanalyse-dimpact-relative-la-protection-des-donnees-aipd-quest-ce-que-cest); [CNIL — Liste des traitements nécessitant une AIPD](https://www.cnil.fr/fr/listes-traitements-avec-aipd-requise)

### Self-Assessment Template (include in onboarding)

For each new client, document:
- [ ] Does the client process sensitive data (health, finance, criminal)?
- [ ] Does the client process data at scale (>100k data subjects)?
- [ ] Does the service include automated profiling or decision-making?

If all "No": no DPIA required. If any "Yes": consult a lawyer and conduct a DPIA before go-live.

---

## 14. Phase-0 Lawyer Brief

**What to commission from a French avocat specialising in droit des nouvelles technologies / droit des affaires, for approximately €1,000–3,000 fixed fee:**

### Deliverables to Request

**Priority 1 — Must have before first paying client:**

1. **DPA template (Article 28)** — 4-6 page standalone document or annex to CGV
   - Includes: eight mandatory clauses, sub-processor schedule (Hetzner, email provider), TOM annex, international transfer mechanism (SCCs if using Postmark; none if Brevo/Scaleway)
   - Self-managed by Guillaume thereafter for new clients

2. **CGV / Conditions Particulières SaaS B2B** — 8-12 pages
   - Covers all mandatory clauses per Section 3 above
   - Two versions: (a) standard production CGV, (b) beta/design partner addendum
   - Lawyer drafts; Guillaume personalises plan names/prices using a bracketed template

3. **Contrat d'apporteur d'affaires — Vernayo / GB Consult** — 4-6 pages
   - Uses all clauses from Section 6
   - Critical: explicit "non-agent commercial" qualification, commission tail, conflict of interest clause

**Priority 2 — Within first 3 months:**

4. **Quick review of AGPL compliance strategy** — 1h consultation
   - Validate the "public fork + API separation" approach
   - Confirm proprietary module boundary

5. **Mentions légales / politique de confidentialité** — for the GB Consult service website
   - LCEN-compliant mentions légales
   - RGPD-compliant privacy policy (for prospecting and website data)

### Budget Allocation Suggestion

| Deliverable | Estimated Cost |
|-------------|---------------|
| DPA template | €300–500 |
| CGV (two versions) | €500–800 |
| Apporteur d'affaires contract | €400–600 |
| AGPL consult (1h) | €150–250 |
| Mentions légales / privacy policy | €150–250 |
| **Total** | **€1,500–2,400** |

Well within the €1–3k budget. Negotiate a fixed-fee package ("forfait documents fondateurs") with a tech/startup-focused avocat.

### How to Find the Lawyer

- **Bar association:** Barreau de Paris — specialisation "droit des technologies de l'information et de la communication"
- **Platforms:** Doctrine.fr, LegalPlace, Captain Contrat (for templates to complement lawyer work)
- **Networks:** France Digitale legal directory, TECH.ROCKS community

---

## 15. Self-Managed Checklist

**Items Guillaume can produce himself, without lawyer involvement:**

### Immediate (Before Beta Launch)

- [ ] **ROPA (Article 30):** Create the records of processing activities using CNIL's free online tool
- [ ] **Sub-processor list:** Draft and maintain spreadsheet (name, country, purpose, transfer mechanism)
- [ ] **TOMs document:** Write the Technical and Organisational Measures annex (use Section 1 security list above)
- [ ] **GitHub fork:** Create `github.com/[your-account]/twenty`, push your deployment, add footer attribution link
- [ ] **Source disclosure footer:** Add "Powered by Twenty CRM — Source code: [link]" to CRM interface
- [ ] **INPI trademark check:** Search your chosen brand name at [inpi.fr/recherche](https://data.inpi.fr/) to verify availability (free)
- [ ] **Insurance quotes:** Get 3 RC Pro quotes (Stello, Hiscox, Orus) — 30 minutes online

### Before First Invoicing

- [ ] **Mentions légales:** Add to your service website (template widely available; lawyer review optional)
- [ ] **Invoice template:** Include mandatory late payment penalty mention and €40 forfait recouvrement
- [ ] **SIRET on all documents:** Verify URSSAF registration; check SIRET status on [annuaire-entreprises.data.gouv.fr](https://annuaire-entreprises.data.gouv.fr)
- [ ] **TVA monitoring:** Set up a simple spreadsheet to track monthly revenue vs €37,500 TVA threshold

### Ongoing

- [ ] **Monthly ROPA updates:** Add new clients, update sub-processor list when changes occur
- [ ] **Incident log:** Maintain a log of any service incidents (date, duration, severity, resolution) — needed for RGPD breach assessment
- [ ] **Sub-processor notifications:** If you change email provider or hosting, notify clients 30 days in advance per DPA
- [ ] **Revenue tracking vs thresholds:** Monitor against €37,500 (TVA) and €83,600 (micro-entrepreneur cap)
- [ ] **HubSpot terms watch:** Check legal.hubspot.com/solutions-partner-program-agreement at each annual renewal for Leo's position
- [ ] **AGPL upstream sync:** Review Twenty HQ releases monthly; apply security patches within 48h

### Deferred (Months 4-12)

- [ ] **INPI trademark filing:** Once brand name confirmed and revenue > €1k/month MRR
- [ ] **DPIA screening:** For each new client onboarding, run the 3-question assessment in Section 13
- [ ] **OSS assessment:** If any EU non-French client comes in, confirm B2B reverse charge applies (likely yes)
- [ ] **Incorporation planning:** When MRR consistently exceeds €3,000/month, engage an expert-comptable

---

*This playbook reflects the legal and regulatory environment as of May 2026. It is not a substitute for advice from a qualified French avocat on specific facts. All figures marked "TO VERIFY" should be confirmed with official sources before reliance.*

*Key sources used: [Légifrance](https://www.legifrance.gouv.fr), [CNIL](https://www.cnil.fr), [URSSAF auto-entrepreneur](https://www.autoentrepreneur.urssaf.fr), [INPI](https://www.inpi.fr), [European Commission — SCCs](https://commission.europa.eu/law/law-topic/data-protection/international-dimension-data-protection/standard-contractual-clauses-scc_en), [GNU AGPL-3.0](https://www.gnu.org/licenses/agpl-3.0.en.html), [HubSpot legal.hubspot.com](https://legal.hubspot.com/solutions-partner-program-agreement), [Postmark EU Privacy](https://postmarkapp.com/eu-privacy)*
