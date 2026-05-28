import { describe, expect, it, afterEach, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as React from 'react';

import { useMe } from './use-me';
import type { Me } from '@/lib/types';

function wrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

function meResponse(role: Me['role']): Me {
  return {
    user_id: '11111111-1111-1111-1111-111111111111',
    role,
    actor_type: 'human_api',
    permissions: {
      can_read: role !== 'none',
      can_write: role === 'admin' || role === 'owner',
      can_manage_members: role === 'owner',
      can_manage_tokens: role === 'owner',
      can_delete_workspace: role === 'owner',
    },
  };
}

describe('useMe', () => {
  const originalFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  function freshClient() {
    return new QueryClient({ defaultOptions: { queries: { retry: false } } });
  }

  it('exposes owner role with full permissions and isOwner=true', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify(meResponse('owner')), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    ) as typeof fetch;

    const { result } = renderHook(() => useMe(), { wrapper: wrapper(freshClient()) });

    await waitFor(() => expect(result.current.role).toBe('owner'));
    expect(result.current.isOwner).toBe(true);
    expect(result.current.permissions.can_manage_members).toBe(true);
    expect(result.current.permissions.can_write).toBe(true);
  });

  it('exposes member role as read-only (no write, no manage, not owner)', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify(meResponse('member')), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    ) as typeof fetch;

    const { result } = renderHook(() => useMe(), { wrapper: wrapper(freshClient()) });

    await waitFor(() => expect(result.current.role).toBe('member'));
    expect(result.current.isOwner).toBe(false);
    expect(result.current.permissions.can_read).toBe(true);
    expect(result.current.permissions.can_write).toBe(false);
    expect(result.current.permissions.can_manage_members).toBe(false);
  });

  it('fails closed: permissions default to all-false while unauthorized', async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response('authentication required', { status: 401 }),
    ) as typeof fetch;

    const { result } = renderHook(() => useMe(), { wrapper: wrapper(freshClient()) });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.role).toBe('none');
    expect(result.current.isOwner).toBe(false);
    expect(result.current.permissions.can_read).toBe(false);
    expect(result.current.permissions.can_write).toBe(false);
    expect(result.current.permissions.can_manage_members).toBe(false);
  });
});
