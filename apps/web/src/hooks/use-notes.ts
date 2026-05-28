import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { EntityType, Note } from '@/lib/types';

// Notes endpoints are nested under the parent entity for list/create and
// flat under /v1/notes/{id} for update/delete. The API derives authorship
// from the request body (author_id) until session-derived identity lands
// (anc_handlers.go), so callers pass the current user's id.

function listPath(entityType: EntityType, entityId: string) {
  return `/v1/${entityType === 'company' ? 'companies' : entityType + 's'}/${entityId}/notes`;
}

export function useNotes(entityType: EntityType, entityId: string) {
  return useQuery<Note[]>({
    queryKey: ['notes', entityType, entityId],
    queryFn: async () => {
      const res = await api.get<{ data: Note[] }>(listPath(entityType, entityId));
      return res.data ?? [];
    },
    enabled: !!entityId,
  });
}

export function useCreateNote(entityType: EntityType, entityId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { body: string; author_id: string }) =>
      api.post<Note>(listPath(entityType, entityId), input),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ['notes', entityType, entityId] }),
  });
}

export function useUpdateNote(entityType: EntityType, entityId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { id: string; body: string; author_id: string }) =>
      api.put<Note>(`/v1/notes/${input.id}`, {
        body: input.body,
        author_id: input.author_id,
      }),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ['notes', entityType, entityId] }),
  });
}

export function useDeleteNote(entityType: EntityType, entityId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { id: string; author_id: string }) =>
      api.delete<void>(`/v1/notes/${input.id}?author_id=${input.author_id}`),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ['notes', entityType, entityId] }),
  });
}
