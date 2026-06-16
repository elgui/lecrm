---
id: 20260601-110828-76e8
title: [Integrator gap] Duplicate detection + record merge for contacts & companies
status: done
priority: p2
created: 2026-06-01
updated: 2026-06-01
done: 2026-06-01
tags: [dedup, merge, data-quality, integrator-gap, leo]
category: project
group: lecrm-integrator-gap-closure
order: 3
plan: true
---

## Context

Anticipating Léo's HubSpot reflexes. A large share of his ChefCheffe work was
**data-quality / dedup**: company dedup by Shopify ID with a secondary lookup
(vault tasket #1072), a documented multi-email "no-merge" rule (#1060), and
order↔deal consistency checks (#588). HubSpot integrators live in dedup/merge.

leCRM has **nothing** here today — the only "merge"/"dedup" taskets in the repo
refer to *slug* merging and the *Save-button* merge, not record data. Once import
(sibling tasket in this group) lands, duplicates are inevitable and this becomes
required.

## Goal

Duplicate **detection** + a manual **merge** flow for Contacts and Companies,
preserving all linked notes / activities / tasks / deals, with an audit trail.

## Steps

1. **Duplicate detection** — list likely duplicates per workspace:
   - contacts: exact email match + fuzzy on name,
   - companies: name + domain fuzzy match.
2. **Merge UI** — pick the surviving record; resolve conflicting fields
   (incl. custom properties) field-by-field; show what will be re-pointed.
3. **Re-point relations** — move notes, activities, tasks and deals from the merged
   record onto the survivor; never orphan a relation.
4. **Optional no-merge rule** — let the user mark two records as "distinct, never
   suggest again" (mirrors Léo's multi-email no-merge rule, vault #1060).
5. **Audit** — emit an audit-log event per ADR-007 capturing both record ids and
   the surviving id; the merge must be traceable.
6. **Tenant isolation** — operates only within the caller's workspace schema
   (cross-tenant test green).
7. Gate on `tsc`, `eslint`, `vitest` (web) + `go build ./apps/...`.

## Done When

- [ ] Duplicate list surfaces contact (email/name) and company (name/domain) candidates.
- [ ] User can merge two records, resolving field conflicts incl. custom properties.
- [ ] Notes / activities / tasks / deals are all re-pointed to the survivor (none orphaned).
- [ ] "Mark as distinct / never suggest" rule persists per workspace.
- [ ] Merge emits an audit event (both ids + survivor); workspace-scoped (cross-tenant test green).
- [ ] tsc + eslint + vitest + go build green; commit scoped to dedup/merge only.

## References

- Léo's HubSpot dedup practice: vault taskets #1072 (Shopify-id dedup), #1060 (no-merge rule), #588 (consistency)
- ADR-007 audit catalogue (merge event)
- Depends conceptually on the CSV import tasket in this group (duplicates arrive via import).
