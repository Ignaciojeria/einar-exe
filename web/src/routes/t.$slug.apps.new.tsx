import { useState, type FormEvent } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { api, ApiError } from '../api/client';

interface CreateResponse {
  id: string;
  name: string;
  origin: string;
}

// `/t/$slug/apps/new` — form para registrar una embedded app.
//
// El backend valida:
//   - role del user (owner/admin)
//   - origin format (scheme://host[:port], sin path/query/fragment)
//   - unicidad por (tenant_id, origin)
export const Route = createFileRoute('/t/$slug/apps/new')({
  component: NewAppPage,
});

function NewAppPage() {
  const { slug } = Route.useParams();
  const nav = useNavigate();
  const qc = useQueryClient();

  const [name, setName] = useState('');
  const [origin, setOrigin] = useState('');
  const [iconUrl, setIconUrl] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const res = await api<CreateResponse>('/api/embedded-apps', {
        method: 'POST',
        json: {
          name: name.trim(),
          origin: origin.trim().replace(/\/$/, ''),
          iconUrl: iconUrl.trim() || undefined,
        },
      });
      await qc.invalidateQueries({ queryKey: ['embedded-apps'] });
      nav({
        to: '/t/$slug/app/$appId',
        params: { slug, appId: res.id },
        replace: true,
      });
    } catch (err) {
      setError(err instanceof ApiError ? (err.detail || err.message) : 'Error inesperado');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div style={s.wrap}>
      <form style={s.card} onSubmit={onSubmit}>
        <h1 style={{ marginTop: 0 }}>Registrar embedded app</h1>
        <p style={s.muted}>
          Cualquier web que sirva un origen válido puede embeberse. Va a recibir
          el token y user via <code>postMessage</code> usando el SDK de
          {' '}<code>/sdk/v1.js</code>.
        </p>

        <label style={s.label}>
          Nombre
          <input
            value={name}
            onChange={e => setName(e.target.value)}
            required maxLength={80}
            placeholder="Mi app"
            style={s.input}
          />
        </label>

        <label style={s.label}>
          Origin
          <input
            value={origin}
            onChange={e => setOrigin(e.target.value)}
            required
            type="url"
            placeholder="https://app.dominio.com"
            style={s.input}
          />
          <small style={s.hint}>
            scheme://host[:port], sin path/query. Ej: <code>https://app.foo.com</code>,
            <code> http://localhost:3001</code>.
          </small>
        </label>

        <label style={s.label}>
          Icon URL (opcional)
          <input
            value={iconUrl}
            onChange={e => setIconUrl(e.target.value)}
            type="url"
            placeholder="https://..."
            style={s.input}
          />
        </label>

        <button type="submit" disabled={submitting} style={s.btn}>
          {submitting ? 'Registrando…' : 'Registrar'}
        </button>
        {error && <div style={s.error}>{error}</div>}
      </form>
    </div>
  );
}

const s = {
  wrap: { padding: '2rem 3rem', height: '100%', overflowY: 'auto' } as const,
  card: { background: '#fff', padding: '2rem 2.5rem', borderRadius: 8, border: '1px solid #e0e0e0', maxWidth: 540 } as const,
  muted: { color: '#666', fontSize: '.9rem' } as const,
  label: { display: 'block', marginTop: '1rem', fontWeight: 600 } as const,
  input: { display: 'block', width: '100%', marginTop: '.4rem', padding: '.5rem', fontSize: '1rem', border: '1px solid #ccc', borderRadius: 4, boxSizing: 'border-box' } as const,
  hint: { color: '#888', display: 'block', marginTop: '.25rem' } as const,
  btn: { width: '100%', marginTop: '1.5rem', padding: '.7rem 1rem', background: '#111', color: '#fff', border: 0, borderRadius: 4, fontSize: '1rem', cursor: 'pointer' } as const,
  error: { marginTop: '1rem', padding: '.7rem', background: '#fee', color: '#c00', border: '1px solid #fcc', borderRadius: 4 } as const,
};
