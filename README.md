# einar-exe

## Requisitos

- Docker
- Docker Compose

## Levantar el proyecto

```bash
docker compose up
```

Esto levanta:

| Servicio | Puerto | Detalle |
|---|---|---|
| **App (Go)** | 8080 | Hot-reload automático con Air |
| **PostgreSQL 17 + PostGIS 3.5** | 5432 | Base de datos geoespacial |

La app recompila automáticamente cada vez que guardas cambios en archivos `.go`.

Para correr en background:

```bash
docker compose up -d
docker compose logs -f app   # ver logs en vivo
```

## Conexión a PostgreSQL

Desde el contenedor de la app, usa la variable de entorno `DATABASE_URL`:

```
postgres://postgres:postgres@db:5432/einar?sslmode=disable
```

Desde tu máquina local o un cliente remoto (DBeaver, pgAdmin, TablePlus, etc.):

| Parámetro | Valor |
|---|---|
| Host | `localhost` (o la IP del servidor) |
| Puerto | `5432` |
| Usuario | `postgres` |
| Contraseña | `postgres` |
| Base de datos | `einar` |

Conexión por terminal:

```bash
# Desde el host
psql -h localhost -U postgres -d einar

# Desde dentro del contenedor
docker compose exec db psql -U postgres -d einar
```

## Persistencia de datos

La base de datos usa un volume nombrado (`pgdata`). La data se mantiene entre reinicios.

| Comando | ¿Se pierde la data? |
|---|---|
| `docker compose down` | ❌ No |
| `docker compose restart` | ❌ No |
| `docker compose up --build` | ❌ No |
| `docker compose down -v` | ⚠️ **Sí** — el flag `-v` elimina los volumes |

## Rebuild

Si modificas el `Dockerfile`:

```bash
docker compose up --build
```
