import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Me, Permissions } from '@/lib/types';

const NO_PERMISSIONS: Permissions = {
  can_read: false,
  can_write: false,
  can_manage_members: false,
  can_manage_tokens: false,
  can_delete_workspace: false,
};

/**
 * useMe fetches the current user's workspace role and capability bundle from
 * GET /v1/workspace/me. Components use the returned `permissions` to gate
 * write/admin controls without re-deriving the role hierarchy client-side.
 *
 * While loading (or on error) permissions default to all-false, so controls
 * stay hidden until the role is confirmed — fail closed, never flash an
 * unauthorized button.
 */
export function useMe() {
  const { data, isLoading, error } = useQuery<Me>({
    queryKey: ['workspace', 'me'],
    queryFn: () => api.get<Me>('/v1/workspace/me'),
    retry: false,
    staleTime: 5 * 60 * 1000,
  });

  return {
    me: data ?? null,
    role: data?.role ?? 'none',
    permissions: data?.permissions ?? NO_PERMISSIONS,
    isOwner: data?.role === 'owner',
    isLoading,
    error,
  };
}
