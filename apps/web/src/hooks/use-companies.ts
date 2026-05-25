import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Company, PaginatedResponse } from '@/lib/types';

export function useCompanies(cursor?: string) {
  const params = new URLSearchParams();
  if (cursor) params.set('cursor', cursor);
  const qs = params.toString();
  const path = `/v1/companies${qs ? `?${qs}` : ''}`;

  return useQuery<PaginatedResponse<Company>>({
    queryKey: ['companies', { cursor }],
    queryFn: () => api.get<PaginatedResponse<Company>>(path),
  });
}
