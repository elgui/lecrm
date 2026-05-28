import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Company, PaginatedResponse } from '@/lib/types';

export interface CompanyInput {
  name: string;
  domain: string | null;
  industry: string | null;
  size: string | null;
}

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

export function useCompany(id: string) {
  return useQuery<Company>({
    queryKey: ['companies', id],
    queryFn: () => api.get<Company>(`/v1/companies/${id}`),
    enabled: !!id,
  });
}

export function useCreateCompany() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CompanyInput) => api.post<Company>('/v1/companies', data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['companies'] }),
  });
}

export function useUpdateCompany(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: Partial<CompanyInput>) =>
      api.put<Company>(`/v1/companies/${id}`, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['companies', id] });
      qc.invalidateQueries({ queryKey: ['companies'] });
    },
  });
}

export function useDeleteCompany() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/v1/companies/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['companies'] }),
  });
}
