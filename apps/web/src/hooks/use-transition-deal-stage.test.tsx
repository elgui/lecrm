import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as React from 'react';

import { useTransitionDealStage } from './use-deals';
import type { Deal, PaginatedResponse } from '@/lib/types';

function makeDeal(id: string, stageId: string | null, title = `Deal ${id}`): Deal {
  return {
    id,
    title,
    amount: null,
    currency: null,
    stage_id: stageId,
    contact_id: null,
    company_id: null,
    owner_id: null,
    expected_close_date: null,
    closed_at: null,
    created_at: '2026-05-28T00:00:00Z',
    updated_at: '2026-05-28T00:00:00Z',
  };
}

function wrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

describe('useTransitionDealStage', () => {
  const originalFetch = globalThis.fetch;
  const calls: Array<{ url: string; init?: RequestInit }> = [];

  beforeEach(() => {
    calls.length = 0;
  });
  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('PATCHes /v1/deals/{id}/stage with the new stage_id and optimistically updates the cached list', async () => {
    globalThis.fetch = vi.fn(async (url, init) => {
      calls.push({ url: String(url), init });
      const body = JSON.parse(String(init?.body ?? '{}'));
      return new Response(
        JSON.stringify({
          ...makeDeal('d1', body.stage_id),
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    }) as typeof fetch;

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const cachedList: PaginatedResponse<Deal> = {
      data: [makeDeal('d1', 's1'), makeDeal('d2', 's2')],
      next_cursor: null,
      has_more: false,
    };
    qc.setQueryData(['deals', { cursor: undefined }], cachedList);

    const { result } = renderHook(() => useTransitionDealStage(), {
      wrapper: wrapper(qc),
    });

    await act(async () => {
      result.current.mutate({ id: 'd1', stage_id: 's3' });
    });

    // Optimistic cache update — d1's stage_id is now s3 before the request returns.
    await waitFor(() => {
      const updated = qc.getQueryData<PaginatedResponse<Deal>>([
        'deals',
        { cursor: undefined },
      ]);
      expect(updated?.data.find((d) => d.id === 'd1')?.stage_id).toBe('s3');
    });

    // Mutation completes successfully.
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(calls).toHaveLength(1);
    expect(calls[0]!.url).toBe('/v1/deals/d1/stage');
    expect(calls[0]!.init?.method).toBe('PATCH');
    expect(JSON.parse(String(calls[0]!.init?.body))).toEqual({ stage_id: 's3' });
  });

  it('rolls back the optimistic update when the server rejects the transition', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ error: 'stage not found' }), {
        status: 400,
        headers: { 'Content-Type': 'application/json' },
      }),
    ) as typeof fetch;

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const cachedList: PaginatedResponse<Deal> = {
      data: [makeDeal('d1', 's1')],
      next_cursor: null,
      has_more: false,
    };
    qc.setQueryData(['deals', { cursor: undefined }], cachedList);

    const { result } = renderHook(() => useTransitionDealStage(), {
      wrapper: wrapper(qc),
    });

    await act(async () => {
      result.current.mutate({ id: 'd1', stage_id: 'bogus' });
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    const rolledBack = qc.getQueryData<PaginatedResponse<Deal>>([
      'deals',
      { cursor: undefined },
    ]);
    expect(rolledBack?.data.find((d) => d.id === 'd1')?.stage_id).toBe('s1');
  });
});
