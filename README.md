# einar-exe

App en Go con Postgres (+ PostGIS) y Casdoor para auth.

## Requisitos

- Docker + Docker Compose v2
- `openssl` (para generar secretos)

## Setup

```bash
./scripts/setup.sh
```

El script es idempotente: detecta qué falta (`.env`, usuarios, databases, migraciones) y solo ejecuta lo necesario. Puedes correrlo cuantas veces quieras.

## Servicios

### Desarrollo local

| Servicio    | URL                            |
|-------------|--------------------------------|
| App         | http://localhost:8080          |
| Casdoor     | http://localhost:8000          |
| OpenObserve | http://localhost:5080          |
| Redash      | http://localhost:5000          |
| Postgres    | `localhost:5432` (user: einar) |

### Producción

Todos los servicios viven bajo el mismo dominio `einar.exe.xyz`,
distinguidos por puerto.

| Servicio    | URL                              |
|-------------|----------------------------------|
| Casdoor     | https://einar.exe.xyz            |
| App         | https://einar.exe.xyz:8080       |
| Redash      | https://einar.exe.xyz:5000       |
| OpenObserve | https://einar.exe.xyz:5080       |
| Postgres    | `einar.exe.xyz:5432` (user: einar) |

> Las URLs públicas en prod deben coincidir **exactamente** con
> `CASDOOR_ORIGIN` y `APP_PUBLIC_URL` en `.env`. Si difieren, los
> redirects OAuth fallan (`redirect_uri mismatch`) y la validación de
> `iss` del JWT rechaza tokens válidos.

## Endpoints de autenticación

| Endpoint                | Verb     | Descripción                                                            |
|-------------------------|----------|------------------------------------------------------------------------|
| `/auth/login`           | GET      | Inicia OIDC. Genera `state` CSRF y redirige a Casdoor con Google preseleccionado. Acepta `?return=/ruta` para volver tras el login. |
| `/auth/callback`        | GET      | Recibe `code` de Casdoor, valida `state`, intercambia por `id_token`, verifica firma contra JWKS dinámico, setea cookie `einar_session` HttpOnly. |
| `/auth/logout`          | GET/POST | Borra cookie local y redirige a `/api/logout` de Casdoor.              |
| `/me`                   | GET      | Ruta protegida de ejemplo. Devuelve los claims del usuario autenticado o `401`. |

### Endpoints OIDC del IdP (Casdoor)

`{CASDOOR_ORIGIN}` = `http://localhost:8000` (dev) o `https://einar.exe.xyz` (prod, puerto 443).

| Endpoint OIDC  | Path                                  |
|----------------|---------------------------------------|
| Discovery      | `/.well-known/openid-configuration`   |
| Authorization  | `/login/oauth/authorize`              |
| Token          | `/api/login/oauth/access_token`       |
| UserInfo       | `/api/userinfo`                       |
| JWKS           | `/.well-known/jwks`                   |

Detalle del flujo y truco de discovery interno vs `iss` público:
[`docs/casdoor-integration-plan.md` §12bis](docs/casdoor-integration-plan.md).

### Pantalla de login de Casdoor

Directo a la app `einar-app`:

- Dev: http://localhost:8000/login/einar
- Prod: https://einar.exe.xyz/login/einar

En el flujo normal **no se usa** porque `/auth/login` pasa
`provider=provider_google_einar` y Casdoor salta su UI yendo directo a Google.

## Comandos útiles

```bash
docker compose ps                                      # estado
docker compose logs -f app                             # logs
docker compose exec db psql -U einar -d einar          # conectar a la DB
docker compose --profile tools run --rm migrate        # aplicar migraciones
docker compose --profile tools run --rm migrate down 1 # rollback
docker compose restart app                             # reiniciar app
```

## Secretos (JWT, certs)

La app valida los `id_token` de Casdoor vía **JWKS dinámico** (endpoint
`/.well-known/jwks`). No hace falta copiar la PEM a mano: la librería
`coreos/go-oidc` descarga y rota las claves automagícamente.

```bash
# (Opcional, fallback) si por algún motivo necesitas la clave embebida:
# Casdoor UI → Certs → Public key → PEM → secrets/casdoor-jwt.pem
```

La carpeta `secrets/` está en `.gitignore`.

> Variable de override `CASDOOR_JWT_PUBLIC_KEY_FILE` queda como fallback
> opcional; en el camino feliz no se usa. Ver
> [`docs/casdoor-integration-plan.md` §12](docs/casdoor-integration-plan.md).

## OpenObserve

UI de observabilidad (logs, métricas, traces) en **http://localhost:5080**

Credenciales configurables en `.env`:

```env
ZO_ROOT_USER_EMAIL=admin@example.com
ZO_ROOT_USER_PASSWORD=admin123
```

A diferencia de PostgreSQL, OpenObserve **re-aplica** las credenciales en cada reinicio.

## Persistencia de datos

PostgreSQL (`pgdata`) y OpenObserve (`o2data`) usan volumes nombrados.

| Comando | ¿Se pierde la data? |
|---|---|
| `docker compose down` | ❌ No |
| `docker compose restart` | ❌ No |
| `docker compose up --build` | ❌ No |
| `docker compose down -v` | ⚠️ **Sí** — el flag `-v` elimina los volumes |

## Reset completo

```bash
docker compose down -v && ./scripts/setup.sh   # ⚠️ borra todos los volumes y rearma todo
```
