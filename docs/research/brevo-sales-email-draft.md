# Brevo Sales Email Draft — leCRM Architecture Questions

**Send to:** sales@brevo.com (or via https://www.brevo.com/contact/)
**From:** Guillaume (hewindmindt@gmail.com)
**Context:** Pre-sales technical questions before subscribing to Brevo for a multi-tenant SaaS CRM platform (leCRM — a Twenty CRM fork). EU-based, targeting French SMBs. Phase 1 volume: ~5k emails/month. Phase 3 target: ~150k emails/month.

---

## Version anglaise / English version

**Subject:** Pre-sales technical questions — inbound parse & sub-accounts for SaaS CRM (leCRM)

Hi Brevo Sales team,

I am building leCRM, a multi-tenant SaaS CRM targeting French SMBs, built on top of the open-source Twenty CRM platform (NestJS/React, hosted on EU infrastructure). I am evaluating Brevo as our transactional email and inbound parse provider and have two specific technical questions I could not resolve from your public documentation before we sign up.

**Question 1 — Inbound Parse Webhook plan tier**

Your developer documentation for inbound parse webhooks (https://developers.brevo.com/docs/inbound-parse-webhooks) is technically thorough, but it does not specify which plan tier (Starter, Business, Enterprise) grants access to this feature. The inbound parse webhook is the lynchpin of our v1 reply-detection architecture for email sequences: we route a `replies.clientdomain.com` subdomain to `inbound1.sendinblue.com` / `inbound2.sendinblue.com`, and the `inboundEmailProcessed` webhook fires on each reply, allowing us to halt sequences for interested prospects.

Specific questions:
- Is inbound parse available on Starter or Business plans, or does it require Enterprise?
- Is there a cap on the number of receiving domains, or on inbound messages per month?
- Is there a documented reliability SLA for webhook delivery?
- Are there plans to document these limits in your pricing matrix?

**Question 2 — Sub-accounts / per-tenant API key isolation**

We host 5-20 client tenants on a shared Brevo account. Each tenant authenticates their own sending domain (DKIM/SPF/DMARC) via your domain API. Your help center confirms sub-account management (one Admin account, N child sub-accounts with isolated API keys, suppression lists, and sending stats) is available to "Enterprise clients."

Specific questions:
- What is the minimum Enterprise contract price for a company at our volume (5k–150k emails/month, growing over 12 months)?
- Is there a minimum number of sub-accounts or a minimum commitment period for Enterprise?
- Without Enterprise: on a standard Business plan with multiple authenticated sender domains, are API keys scopeable per domain or per sender? Or do all API keys share account-level access?
- Is there an agency/SaaS reseller program that would suit a multi-tenant CRM platform?

Our GDPR requirement: a single Data Processing Agreement covering all tenant email traffic under one Brevo account. Please confirm the DPA applies to sub-accounts under the same Admin account.

I am happy to schedule a call. Our timeline is to onboard our first paying clients in Q3 2026.

Thank you,
Guillaume
leCRM / GB Consult
hewindmindt@gmail.com

---

## Version francaise / French version

**Objet :** Questions techniques pré-vente — inbound parse & sous-comptes pour un SaaS CRM multi-tenant (leCRM)

Bonjour,

Je développe leCRM, un CRM SaaS multi-tenant destiné aux PME françaises, construit sur la base open-source Twenty CRM (stack NestJS/React, hébergement en UE). J'évalue Brevo comme fournisseur d'envoi transactionnel et d'analyse inbound, et j'ai deux questions techniques précises que je n'ai pas pu résoudre à partir de votre documentation publique avant de souscrire.

**Question 1 — Plan requis pour l'inbound parse webhook**

Votre documentation développeur pour l'inbound parse (https://developers.brevo.com/docs/inbound-parse-webhooks) est techniquement complète mais ne précise pas quel plan tarifaire (Starter, Business, Enterprise) donne accès à cette fonctionnalité. L'inbound parse est la brique centrale de notre architecture v1 de détection de réponses pour les séquences email : on délègue un sous-domaine `replies.clientdomain.com` vers `inbound1.sendinblue.com` / `inbound2.sendinblue.com`, et le webhook `inboundEmailProcessed` se déclenche à chaque réponse.

Questions précises :
- L'inbound parse est-il disponible sur les plans Starter ou Business, ou nécessite-t-il un plan Enterprise ?
- Y a-t-il une limite sur le nombre de domaines de réception, ou sur le volume de messages entrants par mois ?
- Existe-t-il un SLA de fiabilité documenté pour la livraison des webhooks ?

**Question 2 — Sous-comptes / isolation API par tenant**

Nous hébergeons 5 à 20 clients sur un compte Brevo partagé. Chaque client authentifie son propre domaine d'envoi (DKIM/SPF/DMARC) via votre API. Votre documentation précise que la gestion des sous-comptes (un compte Admin, N sous-comptes avec clés API isolées, listes de suppression et statistiques séparées) est réservée aux clients Enterprise.

Questions précises :
- Quel est le prix minimum d'un contrat Enterprise pour un volume de 5k à 150k emails/mois en croissance sur 12 mois ?
- Y a-t-il un nombre minimum de sous-comptes ou une durée d'engagement minimum pour accéder à l'offre Enterprise ?
- Sans Enterprise : sur un plan Business avec plusieurs domaines authentifiés, les clés API sont-elles limitables par domaine ou par expéditeur, ou ont-elles toutes un accès global au compte ?
- Existe-t-il un programme revendeur ou agence adapté à une plateforme CRM multi-tenant ?

Exigence RGPD : un DPA unique couvrant le trafic email de tous nos clients sous un seul compte Brevo. Pouvez-vous confirmer que le DPA s'applique aux sous-comptes rattachés au même compte Admin ?

Je suis disponible pour un appel. Notre calendrier prévoit l'onboarding des premiers clients payants en Q3 2026.

Cordialement,
Guillaume
leCRM / GB Consult
hewindmindt@gmail.com

---

## Notes for Guillaume

- **Send both versions** in the same email body, or send French first and mention the English version is below.
- If using the in-app contact form, select "Enterprise / Sales" as the inquiry type to route to a human rather than a bot.
- Follow up after 5 business days if no response.
- Key items to get in writing (not just verbal): plan tier for inbound parse, Enterprise pricing floor, DPA scope for sub-accounts.
