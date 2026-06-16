---
id: 20260601-110828-736b
title: [Integrator gap] CSV import (contacts/companies/deals) with column mapping + dedup-on-import
status: done
priority: p1
created: 2026-06-01
updated: 2026-06-01
tags: [import, onboarding, integrator-gap, leo, data-migration]
category: project
group: lecrm-integrator-gap-closure
order: 2
plan: true
done: 2026-06-01
---

## Context

Anticipating Léo's HubSpot reflexes. His ChefCheffe work involved historical
**backfill / data reprise** (vault tasket #674 "backfill 2026 YTD orders") and
onboarding always starts with getting existing data in. leCRM today ships CSV
**export** (tasket `20260525-1008`) but has **no import path at all** — yet the
core pitch is "sortez de votre tableur / de HubSpot". You cannot onboard the ICP
(spreadsheet / Notion / Airtable / Pipedrive-Free / abandoned-HubSpot-Free crowd,
`docs/ICP-ARCHETYPE.md` filter #4) without import.

Scope note: ICP filter #4 deliberately avoids migrating a *live, deep* HubSpot
deployment in beta. This tasket is the **basic CSV import** for onboarding, not a
full HubSpot API migrator — keep it that way.

## Goal

A per-workspace **CSV import** for Contacts, Companies and Deals with column
mapping, a dry-run preview, dedup-on-import, and an error report.

## Steps

1. **Upload + parse CSV** (one entity type per import: contacts | companies | deals).
2. **Column mapping UI** — map CSV columns to core fields *and* existing custom
   property definitions for that workspace; remember the last mapping.
3. **Dry-run preview** — show N rows to be created vs matched-existing vs errored,
   before committing. Nothing writes until the user confirms.
4. **Dedup-on-import** — match existing records (contacts by email; companies by
   name/domain) and update instead of duplicating; configurable "create new vs
   skip vs update".
5. **Commit + error report** — import in a transaction-safe, idempotent batch;
   produce a downloadable report of skipped/errored rows with reasons.
6. **Tenant isolation** — writes only to the caller's workspace schema; emit an
   audit-log event per ADR-007 for the import batch.
7. Gate on `tsc`, `eslint`, `vitest` (web) + `go build ./apps/...` and a
   cross-tenant isolation test for the import handler.

## Done When

- [ ] User can upload a CSV, map columns (incl. custom properties), preview, and import contacts/companies/deals.
- [ ] Dry-run preview shows create/update/error counts and writes nothing until confirmed.
- [ ] Dedup-on-import matches existing records (email / name+domain) per the chosen policy.
- [ ] Import is idempotent and workspace-scoped (cross-tenant test green); audit event emitted.
- [ ] Downloadable error report lists skipped rows with reasons.
- [ ] tsc + eslint + vitest + go build green; commit scoped to import only.

## References

- Existing export: tasket `20260525-1008` (CSV export + go:embed)
- `docs/ICP-ARCHETYPE.md` filter #4 (scope guard: basic onboarding import, not deep HubSpot migration)
- ADR-007 audit catalogue (import batch event)
- ADR-010 / custom property definitions (mapping target)
- Pairs naturally with the dedup/merge tasket in this group (run import first).
