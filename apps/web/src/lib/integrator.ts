import type { AccessibleWorkspace } from './types';

export interface IntegratorContext {
  /** True when the signed-in user administers the current workspace as an integrator. */
  isIntegrator: boolean;
  /** Slug of the client workspace currently being administered. */
  clientSlug: string;
  /** Human-readable client name derived from the slug. */
  clientLabel: string;
}

/**
 * Title-case a workspace slug for display:
 *   "bistrot-halles" → "Bistrot Halles", "menuiserie_vasseur" → "Menuiserie Vasseur".
 * Workspaces have no separate display name in the accessible-workspaces payload,
 * so the humanized slug is the best label we can surface client-side.
 */
export function humanizeSlug(slug: string): string {
  return slug
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

/**
 * Determine whether the signed-in user is acting as an integrator in the
 * current workspace. An integrator administers a CLIENT's data, so the UI must
 * surface an unmissable "you are administering {client}" signal (banner +
 * switcher caption) rather than pretend the data is the integrator's own.
 *
 * The role lives on the accessible-workspaces list (the `/v1/workspace/me`
 * Role enum has no `integrator` member), so we match the current slug against
 * that list — the same source the workspace switcher reads.
 */
export function selectIntegratorContext(
  workspaces: AccessibleWorkspace[],
  currentSlug: string,
): IntegratorContext {
  const isIntegrator =
    currentSlug !== '' &&
    workspaces.some(
      (ws) => ws.slug === currentSlug && ws.role === 'integrator',
    );
  return {
    isIntegrator,
    clientSlug: currentSlug,
    clientLabel: humanizeSlug(currentSlug),
  };
}
