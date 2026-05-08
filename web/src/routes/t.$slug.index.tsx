import { createFileRoute } from '@tanstack/react-router';
import type { Session } from '../hooks/useSession';

// `/t/$slug/` (index del workspace)
//
// Hereda contexto del padre `t.$slug.tsx` (incluida la sesión validada).
// Esta es la "home" del workspace; las sub-rutas serán las embedded apps.
export const Route = createFileRoute('/t/$slug/')({
  component: WorkspaceHome,
});

function WorkspaceHome() {
  const { session } = Route.useRouteContext() as { session: Session };
  const { slug } = Route.useParams();

  return (
    <>
      <h1>Workspace listo 🎉</h1>
      <p style={{ color: '#666' }}>
        TanStack Router activo. Auth por cookie HttpOnly (BFF).
        El sidenav todavía no tiene apps porque la Fase 4 viene después.
      </p>

      <section style={s.card}>
        <h3 style={s.cardTitle}>Tu sesión</h3>
        <dl style={s.dl}>
          <dt>Email</dt>           <dd>{session.user.email}</dd>
          <dt>Display name</dt>    <dd>{session.user.displayName || '—'}</dd>
          <dt>Casdoor sub</dt>     <dd><code>{session.user.sub}</code></dd>
          <dt>Tenant slug</dt>     <dd><code>{slug}</code></dd>
        </dl>
      </section>
    </>
  );
}

const s = {
  card: { background: '#fff', border: '1px solid #e0e0e0', borderRadius: 6, padding: '1.5rem', maxWidth: 640, marginTop: '1rem' } as const,
  cardTitle: { marginTop: 0, fontSize: '1rem', color: '#333' } as const,
  dl: { display: 'grid', gridTemplateColumns: '160px 1fr', gap: '.5rem', margin: 0 } as const,
};
