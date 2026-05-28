import { useQuery } from '@tanstack/react-query';
import { api, ApiError } from '@/lib/api';

export interface EmbedToken {
  token: string;
  expires_at: string;
}

// Exported separately from the hook so it can be tested without
// React/QueryClient wiring. The backend handler (apps/api
// internal/reports/handler.go) resolves the workspace from the request
// host + session cookie — no payload needed.
export async function fetchEmbedToken(): Promise<EmbedToken> {
  return api.post<EmbedToken>('/v1/reports/embed-token', {});
}

export function useEmbedToken() {
  return useQuery<EmbedToken, Error>({
    queryKey: ['reports', 'embed-token'],
    queryFn: fetchEmbedToken,
    // Refresh comfortably before the 5-minute server TTL.
    staleTime: 4 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: (failureCount, error) => {
      if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
        return false;
      }
      return failureCount < 1;
    },
  });
}
