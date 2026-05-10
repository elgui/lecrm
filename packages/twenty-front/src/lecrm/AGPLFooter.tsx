/*
 * leCRM patch — AGPL §13 footer (AGPL-3.0)
 *
 * Renders the AGPL §13 "Appropriate Legal Notices" required for an
 * interactive program operated as SaaS. Must appear on every page
 * served over the network.
 *
 * The link points to the public source repository. Future revision:
 * resolve the running version at runtime by hitting `/api/version`
 * (exposed by `gbconsult/version/version.controller.ts`) and append
 * the patch revision so the footer surface tracks the deployed tag.
 *
 * Mounting status:
 *   This component is NOT yet wired into Twenty's React app shell.
 *   A follow-up sub-tasket will mount it through Twenty's extension API
 *   (preferred: twenty-sdk app extension) or, if no extension hook is
 *   available, via a minimal touch on the layout component
 *   (cost: one additional file touched in the upstream tree, weighed
 *   against the "single touchpoint" budget of ADR-002 §2).
 *
 * The footer copy is the strict AGPL §13 wording approved per the
 * legal playbook (private project) — do not change without a paired
 * legal review.
 */

import { type CSSProperties, type ReactElement } from 'react';

const FOOTER_STYLE: CSSProperties = {
  position: 'fixed',
  bottom: 0,
  left: 0,
  right: 0,
  padding: '4px 12px',
  fontSize: '11px',
  fontFamily: 'system-ui, -apple-system, sans-serif',
  color: 'var(--text-tertiary, #999)',
  background: 'var(--background-secondary, rgba(255, 255, 255, 0.9))',
  borderTop: '1px solid var(--border-color-light, #eee)',
  textAlign: 'center',
  zIndex: 1000,
  pointerEvents: 'auto',
};

const LINK_STYLE: CSSProperties = {
  color: 'var(--text-tertiary, #999)',
  textDecoration: 'underline',
};

export const LECRM_SOURCE_URL = 'https://github.com/elgui/lecrm';

export function AGPLFooter(): ReactElement {
  return (
    <div role="contentinfo" aria-label="AGPL §13 source attribution" style={FOOTER_STYLE}>
      Powered by{' '}
      <a
        href="https://github.com/twentyhq/twenty"
        rel="noopener noreferrer"
        target="_blank"
        style={LINK_STYLE}
      >
        Twenty CRM
      </a>{' '}
      (AGPL-3.0) — source:{' '}
      <a
        href={LECRM_SOURCE_URL}
        rel="noopener noreferrer"
        target="_blank"
        style={LINK_STYLE}
      >
        github.com/elgui/lecrm
      </a>
    </div>
  );
}

export default AGPLFooter;
