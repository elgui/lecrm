import { useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Company, PaginatedResponse } from '@/lib/types';

export interface CompanyInput {
  name: string;
  domain: string | null;
  industry: string | null;
  size: string | null;
}

/** Build an id → name lookup from a page of companies. Pure; unit-tested. */
export function companyNameMap(companies: Company[]): Map<string, string> {
  const m = new Map<string, string>();
  for (const c of companies) m.set(c.id, c.name);
  return m;
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

/**
 * Fetch every company across all pages by following the cursor. Used to build a
 * complete id → name lookup for list views, so a workspace with more than one
 * page of companies (>50) still resolves linked names instead of falling back
 * to a dash. Bounded by a hard page cap so a stuck cursor can't loop forever.
 */
async function fetchAllCompanies(): Promise<Company[]> {
  const all: Company[] = [];
  let cursor: string | undefined;
  // Safety cap: 100 pages × 50 = 5000 companies. Far beyond any SMB workspace;
  // prevents an unbounded loop if the API ever returns a stuck cursor.
  for (let page = 0; page < 100; page++) {
    const params = new URLSearchParams();
    if (cursor) params.set('cursor', cursor);
    const qs = params.toString();
    const resp = await api.get<PaginatedResponse<Company>>(
      `/v1/companies${qs ? `?${qs}` : ''}`,
    );
    all.push(...resp.data);
    if (!resp.has_more || !resp.next_cursor) break;
    cursor = resp.next_cursor;
  }
  return all;
}

/**
 * Resolve company ids to names for list views (Contacts shows the company
 * name, not the raw UUID). Walks every page so the lookup is complete even for
 * workspaces with more than one page of companies. Keyed under ['companies'] so
 * a company create/update/delete invalidation also refreshes the name map.
 */
export function useCompanyMap() {
  const { data } = useQuery<Company[]>({
    queryKey: ['companies', 'all'],
    queryFn: fetchAllCompanies,
  });
  return useMemo(() => companyNameMap(data ?? []), [data]);
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
