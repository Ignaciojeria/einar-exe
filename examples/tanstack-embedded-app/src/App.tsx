import { useQuery } from '@tanstack/react-query';
import { einarClient } from './einar';
import { useEinar } from './useEinar';

// Demo: cuando el SDK confirma auth, hacemos un query a un endpoint del
// shell (la app embedded SOLO debería llamar a SU PROPIO backend en
// producción; aquí pegamos a /api/me del shell solo para mostrar que
// el token funciona).
//
// Pattern para tu propia app:
//   queryFn: () => einarClient.fetch('https://tu-api.com/data').then(r => r.json())

export default function App() {
  const auth = useEinar();

  const { data, isLoading, error } = useQuery({
    queryKey: ['me', auth?.token],          // refetch al rotar el token
    enabled: !!auth,                         // espera al primer auth
    queryFn: async () => {
      // einarClient.fetch agrega Authorization: Bearer auto.
      const res = await einarClient.fetch('https://einar.exe.xyz/api/me');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      return res.json() as Promise<{ sub: string; email: string; displayName?: string }>;
    },
  });

  if (!auth) {
    return (
      <Layout title="Esperando handshake con el shell…">
        <p style={s.muted}>
          Si esta app NO está embebida en einar, no vas a ver nada porque
          no hay parent al que mandarle <code>einar:ready</code>.
        </p>
      </Layout>
    );
  }

  return (
    <Layout title={`Hola, ${auth.user.displayName || auth.user.email}`}>
      <section style={s.card}>
        <h3 style={s.h3}>Sesión recibida via postMessage</h3>
        <dl style={s.dl}>
          <dt>Email</dt>      <dd>{auth.user.email}</dd>
          <dt>Sub</dt>        <dd><code>{auth.user.sub}</code></dd>
          <dt>Token</dt>      <dd><code>{auth.token.slice(0, 32)}…</code></dd>
          <dt>Expira</dt>     <dd>{new Date(auth.expiresAt * 1000).toLocaleString()}</dd>
        </dl>
      </section>

      <section style={s.card}>
        <h3 style={s.h3}>Fetch autenticado a /api/me del shell</h3>
        {isLoading && <p style={s.muted}>Cargando…</p>}
        {error && <p style={s.error}>Error: {(error as Error).message}</p>}
        {data && (
          <pre style={s.pre}>{JSON.stringify(data, null, 2)}</pre>
        )}
        <p style={s.muted}>
          Notá que <code>einarClient.fetch()</code> ya agregó{' '}
          <code>Authorization: Bearer &lt;token&gt;</code>. En tu app real
          apuntás a tu propio backend, que valida el JWT contra el JWKS
          público del shell.
        </p>
      </section>

      <button onClick={() => einarClient.logout()} style={s.btn}>
        Cerrar sesión global
      </button>
    </Layout>
  );
}

function Layout({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={s.wrap}>
      <h1 style={s.title}>{title}</h1>
      {children}
    </div>
  );
}

const s = {
  wrap: { fontFamily: 'system-ui, sans-serif', maxWidth: 720, margin: '2rem auto', padding: '0 1.5rem' } as const,
  title: { fontSize: '1.4rem', marginBottom: '1.5rem' } as const,
  muted: { color: '#666' } as const,
  card: { background: '#fff', border: '1px solid #e0e0e0', borderRadius: 6, padding: '1.25rem 1.5rem', marginBottom: '1rem' } as const,
  h3: { marginTop: 0, fontSize: '1rem', color: '#333' } as const,
  dl: { display: 'grid', gridTemplateColumns: '120px 1fr', gap: '.4rem', margin: 0 } as const,
  pre: { background: '#1a1a1a', color: '#eee', padding: '.75rem', borderRadius: 4, overflow: 'auto', fontSize: '.8rem' } as const,
  error: { color: '#c00' } as const,
  btn: { padding: '.5rem 1rem', background: '#fff', border: '1px solid #ccc', borderRadius: 4, cursor: 'pointer' } as const,
};
