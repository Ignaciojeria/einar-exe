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

Servicios:

| Servicio | URL |
|---|---|
| App | http://localhost:8080 |
| Casdoor | http://localhost:8000 |
| OpenObserve | http://localhost:5080 |
| Postgres | `localhost:5432` |

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

La clave pública de Casdoor para verificar JWT se monta como archivo:

```bash
# Copiar la PEM desde Casdoor UI → Certs → Public key
cp tu-clave.pem secrets/casdoor-jwt.pem
```

La app lee el path desde `CASDOOR_JWT_PUBLIC_KEY_FILE`. La carpeta `secrets/` está en `.gitignore`.

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
