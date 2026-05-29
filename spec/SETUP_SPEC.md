# SETUP_SPEC — Levantar ecosistema interno completo

Objetivo: dejar la VM en estado operativo con todo el stack interno de `einar-exe` corriendo y verificable.

## Alcance

Este spec cubre:
- Bootstrap de entorno (`.env`, secretos)
- Base de datos (roles/dbs/migraciones)
- Casdoor (init data + admin)
- Metabase (bootstrap admin)
- App Go + Caddy + OpenObserve + pgweb

No cubre:
- Hardening de producción
- Backups automáticos
- CI/CD

## Precondiciones

1. SO Linux con permisos para usar Docker.
2. Binarios disponibles:
   - `docker`
   - `docker compose`
   - `openssl`
   - `python3`
   - `npm`
3. Puerto host `8000` libre (usado por Caddy).
4. Repo clonado en directorio de trabajo.

## Inputs

- Código fuente del repo.
- `.env.example` válido.
- (Opcional) Credenciales OAuth reales para Google en `.env`.

## Procedimiento canónico

### Paso 1 — Entrar al repo

```bash
cd einar-exe
```

### Paso 2 — Ejecutar setup idempotente

```bash
./scripts/setup.sh
```

### Paso 3 — Verificar estado de contenedores

```bash
docker compose ps
```

Criterio de éxito:
- Servicios esenciales en estado `Up`:
  - `db`
  - `app`
  - `casdoor`
  - `metabase`
  - `caddy`
  - `openobserve`
  - `pgweb`
- Si hay healthcheck, debe estar en `healthy`.

### Paso 4 — Verificar endpoints mínimos

```bash
curl -I http://localhost:8080
curl -I http://localhost:8000
```

Criterio de éxito:
- Respuesta HTTP válida (2xx/3xx aceptable).

## Invariantes de idempotencia

- Re-ejecutar `./scripts/setup.sh` NO debe romper el estado existente.
- Si `.env` existe, no se regenera.
- Roles/DBs ya creados se preservan y solo se alinean passwords/ownership.
- Migraciones se aplican incrementalmente.
- Bootstrap de Metabase se salta si ya fue ejecutado.

## Recoverability / Retry

Si falla un paso por timing/healthcheck:
1. Revisar logs del servicio afectado:
   ```bash
   docker compose logs --tail=200 <service>
   ```
2. Re-ejecutar setup:
   ```bash
   ./scripts/setup.sh
   ```

## Reset completo (destructivo)

```bash
docker compose down -v && ./scripts/setup.sh
```

Efecto:
- Se eliminan volumes persistentes y se recrea todo desde cero.

## Criterio final de aceptación

El setup se considera exitoso si:
1. `docker compose ps` muestra el stack esencial operativo.
2. App responde en `http://localhost:8080`.
3. Proxy/Caddy responde en `http://localhost:8000`.
4. No hay errores bloqueantes en logs al arranque.

## Referencias

- Guía humana: [`README_SETUP.md`](../README_SETUP.md)
- Resumen del proyecto: [`README.md`](../README.md)
