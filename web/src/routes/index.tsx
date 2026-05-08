import { createFileRoute, redirect } from '@tanstack/react-router';
import { sessionQuery } from '../hooks/useSession';

// `/` — router de entrada. No renderiza UI propia; solo decide:
//
//   sin sesión       → /auth/login (full redirect, no es ruta TSR)
//   con tenant       → /t/{slug}/
//   sin tenant       → /signup
//
// Si en el futuro querés una landing pública (marketing, SEO),
// reintroducís el componente y lo renderizas en el branch "sin sesión".
export const Route = createFileRoute('/')({
  beforeLoad: async ({ context }) => {
    const session = await context.queryClient.ensureQueryData(sessionQuery());

    if (!session) {
      // Full-page redirect a /auth/login (handler Go, fuera de TSR).
      // Throw para abortar el render del componente.
      window.location.href = '/auth/login';
      throw new Error('redirecting to login');
    }
    if (!session.tenantSlug) throw redirect({ to: '/signup' });
    throw redirect({
      to: '/t/$slug',
      params: { slug: session.tenantSlug },
    });
  },
  // Componente de fallback que nunca se renderiza (beforeLoad siempre
  // redirige). Existe solo para satisfacer la API de TSR.
  component: () => null,
});
