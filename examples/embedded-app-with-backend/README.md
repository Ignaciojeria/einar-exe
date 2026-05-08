# Embedded app with backend — example

Full-stack starter: una app que se embebe en einar y tiene SU PROPIO
backend que valida los JWTs del SDK.

```
┌──────────────────────────────┐         ┌─────────────────────────┐
│  einar shell (einar.exe.xyz) │         │ tu app embebida         │
│                              │         │                         │
│  ┌────────────────────────┐  │         │  ┌──────────────────┐   │
│  │ /sdk/v1.js (postMsg)   │──┼─token──→│  │ frontend (React) │   │
│  └────────────────────────┘  │         │  └────────┬─────────┘   │
│                              │         │           │ Bearer JWT  │
│  /api/embedded-token         │         │           ↓             │
│    └─→ mints einar JWT       │         │  ┌──────────────────┐   │
│        signed RS256          │         │  │ backend (Go)     │   │
│                              │         │  │  validates JWT   │   │
│  /.well-known/einar/jwks ────┼─JWKS───→│  │  vs JWKS público │   │
│                              │         │  └──────────────────┘   │
└──────────────────────────────┘         └─────────────────────────┘
```

## Estructura

```
embedded-app-with-backend/
├── frontend/        # React + Vite + SDK einar
│   ├── index.html
│   ├── package.json
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       └── api.ts   # wrapper fetch que mete Bearer
├── backend/         # Go server con middleware JWT
│   ├── go.mod
│   ├── main.go
│   └── auth.go      # JWKS fetch + verify
├── docker-compose.yml
└── README.md (este archivo)
```

## Cómo funciona

### 1. Frontend recibe el JWT del shell

```html
<script src="https://einar.exe.xyz/sdk/v1.js"></script>
```

```ts
const e = einar.create({ appId: 'mi-app', shellOrigin: 'https://einar.exe.xyz' });
await e.ready();

const token   = e.getToken();   // JWT firmado por einar (RS256)
const user    = e.getUser();    // { sub, email, name, ... }
const issuer  = e.getIssuer();  // "https://einar.exe.xyz"
const jwksUri = e.getJwksUri(); // "https://einar.exe.xyz/.well-known/einar/jwks"

// Llamar TU backend con el token
const r = await fetch('https://tu-app.com/api/secret', {
  headers: { Authorization: 'Bearer ' + token },
});
```

### 2. Backend Go valida el JWT

El backend hace fetch del JWKS al startup (con cache TTL), y en cada
request:

1. Lee `Authorization: Bearer <jwt>` del header.
2. Verifica firma RS256 contra la public key del JWKS (matching por `kid`).
3. Verifica `iss == https://einar.exe.xyz`.
4. Verifica `exp > now`.
5. Inyecta los claims en el context.

Después del middleware, tu handler tiene:

```go
claims := AuthFromContext(r.Context())
log.Printf("user=%s tenant=%s role=%s",
    claims.Email, claims.TenantSlug, claims.Role)
```

Ver `backend/auth.go` para la implementación.

## Probarlo localmente

### 1. Levantar el backend

```bash
cd backend/
go run .
# Server listens on :3001
```

### 2. Levantar el frontend

```bash
cd frontend/
npm install
npm run dev
# Vite serves on :5173
```

### 3. Registrar la app en einar

En `https://einar.exe.xyz/t/{tu-slug}/apps/new`:

- Name: `My embedded app`
- Origin: `http://localhost:5173`
- (priority): `100`

### 4. Probar

Navegá a `https://einar.exe.xyz/t/{tu-slug}/app/{uuid-recien-creado}`
y la app debería:

- Cargar el iframe.
- Mostrar tu email y tenant slug (vienen del JWT validado en backend).
- Cualquier request al backend con el JWT funciona.
- Si manipulás el JWT, el backend devuelve 401.

## Validación alternativa: `/whoami`

Si no querés implementar JWKS validation en tu backend (prototipos,
scripts), podés delegar:

```go
req, _ := http.NewRequest("GET", "https://einar.exe.xyz/whoami", nil)
req.Header.Set("Authorization", "Bearer "+token)
resp, _ := http.DefaultClient.Do(req)
// resp.Body tiene { sub, email, tenant_slug, role, ... }
```

Round-trip extra por request, pero cero crypto en tu backend. Útil para
arrancar; migrá a validación local cuando importe la latencia.

## Claims disponibles

```json
{
  "iss": "https://einar.exe.xyz",
  "sub": "user-id-estable-de-casdoor",
  "email": "alice@acme.com",
  "name": "Alice",
  "tenant_id": "uuid-del-tenant",
  "tenant_slug": "acme",
  "role": "owner",
  "iat": 1730000000,
  "exp": 1730000600,
  "nbf": 1730000000
}
```

`role` ∈ `{owner, admin, member}`. Usalo para autorización en tu backend.

## Producción

- **Cachear JWKS**: la lib `jose` (Node) y `go-oidc` (Go) ya lo hacen
  automático con TTL razonable. Si lo armás manual, cachéa al menos
  10 minutos.
- **Rotación de keys**: si einar rota su clave privada, el JWT viene
  con un `kid` distinto. Tu lib debería refetchear JWKS automáticamente
  on-cache-miss. Verificá que tu lib lo haga.
- **Audience**: por ahora einar mintea sin audience. En el futuro podría
  mintear con `aud: "https://tu-app.com"` para que un token robado a una
  embedded app no sirva en otra. Configurá tu validador con audience
  cuando esté disponible.
