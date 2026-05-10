/*
 * leCRM patch — version constants (AGPL-3.0)
 *
 * These constants are the single source of truth for the running
 * upstream Twenty version and the leCRM patch revision. They are used
 * by the `/api/version` endpoint and the front-end footer (when wired).
 *
 * Build pipeline (post-v0): replace the literal values below with
 * compile-time injection from the git tag, e.g.
 *
 *   LECRM_UPSTREAM_VERSION = process.env.LECRM_UPSTREAM_VERSION ?? '...';
 *
 * For v0, hardcoded constants are acceptable because the deployment
 * pipeline produces one image per tagged release.
 */

export const LECRM_UPSTREAM_VERSION = 'twenty-2.2.0';
export const LECRM_PATCH_REVISION = 'lecrm.0';
export const LECRM_FULL_VERSION = `${LECRM_UPSTREAM_VERSION}+${LECRM_PATCH_REVISION}`;
export const LECRM_SOURCE_URL = 'https://github.com/elgui/lecrm';
export const LECRM_AGPL_FOOTER_TEXT = `Powered by Twenty CRM (AGPL-3.0) — source: ${LECRM_SOURCE_URL.replace('https://', '')}`;
