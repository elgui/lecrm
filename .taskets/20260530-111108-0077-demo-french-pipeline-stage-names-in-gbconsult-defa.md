---
id: 20260530-111108-0077
title: "Demo: French pipeline stage names in gbconsult-default template (+ data-fix live demo)"
status: later
priority: p2
created: 2026-05-30
updated: 2026-05-30
tags: [demo, pipeline, i18n, first-impression, polish]
category: engineering
group: lecrm-demo-polish
group_order: 250
order: 2
plan: true
---

## Context

The demo pipeline renders stage names in ENGLISH (Discovery, Qualified, Proposal Sent, Negotiation, Closed-Won/Lost) while all deal titles and the ICP are French. Verified live 2026-05-30 (kanban + deal detail). Stages come from the `gbconsult-default` provisioning template (`core.lecrm_provision_workspace_with_registry(..., 'gbconsult-default')`).

## Why

A FR/EN mix reads as unfinished to a French CRM integrator and the French-SMB ICP. Leo is terminology-sensitive (LeoCollab: Won/Lost have distinct semantics). Cheap, high-leverage polish. Not a blocker.

## Approach

1. Rename `gbconsult-default` stages to French: Decouverte, Qualifie, Proposition envoyee, Negociation, then DECIDE keep single "Gagne/Perdu" vs SPLIT into Gagne + Perdu (Leo treats Won vs Lost as distinct; splitting is more correct but changes stage count - flag for Guillaume).
2. Update the template/registry so NEW workspaces get French stages.
3. Data-fix the EXISTING demo: renaming the template does NOT retro-update the live demo. UPDATE the demo schema `pipeline_stages` labels. KEEP stage IDs + sort order stable so deals keep `stage_id` (no FK break).
4. Re-verify kanban + Deals table show French stages, all 6 seeded deals stay in correct columns.

## Done When

- [ ] `gbconsult-default` seeds French stage names for new workspaces
- [ ] Live demo shows French stages; all 6 deals retain correct stage
- [ ] Won/Lost split decision recorded

## References

- `deploy/README.md` staging runbook (`lecrm_provision_workspace_with_registry`, `gbconsult-default`)
- Search migrations/templates for `gbconsult-default` + `pipeline_stages`
- Live host `51.77.146.49`, container `lecrm-postgres`, db `lecrm` (demo schema = `core.workspaces.role_name WHERE slug='demo'`)
- LeoCollab: Won/Lost semantics
