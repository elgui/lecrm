---
id: 1056
title: "[Fix] leCRM v0 — Test strategy and non-negotiable quality regression"
status: done
priority: p0
created: 2026-05-14
updated: 2026-05-14
done: 2026-05-14
category: engineering
group: lecrm-v0-sprint-3
group_order: 3
order: 2
remediates: 20260514-114210-9b41
---

## Remediation outcome

The previous task 9b41 was flagged "partial_success" because the automator's progress JSON
captured a snapshot before the commit landed. Verification confirms the deliverable was
actually committed:

- Commit `b38e310 docs(test-strategy): commit v0 test strategy + 4 non-negotiable categories (tasket 9b41)`
- Files committed: `docs/test-strategy.md` (247 lines, 19 944 bytes) + `.taskets/20260514-114210-9b41-*.md` frontmatter flipped to `done`.

### Structural checks against the remediation request

| Required | Status |
|---|---|
| `docs/test-strategy.md` exists | yes |
| In-scope vs out-of-scope sections | §3.1 / §3.2 |
| Four non-negotiable regression categories | §4 (tenant isolation ≥15, RBAC ≥30, JSONB metadata ≥8, OAuth lifecycle ≥10) |
| Tenant isolation coverage commitment | §4.a |
| RBAC coverage commitment | §4.b |
| Auth token lifecycle coverage commitment | §4.d |
| Tasket frontmatter status = done | yes |
| File + tasket update committed | yes, in `b38e310` |

### Build / test sanity

`go build ./...` and `go test ./...` in `apps/api` both clean (cached pass for `internal/auth`
and `internal/workspace`; remaining packages have no tests yet, expected at this sprint stage).
No new failures introduced — this remediation makes no code changes, only verifies prior work.
