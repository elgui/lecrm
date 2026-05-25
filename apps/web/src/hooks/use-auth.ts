import { useQuery } from '@tanstack/react-query';
import { api, ApiError } from '@/lib/api';
import type { User } from '@/lib/types';

export function useAuth() {
  const { data, isLoading, error } = useQuery<User>({
    queryKey: ['auth', 'me'],
    queryFn: () => api.get<User>('/auth/me'),
    retry: false,
    staleTime: 5 * 60 * 1000,
  });

  const isUnauthenticated =
    error instanceof ApiError && error.status === 401;

  return {
    user: data ?? null,
    isLoading,
    isAuthenticated: !!data,
    isUnauthenticated,
  };
}
