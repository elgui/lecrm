import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type { Member, Role } from '@/lib/types';

const KEY = ['workspace', 'members'] as const;

interface MemberList {
  data: Member[];
}

export function useMembers() {
  return useQuery<Member[]>({
    queryKey: KEY,
    queryFn: async () => {
      const res = await api.get<MemberList>('/v1/workspace/members');
      return res.data ?? [];
    },
    retry: false,
  });
}

export function useInviteMember() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { email: string; role: Role }) =>
      api.post<Member>('/v1/workspace/members/invite', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useUpdateMemberRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { userId: string; role: Role }) =>
      api.patch<unknown>(`/v1/workspace/members/${input.userId}/role`, {
        role: input.role,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useRemoveMember() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: string) =>
      api.delete<void>(`/v1/workspace/members/${userId}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}
