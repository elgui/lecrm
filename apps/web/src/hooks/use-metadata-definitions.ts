import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { PropertyDefinition, PropertyType } from '@/lib/types';

export type DefinitionParentType = 'contact' | 'deal';

export interface CreateDefinitionInput {
  parent_type: DefinitionParentType;
  property_key: string;
  property_type: PropertyType;
  allowed_values?: string[];
  required: boolean;
}

/**
 * useDefinitions lists the custom property definitions for one parent type.
 * The query key matches the shape used by useDealDefinitions /
 * useContactDefinitions (['metadata', 'definitions', parentType]) so the
 * record-detail editor and this admin page share a single cache entry.
 */
export function useDefinitions(parentType: DefinitionParentType) {
  return useQuery<PropertyDefinition[]>({
    queryKey: ['metadata', 'definitions', parentType],
    queryFn: async () => {
      const res = await api.get<{ definitions: PropertyDefinition[] }>(
        `/v1/metadata/definitions?parent_type=${parentType}`,
      );
      return res.definitions ?? [];
    },
  });
}

export function useCreateDefinition() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateDefinitionInput) =>
      api.post<PropertyDefinition>('/v1/metadata/definitions', input),
    // Invalidate the whole definitions namespace so both this page and the
    // record-detail editors (contact + deal) pick up the new field without a
    // reload.
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ['metadata', 'definitions'] }),
  });
}

export function useDeleteDefinition() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.delete<void>(`/v1/metadata/definitions/${id}`),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ['metadata', 'definitions'] }),
  });
}
