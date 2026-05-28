import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { EntityType, Task } from '@/lib/types';

export interface TaskScope {
  entity_type: EntityType;
  entity_id: string;
}

export interface TaskInput {
  title: string;
  description?: string | null;
  due_date?: string | null;
  entity_type?: EntityType | null;
  entity_id?: string | null;
  assignee_id?: string | null;
}

// useTasks lists tasks, optionally scoped to one entity. The query key
// folds the scope in so an entity's task panel and the global list stay
// independently cached.
export function useTasks(scope?: TaskScope) {
  const params = new URLSearchParams();
  if (scope) {
    params.set('entity_type', scope.entity_type);
    params.set('entity_id', scope.entity_id);
  }
  const qs = params.toString();
  return useQuery<Task[]>({
    queryKey: ['tasks', scope ?? 'all'],
    queryFn: async () => {
      const res = await api.get<{ data: Task[] }>(`/v1/tasks${qs ? `?${qs}` : ''}`);
      return res.data ?? [];
    },
  });
}

export function useCreateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: TaskInput) => api.post<Task>('/v1/tasks', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  });
}

export function useUpdateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { id: string } & TaskInput) => {
      const { id, ...body } = input;
      return api.put<Task>(`/v1/tasks/${id}`, body);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  });
}

export function useToggleTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.patch<Task>(`/v1/tasks/${id}/complete`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  });
}

export function useDeleteTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/v1/tasks/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  });
}
