import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Contact, PaginatedResponse, PropertyDefinition } from '@/lib/types';

export interface ContactInput {
  first_name: string;
  last_name: string;
  email: string | null;
  phone: string | null;
  company_id: string | null;
}

export function useContacts(cursor?: string) {
  const params = new URLSearchParams();
  if (cursor) params.set('cursor', cursor);
  const qs = params.toString();
  const path = `/v1/contacts${qs ? `?${qs}` : ''}`;

  return useQuery<PaginatedResponse<Contact>>({
    queryKey: ['contacts', { cursor }],
    queryFn: () => api.get<PaginatedResponse<Contact>>(path),
  });
}

export function useContact(id: string) {
  return useQuery<Contact>({
    queryKey: ['contacts', id],
    queryFn: () => api.get<Contact>(`/v1/contacts/${id}`),
    enabled: !!id,
  });
}

export function useCreateContact() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: ContactInput) => api.post<Contact>('/v1/contacts', data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['contacts'] }),
  });
}

export function useUpdateContact(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: Partial<ContactInput>) =>
      api.put<Contact>(`/v1/contacts/${id}`, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['contacts', id] });
      qc.invalidateQueries({ queryKey: ['contacts'] });
    },
  });
}

export function useDeleteContact() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/v1/contacts/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['contacts'] }),
  });
}

// The properties endpoint returns `{ properties: { key: value } }`. Unwrap
// to the flat record the editor consumes.
export function useContactProperties(id: string) {
  return useQuery<Record<string, unknown>>({
    queryKey: ['contacts', id, 'properties'],
    queryFn: async () => {
      const res = await api.get<{ properties: Record<string, unknown> }>(
        `/v1/contacts/${id}/properties`,
      );
      return res.properties ?? {};
    },
    enabled: !!id,
  });
}

export function useUpdateContactProperties(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: Record<string, unknown>) =>
      api.put<{ status: string }>(`/v1/contacts/${id}/properties`, data),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ['contacts', id, 'properties'] }),
  });
}

export function useContactDefinitions() {
  return useQuery<PropertyDefinition[]>({
    queryKey: ['metadata', 'definitions', 'contact'],
    queryFn: async () => {
      const res = await api.get<{ definitions: PropertyDefinition[] }>(
        '/v1/metadata/definitions?parent_type=contact',
      );
      return res.definitions ?? [];
    },
  });
}
