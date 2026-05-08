import { queryOptions, useQuery } from '@tanstack/react-query';
import { api } from '../api/client';

export interface OpenObserveCreds {
  loginUrl: string;
  email: string;
  password: string;
  orgId: string;
}

export interface MetabaseCreds {
  loginUrl: string;
  email: string;
  password: string;
  groupId: number;
  collectionId: number;
}

export interface Credentials {
  openobserve?: OpenObserveCreds;
  metabase?: MetabaseCreds;
}

// staleTime: corto. Las credenciales pueden rotarse desde la UI; queremos
// reflejar el cambio rápido. Igual TanStack Query refetchea on focus.
export const credentialsQuery = () => queryOptions<Credentials>({
  queryKey: ['credentials'],
  queryFn: () => api<Credentials>('/api/credentials'),
  staleTime: 10 * 1000,
});

export function useCredentials() {
  return useQuery(credentialsQuery());
}
