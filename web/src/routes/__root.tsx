import { createRootRouteWithContext, Outlet } from '@tanstack/react-router';
import type { QueryClient } from '@tanstack/react-query';

// Context que viaja por todo el árbol de rutas. Permite que cualquier
// loader / beforeLoad acceda al QueryClient sin tener que importarlo.
//
// Pattern oficial TanStack Router para integrarse con TanStack Query.
export interface RouterContext {
  queryClient: QueryClient;
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
});

function RootLayout() {
  return <Outlet />;
}
