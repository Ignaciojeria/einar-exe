import { queryOptions, useQuery } from '@tanstack/react-query';
import { api, ApiError } from '../api/client';

export interface SessionUser {
  sub: string;
  email: string;
  name?: string;
  displayName?: string;
  picture?: string;
}

export interface Session {
  user: SessionUser;
  authenticated: true;
  tenantSlug?: string;
}

// sessionQuery: factoría reusable de QueryOptions para `/api/session`.
// Lo usan tanto el hook (componentes) como los loaders de TSR
// (router-side, antes de montar). Comparten cache via TanStack Query.
//
// Returns:
//   - 401 → null  (no autenticado)
//   - 200 → Session
//   - otro error → throw (TSR / ErrorBoundary lo agarra)
export const sessionQuery = () => queryOptions<Session | null>({
  queryKey: ['session'],
  queryFn: async () => {
    try {
      return await api<Session>('/api/session');
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) return null;
      throw e;
    }
  },
  staleTime: 5 * 60 * 1000,
  retry: false,
});

// useSession: para componentes que NO están dentro de una ruta con loader,
// o cuando querés re-fetchear/observar. En las rutas usar mejor
// `Route.useRouteContext().session` (poblado por beforeLoad).
export function useSession() {
  return useQuery(sessionQuery());
}
