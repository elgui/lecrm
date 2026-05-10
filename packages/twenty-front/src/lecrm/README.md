# `lecrm/` — front-end patch directory

Mirror of the server-side `packages/twenty-server/src/engine/gbconsult/` pattern, applied to the React front-end. Components added here are leCRM customisations that must be mountable without modifying upstream Twenty front-end files (per ADR-002 §2 single-touchpoint rule).

## Current contents

- `AGPLFooter.tsx` — AGPL §13 attribution footer. Renders on every page once mounted.

## Mounting strategy (to resolve)

Twenty's React app shell does not yet expose an officially-documented "global footer" extension hook. Options being weighed:

1. **`twenty-sdk` app extension.** Preferred. Wraps the front-end in a leCRM-owned extension that mounts `AGPLFooter` via the SDK's app hooks. Zero upstream-file touches. Requires authoring a small `twenty-sdk` package (`packages/twenty-sdk-lecrm/` or similar) — moderate complexity, ~1 day of work.

2. **Minimal layout touch.** Add a single `import` + `<AGPLFooter />` line in Twenty's top-level layout component. Costs: one additional upstream file in the rebase conflict surface (currently zero in the front-end). Quickest to implement but trades off rebase hygiene.

3. **Server-side HTML injection.** Intercept `index.html` in the NestJS `ServeStaticModule` middleware and inject a footer `<div>` with hardcoded styles. Avoids React entirely — purely cosmetic but legally compliant. Hacky.

Decision deferred to the v0-build sub-tasket; option 1 is the target, option 2 is the fallback if the twenty-sdk authoring expands the timeline.
