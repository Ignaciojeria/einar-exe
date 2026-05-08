import { useState, type ReactNode } from 'react';
import { createFileRoute } from '@tanstack/react-router';
import {
  useCredentials,
  type OpenObserveCreds,
  type MetabaseCreds,
} from '../hooks/useCredentials';

// `/t/$slug/credentials` — credenciales de las apps system del tenant.
//
// Cada herramienta tiene su tarjeta con email/password/IDs internos.
// El layout padre (t.$slug.tsx) ya valida sesión + tenant.
export const Route = createFileRoute('/t/$slug/credentials')({
  component: CredentialsPage,
});

function CredentialsPage() {
  const { data, isLoading, error } = useCredentials();

  return (
    <div style={s.wrap}>
      <header>
        <h1 style={s.h1}>App credentials</h1>
        <p style={s.muted}>
          Generadas automáticamente al crear el workspace. Úsalas para loguearte
          en cada herramienta o para llamar sus APIs directamente.
        </p>
      </header>

      {isLoading && <div>Cargando…</div>}
      {error && <div style={s.cardError}>Error: {(error as Error).message}</div>}

      {data?.openobserve && <OpenObserveCard creds={data.openobserve} />}
      {data?.metabase && <MetabaseCard creds={data.metabase} />}

      {!isLoading && !data?.openobserve && !data?.metabase && (
        <div style={s.cardWarn}>
          No hay credenciales aún. Probable causa: el provisioning falló al
          signup. Revisa los logs del backend.
        </div>
      )}
    </div>
  );
}

// ─── Cards por herramienta ────────────────────────────────────────────

function OpenObserveCard({ creds }: { creds: OpenObserveCreds }) {
  return (
    <Card title="OpenObserve" loginUrl={creds.loginUrl}>
      <Field label="Email"    value={creds.email} />
      <Field label="Password" value={creds.password} secret />
      <Field label="Org ID"   value={creds.orgId} />

      <Details summary="API endpoints">
        <Pre>{`# Search logs
curl -u "${creds.email}:${creds.password}" \\
  https://einar.exe.xyz/o2/api/${creds.orgId}/_search \\
  -H "Content-Type: application/json" \\
  -d '{"query":{"sql":"SELECT * FROM default LIMIT 10"}}'

# Ingest log
curl -u "${creds.email}:${creds.password}" \\
  https://einar.exe.xyz/o2/api/${creds.orgId}/default/_json \\
  -H "Content-Type: application/json" \\
  -d '[{"level":"info","msg":"hello"}]'`}</Pre>
      </Details>
    </Card>
  );
}

function MetabaseCard({ creds }: { creds: MetabaseCreds }) {
  return (
    <Card title="Metabase" loginUrl={creds.loginUrl}>
      <Field label="Email"          value={creds.email} />
      <Field label="Password"       value={creds.password} secret />
      <Field label="Group ID"       value={String(creds.groupId)} />
      <Field label="Collection ID"  value={String(creds.collectionId)} />

      <Details summary="API endpoints">
        <Pre>{`# Login (devuelve session id en cookie/JSON)
curl -X POST https://einar.exe.xyz/mb/api/session \\
  -H "Content-Type: application/json" \\
  -d '{"username":"${creds.email}","password":"${creds.password}"}'

# Listar dashboards de tu collection
curl https://einar.exe.xyz/mb/api/collection/${creds.collectionId}/items \\
  -H "X-Metabase-Session: <session_id>"`}</Pre>
      </Details>
    </Card>
  );
}

// ─── Building blocks ──────────────────────────────────────────────────

function Card({ title, loginUrl, children }: { title: string; loginUrl: string; children: ReactNode }) {
  return (
    <section style={s.card}>
      <header style={s.cardHeader}>
        <h2 style={s.h2}>{title}</h2>
        <a href={loginUrl} target="_blank" rel="noreferrer" style={s.linkOut}>
          Abrir login ↗
        </a>
      </header>
      {children}
    </section>
  );
}

function Field({ label, value, secret = false }: { label: string; value: string; secret?: boolean }) {
  const [revealed, setRevealed] = useState(!secret);
  const [copied, setCopied] = useState(false);

  function copy() {
    navigator.clipboard.writeText(value).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    });
  }

  return (
    <div style={s.field}>
      <label style={s.label}>{label}</label>
      <div style={s.fieldRow}>
        <code style={s.code}>{revealed ? value : '•'.repeat(Math.min(value.length, 32))}</code>
        {secret && (
          <button onClick={() => setRevealed(r => !r)} style={s.btnGhost}>
            {revealed ? 'Ocultar' : 'Mostrar'}
          </button>
        )}
        <button onClick={copy} style={s.btnGhost}>
          {copied ? '✓ Copiado' : 'Copiar'}
        </button>
      </div>
    </div>
  );
}

function Details({ summary, children }: { summary: string; children: ReactNode }) {
  return (
    <details style={s.details}>
      <summary style={s.summary}>{summary}</summary>
      {children}
    </details>
  );
}

function Pre({ children }: { children: ReactNode }) {
  return <pre style={s.pre}>{children}</pre>;
}

const s = {
  wrap: { padding: '2rem 3rem', height: '100%', overflowY: 'auto', maxWidth: 800 } as const,
  h1: { margin: 0 } as const,
  h2: { margin: 0, fontSize: '1.1rem' } as const,
  muted: { color: '#666', marginTop: '.25rem' } as const,
  card: { background: '#fff', border: '1px solid #e0e0e0', borderRadius: 8, padding: '1.25rem 1.5rem', marginTop: '1.5rem' } as const,
  cardHeader: { display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', marginBottom: '1rem', borderBottom: '1px solid #f0f0f0', paddingBottom: '.75rem' } as const,
  cardWarn: { background: '#fff8e1', border: '1px solid #ffe082', color: '#7a4f00', padding: '.9rem 1rem', borderRadius: 6, marginTop: '1rem' } as const,
  cardError: { background: '#fee', border: '1px solid #fcc', color: '#c00', padding: '.9rem 1rem', borderRadius: 6, marginTop: '1rem' } as const,
  linkOut: { fontSize: '.85rem', color: '#0a66ff', textDecoration: 'none' } as const,
  field: { marginBottom: '.75rem' } as const,
  label: { display: 'block', fontSize: '.75rem', textTransform: 'uppercase', letterSpacing: '.05em', color: '#888', marginBottom: '.25rem' } as const,
  fieldRow: { display: 'flex', alignItems: 'center', gap: '.5rem', flexWrap: 'wrap' } as const,
  code: { flex: 1, minWidth: 280, background: '#f7f7f7', padding: '.5rem .75rem', borderRadius: 4, fontFamily: 'monospace', fontSize: '.9rem', overflow: 'auto' } as const,
  btnGhost: { padding: '.4rem .75rem', background: '#fff', border: '1px solid #ccc', borderRadius: 4, fontSize: '.8rem', cursor: 'pointer' } as const,
  details: { marginTop: '1rem', borderTop: '1px solid #f0f0f0', paddingTop: '.75rem' } as const,
  summary: { cursor: 'pointer', fontSize: '.85rem', color: '#555' } as const,
  pre: { marginTop: '.75rem', background: '#1a1a1a', color: '#eee', padding: '.85rem', borderRadius: 4, fontSize: '.75rem', overflow: 'auto' } as const,
};
