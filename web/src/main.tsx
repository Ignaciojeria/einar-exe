import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RouterProvider, createRouter } from '@tanstack/react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { routeTree } from './routeTree.gen';

// QueryClient compartido entre router (loaders) y componentes (hooks).
const queryClient = new QueryClient();

// Router con context que viaja a todos los loaders / beforeLoad.
const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: 'intent',  // hover sobre <Link> prefetchea
});

// TS module augmentation: registra el tipo del router para que el
// type-check de `<Link to="...">` funcione globalmente.
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
