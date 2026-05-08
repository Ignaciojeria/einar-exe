import { useState } from 'react';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { api, ApiError } from '../api/client';
import { useEmbeddedApps, type EmbeddedApp } from '../hooks/useEmbeddedApps';

// `/t/$slug/apps` — gestión de embedded apps del tenant.
//
// Lista todas las apps (system + custom), permite editar nombre/icon/posición
// inline, y borrar las custom. Las system muestran un badge y no tienen
// botón de borrar.
export const Route = createFileRoute('/t/$slug/apps/')({
  component: AppsManager,
});

function AppsManager() {
  const { slug } = Route.useParams();
  const { data: apps = [], isLoading } = useEmbeddedApps();

  const systemApps = apps.filter(a => a.isSystem);
  const customApps = apps.filter(a => !a.isSystem);

  return (
    <div style={s.wrap}>
      <header style={s.header}>
        <div>
          <h1 style={s.title}>Embedded apps</h1>
          <p style={s.muted}>
            Aplicaciones que aparecen en el sidenav y reciben el token via postMessage.
          </p>
        </div>
        <Link to="/t/$slug/apps/new" params={{ slug }} style={s.btnPrimary}>
          + Registrar app
        </Link>
      </header>

      {isLoading && <div style={s.muted}>Cargando…</div>}

      {systemApps.length > 0 && (
        <Section title="Sistema" hint="Aprovisionadas por la plataforma. No se pueden borrar.">
          {systemApps.map(app => <AppRow key={app.id} app={app} slug={slug} />)}
        </Section>
      )}

      <Section
        title="Custom"
        hint={customApps.length === 0 ? 'Aún no registraste apps custom.' : undefined}
      >
        {customApps.map(app => <AppRow key={app.id} app={app} slug={slug} />)}
      </Section>
    </div>
  );
}

function Section({ title, hint, children }: { title: string; hint?: string; children?: React.ReactNode }) {
  return (
    <section style={s.section}>
      <h2 style={s.sectionTitle}>{title}</h2>
      {hint && <p style={s.muted}>{hint}</p>}
      <div style={s.list}>{children}</div>
    </section>
  );
}

function AppRow({ app, slug }: { app: EmbeddedApp; slug: string }) {
  const qc = useQueryClient();
  const nav = useNavigate();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(app.name);
  const [iconUrl, setIconUrl] = useState(app.iconUrl ?? '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save() {
    setError(null);
    setBusy(true);
    try {
      await api(`/api/embedded-apps/${app.id}`, {
        method: 'PATCH',
        json: {
          name: name.trim(),
          iconUrl: iconUrl.trim() || null,
        },
      });
      await qc.invalidateQueries({ queryKey: ['embedded-apps'] });
      setEditing(false);
    } catch (e) {
      setError(e instanceof ApiError ? (e.detail || e.message) : 'Error');
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!confirm(`¿Borrar "${app.name}"? Esto no se puede deshacer.`)) return;
    setBusy(true);
    try {
      await api(`/api/embedded-apps/${app.id}`, { method: 'DELETE' });
      await qc.invalidateQueries({ queryKey: ['embedded-apps'] });
    } catch (e) {
      setError(e instanceof ApiError ? (e.detail || e.message) : 'Error');
      setBusy(false);
    }
  }

  if (editing) {
    return (
      <div style={s.row}>
        <div style={s.rowMain}>
          <input
            value={name} onChange={e => setName(e.target.value)}
            style={s.inlineInput} placeholder="Nombre"
          />
          <input
            value={iconUrl} onChange={e => setIconUrl(e.target.value)}
            style={s.inlineInput} placeholder="Icon URL (opcional)"
          />
        </div>
        <div style={s.rowActions}>
          <button onClick={save} disabled={busy} style={s.btnSm}>Guardar</button>
          <button onClick={() => { setEditing(false); setName(app.name); setIconUrl(app.iconUrl ?? ''); }} style={s.btnSmGhost}>Cancelar</button>
        </div>
        {error && <div style={s.error}>{error}</div>}
      </div>
    );
  }

  return (
    <div style={s.row}>
      <div style={s.rowMain}>
        {app.iconUrl && <img src={app.iconUrl} alt="" style={s.icon} />}
        <div>
          <div style={s.rowName}>
            {app.name}
            {app.isSystem && <span style={s.badge}>system</span>}
          </div>
          <code style={s.rowOrigin}>{app.origin}</code>
        </div>
      </div>
      <div style={s.rowActions}>
        <button
          onClick={() => nav({ to: '/t/$slug/app/$appId', params: { slug, appId: app.id } })}
          style={s.btnSmGhost}
        >
          Abrir
        </button>
        <button onClick={() => setEditing(true)} disabled={busy} style={s.btnSmGhost}>Editar</button>
        {!app.isSystem && (
          <button onClick={remove} disabled={busy} style={s.btnSmDanger}>Borrar</button>
        )}
      </div>
      {error && <div style={s.error}>{error}</div>}
    </div>
  );
}

const s = {
  wrap: { padding: '2rem 3rem', height: '100%', overflowY: 'auto' } as const,
  header: { display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '2rem' } as const,
  title: { margin: 0 } as const,
  muted: { color: '#666', marginTop: '.25rem' } as const,
  btnPrimary: { padding: '.5rem 1rem', background: '#111', color: '#fff', textDecoration: 'none', borderRadius: 4, fontSize: '.9rem' } as const,
  section: { marginBottom: '2.5rem' } as const,
  sectionTitle: { fontSize: '.85rem', textTransform: 'uppercase', letterSpacing: '.05em', color: '#888', marginBottom: '.5rem' } as const,
  list: { display: 'flex', flexDirection: 'column', gap: '.5rem', marginTop: '.75rem' } as const,
  row: { display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '1rem 1.25rem', background: '#fff', border: '1px solid #e0e0e0', borderRadius: 6, gap: '1rem', flexWrap: 'wrap' } as const,
  rowMain: { display: 'flex', alignItems: 'center', gap: '.75rem', flex: 1, minWidth: 240 } as const,
  rowName: { fontWeight: 600 } as const,
  rowOrigin: { fontSize: '.8rem', color: '#888' } as const,
  rowActions: { display: 'flex', gap: '.4rem' } as const,
  icon: { width: 32, height: 32, borderRadius: 4 } as const,
  badge: { marginLeft: '.5rem', padding: '0 .4rem', background: '#eef', color: '#557', fontSize: '.7rem', borderRadius: 3, fontWeight: 'normal', textTransform: 'uppercase' } as const,
  btnSm: { padding: '.35rem .75rem', background: '#111', color: '#fff', border: 0, borderRadius: 4, cursor: 'pointer', fontSize: '.85rem' } as const,
  btnSmGhost: { padding: '.35rem .75rem', background: '#fff', color: '#333', border: '1px solid #ccc', borderRadius: 4, cursor: 'pointer', fontSize: '.85rem' } as const,
  btnSmDanger: { padding: '.35rem .75rem', background: '#fff', color: '#c00', border: '1px solid #fcc', borderRadius: 4, cursor: 'pointer', fontSize: '.85rem' } as const,
  inlineInput: { flex: 1, padding: '.4rem .5rem', fontSize: '.9rem', border: '1px solid #ccc', borderRadius: 4 } as const,
  error: { width: '100%', marginTop: '.5rem', padding: '.5rem .75rem', background: '#fee', color: '#c00', border: '1px solid #fcc', borderRadius: 4, fontSize: '.85rem' } as const,
};
