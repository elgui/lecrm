---
id: 1120
title: "[Fix] Re-issue ADR-004 (sequences architecture) for Go + river stack"
status: done
priority: p2
created: 2026-05-28
updated: 2026-05-28
done: 2026-05-28
tags: [adr, sequences, v1-readiness, remediation]
category: project
group: lecrm-v1-readiness
group_order: 80
order: 1
remediates: 20260528-142535-c8a3
---

## Remediation outcome

The previous task `20260528-142535-c8a3` was flagged `partial_success` because the automator's progress JSON captured a snapshot before the commit landed. Verification confirms the deliverable was actually committed.

- Commit `a24f67a8 docs(adr): re-issue ADR-004 sequences architecture for Go + river stack`
- Files committed:
  - `docs/adr/ADR-004-rev2-sequences-architecture.md` (375 lines, new file)
  - `docs/adr/ADR-004-sequences-architecture.md` (superseded notice + forward pointer to rev 2)
  - `.taskets/20260528-142535-c8a3-*.md` frontmatter flipped to `done`

### Structural checks against the remediation request

| Required | Status |
|---|---|
| ADR-004 rev2 file exists | yes |
| ADR-004 rev1 marked superseded | yes |
| Cross-references to ADR-009 (stack), ADR-007 (audit catalogue), ADR-011 (connector seam) | yes |
| Brandur-style partial unique index on `(enrollment_id, step_index)` | yes |
| Four river jobs (enroll / send_step / poll_reply / finalize) documented | yes |
| Gmail-first reply detection + Brevo inbound parse catch-all | yes |
| Files staged + committed to main | yes — commit `a24f67a8` |

### Build / test sanity

No code changes — documentation-only commit. `git status` reports clean working tree. No new failures introduced; this remediation simply verifies prior work landed in git.

Remediates: `20260528-142535-c8a3`
