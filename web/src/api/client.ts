// Cliente HTTP del shell. Toda llamada al backend pasa por aquí para
// homogeneizar:
//   - credentials: 'include' (la cookie HttpOnly viaja sola, BFF)
//   - Accept: application/json (forzamos JSON, evita content-negotiation)
//   - manejo uniforme de errores → throw Error con detalle del backend

export class ApiError extends Error {
  status: number;
  detail?: string;
  constructor(status: number, message: string, detail?: string) {
    super(message);
    this.status = status;
    this.detail = detail;
  }
}

interface FuegoError { title?: string; detail?: string; status?: number }
interface MiddlewareError { error?: string; detail?: string; status?: number }

async function parseError(res: Response): Promise<ApiError> {
  let body: FuegoError & MiddlewareError | null = null;
  try { body = await res.json(); } catch { /* ignore */ }
  const msg = body?.title || body?.error || `HTTP ${res.status}`;
  return new ApiError(res.status, msg, body?.detail);
}

export async function api<T>(
  path: string,
  init?: RequestInit & { json?: unknown },
): Promise<T> {
  const opts: RequestInit = {
    credentials: 'include',
    headers: {
      Accept: 'application/json',
      ...(init?.json !== undefined && { 'Content-Type': 'application/json' }),
      ...(init?.headers || {}),
    },
    ...(init?.json !== undefined && { body: JSON.stringify(init.json) }),
    ...init,
  };

  const res = await fetch(path, opts);
  if (!res.ok) throw await parseError(res);
  // 204 No Content
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}
