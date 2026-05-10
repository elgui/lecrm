/*
 * leCRM patch — AGPL §13 version endpoint (AGPL-3.0)
 *
 * GET /api/version returns the running upstream Twenty version and the
 * leCRM patch revision in machine- and human-readable form. This is the
 * AGPL §13 source-build correspondence anchor: any operator or auditor
 * inspecting a leCRM deployment can hit this endpoint to learn exactly
 * what to check out from `github.com/elgui/lecrm` to inspect or rebuild
 * the running code.
 *
 * Mounted on the public path `/api/version` (no auth) intentionally —
 * AGPL §13 requires that the source URL be visible to anyone interacting
 * with the running service.
 */

import { Controller, Get } from '@nestjs/common';

import {
  LECRM_AGPL_FOOTER_TEXT,
  LECRM_FULL_VERSION,
  LECRM_PATCH_REVISION,
  LECRM_SOURCE_URL,
  LECRM_UPSTREAM_VERSION,
} from './version.constants';

@Controller('api/version')
export class GBConsultVersionController {
  @Get()
  getVersion(): {
    upstream: string;
    patch: string;
    full: string;
    sourceUrl: string;
    agplFooter: string;
  } {
    return {
      upstream: LECRM_UPSTREAM_VERSION,
      patch: LECRM_PATCH_REVISION,
      full: LECRM_FULL_VERSION,
      sourceUrl: LECRM_SOURCE_URL,
      agplFooter: LECRM_AGPL_FOOTER_TEXT,
    };
  }
}
