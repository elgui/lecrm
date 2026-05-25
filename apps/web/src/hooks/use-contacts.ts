import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Contact, PaginatedResponse } from '@/lib/types';

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

export function useUpdateContact(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: Partial<Contact>) =>
      api.patch<Contact>(`/v1/contacts/${id}`, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['contacts', id] });
      queryClient.invalidateQueries({ queryKey: ['contacts'] });
    },
  });
}

export function useContactProperties(id: string) {
  return useQuery({
    queryKey: ['contacts', id, 'properties'],
    queryFn: () => api.get(`/v1/contacts/${id}/properties`),
    enabled: !!id,
  });
}
