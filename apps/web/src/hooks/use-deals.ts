import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Deal, PaginatedResponse } from '@/lib/types';

export function useDeals(cursor?: string) {
  const params = new URLSearchParams();
  if (cursor) params.set('cursor', cursor);
  const qs = params.toString();
  const path = `/v1/deals${qs ? `?${qs}` : ''}`;

  return useQuery<PaginatedResponse<Deal>>({
    queryKey: ['deals', { cursor }],
    queryFn: () => api.get<PaginatedResponse<Deal>>(path),
  });
}

export function useDeal(id: string) {
  return useQuery<Deal>({
    queryKey: ['deals', id],
    queryFn: () => api.get<Deal>(`/v1/deals/${id}`),
    enabled: !!id,
  });
}

export interface TransitionDealStageVars {
  id: string;
  stage_id: string;
}

// useTransitionDealStage performs an optimistic update on the cached deals
// list, then PATCHes /v1/deals/{id}/stage. On error it invalidates the deals
// query so the server's truth wins.
export function useTransitionDealStage() {
  const qc = useQueryClient();

  return useMutation<Deal, Error, TransitionDealStageVars, { previous: PaginatedResponse<Deal> | undefined }>({
    mutationFn: ({ id, stage_id }) =>
      api.patch<Deal>(`/v1/deals/${id}/stage`, { stage_id }),
    onMutate: async ({ id, stage_id }) => {
      await qc.cancelQueries({ queryKey: ['deals'] });
      const key = ['deals', { cursor: undefined }];
      const previous = qc.getQueryData<PaginatedResponse<Deal>>(key);
      if (previous) {
        qc.setQueryData<PaginatedResponse<Deal>>(key, {
          ...previous,
          data: previous.data.map((d) =>
            d.id === id ? { ...d, stage_id } : d,
          ),
        });
      }
      return { previous };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.previous) {
        qc.setQueryData(['deals', { cursor: undefined }], ctx.previous);
      }
      qc.invalidateQueries({ queryKey: ['deals'] });
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ['deals'] });
    },
  });
}
