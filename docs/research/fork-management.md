# leCRM Fork Management: Twenty CRM Long-Running Fork Process

**Status:** Research artefact — 2026-05-10
**Scope:** Repeatable process for maintaining a shallow AGPL fork of [twentyhq/twenty](https://github.com/twentyhq/twenty) through 50+ bi-weekly releases over a 2-year horizon, solo operator, AI-augmented, ≤4 h/month rebase budget.

---

## 1. Branch Strategy

### Three options evaluated

**Option A — Long-lived fork main + per-release branches**
Track `upstream/main` as a remote. Our `main` is rebased from selected upstream semver tags. Per-release tracking branches (`upstream/v2.X`) give a stable diff base per sprint.

**Option B — Rolling rebase model**
Continuously rebase our patch commits on top of upstream `main`. This is the model used by `git-for-windows` and `microsoft/git`, documented by GitHub Engineering ([friend-zone fork management](https://github.blog/2022-05-02-friend-zone-strategies-friendly-fork-management/)). Proactive, clean linear history, but requires discipline every two weeks.

**Option C — Patch-overlay / vendor subtree**
Vendor upstream as a `git-subtree` or submodule; keep patches as a separate series applied on top. Conceptually clean but adds tooling complexity: `git subtree pull` still requires conflict resolution, and the subtree directory layout breaks Twenty's Yarn workspace assumptions.

### Recommendation: Option B — Rolling rebase, tag-gated

For a shallow fork (3–5 file modifications), rolling rebase is the lowest overhead. The [history-preserving variant](https://amboar.github.io/notes/2021/09/16/history-preserving-fork-maintenance-with-git.html) avoids force-push problems:

```bash
# 1. Fetch upstream tag
git fetch upstream --tags

# 2. Fake merge to join histories without altering tree
git merge -s ours -m "Merge upstream v2.X.Y (history join)" upstream/v2.X.Y
git tag base-before-v2.X.Y HEAD~1   # snapshot old base

# 3. Rebase our patches onto the new upstream tag
git rebase --onto upstream/v2.X.Y base-before-v2.X.Y main

# 4. Advance base tag
git tag -f upstream-base upstream/v2.X.Y
```

Patch subtree (`Option C`) is better when the fork has dozens of files changed and upstream structure is stable — not this situation. Git-subtree in a Yarn/Nx monorepo introduces non-obvious import resolution issues.

**Branch layout:**

```
main               ← our production branch (patches on top of upstream)
upstream/main      ← remote tracking of twentyhq/twenty main
upstream/v2.X.Y    ← upstream release tags fetched as refs
release/lecrm-*    ← our own release tags (see §5)
```

---

## 2. Patch Isolation

### Two sub-options evaluated

**Sub-option A — In-place modification of core files**
Edit `auth.module.ts`, `sso.module.ts`, etc. Small diffs; direct and readable. Risk: every upstream change to those files becomes a conflict surface.

**Sub-option B — Plugin/hook-based via DI override**
NestJS's dependency injection supports token-based provider substitution: `{ provide: AuthService, useClass: OurAuthService }`. You register an override in a module imported *after* the core module; NestJS last-writer-wins semantics apply. This means the core module file is untouched — our additions live entirely in `packages/twenty-server/src/engine/gbconsult/`.

NestJS custom providers ([docs.nestjs.com/fundamentals/custom-providers](https://docs.nestjs.com/fundamentals/custom-providers)) enable:

```typescript
// gbconsult/gbconsult.module.ts
@Module({
  imports: [CoreAuthModule],
  providers: [
    { provide: SSOService, useClass: GBConsultSSOService },
    { provide: RLSInterceptor, useClass: GBConsultRLSInterceptor },
  ],
  exports: [SSOService, RLSInterceptor],
})
export class GBConsultModule {}
```

The application root module imports `GBConsultModule` last, letting it shadow core providers. This is idiomatic in NestJS — the same pattern is used for test overrides (`overrideProvider()`). A single line in `app.module.ts` registers the override.

### Recommendation: Sub-option B, with a minimal anchor in core

Keep exactly **one** touch-point in Twenty core: a one-line import of `GBConsultModule` in `app.module.ts`. This file rarely changes in upstream. All SSO logic, RLS interceptors, audit hooks, and enterprise gate stubs live in `src/engine/gbconsult/` — never touching upstream files again.

Conflict surface is reduced to `app.module.ts` (near-zero churn) vs. four files per release (current state).

---

## 3. Rebase Cadence and Trigger Criteria

### When to adopt upstream

| Trigger | Action | Urgency |
|---|---|---|
| Security CVE in Twenty's deps | Rebase immediately; patch within 72 h | Critical |
| Security CVE in upstream NestJS/TypeORM | Adopt Twenty's fix release | High |
| Breaking schema migration affecting our objects | Adopt; test migration path | High |
| Feature our roadmap needs | Adopt next tag after we validate | Normal |
| Routine bi-weekly release — no overlap with our files | Defer; batch monthly | Low |

### Real cost of being 3–6 months behind

For a shallow fork on internal-only files:

- **3 months behind:** Manageable. Security patch backporting is the main risk. Upstream bug fixes you'd want accumulate (tech debt).
- **6 months behind:** Upstream architectural refactors may have touched `app.module.ts`, `workspace.module.ts`, or auth paths even if our *file list* didn't change. At Twenty's pace (~4 releases/month), 6 months = ~24 releases. Diff-blast at catch-up time balloons past 4 h.
- **Recommended lag:** Stay ≤8 weeks behind (4 releases). Monthly rebase sprint: fetch last 2 upstream tags, rebase, run `pnpm test`, ship.

### Monthly rebase script (automation target)

```bash
#!/bin/bash
UPSTREAM_TAG=$(git tag -l 'v*' --sort=-v:refname | head -1)  # latest upstream tag
git fetch upstream --tags
git rebase --onto $UPSTREAM_TAG upstream-base main
pnpm test --workspace twenty-server
```

Budget: ~2 h conflict resolution + 1 h test triage + 0.5 h deploy = 3.5 h/month average for 2-file conflict surface.

---

## 4. Module-Placement Decision Tree

```
NEW FEATURE REQUEST
│
├─ Does it require modifying Twenty's DB schema (new tables, columns)?
│   └── YES → Option (a): Fork core — add migration + metadata entity
│                         (keep in gbconsult/ using Twenty's entity extension API)
│
├─ Is it purely about auth, billing gating, or audit logging?
│   └── YES → Option (a): Fork core — DI override in GBConsultModule
│
├─ Is it a new CRM object, workflow, or UI panel a customer defines?
│   └── YES → Option (b): twenty-sdk extension package
│               (defineObject, defineField, defineLogicFunction)
│               No fork files touched. Ships as a workspace package.
│
├─ Is it a standalone AI agent, chatbot interface, or async processor?
│   └── YES → Option (c): Separate microservice
│               Communicates over Twenty's GraphQL API or REST webhooks.
│               Zero coupling to fork lifecycle.
│               Examples: AI enrichment agent, WhatsApp/Telegram connector,
│               email summarizer, Chauvé-79 booking bridge.
│
└─ Is it a cross-cutting concern (observability, rate-limiting, tenancy)?
    └── YES → Option (a) DI middleware/interceptor via GBConsultModule,
              OR Option (c) API gateway layer — depends on latency requirements.
```

**Rule of thumb:** If the code would change behavior for ALL Twenty workspaces, it belongs in (a). If it enriches or extends data for ONE customer's workspace, it's (b). If it never needs Twenty's internal state (only public API), it's (c).

---

## 5. Versioning Scheme

### Recommendation: upstream build metadata suffix

Pattern: `twenty-{upstream_semver}+lecrm.{N}`

Examples:
- `twenty-2.4.1+lecrm.0` — first leCRM release on top of Twenty 2.4.1
- `twenty-2.4.1+lecrm.3` — third patch iteration on that base
- `twenty-2.6.0+lecrm.0` — after adopting upstream 2.6.0

This follows the SemVer 2.0 build metadata spec (`+`). Build metadata does not affect version precedence ordering. Tools can parse it straightforwardly.

### Precedent: Forgejo

Forgejo uses `X.Y.Z+gitea-A.B.C` — their own semver before the `+`, upstream Gitea version as metadata ([forgejo.org/docs/next/user/versions](https://forgejo.org/docs/next/user/versions/)). This is the clearest real-world precedent: downstream version is primary, upstream tracking is secondary metadata.

### Git tags

```
git tag twenty-2.4.1+lecrm.0
git tag twenty-2.4.1+lecrm.1   # hotfix on same upstream base
git tag twenty-2.6.0+lecrm.0   # after upstream rebase
```

Docker images: `ghcr.io/gbconsult/lecrm:twenty-2.6.0-lecrm.0` (Docker doesn't accept `+` in tags; replace with `-`).

---

## 6. AGPL Compliance Publishing

Twenty is licensed AGPLv3. Our fork is a modified work and must be published under AGPLv3 ([gnu.org/licenses/agpl-3.0.en.html](https://www.gnu.org/licenses/agpl-3.0.en.html)). The network-use clause means any user interacting with leCRM over a network is entitled to the source. A public GitHub repo satisfies this obligation, but the presentation matters.

### Compliance checklist

- [ ] **LICENSE file** — AGPL-3.0 text verbatim at repo root. Do not modify.
- [ ] **NOTICE file** — List upstream copyright holders: `Copyright (c) Twenty, Inc.` + any other upstream NOTICE entries. Add our own: `Modifications Copyright (c) GB Consult SARL`.
- [ ] **README.md — upstream attribution section:**
  ```
  ## License & Attribution
  leCRM is a modified fork of [Twenty CRM](https://github.com/twentyhq/twenty),
  licensed under AGPLv3. Source code for all modifications is available at
  [this repository]. See LICENSE and NOTICE for details.
  ```
- [ ] **Footer in UI** — Any leCRM UI served over the network must display "Powered by Twenty CRM (AGPL-3.0) — source: [link]" or equivalent. AGPL §13 requires "Appropriate Legal Notices" to be displayed.
- [ ] **No obfuscation of `/* @license Enterprise */` headers** — These mark Twenty's commercial-licensed files. Keep them intact; do not mix with AGPL-covered code.
- [ ] **CHANGELOG or diff link** — Document what changed from upstream. A pinned `CHANGES.md` or GitHub comparison link (`twentyhq/twenty...gbconsult/lecrm`) satisfies the "prominent notice" requirement.
- [ ] **Source always accessible** — The public repo must not be taken private or deleted while the service is running. If repo is archived, mirror on an alternative host.

### What AGPL does NOT require

- You do not have to contribute patches upstream (no copyleft obligation to contribute back — only to publish).
- You do not have to publish customer data or configs, only source code.
- You do not need a special "AGPL compliance page" — a public repo link in the product is sufficient.

---

## 7. CLA-Ratchet Contingency

Twenty's current license is dual: AGPLv3 for most files, commercial for `@license Enterprise` files. The LICENSE file examined contains **no CLA reference** — contributors submit under AGPL terms. However, the commercial license and YC-backed status create non-zero tail risk of a future relicense (BSL, SSPL, or full proprietary).

### Playbook

**Trigger:** Twenty announces a license change affecting AGPLv3-covered files.

**Step 1 — Freeze fork at last AGPL tag.**
```bash
git tag lecrm-agpl-freeze $(git log --oneline upstream/main | grep "last-agpl-release" | awk '{print $1}')
git checkout -b lecrm-agpl-frozen lecrm-agpl-freeze
```

**Step 2 — Evaluate the new license scope.** BSL typically has a "Change Date" (e.g., 4 years) after which it converts to Apache 2.0. If the change date is acceptable, staying on frozen code for that period may be viable.

**Step 3 — Maintenance-only mode on frozen branch:**
- Security vulnerabilities in frozen upstream: backport patches manually (NVD monitoring, CVE feeds)
- Dependency updates (TypeORM, NestJS, Passport): apply directly to frozen branch via `pnpm update`
- Estimated cost: +2 h/month for security triage; +8 h/month if upstream has active CVE patching

**Step 4 — Migration path if frozen branch becomes untenable (>18 months):**
- Evaluate Twenty competitors with clean AGPL: EspoCRM, SuiteCRM (both GPL)
- Or: commission custom migration of our `gbconsult/` layer onto a new base (our DI override pattern minimizes coupling cost)

### Precedent: OpenTofu/Terraform

HashiCorp switched Terraform from MPL 2.0 to BSL in August 2023. Within 6 weeks, OpenTofu forked from the last MPL tag (1.5.7) and entered the Linux Foundation. The fork has remained maintained and is now a CNCF project. Key lesson: a well-resourced community can keep a frozen fork alive, but a solo operator cannot match that pace. Our mitigation is the DI isolation pattern — switching CRM base is a defined engineering task, not an unknown.

---

## 8. Real-World Precedents

### Forgejo (hard fork of Gitea)

- Started as a soft fork: weekly cherry-pick of Gitea commits onto Forgejo's branch.
- Became a hard fork in February 2024 ([forgejo.org/2024-02-forking-forward](https://forgejo.org/2024-02-forking-forward/)).
- Key lesson: The soft-fork cherry-pick model worked well for 2+ years when patches were shallow. It became unsustainable only when architectural divergence grew. For leCRM's 3–5 file patch surface, the soft-fork model (equivalent to our Option B rebase) is appropriate for the full 2-year horizon.
- Versioning: `X.Y.Z+gitea-A.B.C` — adopted directly as our scheme.

### Vaultwarden (compatibility layer on Bitwarden)

- Not a fork of Bitwarden source — an independent reimplementation of the Bitwarden API in Rust.
- License evolution: moved from GPLv3 to AGPLv3 proactively to close a commercial-use loophole ([github.com/dani-garcia/vaultwarden/discussions/2450](https://github.com/dani-garcia/vaultwarden/discussions/2450)).
- Lesson for us: AGPLv3 is the correct license to prevent managed-service exploitation without copyleft backfire. Vaultwarden's model also shows that an API-compatibility layer (our Option C for some features) sidesteps fork maintenance entirely.

### Cal.com → Cal.diy split

- Cal.com moved to closed-source in 2026 ([thenewstack.io, 2026](https://thenewstack.io/cal-com-codebase-security-ai/)). Community forked as Cal.diy (MIT).
- Their pain: "the codebase powering Cal.com's hosted platform had begun to diverge from the publicly available codebase." This is the open-core drift problem.
- Lesson: Never let our fork grow enterprise features that we can't afford to maintain independently. Keep (b) and (c) paths healthy so our customizations aren't locked to Twenty's internals.

### OpenTofu (fork of Terraform at MPL freeze point)

- Forked Terraform 1.5.7 after HashiCorp BSL announcement. Entered Linux Foundation within 6 weeks.
- Terraform 1.5.7 reached upstream EOL (January 2024), but OpenTofu provides its own patch stream.
- Lesson for freeze strategy: a solo operator cannot replicate OpenTofu's community-driven security maintenance. Our freeze contingency is valid for ≤18 months; after that, migration is cheaper than maintaining a dead upstream.

### Die Antwort — git-tricks for long-lived forks

A practical post ([die-antwort.eu/techblog/2016-08-git-tricks-for-maintaining-a-long-lived-fork](https://die-antwort.eu/techblog/2016-08-git-tricks-for-maintaining-a-long-lived-fork/)) documents atomic patch commits and `git rebase --onto` workflows used in production for multi-year forks. Core insight: **atomic, single-purpose commits** in our patch stack make conflict triage linear rather than combinatorial. Each of our 3–5 patch commits should be a single concern (e.g., one commit for SSO override, one for RLS interceptor, one for audit hook).

---

## Sources

- [GitHub: Friend-zone strategies for fork management](https://github.blog/2022-05-02-friend-zone-strategies-friendly-fork-management/)
- [History-preserving fork maintenance with git (amboar.github.io)](https://amboar.github.io/notes/2021/09/16/history-preserving-fork-maintenance-with-git.html)
- [Forgejo: Forking forward (hard fork announcement)](https://forgejo.org/2024-02-forking-forward/)
- [Forgejo versioning docs](https://forgejo.org/docs/next/user/versions/)
- [NestJS Custom Providers](https://docs.nestjs.com/fundamentals/custom-providers)
- [AGPL-3.0 license text (GNU)](https://www.gnu.org/licenses/agpl-3.0.en.html)
- [Vaultwarden AGPL relicensing discussion](https://github.com/dani-garcia/vaultwarden/discussions/2450)
- [Cal.com goes closed-source](https://thenewstack.io/cal-com-codebase-security-ai/)
- [OpenTofu: What is it (2026)](https://scalr.com/learning-center/what-is-opentofu/)
- [HashiCorp BSL announcement](https://discuss.hashicorp.com/t/hashicorp-projects-changing-license-to-business-source-license-v1-1/57106)
- [Git subtree vs submodule (Atlassian)](https://www.atlassian.com/git/tutorials/git-subtree)
- [Merging vs. Rebasing (Atlassian)](https://www.atlassian.com/git/tutorials/merging-vs-rebasing)
- [SemVer 2.0.0 spec](https://semver.org/)
- [Forgejo semver docs](https://forgejo.org/docs/v1.19/user/semver/)
- [Twenty CRM GitHub](https://github.com/twentyhq/twenty)
- [FOSSA: AGPL License 101](https://fossa.com/blog/open-source-software-licenses-101-agpl-license/)
- [AGPL common misconceptions (danb.me)](https://danb.me/blog/common-agpl-misconceptions/)
- [Git tricks for long-lived forks (die-antwort.eu)](https://die-antwort.eu/techblog/2016-08-git-tricks-for-maintaining-a-long-lived-fork/)
