import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { ReportDefinition, RunResult, SavedReport } from '@/lib/report-builder';

// useRunReport runs a report definition against the workspace and returns the
// aggregated result. Keyed by the definition so flipping any field re-runs and
// caches independently. Disabled by default unless `enabled` is passed.
export function useRunReport(definition: ReportDefinition | null) {
  return useQuery<RunResult>({
    queryKey: ['reports', 'run', definition],
    enabled: !!definition,
    queryFn: () => api.post<RunResult>('/v1/reports/run', definition),
  });
}

// useSavedReports lists the workspace's saved report definitions.
export function useSavedReports() {
  return useQuery<SavedReport[]>({
    queryKey: ['reports', 'definitions'],
    queryFn: async () => {
      const res = await api.get<{ data: SavedReport[] }>('/v1/reports/definitions');
      return res.data ?? [];
    },
  });
}

export function useCreateSavedReport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (def: ReportDefinition) =>
      api.post<SavedReport>('/v1/reports/definitions', def),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['reports', 'definitions'] }),
  });
}

export function useUpdateSavedReport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, def }: { id: string; def: ReportDefinition }) =>
      api.put<SavedReport>(`/v1/reports/definitions/${id}`, def),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['reports', 'definitions'] }),
  });
}

export function useDeleteSavedReport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete<void>(`/v1/reports/definitions/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['reports', 'definitions'] }),
  });
}
