import { useEffect, useState } from 'react';

// einar viene del SDK cargado en index.html.
// El shape mínimo: einar.create({ appId, shellOrigin? })
declare global {
  interface Window {
    einar: {
      create: (opts: { appId: string; shellOrigin?: string }) => {
        ready: () => Promise<void>;
        getToken: () => string | null;
        getUser: () => { sub: string; email?: string; name?: string } | null;
        getIssuer: () => string | null;
        getJwksUri: () => string | null;
      };
    };
  }
}

const BACKEND_URL = (import.meta.env.VITE_BACKEND_URL as string) || 'http://localhost:3001';

export default function App() {
  const [status, setStatus] = useState<'init' | 'ready' | 'error'>('init');
  const [user, setUser] = useState<{ email?: string; name?: string } | null>(null);
  const [secret, setSecret] = useState<unknown>(null);
  const [error, setError] = useState<string>('');

  useEffect(() => {
    const e = window.einar.create({
      appId: 'embedded-with-backend',
      // Para probar contra localhost del shell, descomentar:
      // shellOrigin: 'http://localhost:8000',
    });

    (async () => {
      try {
        await e.ready();
        const u = e.getUser();
        setUser(u);
        setStatus('ready');

        // Llamar al backend con el JWT del shell.
        const token = e.getToken();
        const r = await fetch(BACKEND_URL + '/api/secret', {
          headers: { Authorization: 'Bearer ' + token },
        });
        if (!r.ok) {
          setError(`backend ${r.status}: ${await r.text()}`);
          return;
        }
        setSecret(await r.json());
      } catch (err) {
        setStatus('error');
        setError(err instanceof Error ? err.message : String(err));
      }
    })();
  }, []);

  return (
    <div style={{ fontFamily: 'system-ui', padding: '2rem', maxWidth: 720 }}>
      <h1>einar embedded app + backend</h1>
      <p>
        Esta app se embebió en einar, recibió un JWT firmado por el shell, y
        llamó a su propio backend pasando el token. El backend validó el JWT
        contra <code>{BACKEND_URL ? '/api/secret' : ''}</code> y devolvió datos
        personalizados.
      </p>

      {status === 'init' && <p>Cargando…</p>}
      {status === 'error' && <p style={{ color: 'red' }}>Error: {error}</p>}

      {status === 'ready' && (
        <>
          <h2>User (del shell, vía SDK)</h2>
          <pre style={preStyle}>{JSON.stringify(user, null, 2)}</pre>

          <h2>Backend response (validado server-side)</h2>
          {error && <p style={{ color: 'orange' }}>{error}</p>}
          <pre style={preStyle}>{JSON.stringify(secret, null, 2)}</pre>
        </>
      )}
    </div>
  );
}

const preStyle: React.CSSProperties = {
  background: '#f5f5f5',
  padding: '1rem',
  borderRadius: 4,
  overflow: 'auto',
  fontSize: '.85rem',
};
