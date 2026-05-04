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

## Reset completo

```bash
docker compose down -v && ./scripts/setup.sh   # ⚠️ borra el volumen pgdata y rearma todo
```
