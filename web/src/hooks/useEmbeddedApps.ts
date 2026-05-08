import { queryOptions, useQuery } from '@tanstack/react-query';
import { api } from '../api/client';

export interface EmbeddedApp {
  id: string;
  name: string;
  origin: string;
  iconUrl?: string;
  position: number;
  isSystem: boolean;
}

interface ListResponse {
  apps: EmbeddedApp[] | null;
}

// Lista de embedded apps del tenant del user actual.
// El backend ya scopea por user.tenant_id, no hace falta pasar slug.
export const embeddedAppsQuery = () => queryOptions<EmbeddedApp[]>({
  queryKey: ['embedded-apps'],
  queryFn: async () => {
    const res = await api<ListResponse>('/api/embedded-apps');
    return res.apps ?? [];
  },
  staleTime: 30 * 1000,
});

export function useEmbeddedApps() {
  return useQuery(embeddedAppsQuery());
}
