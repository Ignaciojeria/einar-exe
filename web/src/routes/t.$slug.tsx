import { createFileRoute, redirect, Outlet, Link } from '@tanstack/react-router';
import { sessionQuery, type Session } from '../hooks/useSession';
import { embeddedAppsQuery, useEmbeddedApps } from '../hooks/useEmbeddedApps';

// Layout del workspace — `/t/$slug`
//
// Guardia (beforeLoad):
//   - asegura sesión (full redirect si no)
//   - asegura tenant en sesión (redirect a /signup si no)
//   - asegura que el slug coincida con el de la sesión
//   - precarga la lista de embedded apps via TSR loader
export const Route = createFileRoute('/t/$slug')({
  beforeLoad: async ({ context, params }) => {
    const session = await context.queryClient.ensureQueryData(sessionQuery());
    if (!session) {
      window.location.href = `/auth/login?return=/t/${params.slug}/`;
      throw new Error('redirecting to login');
    }
    if (!session.tenantSlug) throw redirect({ to: '/signup' });
    if (session.tenantSlug !== params.slug) {
      throw redirect({
        to: '/t/$slug',
        params: { slug: session.tenantSlug },
      });
    }
    return { session };
  },
  // Loader: precarga la lista de apps del tenant. Si falla, no rompemos
  // el render del layout — el sidenav muestra el estado de error.
  loader: ({ context }) =>
    context.queryClient.ensureQueryData(embeddedAppsQuery()).catch(() => []),
  component: WorkspaceLayout,
});

function WorkspaceLayout() {
  const { slug } = Route.useParams();
  const { session } = Route.useRouteContext() as { session: Session };
  const { data: apps = [] } = useEmbeddedApps();

  return (
    <div style={s.layout}>
      <aside style={s.sidenav}>
        <div style={s.brand}>
          <div style={s.brandTitle}>einar</div>
          <div style={s.brandSlug}>/t/{slug}</div>
        </div>
        <nav style={s.nav}>
          <div style={s.navHeader}>Workspace</div>
          <Link
            to="/t/$slug"
            params={{ slug }}
            style={s.navLink}
            activeProps={{ style: { ...s.navLink, ...s.navLinkActive } }}
            activeOptions={{ exact: true }}
          >
            Inicio
          </Link>
          <Link
            to="/t/$slug/dev"
            params={{ slug }}
            style={s.navLink}
            activeProps={{ style: { ...s.navLink, ...s.navLinkActive } }}
          >
            🧪 Test your app
          </Link>
          <Link
            to="/t/$slug/apps"
            params={{ slug }}
            style={s.navLink}
            activeProps={{ style: { ...s.navLink, ...s.navLinkActive } }}
          >
            Manage apps
          </Link>

          <div style={{ ...s.navHeader, marginTop: '1.5rem' }}>Embedded apps</div>
          {apps.length === 0 ? (
            <div style={s.navEmpty}>— ninguna registrada —</div>
          ) : (
            apps.map(app => (
              <Link
                key={app.id}
                to="/t/$slug/app/$appId"
                params={{ slug, appId: app.id }}
                style={s.navLink}
                activeProps={{ style: { ...s.navLink, ...s.navLinkActive } }}
              >
                {app.iconUrl && <img src={app.iconUrl} alt="" style={s.icon} />}
                <span>{app.name}</span>
              </Link>
            ))
          )}
        </nav>
        <div style={s.userBox}>
          <div style={s.userName}>{session.user.displayName || session.user.email}</div>
          <div style={s.userEmail}>{session.user.email}</div>
          <a href="/auth/logout" style={s.logout}>Cerrar sesión</a>
        </div>
      </aside>
      <main style={s.main}>
        <Outlet />
      </main>
    </div>
  );
}

const s = {
  layout: { display: 'grid', gridTemplateColumns: '260px 1fr', minHeight: '100vh', fontFamily: 'system-ui, sans-serif' } as const,
  sidenav: { background: '#1a1a1a', color: '#eee', display: 'flex', flexDirection: 'column', padding: '1.5rem 1rem' } as const,
  brand: { marginBottom: '2rem' } as const,
  brandTitle: { fontSize: '1.3rem', fontWeight: 700 } as const,
  brandSlug: { color: '#888', fontFamily: 'monospace', fontSize: '.85rem', marginTop: '.25rem' } as const,
  nav: { flex: 1 } as const,
  navHeader: { color: '#aaa', fontSize: '.75rem', textTransform: 'uppercase', letterSpacing: '.05em', marginBottom: '.5rem' } as const,
  navEmpty: { color: '#555', fontStyle: 'italic', fontSize: '.85rem', padding: '.5rem 0' } as const,
  navLink: { display: 'flex', alignItems: 'center', gap: '.5rem', color: '#ddd', textDecoration: 'none', padding: '.4rem .6rem', borderRadius: 4, fontSize: '.9rem' } as const,
  navLinkActive: { background: '#333', color: '#fff' } as const,
  icon: { width: 18, height: 18, borderRadius: 3 } as const,
  userBox: { borderTop: '1px solid #333', paddingTop: '1rem', marginTop: '1rem' } as const,
  userName: { fontWeight: 600, fontSize: '.9rem' } as const,
  userEmail: { color: '#888', fontSize: '.8rem', marginTop: '.15rem' } as const,
  logout: { display: 'inline-block', marginTop: '.75rem', color: '#ddd', fontSize: '.85rem', textDecoration: 'underline' } as const,
  main: { background: '#fafafa', overflow: 'hidden', display: 'flex', flexDirection: 'column' } as const,
};
