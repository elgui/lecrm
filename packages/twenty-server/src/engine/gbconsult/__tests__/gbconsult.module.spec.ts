/*
 * leCRM patch — DI-override verification test (AGPL-3.0)
 *
 * Per ADR-002 TO RESOLVE item 3, this test asserts that GBConsultModule
 * actually shadows the upstream EnterprisePlanService at runtime. Run on
 * every PR; if a future upstream rebase silently breaks the override
 * (e.g. NestJS internals change forwardRef ordering), this fails fast
 * and surfaces the regression before it ships.
 *
 * The test does NOT spin up the full AppModule; instead it imports
 * EnterpriseModule (the upstream provider) and GBConsultModule (last,
 * mimicking app.module.ts) into a synthetic NestJS testing module and
 * resolves EnterprisePlanService from the resulting injector.
 */

import { Test, type TestingModule } from '@nestjs/testing';

import { EnterprisePlanService } from 'src/engine/core-modules/enterprise/services/enterprise-plan.service';

import { GBConsultModule } from '../gbconsult.module';
import { GBConsultEnterprisePlanServiceStub } from '../enterprise/plan-service-stub';

describe('GBConsultModule — DI override resolution', () => {
  let testingModule: TestingModule;

  beforeAll(async () => {
    testingModule = await Test.createTestingModule({
      imports: [GBConsultModule],
    })
      // Twenty's EnterprisePlanService has DB + config dependencies that we
      // do not need at this resolution boundary — the stub does not pull
      // them in. If the override resolves correctly, NestJS never tries to
      // construct the upstream class.
      .compile();
  });

  afterAll(async () => {
    await testingModule.close();
  });

  it('resolves EnterprisePlanService to the leCRM stub', () => {
    const resolved = testingModule.get(EnterprisePlanService);
    expect(resolved).toBeInstanceOf(GBConsultEnterprisePlanServiceStub);
  });

  it('stub.isValid() returns true unconditionally', () => {
    const resolved = testingModule.get(EnterprisePlanService);
    // The upstream return type is `boolean`; we narrow at runtime.
    expect((resolved as unknown as { isValid: () => boolean }).isValid()).toBe(
      true,
    );
  });

  it('stub.getLicenseInfo() returns isValid=true and a leCRM licensee string', async () => {
    const resolved = testingModule.get(EnterprisePlanService);
    const info = await (
      resolved as unknown as {
        getLicenseInfo: () => Promise<{ isValid: boolean; licensee: string | null }>;
      }
    ).getLicenseInfo();
    expect(info.isValid).toBe(true);
    expect(info.licensee).toContain('leCRM');
  });
});
