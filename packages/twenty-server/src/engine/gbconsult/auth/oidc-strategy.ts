/*
 * leCRM patch — clean-room Passport OIDC strategy for leCRM (AGPL-3.0)
 *
 * Functional replacement for upstream's `OIDCAuthStrategy` (file:
 * src/engine/core-modules/auth/strategies/oidc.auth.strategy.ts —
 * marked `@license Enterprise`). Written from scratch against the
 * public openid-client + Passport interfaces.
 *
 * Wiring status:
 *   This strategy is exported by `gbconsult.module.ts` as a NestJS
 *   provider. The runtime path that currently invokes upstream's
 *   `new OIDCAuthStrategy(...)` (inside `OIDCAuthGuard.canActivate`)
 *   is NOT yet redirected to this implementation — see
 *   `gbconsult/README.md` for the follow-up sub-tasket that replaces
 *   the SSO controller surface.
 *
 *   Until that follow-up lands, this file exists to:
 *     a) Establish the AGPL-3.0 leCRM equivalent of the strategy.
 *     b) Be importable by tests that exercise the strategy in
 *        isolation (verifying we have a working, license-clean OIDC
 *        path).
 *
 * Reference for the openid-client API used here:
 *   https://github.com/panva/node-openid-client (v5).
 */

import { Injectable } from '@nestjs/common';
import { PassportStrategy } from '@nestjs/passport';

import { type Request } from 'express';
import {
  Strategy,
  type StrategyOptions,
  type TokenSet,
} from 'openid-client';

export type LeCRMOIDCRequest = Omit<
  Request,
  'user' | 'workspace' | 'workspaceMetadataVersion'
> & {
  user: {
    identityProviderId: string;
    email: string;
    firstName?: string | null;
    lastName?: string | null;
    workspaceInviteHash?: string;
    oidcTokenClaims?: Record<string, unknown>;
  };
};

@Injectable()
export class GBConsultOIDCStrategy extends PassportStrategy(
  Strategy,
  'openidconnect',
) {
  constructor(client: StrategyOptions['client'], sessionKey: string) {
    super({
      params: {
        scope: 'openid email profile',
        code_challenge_method: 'S256',
      },
      client,
      usePKCE: true,
      passReqToCallback: true,
      sessionKey,
    });
  }

  async authenticate(req: Request, options: unknown): Promise<unknown> {
    const inviteHash =
      typeof req.query.workspaceInviteHash === 'string'
        ? { workspaceInviteHash: req.query.workspaceInviteHash }
        : {};

    return super.authenticate(req, {
      ...(options as Record<string, unknown>),
      state: JSON.stringify({
        identityProviderId: req.params.identityProviderId,
        ...inviteHash,
      }),
    });
  }

  private extractState(req: Request): {
    identityProviderId: string;
    workspaceInviteHash?: string;
  } {
    const raw =
      typeof req.query.state === 'string' ? req.query.state : '{}';

    let parsed: { identityProviderId?: string; workspaceInviteHash?: string };
    try {
      parsed = JSON.parse(raw);
    } catch {
      throw new Error('OIDC state parse failed');
    }

    if (!parsed.identityProviderId) {
      throw new Error('OIDC state missing identityProviderId');
    }

    return {
      identityProviderId: parsed.identityProviderId,
      workspaceInviteHash: parsed.workspaceInviteHash,
    };
  }

  async validate(
    req: Request,
    tokenset: TokenSet,
    done: (err: Error | null, user?: LeCRMOIDCRequest['user']) => void,
  ): Promise<void> {
    try {
      const state = this.extractState(req);

      const userinfo = await (
        this as unknown as {
          client: { userinfo: (t: TokenSet) => Promise<Record<string, unknown>> };
        }
      ).client.userinfo(tokenset);

      const emailRaw = userinfo['email'] ?? userinfo['upn'];
      const email = typeof emailRaw === 'string' ? emailRaw : null;

      if (!email) {
        done(new Error('OIDC userinfo missing email'));
        return;
      }

      const givenName = userinfo['given_name'];
      const familyName = userinfo['family_name'];

      done(null, {
        email,
        identityProviderId: state.identityProviderId,
        workspaceInviteHash: state.workspaceInviteHash,
        ...(typeof givenName === 'string' ? { firstName: givenName } : {}),
        ...(typeof familyName === 'string' ? { lastName: familyName } : {}),
        oidcTokenClaims: tokenset.claims() as Record<string, unknown>,
      });
    } catch (err) {
      done(err instanceof Error ? err : new Error(String(err)));
    }
  }
}
