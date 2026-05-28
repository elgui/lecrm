import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';

import { fetchPipelineStages } from './use-pipeline-stages';
import { ApiError } from '@/lib/api';

describe('fetchPipelineStages', () => {
  const originalFetch = globalThis.fetch;
  const calls: Array<{ url: string; init?: RequestInit }> = [];

  beforeEach(() => {
    calls.length = 0;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('GETs /v1/pipeline/stages and returns the data array (happy path)', async () => {
    globalThis.fetch = vi.fn(async (url, init) => {
      calls.push({ url: String(url), init });
      return new Response(
        JSON.stringify({
          data: [
            {
              id: '11111111-1111-1111-1111-111111111111',
              name: 'Discovery',
              order_index: 1,
              created_at: '2026-05-28T00:00:00Z',
            },
          ],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    }) as typeof fetch;

    const result = await fetchPipelineStages();
    expect(result.data).toHaveLength(1);
    expect(result.data[0]!.name).toBe('Discovery');
    expect(calls).toHaveLength(1);
    expect(calls[0]!.url).toBe('/v1/pipeline/stages');
  });

  it('throws ApiError 401 when the session is missing', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response('authentication required', { status: 401 }),
    ) as typeof fetch;

    await expect(fetchPipelineStages()).rejects.toMatchObject({
      name: 'ApiError',
      status: 401,
    });
  });

  it('throws ApiError 503 when the workspace database is unreachable', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify({ error: 'list pipeline stages failed' }),
        { status: 503 },
      ),
    ) as typeof fetch;

    await expect(fetchPipelineStages()).rejects.toBeInstanceOf(ApiError);
  });
});
