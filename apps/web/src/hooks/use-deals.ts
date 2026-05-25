import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Deal, PaginatedResponse } from '@/lib/types';

export function useDeals(cursor?: string) {
  const params = new URLSearchParams();
  if (cursor) params.set('cursor', cursor);
  const qs = params.toString();
  const path = `/v1/deals${qs ? `?${qs}` : ''}`;

  return useQuery<PaginatedResponse<Deal>>({
    queryKey: ['deals', { cursor }],
    queryFn: () => api.get<PaginatedResponse<Deal>>(path),
  });
}

export function useDeal(id: string) {
  return useQuery<Deal>({
    queryKey: ['deals', id],
    queryFn: () => api.get<Deal>(`/v1/deals/${id}`),
    enabled: !!id,
  });
}
