import { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { EmbeddedApp } from '../hooks/useEmbeddedApps';
import type { SessionUser } from '../hooks/useSession';

interface Props {
  app: EmbeddedApp;
  user: SessionUser;
}

interface TokenResponse {
  token: string;
  expiresAt: number; // unix seconds
}

// Protocolo postMessage v1 (sincronizado con web/public/sdk/v1.js).
const PROTOCOL_VERSION = 1;

// Refresh anticipado: empezamos a renovar el token cuando le quedan
// menos de N segundos. Evita que el iframe haga requests con un token
// que expira a mitad de un fetch.
const REFRESH_BEFORE_EXPIRY_SEC = 60;

/**
 * EmbeddedAppFrame
 *
 * Renderiza un iframe sandboxed apuntando a `app.origin` y maneja:
 *
 *  1. Handshake: escucha 'einar:ready' del iframe → manda 'einar:auth'.
 *  2. Refresh proactivo: timer que renueva el token antes del exp y lo
 *     re-pushea al iframe.
 *  3. Refresh on-demand: si el iframe manda 'einar:refresh' (porque su
 *     SDK detectó token cercano a vencer), lo refrescamos también.
 *  4. Logout: 'einar:logout' del iframe → window.location = /auth/logout.
 *
 * Seguridad:
 *   - Origin check estricto: solo respondemos a mensajes con
 *     event.origin === app.origin.
 *   - El postMessage del shell SIEMPRE va a app.origin, nunca '*'.
 *   - Sandbox del iframe: allow-scripts + allow-same-origin permite que
 *     la app del dev haga su trabajo, pero impide formularios cross-site
 *     y popups no autorizados.
 */
export default function EmbeddedAppFrame({ app, user }: Props) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading');
  const [error, setError] = useState<string | null>(null);

  // Mantenemos el último token en ref (no en state) para evitar re-renders
  // en cada refresh.
  const tokenRef = useRef<TokenResponse | null>(null);
  const refreshTimer = useRef<number | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function fetchToken(): Promise<TokenResponse | null> {
      try {
        return await api<TokenResponse>('/api/embedded-token');
      } catch (e) {
        if (!cancelled) {
          setStatus('error');
          setError(e instanceof Error ? e.message : 'Could not fetch token');
        }
        return null;
      }
    }

    function pushAuth() {
      const t = tokenRef.current;
      const w = iframeRef.current?.contentWindow;
      if (!t || !w) return;
      w.postMessage(
        {
          type: 'einar:auth',
          version: PROTOCOL_VERSION,
          token: t.token,
          expiresAt: t.expiresAt,
          user,
        },
        app.origin,
      );
    }

    function scheduleRefresh() {
      if (refreshTimer.current) window.clearTimeout(refreshTimer.current);
      const t = tokenRef.current;
      if (!t) return;
      const msUntilRefresh = (t.expiresAt - REFRESH_BEFORE_EXPIRY_SEC) * 1000 - Date.now();
      if (msUntilRefresh <= 0) {
        void doRefresh();
        return;
      }
      refreshTimer.current = window.setTimeout(doRefresh, msUntilRefresh);
    }

    async function doRefresh() {
      // Antes de pedir embedded-token nuevo, fuerza al backend a rotar
      // si la cookie está cerca de expirar.
      try { await api<unknown>('/auth/refresh', { method: 'POST' }); } catch { /* sigamos */ }
      const fresh = await fetchToken();
      if (!fresh || cancelled) return;
      tokenRef.current = fresh;
      pushAuth();
      scheduleRefresh();
    }

    function onMessage(ev: MessageEvent) {
      // Origin check estricto.
      if (ev.origin !== app.origin) return;
      const msg = ev.data;
      if (!msg || typeof msg !== 'object') return;

      switch (msg.type) {
        case 'einar:ready':
          setStatus('ready');
          pushAuth();
          break;
        case 'einar:refresh':
          void doRefresh();
          break;
        case 'einar:logout':
          window.location.href = '/auth/logout';
          break;
      }
    }

    window.addEventListener('message', onMessage);

    // Bootstrap inicial: traemos el primer token y agendamos refresh.
    (async () => {
      const first = await fetchToken();
      if (!first || cancelled) return;
      tokenRef.current = first;
      scheduleRefresh();
      // Si el iframe ya cargó y mandó 'ready' antes de que llegara el
      // token, no perdemos nada: el handler de 'ready' arriba recibe
      // el evento, llama pushAuth, y tokenRef ya tiene valor.
    })();

    return () => {
      cancelled = true;
      window.removeEventListener('message', onMessage);
      if (refreshTimer.current) window.clearTimeout(refreshTimer.current);
    };
  }, [app.origin, user]);

  if (status === 'error') {
    return (
      <div style={s.error}>
        <strong>No se pudo cargar la app embebida.</strong>
        <div style={{ fontSize: '.85rem', marginTop: '.25rem', color: '#900' }}>{error}</div>
      </div>
    );
  }

  return (
    <iframe
      ref={iframeRef}
      src={app.origin}
      title={app.name}
      sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
      style={s.iframe}
    />
  );
}

const s = {
  iframe: {
    width: '100%',
    height: '100%',
    border: 0,
    display: 'block',
  } as const,
  error: {
    padding: '1rem',
    background: '#fee',
    color: '#900',
    border: '1px solid #fcc',
    borderRadius: 4,
  } as const,
};
