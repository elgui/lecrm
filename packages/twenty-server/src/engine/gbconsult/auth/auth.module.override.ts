/*
 * leCRM patch — auth-related provider overrides (AGPL-3.0)
 *
 * Currently exports the clean-room OIDC strategy for tests and for the
 * follow-up SSO controller replacement. No live auth pipeline is
 * rewired here yet — see `../README.md` for the follow-up plan.
 */

import { Module } from '@nestjs/common';

import { GBConsultOIDCStrategy } from './oidc-strategy';

@Module({
  providers: [GBConsultOIDCStrategy],
  exports: [GBConsultOIDCStrategy],
})
export class GBConsultAuthOverrideModule {}
