import { createFileRoute, notFound } from '@tanstack/react-router';
import { useEmbeddedApps } from '../hooks/useEmbeddedApps';
import EmbeddedAppFrame from '../components/EmbeddedAppFrame';
import type { Session } from '../hooks/useSession';

// `/t/$slug/app/$appId` — renderiza un iframe de la app embebida.
//
// El layout padre ya validó sesión + tenant. Acá solo buscamos la app
// en la lista (cacheada por TanStack Query) y la pasamos al componente
// que maneja el postMessage choreography.
export const Route = createFileRoute('/t/$slug/app/$appId')({
  component: EmbeddedAppPage,
});

function EmbeddedAppPage() {
  const { appId } = Route.useParams();
  const { session } = Route.useRouteContext() as { session: Session };
  const { data: apps = [], isLoading } = useEmbeddedApps();

  if (isLoading) return <div style={{ padding: 24 }}>Cargando…</div>;

  const app = apps.find(a => a.id === appId);
  if (!app) throw notFound();

  return (
    <div style={s.wrap}>
      <header style={s.header}>
        <h1 style={s.title}>{app.name}</h1>
        <code style={s.origin}>{app.origin}</code>
      </header>
      <div style={s.frameWrap}>
        <EmbeddedAppFrame app={app} user={session.user} />
      </div>
    </div>
  );
}

const s = {
  wrap: { display: 'flex', flexDirection: 'column', height: '100vh' } as const,
  header: { padding: '1rem 1.5rem', borderBottom: '1px solid #e0e0e0', background: '#fff' } as const,
  title: { margin: 0, fontSize: '1.1rem' } as const,
  origin: { fontSize: '.8rem', color: '#888' } as const,
  frameWrap: { flex: 1, overflow: 'hidden' } as const,
};
