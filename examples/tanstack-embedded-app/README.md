# tanstack-embedded-app

Example de embedded app para **einar** usando React + TanStack Query.

## ¿Qué demuestra?

- Cómo recibir el token del shell vía `postMessage` (SDK `/sdk/v1.js`).
- Cómo refrescarlo automáticamente cuando el shell rota.
- Cómo hacer fetch autenticado con `einarClient.fetch()`.
- Cómo integrarlo con TanStack Query (refetch on token rotation).

## Correr local

```bash
npm install
npm run dev          # http://localhost:5174
```

Después, en el shell:

1. Login: `https://einar.exe.xyz/`
2. Ir a tu workspace → **🧪 Test your app** en el sidenav
3. Escribir `http://localhost:5174` → "Cargar"
4. Vas a ver tu app embebida con sesión real

## Estructura

```
src/
├─ einar.ts        — wrapper TS sobre window.einar
├─ useEinar.ts     — hook React (useSyncExternalStore)
├─ App.tsx         — UI demo + TanStack Query
└─ main.tsx        — entrypoint
```

## Pattern: integración TanStack Query

```tsx
const { data } = useQuery({
  queryKey: ['orders', auth?.token],   // refetch al rotar token
  enabled: !!auth,                      // espera primer 'einar:auth'
  queryFn: () =>
    einarClient.fetch('/api/orders').then(r => r.json()),
});
```

Por qué `auth?.token` en la queryKey: TanStack invalida el cache si el
token cambia, garantizando que ningún render quede mostrando data
asociada a una sesión anterior.

## Validar el token en TU backend

Tu backend (sea Go, Node, Python, etc.) recibe el JWT en
`Authorization: Bearer <jwt>`. Lo validás contra el JWKS público de
Casdoor:

```
https://einar.exe.xyz/.well-known/jwks
```

Audience esperado: el `client_id` de la app `einar-app` (o el que
corresponda según el setup multi-tenant).

## Para producción

Cuando tu app esté deployada con su URL real:

1. Registrala en el shell: **+ Registrar app** → name + origin.
2. El shell solo va a hablarle a ese origin exacto.
3. La entrada del Dev tester (`/t/{slug}/dev`) NO sirve para apps remotas
   por seguridad — solo localhost / 127.0.0.1.
