// Wrapper TS sobre el SDK global `window.einar` que el script-tag de
// `https://einar.exe.xyz/sdk/v1.js` deja en window.
//
// El SDK real está escrito en JS plano por simplicidad; acá le ponemos
// tipos y un singleton inicializado para todo el módulo.

declare global {
  interface Window {
    einar: {
      create: (opts: { appId: string; shellOrigin?: string }) => EinarClient;
      version: number;
    };
  }
}

export interface EinarUser {
  sub: string;
  email: string;
  name?: string;
  displayName?: string;
  picture?: string;
}

export interface EinarAuth {
  token: string;
  user: EinarUser;
  expiresAt: number;
}

export interface EinarClient {
  ready(): Promise<void>;
  getToken(): string | null;
  getUser(): EinarUser | null;
  getExpiresAt(): number;
  onAuth(cb: (a: EinarAuth) => void): () => void;
  onSessionEnded(cb: () => void): () => void;
  logout(): void;
  fetch(input: RequestInfo, init?: RequestInit): Promise<Response>;
}

// Singleton: una sola instancia para toda la app.
//
// `shellOrigin` lee de Vite env var. Default a producción.
export const einarClient: EinarClient = window.einar.create({
  appId: 'tanstack-embedded-app',
  shellOrigin: import.meta.env.VITE_EINAR_SHELL ?? 'https://einar.exe.xyz',
});
