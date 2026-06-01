import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type {
  DedupContactPair,
  DedupCompanyPair,
  MergeResult,
} from '@/lib/types';

// --- contact dedup ---

export function useContactDuplicates() {
  return useQuery<{ pairs: DedupContactPair[] }>({
    queryKey: ['dedup', 'contacts'],
    queryFn: () => api.get<{ pairs: DedupContactPair[] }>('/v1/dedup/contacts'),
  });
}

export interface MergeContactInput {
  survivor_id: string;
  loser_id: string;
  fields?: Record<string, 'survivor' | 'loser'>;
}

export function useMergeContacts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: MergeContactInput) =>
      api.post<MergeResult>('/v1/dedup/contacts/merge', data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['contacts'] });
      qc.invalidateQueries({ queryKey: ['dedup', 'contacts'] });
    },
  });
}

export function useMarkContactsDistinct() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id_a, id_b }: { id_a: string; id_b: string }) =>
      api.post<{ distinct: boolean }>('/v1/dedup/contacts/distinct', { id_a, id_b }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['dedup', 'contacts'] });
    },
  });
}

// --- company dedup ---

export function useCompanyDuplicates() {
  return useQuery<{ pairs: DedupCompanyPair[] }>({
    queryKey: ['dedup', 'companies'],
    queryFn: () => api.get<{ pairs: DedupCompanyPair[] }>('/v1/dedup/companies'),
  });
}

export interface MergeCompanyInput {
  survivor_id: string;
  loser_id: string;
  fields?: Record<string, 'survivor' | 'loser'>;
}

export function useMergeCompanies() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: MergeCompanyInput) =>
      api.post<MergeResult>('/v1/dedup/companies/merge', data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['companies'] });
      qc.invalidateQueries({ queryKey: ['dedup', 'companies'] });
    },
  });
}

export function useMarkCompaniesDistinct() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id_a, id_b }: { id_a: string; id_b: string }) =>
      api.post<{ distinct: boolean }>('/v1/dedup/companies/distinct', { id_a, id_b }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['dedup', 'companies'] });
    },
  });
}
