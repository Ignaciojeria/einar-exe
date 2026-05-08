import { createFileRoute, redirect } from '@tanstack/react-router';
import { sessionQuery } from '../hooks/useSession';

// Landing — `/`
//
// Si hay sesión:
//   - sin tenant → redirect a /signup
//   - con tenant → redirect a /t/{slug}
// Sin sesión: muestra la landing con botón "Sign in with Google".
//
// `beforeLoad` corre del lado del router (server-side feel), no espera
// a que el componente monte. Eso evita el flash de la landing antes
// de redirigir.
export const Route = createFileRoute('/')({
  beforeLoad: async ({ context }) => {
    const session = await context.queryClient.ensureQueryData(sessionQuery());
    if (!session) return; // no auth → render Landing
    if (!session.tenantSlug) throw redirect({ to: '/signup' });
    throw redirect({
      to: '/t/$slug',
      params: { slug: session.tenantSlug },
    });
  },
  component: Landing,
});

function Landing() {
  return (
    <div style={s.wrap}>
      <div style={s.card}>
        <h1 style={s.title}>einar</h1>
        <p style={s.lead}>Plataforma para construir y embeber apps en tu workspace.</p>
        <a href="/auth/login" style={s.btn}>Sign in with Google</a>
      </div>
    </div>
  );
}

const s = {
  wrap: {
    minHeight: '100vh',
    display: 'grid',
    placeItems: 'center',
    background: 'linear-gradient(135deg, #fafafa, #f0f0f0)',
    fontFamily: 'system-ui, sans-serif',
  } as const,
  card: {
    background: '#fff',
    padding: '3rem 2.5rem',
    borderRadius: 8,
    boxShadow: '0 2px 12px rgba(0,0,0,0.06)',
    textAlign: 'center',
    maxWidth: 420,
  } as const,
  title: { fontSize: '2rem', margin: '0 0 .5rem' } as const,
  lead: { color: '#666', margin: '0 0 1.5rem' } as const,
  btn: {
    display: 'inline-block',
    padding: '.7rem 1.5rem',
    background: '#111',
    color: '#fff',
    textDecoration: 'none',
    borderRadius: 4,
    fontWeight: 600,
  } as const,
};
