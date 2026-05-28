import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';

export interface PipelineStage {
  id: string;
  name: string;
  order_index: number;
  created_at: string;
}

interface PipelineStagesResponse {
  data: PipelineStage[];
}

export function fetchPipelineStages(): Promise<PipelineStagesResponse> {
  return api.get<PipelineStagesResponse>('/v1/pipeline/stages');
}

export function usePipelineStages() {
  return useQuery<PipelineStagesResponse>({
    queryKey: ['pipeline-stages'],
    queryFn: fetchPipelineStages,
  });
}
