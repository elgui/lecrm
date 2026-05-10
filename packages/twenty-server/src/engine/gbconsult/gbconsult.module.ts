/*
 * leCRM patch — single override module (AGPL-3.0)
 *
 * This is the ONLY module that `app.module.ts` imports from `gbconsult/`.
 * It re-exports replacement providers via the standard NestJS custom-
 * providers pattern. NestJS resolves the last-imported provider for any
 * given token, so importing `GBConsultModule` after `AuthModule` /
 * `EnterpriseModule` in `app.module.ts` causes leCRM's stubs to win.
 *
 * See `README.md` in this directory for the override pattern, and
 * `ADR-002` (private architecture project) for the rationale.
 */

import { Module } from '@nestjs/common';

import { EnterprisePlanService } from 'src/engine/core-modules/enterprise/services/enterprise-plan.service';

import { GBConsultAuthOverrideModule } from './auth/auth.module.override';
import { GBConsultEnterprisePlanServiceStub } from './enterprise/plan-service-stub';
import { GBConsultVersionController } from './version/version.controller';

@Module({
  imports: [GBConsultAuthOverrideModule],
  controllers: [GBConsultVersionController],
  providers: [
    {
      provide: EnterprisePlanService,
      useClass: GBConsultEnterprisePlanServiceStub,
    },
  ],
  exports: [EnterprisePlanService],
})
export class GBConsultModule {}
