import { useState, type FormEvent } from 'react';
import { createFileRoute, redirect } from '@tanstack/react-router';
import { api, ApiError } from '../api/client';
import { sessionQuery } from '../hooks/useSession';

interface SignupResponse {
  tenantId: string;
  slug: string;
  displayName: string;
  redirectTo: string;
}

// Signup — `/signup`
//
// Solo accesible para users autenticados sin tenant. El guardia vive
// en `beforeLoad` para no parpadear la UI.
export const Route = createFileRoute('/signup')({
  beforeLoad: async ({ context }) => {
    const session = await context.queryClient.ensureQueryData(sessionQuery());
    if (!session) {
      // Sin sesión: full-page redirect a auth/login (no es ruta de TSR).
      window.location.href = '/auth/login?return=/signup';
      throw new Error('redirecting to login');
    }
    if (session.tenantSlug) {
      throw redirect({
        to: '/t/$slug',
        params: { slug: session.tenantSlug },
      });
    }
  },
  component: SignupPage,
});

function SignupPage() {
  const [slug, setSlug] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const res = await api<SignupResponse>('/api/signup', {
        method: 'POST',
        json: {
          slug: slug.trim().toLowerCase(),
          displayName: displayName.trim(),
        },
      });
      // Full reload tras signup: garantiza cache limpio (queries de
      // sesión + embedded-apps + credentials se hidratan desde cero
      // con el tenant ya asignado). Ahorra debug de race conditions.
      window.location.href = res.redirectTo;
      return;
    } catch (err) {
      if (err instanceof ApiError) setError(err.detail || err.message);
      else setError('Error inesperado.');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div style={s.wrap}>
      <form style={s.card} onSubmit={onSubmit}>
        <h1 style={{ marginTop: 0 }}>Crear tu workspace</h1>
        <p style={s.muted}>Definí cómo se llamará tu workspace.</p>

        <label style={s.label}>
          Slug
          <input
            value={slug}
            onChange={e => setSlug(e.target.value)}
            required
            pattern="[a-z0-9][a-z0-9-]{1,30}[a-z0-9]"
            placeholder="acme"
            autoComplete="off"
            style={s.input}
          />
          <small style={s.hint}>3-32 caracteres. Lowercase + dígitos + guiones.</small>
        </label>

        <label style={s.label}>
          Nombre visible
          <input
            value={displayName}
            onChange={e => setDisplayName(e.target.value)}
            required
            maxLength={80}
            placeholder="Acme Corp"
            style={s.input}
          />
        </label>

        <button type="submit" disabled={submitting} style={s.btn}>
          {submitting ? 'Creando…' : 'Crear workspace'}
        </button>
        {error && <div style={s.error}>{error}</div>}
      </form>
    </div>
  );
}

const s = {
  wrap: { minHeight: '100vh', display: 'grid', placeItems: 'center', background: '#fafafa', fontFamily: 'system-ui, sans-serif' } as const,
  card: { background: '#fff', padding: '2rem 2.5rem', borderRadius: 8, boxShadow: '0 2px 12px rgba(0,0,0,0.06)', width: 420, maxWidth: '90vw' } as const,
  muted: { color: '#666' } as const,
  label: { display: 'block', marginTop: '1rem', fontWeight: 600 } as const,
  input: { display: 'block', width: '100%', marginTop: '.4rem', padding: '.5rem', fontSize: '1rem', border: '1px solid #ccc', borderRadius: 4, boxSizing: 'border-box' } as const,
  hint: { color: '#888' } as const,
  btn: { width: '100%', marginTop: '1.5rem', padding: '.7rem 1rem', background: '#111', color: '#fff', border: 0, borderRadius: 4, fontSize: '1rem', cursor: 'pointer' } as const,
  error: { marginTop: '1rem', padding: '.7rem', background: '#fee', color: '#c00', border: '1px solid #fcc', borderRadius: 4 } as const,
};
