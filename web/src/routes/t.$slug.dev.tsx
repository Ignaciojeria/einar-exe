import { useState } from 'react';
import { createFileRoute, Link } from '@tanstack/react-router';
import EmbeddedAppFrame from '../components/EmbeddedAppFrame';
import type { Session } from '../hooks/useSession';

// `/t/$slug/dev` — Modo desarrollador.
//
// Permite cargar una app local (localhost o 127.0.0.1) como iframe sin
// tener que registrarla en la DB. Útil para ciclo dev: corres tu app en
// `localhost:5174`, abrís este panel, y ya tenés auth real via postMessage.
//
// Restricción de origin: SOLO http(s)://localhost:PORT o 127.0.0.1:PORT.
// Esto previene que alguien use este endpoint como "embed cualquier
// dominio" en producción. Para apps reales, hay que registrarlas.

export const Route = createFileRoute('/t/$slug/dev')({
  component: DevTester,
});

const STORAGE_KEY = 'einar:dev:lastOrigin';
const DEFAULT_ORIGIN = 'http://localhost:5174';

function isAllowedDevOrigin(origin: string): boolean {
  try {
    const u = new URL(origin);
    if (u.pathname && u.pathname !== '/') return false;
    if (u.search || u.hash) return false;
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return false;
    return u.hostname === 'localhost' || u.hostname === '127.0.0.1';
  } catch {
    return false;
  }
}

function DevTester() {
  const { slug } = Route.useParams();
  const { session } = Route.useRouteContext() as { session: Session };

  const [origin, setOrigin] = useState(() => {
    return localStorage.getItem(STORAGE_KEY) || DEFAULT_ORIGIN;
  });
  const [activeOrigin, setActiveOrigin] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [error, setError] = useState<string | null>(null);

  function load() {
    const cleaned = origin.trim().replace(/\/$/, '');
    if (!isAllowedDevOrigin(cleaned)) {
      setError('Solo se permite http(s)://localhost:PUERTO o 127.0.0.1:PUERTO');
      return;
    }
    setError(null);
    setActiveOrigin(cleaned);
    localStorage.setItem(STORAGE_KEY, cleaned);
    setReloadKey(k => k + 1);
  }

  function reload() {
    setReloadKey(k => k + 1);
  }

  return (
    <div style={s.wrap}>
      <header style={s.header}>
        <div>
          <h1 style={s.title}>🧪 Test your app</h1>
          <div style={s.muted}>
            Carga una app que estés desarrollando localmente. Recibe el token
            y user del usuario autenticado vía <code>postMessage</code>.
          </div>
        </div>
        <Link to="/t/$slug" params={{ slug }} style={s.linkBack}>← Volver al inicio</Link>
      </header>

      <div style={s.controls}>
        <input
          value={origin}
          onChange={e => setOrigin(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && load()}
          placeholder="http://localhost:5174"
          style={s.input}
          spellCheck={false}
        />
        <button onClick={load} style={s.btnPrimary}>Cargar</button>
        {activeOrigin && (
          <button onClick={reload} style={s.btnSecondary} title="Recarga el iframe">
            ↻ Reload
          </button>
        )}
      </div>
      {error && <div style={s.error}>{error}</div>}

      {activeOrigin ? (
        <div style={s.frameWrap} key={reloadKey}>
          <EmbeddedAppFrame
            app={{
              id: 'dev-tester',
              name: 'Dev tester',
              origin: activeOrigin,
              position: 0,
            }}
            user={session.user}
          />
        </div>
      ) : (
        <DocsPanel slug={slug} />
      )}
    </div>
  );
}

function DocsPanel({ slug }: { slug: string }) {
  return (
    <div style={s.docs}>
      <h2 style={s.h2}>Cómo construir una embedded app</h2>

      <p>Tu app se carga en un iframe del shell. El shell te entrega:</p>
      <ul>
        <li>El <code>token</code> del user autenticado (JWT de Casdoor).</li>
        <li>El <code>user</code> con email, sub, name, picture.</li>
        <li><code>refresh</code> automático antes de que expire.</li>
      </ul>
      <p>
        Todo via <code>window.postMessage</code> al origin de tu app.
        Tu app NO se autentica por separado — el shell lo hace por ella.
      </p>

      <h3 style={s.h3}>Opción A — vanilla HTML (sin build)</h3>
      <pre style={s.code}>{vanillaExample}</pre>

      <h3 style={s.h3}>Opción B — React + TanStack Query (recomendado)</h3>
      <p>Hay un starter completo en el repo:</p>
      <pre style={s.code}>{`# Clonar y correr el example
cd examples/tanstack-embedded-app
npm install
npm run dev    # levanta en http://localhost:5174

# Después: en este panel pegá http://localhost:5174 y "Cargar"`}</pre>

      <h3 style={s.h3}>Protocolo postMessage</h3>
      <pre style={s.code}>{protocolExample}</pre>

      <h3 style={s.h3}>Cuando esté listo: registralo</h3>
      <p>
        Una vez tu app esté en producción, registrala con su URL real desde
        {' '}<Link to="/t/$slug/apps/new" params={{ slug }} style={s.inlineLink}>
          + Registrar app
        </Link>. Va a aparecer en el sidenav y solo se le hablará por su origin
        registrado.
      </p>
    </div>
  );
}

const vanillaExample = `<!doctype html>
<html>
<head><title>Mi app</title></head>
<body>
  <h1 id="status">Cargando…</h1>
  <script src="https://einar.exe.xyz/sdk/v1.js"></script>
  <script>
    // En dev, el shell vive en otra URL — pasalo por opts.shellOrigin.
    const einarClient = einar.create({
      appId: 'mi-app',
      shellOrigin: 'https://einar.exe.xyz',
    });

    einarClient.ready().then(() => {
      const u = einarClient.getUser();
      document.getElementById('status').textContent =
        \`Hola \${u.email}\`;

      // Ejemplo: fetch a TU backend con auth automática
      einarClient.fetch('/api/mis-pedidos').then(r => r.json());
    });

    einarClient.onAuth(({ token }) => {
      console.log('token rotado:', token.slice(0, 20) + '...');
    });
  </script>
</body>
</html>`;

const protocolExample = `// Iframe → Shell (al cargar, una sola vez)
postMessage({ type: 'einar:ready', version: 1, appId: 'mi-app' }, '*')

// Shell → Iframe (después del 'ready', y cada refresh)
postMessage({
  type: 'einar:auth',
  version: 1,
  token: '<jwt>',
  expiresAt: 1730000000,    // unix seconds
  user: { sub, email, name, displayName, picture }
}, 'https://app.miempresa.com')

// Iframe → Shell (pedir token nuevo si el SDK detecta proximidad de exp)
postMessage({ type: 'einar:refresh', version: 1 }, '*')

// Iframe → Shell (cerrar sesión global)
postMessage({ type: 'einar:logout', version: 1 }, '*')`;

const s = {
  wrap: { display: 'flex', flexDirection: 'column', height: '100vh', overflow: 'hidden' } as const,
  header: { padding: '1rem 1.5rem', borderBottom: '1px solid #e0e0e0', background: '#fff', display: 'flex', justifyContent: 'space-between', alignItems: 'center' } as const,
  title: { margin: 0, fontSize: '1.1rem' } as const,
  muted: { color: '#666', fontSize: '.85rem', marginTop: '.25rem' } as const,
  linkBack: { color: '#666', textDecoration: 'none', fontSize: '.85rem' } as const,
  controls: { display: 'flex', gap: '.5rem', padding: '.75rem 1.5rem', background: '#f5f5f5', borderBottom: '1px solid #e0e0e0' } as const,
  input: { flex: 1, padding: '.5rem .75rem', fontSize: '.9rem', border: '1px solid #ccc', borderRadius: 4, fontFamily: 'monospace' } as const,
  btnPrimary: { padding: '.5rem 1rem', background: '#111', color: '#fff', border: 0, borderRadius: 4, cursor: 'pointer', fontSize: '.9rem' } as const,
  btnSecondary: { padding: '.5rem .75rem', background: '#fff', color: '#333', border: '1px solid #ccc', borderRadius: 4, cursor: 'pointer', fontSize: '.9rem' } as const,
  error: { margin: '0 1.5rem .75rem', padding: '.6rem .8rem', background: '#fee', color: '#c00', border: '1px solid #fcc', borderRadius: 4, fontSize: '.85rem' } as const,
  frameWrap: { flex: 1, overflow: 'hidden' } as const,
  docs: { flex: 1, overflow: 'auto', padding: '2rem 3rem', maxWidth: 820, background: '#fafafa' } as const,
  h2: { fontSize: '1.2rem', marginTop: 0 } as const,
  h3: { fontSize: '1rem', marginTop: '2rem', marginBottom: '.5rem' } as const,
  code: { background: '#1a1a1a', color: '#eee', padding: '1rem', borderRadius: 6, overflow: 'auto', fontSize: '.8rem', lineHeight: 1.5 } as const,
  inlineLink: { color: '#0a66ff' } as const,
};
