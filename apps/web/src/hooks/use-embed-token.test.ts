import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';

import { fetchEmbedToken } from './use-embed-token';
import { ApiError } from '@/lib/api';

describe('fetchEmbedToken', () => {
  const originalFetch = globalThis.fetch;
  const calls: Array<{ url: string; init?: RequestInit }> = [];

  beforeEach(() => {
    calls.length = 0;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('POSTs /v1/reports/embed-token and returns the token payload (happy path)', async () => {
    globalThis.fetch = vi.fn(async (url, init) => {
      calls.push({ url: String(url), init });
      return new Response(
        JSON.stringify({
          token: 'eyJhbGciOi.signed.body',
          expires_at: '2026-05-28T13:05:00Z',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    }) as typeof fetch;

    const result = await fetchEmbedToken();

    expect(result).toEqual({
      token: 'eyJhbGciOi.signed.body',
      expires_at: '2026-05-28T13:05:00Z',
    });
    expect(calls).toHaveLength(1);
    expect(calls[0]!.url).toBe('/v1/reports/embed-token');
    expect(calls[0]!.init?.method).toBe('POST');
    expect(typeof calls[0]!.init?.body).toBe('string');
  });

  it('throws ApiError with status 403 when the backend rejects cross-workspace access', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify({ error: 'workspace mismatch' }),
        { status: 403, headers: { 'Content-Type': 'application/json' } },
      ),
    ) as typeof fetch;

    await expect(fetchEmbedToken()).rejects.toBeInstanceOf(ApiError);
    try {
      await fetchEmbedToken();
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError);
      expect((err as ApiError).status).toBe(403);
    }
  });

  it('throws ApiError with status 401 when the session cookie is missing', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response('authentication required', { status: 401 }),
    ) as typeof fetch;

    await expect(fetchEmbedToken()).rejects.toMatchObject({
      name: 'ApiError',
      status: 401,
    });
  });

  it('throws ApiError with status 503 when LECRM_CUBE_JWT_SECRET is unset on the server', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify({ error: 'embed reporting disabled' }),
        { status: 503 },
      ),
    ) as typeof fetch;

    await expect(fetchEmbedToken()).rejects.toMatchObject({
      name: 'ApiError',
      status: 503,
    });
  });
});
