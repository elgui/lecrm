---
id: 20260515-192005-8c13
title: leCRM v0 — Add metadata.property.upsert event to ADR-007 audit catalogue (Sprint 4)
status: done
priority: p2
created: 2026-05-15
updated: 2026-05-18
done: 2026-05-18
tags: [sprint-4, adr-007, adr-010, audit-log]
category: engineering
group: lecrm-v0-sprint-4
group_order: 4
order: 2
plan: true
---

## Read this cold — full context inline

Small ADR-007 follow-up: add `metadata.property.upsert` event to ADR-007 §3 audit catalogue, emitted on every successful `metadata.Set` per ADR-010 §5. Includes the fail-closed invariant. ADR-010 §TO RESOLVE-2.

## Why this exists

ADR-010 commits JSONB-primary metadata-engine with a `Set` operation (ADR-010 §5) that upserts the full custom-property bag per record. Every successful `Set` must emit an audit-log row per ADR-009 §7.2 (fail-closed: "A mutation that cannot be audit-logged must be rejected, not silently allowed"). Without an explicit event name in the ADR-007 catalogue, the emission is ad-hoc and risks (a) inconsistent naming across handlers, (b) missing fields, (c) the fail-closed invariant silently degrading to fail-open.

Priority p2 because it's a small documentation + small code-emission change. Bundled with Sprint 4 metadata-engine implementation work.

## Prerequisite (DOR)

- ADR-010 committed (commit `e875fb8`).
- ADR-007 §3 audit-log catalogue exists (`docs/adr/ADR-007-encryption-secrets-audit.md`).

## Approach

### A. ADR-007 §3 catalogue entry
Add a new row to the audit-event catalogue in `docs/adr/ADR-007-encryption-secrets-audit.md` §3:

| Event | Fields (in `payload` JSONB) | Emitted by |
|---|---|---|
| `metadata.property.upsert` | `parent_type` (string: `contact` or `deal`), `parent_id` (uuid), `property_keys` (string[]) | `apps/api/internal/metadata/Set` on every successful upsert |

### B. Code emission (couples with Sprint 4 metadata-engine implementation)
In `apps/api/internal/metadata/set.go` (created when the metadata engine is implemented):
1. Wrap the JSONB write + audit emission in a single Postgres transaction.
2. If the audit write fails, the entire transaction rolls back — the metadata upsert is rejected.
3. Test: revoke INSERT on `core.audit_log` mid-test, assert `metadata.Set` returns error and the row is NOT written.

## Done When

- [ ] ADR-007 §3 catalogue updated with `metadata.property.upsert` row
- [ ] `apps/api/internal/metadata/set.go` (when written in Sprint 4) emits the event in the same transaction as the JSONB write
- [ ] Fail-closed test: audit write failure rolls back the metadata write
- [ ] Cross-reference: ADR-010 §TO RESOLVE-2 marked addressed

## References

- ADR-010 §5 (metadata.Set signature), §TO RESOLVE-2
- ADR-007 §3 (audit-log catalogue location)
- ADR-009 §7.2 (audit-log fail-closed invariant)
- Sprint 4 sibling tasket (order=1) — provisioning function extension (provides the code surface)
