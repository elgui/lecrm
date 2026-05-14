---
id: 20260514-114224-5d12
title: "leCRM v0 — External-system-sync seam (Gmail as first instance of the pattern)"
status: later
priority: p1
created: 2026-05-14
updated: 2026-05-14
category: engineering
group: lecrm-v0-sprint-10
group_order: 10
order: 1
---

## Read this cold — full context inline

v0 must ship Gmail-sync (per the Exec Summary feature list). The naive path is to code Gmail-sync as a one-off integration. The architect-honest path is to ship Gmail-sync AS the first instance of an "external-system sync" pattern — making future connectors (Shopify per Exec Summary §What Makes This Special #2; whatever Léo's next client needs) additive rather than rewrites.

## Why this exists

From PRD step-02 round-2 (Winston, 2026-05-14):

> "Cost of not anticipating: the metadata engine and the event/automation layer are exactly where connectors plug in. If v0 hardcodes Gmail-sync as a one-off rather than as the first instance of an 'external-system sync' pattern, every future connector is a rewrite. Cheap insurance: define the abstraction at v0 even if Gmail is the only implementation."

Reinforced by Murat: "Connector scope is integration-boundary testing — contract tests, webhook reliability, rate-limit handling. They are an integration boundary, and that boundary needs its own test architecture."

## Prerequisite (DOR)

- API contracts framework in place (post-scaffolding). REST + thin MCP shapes defined for Contacts / Companies / Deals / Notes / Tasks.
- Gmail-sync feature work is scheduled (this tasket runs IN CONJUNCTION with the Gmail-sync implementation — not before, not after).
- Google OAuth tasket `20260514-114238-bf09` is on track (this tasket assumes the auth surface will be available).
- Secrets baseline tasket `20260510-162158-1023` provides per-tenant credential storage primitives.

## Approach

1. **Design the external-system-sync abstraction.** Cover:
   - Sync direction(s) — read-only (Gmail thread import), write-back (link contact ↔ thread; label/star), or bidirectional
   - Identity mapping — external-system entity ID ↔ leCRM record ID
   - Conflict resolution policy (external system wins / leCRM wins / surface conflict to user)
   - Rate limiting + retry policy
   - Webhook vs poll trigger mechanism
   - Per-tenant credential storage (overlaps with secrets baseline tasket `1023`)
   - Failure modes + observability surface

2. **Document the abstraction.** Either inline in `docs/adr/ADR-NNN-external-system-sync.md` (separate ADR if the shape feels stable) OR as a section in the v0 architecture doc. Either works — pick the one that better preserves the design across future read-pass sessions.

3. **Implement Gmail-sync using the abstraction.** Do NOT shortcut — Gmail-sync must consume the seam, not bypass it. The "but it's just one connector for now" temptation is exactly what this tasket exists to prevent.

4. **Exercise a second hypothetical connector on paper.** Walk through implementing a minimal Shopify connector using the same abstraction. If the abstraction requires bending to fit, refactor. The seam is only validated when a second instance fits cleanly without changes.

## Done When

- [ ] External-system-sync abstraction documented (in ADR or arch doc; either is fine)
- [ ] Gmail-sync implementation consumes the abstraction (verifiable: there is no Gmail-specific code path that bypasses the seam)
- [ ] Paper exercise: a 1-page sketch of how a hypothetical second connector (Shopify or chosen alternative) would plug into the same seam, with no abstraction changes needed
- [ ] Cross-link to `test-strategy-scope-doc` for the integration-boundary test architecture commitment

## References

- `{output_folder}/planning-artifacts/prd.md` — Exec Summary §What Makes This Special #2 (Tailorization speed; connector qualifier; v2 integration-boundary test architecture)
- Tasket `20260510-162158-1023` — secrets baseline (per-tenant credential storage)
- Tasket `20260514-114210-9b41` — test strategy doc (integration-boundary section)
- Tasket `20260514-114238-bf09` — G4 Google OAuth submission (provides the auth surface this seam consumes)
