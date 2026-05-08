import { useSyncExternalStore } from 'react';
import { einarClient, type EinarAuth } from './einar';

// useEinar: hook React que se actualiza cada vez que el SDK rota el token.
//
// useSyncExternalStore es la API estándar para suscribirse a stores
// externos en React 18+. Garantiza que el componente re-renderice
// cuando llegue 'einar:auth' del shell.
export function useEinar(): EinarAuth | null {
  return useSyncExternalStore(
    (cb) => einarClient.onAuth(() => cb()),
    () => snapshot(),
    () => null,
  );
}

function snapshot(): EinarAuth | null {
  const token = einarClient.getToken();
  const user = einarClient.getUser();
  const expiresAt = einarClient.getExpiresAt();
  if (!token || !user) return null;
  return { token, user, expiresAt };
}
