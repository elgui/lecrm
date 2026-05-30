import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { AccessibleWorkspace } from '@/lib/types';

interface WorkspacesResponse {
  data: AccessibleWorkspace[];
}

export function useWorkspaces() {
  const { data, isLoading, error } = useQuery<WorkspacesResponse>({
    queryKey: ['workspaces'],
    queryFn: () => api.get<WorkspacesResponse>('/auth/workspaces'),
    retry: false,
    staleTime: 5 * 60 * 1000,
  });

  return {
    workspaces: data?.data ?? [],
    isLoading,
    error,
  };
}
