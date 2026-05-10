/*
 * leCRM patch — clean-room implementation by GB Consult (AGPL-3.0)
 *
 * Replaces upstream's `EnterprisePlanService` (file:
 * src/engine/core-modules/enterprise/services/enterprise-plan.service.ts —
 * marked `@license Enterprise`) at the dependency-injection layer.
 *
 * Behaviour: all gating predicates return `true` / "valid". leCRM does not
 * use upstream's licence-validation surface; gating decisions are made at
 * the operator level (this is a single-tenant-per-VPS deployment in Phase
 * 1 and a self-operated multi-tenant cluster in Phase 2 — see ADR-001).
 *
 * Outbound calls (refreshValidityToken, reportSeats, getPortalUrl,
 * getCheckoutUrl, getSubscriptionStatus) are no-ops; they return values
 * that mark "no upstream subscription" without raising errors. Consumers
 * that branch on those values (e.g. admin panel UI) get a consistent
 * "self-operated" presentation.
 *
 * This file is AGPL-3.0 leCRM code, written from scratch against the
 * public method signatures of upstream's class. No upstream Enterprise
 * source is incorporated.
 */

import { Injectable, Logger } from '@nestjs/common';

@Injectable()
export class GBConsultEnterprisePlanServiceStub {
  private readonly logger = new Logger(GBConsultEnterprisePlanServiceStub.name);

  hasValidSignedEnterpriseKey(): boolean {
    return true;
  }

  hasValidEnterpriseValidityToken(): boolean {
    return true;
  }

  hasValidEnterpriseKey(): boolean {
    return true;
  }

  isValid(): boolean {
    return true;
  }

  isValidEnterpriseKeyFormat(_key: string): boolean {
    return true;
  }

  async getLicenseInfo(): Promise<{
    isValid: boolean;
    licensee: string | null;
    expiresAt: Date | null;
    subscriptionId: string | null;
  }> {
    return {
      isValid: true,
      licensee: 'leCRM (self-operated)',
      expiresAt: null,
      subscriptionId: null,
    };
  }

  async setEnterpriseKey(_enterpriseKey: string): Promise<void> {
    this.logger.debug(
      'setEnterpriseKey called on leCRM stub — ignored (no upstream subscription).',
    );
  }

  async refreshValidityToken(): Promise<boolean> {
    return true;
  }

  async reportSeats(_seatCount: number): Promise<boolean> {
    return true;
  }

  async getSubscriptionStatus(): Promise<{
    status: string;
    licensee: string | null;
    expiresAt: Date | null;
    cancelAt: Date | null;
    currentPeriodEnd: Date | null;
    isCancellationScheduled: boolean;
  } | null> {
    return {
      status: 'self-operated',
      licensee: 'leCRM (self-operated)',
      expiresAt: null,
      cancelAt: null,
      currentPeriodEnd: null,
      isCancellationScheduled: false,
    };
  }

  async getPortalUrl(_returnUrl?: string): Promise<string | null> {
    return null;
  }

  async getCheckoutUrl(
    _billingInterval: 'monthly' | 'yearly' = 'monthly',
    _seatCount = 1,
  ): Promise<string | null> {
    return null;
  }

  async onModuleInit(): Promise<void> {
    this.logger.log(
      'leCRM EnterprisePlanService stub active — all gating predicates return valid.',
    );
  }
}
