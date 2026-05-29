# Setup del ecosistema interno (einar-exe)

Guía operativa para levantar **todo el stack** en una VM nueva.

> Para ejecución estricta por agente, usa también: [`spec/SETUP_SPEC.md`](spec/SETUP_SPEC.md).

## 1) Requisitos

- Linux x86_64
- Docker + Docker Compose v2 (`docker compose`)
- `openssl`
- `python3`
- `npm` (solo si vas a compilar el frontend en `web/`)

Checks rápidos:

```bash
docker --version
docker compose version
openssl version
python3 --version
npm --version
```

## 2) Clonar

```bash
git clone <URL_DEL_REPO> einar-exe
cd einar-exe
```

## 3) Configurar entorno

El setup crea `.env` automáticamente desde `.env.example` con secretos aleatorios si no existe.

Si necesitas valores concretos (OAuth/URLs), edita `.env` después del primer run.

Variables críticas a revisar:
- `APP_PUBLIC_URL`
- `CASDOOR_ORIGIN`
- `CASDOOR_ORIGIN_FRONTEND`
- `GOOGLE_CLIENT_ID`
- `GOOGLE_CLIENT_SECRET`

## 4) Levantar stack completo (idempotente)

```bash
./scripts/setup.sh
```

Qué hace (resumen):
1. Crea `secrets/` si falta.
2. Crea `.env` con secretos si falta.
3. Levanta PostgreSQL y espera `healthy` estable.
4. Crea/alinea roles y databases (`einar`, `casdoor`, `metabase`).
5. Ejecuta migraciones SQL (`migrate`).
6. Renderiza `casdoor/init_data.json` desde template.
7. Compila SPA (`web/`) para `internal/web/dist/`.
8. Levanta todo el `docker-compose`.
9. Alinea admin de Casdoor.
10. Bootstrap admin de Metabase.

## 5) Verificación de salud

```bash
docker compose ps
```

Esperado: `db`, `app`, `casdoor`, `metabase`, `caddy`, `openobserve`, `pgweb` arriba (y healthchecks OK donde aplique).

Pruebas HTTP locales:

```bash
curl -I http://localhost:8080
curl -I http://localhost:8000
```

## 6) URLs

### Local
- App: `http://localhost:8080`
- Casdoor (vía Caddy): `http://localhost:8000`

### exe.dev (pública)
- URL base: `https://einar.exe.xyz:8000`
- Caddy enruta internamente por path al resto de servicios.

## 7) Operación diaria

```bash
docker compose logs -f app
docker compose restart app
docker compose down
docker compose up -d
```

Migraciones:

```bash
docker compose --profile tools run --rm migrate
docker compose --profile tools run --rm migrate down 1
```

## 8) Reset total (destructivo)

```bash
docker compose down -v && ./scripts/setup.sh
```

⚠️ `down -v` elimina volumes (`pgdata`, `o2data`, etc.).

## 9) Problemas comunes

- **`redirect_uri mismatch` en login**
  - Verifica que URLs públicas en `.env` coincidan exactamente con los dominios/puertos reales.

- **Metabase/Casdoor no llegan a healthy en primer intento**
  - Reintenta `./scripts/setup.sh` (es idempotente).

- **Error compilando frontend**
  - Asegura Node/npm instalados y ejecuta `npm install` dentro de `web/`.
