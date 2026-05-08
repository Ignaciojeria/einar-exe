#!/usr/bin/env bash
# ============================================================
# einar-exe — Setup idempotente
# ============================================================
# Ejecutar muchas veces es seguro: cada paso detecta si ya está
# hecho y lo salta. Útil para:
#   - Primer arranque desde cero.
#   - Recuperarse de un setup parcial.
#   - Re-correr tras un `docker compose down -v`.
# ============================================================
set -euo pipefail

cd "$(dirname "$0")/.."

# sed -i portable (BSD/GNU). Usado para .env y para renderizar templates.
sedi() { if sed --version >/dev/null 2>&1; then sed -i "$@"; else sed -i '' "$@"; fi; }

# Colores opcionales
if [[ -t 1 ]]; then
    BOLD=$'\e[1m'; DIM=$'\e[2m'; OK=$'\e[32m'; WARN=$'\e[33m'; RST=$'\e[0m'
else
    BOLD=""; DIM=""; OK=""; WARN=""; RST=""
fi
step() { echo "${BOLD}→ $*${RST}"; }
done_() { echo "  ${OK}✓${RST} $*"; }
skip() { echo "  ${DIM}↷ $* (ya hecho)${RST}"; }

# ------------------------------------------------------------
# 1. .env
# ------------------------------------------------------------
# ------------------------------------------------------------
# 0. secrets/
# ------------------------------------------------------------
step "Verificando secrets/"
mkdir -p secrets
if [[ ! -d secrets ]]; then
    mkdir -p secrets
    done_ "secrets/ creado (carpeta vacía; queda como placeholder por si agregas keys futuras)"
else
    skip "secrets/ existe"
# La app valida JWTs vía JWKS dinámico, no usamos PEM.
fi

# ------------------------------------------------------------
# 1. .env
# ------------------------------------------------------------
step "Verificando .env"
if [[ ! -f .env ]]; then
    if [[ ! -f .env.example ]]; then
        echo "ERROR: falta .env.example" >&2; exit 1
    fi
    cp .env.example .env

    gen_pw()     { openssl rand -base64 24 | tr -d '/+=' | cut -c1-24; }
    gen_secret() { openssl rand -hex 32; }
    gen_id()     { openssl rand -hex 10; }

    PW_ROOT=$(gen_pw); PW_EINAR=$(gen_pw); PW_CASDOOR=$(gen_pw); PW_ADMIN=$(gen_pw)
    PW_ZO=$(gen_pw)
    CLIENT_ID=$(gen_id); CLIENT_SECRET=$(gen_secret)

    sedi -e "s|CHANGE_ME_ROOT|${PW_ROOT}|g" \
         -e "s|CHANGE_ME_EINAR|${PW_EINAR}|g" \
         -e "s|CHANGE_ME_CASDOOR|${PW_CASDOOR}|g" \
         -e "s|CHANGE_ME_ADMIN|${PW_ADMIN}|g" \
         -e "s|CHANGE_ME_CLIENT_ID|${CLIENT_ID}|g" \
         -e "s|CHANGE_ME_CLIENT_SECRET|${CLIENT_SECRET}|g" \
         -e "s|CHANGE_ME_ZO|${PW_ZO}|g" \
         -e "s|CHANGE_ME_RDPW|$(gen_pw)|g" \
         -e "s|CHANGE_ME_RDSK|$(gen_secret)|g" \
         -e "s|CHANGE_ME_RDCK|$(gen_secret)|g" \
         .env
    done_ ".env creado con secretos generados"
else
    skip ".env existe"
fi

# Leer solo las variables que el script necesita.
# Evitamos `. ./.env` porque valores con espacios sin comillar
# se rompen en bash (cada token se trata como una asignación).
env_get() {
    local key="$1"
    local val
    val=$(grep -E "^${key}=" .env | tail -1 | cut -d= -f2-)
    # Quitar comillas envolventes si las hay
    val="${val%\"}"; val="${val#\"}"
    val="${val%\'}"; val="${val#\'}"
    printf '%s' "$val"
}
POSTGRES_PASSWORD=$(env_get POSTGRES_PASSWORD)
EINAR_DB_USER=$(env_get EINAR_DB_USER)
EINAR_DB_PASSWORD=$(env_get EINAR_DB_PASSWORD)
EINAR_DB_NAME=$(env_get EINAR_DB_NAME)
CASDOOR_DB_USER=$(env_get CASDOOR_DB_USER)
CASDOOR_DB_PASSWORD=$(env_get CASDOOR_DB_PASSWORD)
CASDOOR_DB_NAME=$(env_get CASDOOR_DB_NAME)
CASDOOR_ADMIN_USERNAME=$(env_get CASDOOR_ADMIN_USERNAME)
CASDOOR_ADMIN_PASSWORD=$(env_get CASDOOR_ADMIN_PASSWORD)
CASDOOR_CLIENT_ID=$(env_get CASDOOR_CLIENT_ID)
CASDOOR_CLIENT_SECRET=$(env_get CASDOOR_CLIENT_SECRET)
GOOGLE_CLIENT_ID=$(env_get GOOGLE_CLIENT_ID)
GOOGLE_CLIENT_SECRET=$(env_get GOOGLE_CLIENT_SECRET)
APP_PUBLIC_URL=$(env_get APP_PUBLIC_URL)
REDASH_DB_USER=$(env_get REDASH_DB_USER)
REDASH_DB_PASSWORD=$(env_get REDASH_DB_PASSWORD)
REDASH_DB_NAME=$(env_get REDASH_DB_NAME)
APP_PORT=$(env_get APP_PORT)
CASDOOR_PORT=$(env_get CASDOOR_PORT)
REDASH_PORT=$(env_get REDASH_PORT)

# ------------------------------------------------------------
# 2. Postgres up
# ------------------------------------------------------------
step "Levantando Postgres"
docker compose up -d db >/dev/null
printf "  esperando healthy"
# Tras `down -v`, Postgres pasa por una fase de initdb donde acepta
# conexiones brevemente y luego se reinicia. Exigimos N respuestas
# OK consecutivas para asegurar que el cluster ya está estable.
stable_count=0
while (( stable_count < 3 )); do
    if docker compose exec -T db pg_isready -U postgres -q 2>/dev/null; then
        stable_count=$(( stable_count + 1 ))
    else
        stable_count=0
    fi
    printf "."; sleep 1
done
echo
done_ "Postgres healthy"

# ------------------------------------------------------------
# 3. Detectar password actual del superuser
# ------------------------------------------------------------
# Tras el primer bootstrap el password root ya fue rotado al del .env.
# Antes, sigue siendo el default "postgres".
step "Autenticando contra Postgres"
PG_PW=""
for candidate in "$POSTGRES_PASSWORD" "postgres"; do
    if docker compose exec -T -e PGPASSWORD="$candidate" db \
        psql -U postgres -c '\q' >/dev/null 2>&1; then
        PG_PW="$candidate"; break
    fi
done
if [[ -z "$PG_PW" ]]; then
    echo "ERROR: no puedo autenticarme con ningún password conocido." >&2
    echo "Si rotaste el password manualmente, alinéalo en el .env o resetea:" >&2
    echo "  docker compose down -v && ./scripts/setup.sh" >&2
    exit 1
fi
done_ "autenticado"

psql_root() {
    docker compose exec -T -e PGPASSWORD="$PG_PW" db \
        psql -U postgres -v ON_ERROR_STOP=1 "$@"
}

# ------------------------------------------------------------
# 4. Bootstrap idempotente: usuarios + databases
# ------------------------------------------------------------
step "Bootstrap de usuarios y databases"

# Postgres no tiene CREATE USER IF NOT EXISTS; usamos pg_roles + DO block.
# ALTER ... WITH PASSWORD se ejecuta siempre para mantener el .env como
# fuente de verdad (rota el password si cambia).
psql_root <<EOF >/dev/null
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${EINAR_DB_USER}') THEN
        CREATE ROLE "${EINAR_DB_USER}" LOGIN PASSWORD '${EINAR_DB_PASSWORD}';
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${CASDOOR_DB_USER}') THEN
        CREATE ROLE "${CASDOOR_DB_USER}" LOGIN PASSWORD '${CASDOOR_DB_PASSWORD}' CREATEDB;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${REDASH_DB_USER}') THEN
        CREATE ROLE "${REDASH_DB_USER}" LOGIN PASSWORD '${REDASH_DB_PASSWORD}';
    END IF;
END\$\$;

ALTER ROLE "${EINAR_DB_USER}"   WITH LOGIN PASSWORD '${EINAR_DB_PASSWORD}';
ALTER ROLE "${CASDOOR_DB_USER}" WITH LOGIN PASSWORD '${CASDOOR_DB_PASSWORD}' CREATEDB;
ALTER ROLE "${REDASH_DB_USER}"  WITH LOGIN PASSWORD '${REDASH_DB_PASSWORD}';
EOF
done_ "usuarios einar, casdoor y redash"

# CREATE DATABASE no soporta IF NOT EXISTS: usamos \gexec condicional.
psql_root <<EOF >/dev/null
SELECT 'CREATE DATABASE "${CASDOOR_DB_NAME}" OWNER "${CASDOOR_DB_USER}"'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${CASDOOR_DB_NAME}')\gexec
EOF
done_ "database ${CASDOOR_DB_NAME}"

psql_root <<EOF >/dev/null
SELECT 'CREATE DATABASE "${REDASH_DB_NAME}" OWNER "${REDASH_DB_USER}"'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${REDASH_DB_NAME}')\gexec
EOF
done_ "database ${REDASH_DB_NAME}"

# Asegurar ownership de la DB einar (idempotente).
psql_root -c "ALTER DATABASE \"${EINAR_DB_NAME}\" OWNER TO \"${EINAR_DB_USER}\";" >/dev/null
done_ "database ${EINAR_DB_NAME} owned by ${EINAR_DB_USER}"

# Rotar password del superuser al del .env (siempre, idempotente).
psql_root -c "ALTER USER postgres WITH PASSWORD '${POSTGRES_PASSWORD}';" >/dev/null
done_ "password de postgres alineado con .env"

# ------------------------------------------------------------
# 5. Migraciones (golang-migrate es idempotente por diseño)
# ------------------------------------------------------------
step "Aplicando migraciones"
docker compose --profile tools run --rm migrate 2>&1 | grep -v "^ \(Container\|Pulled\|Pull\|Status\)" || true
done_ "schema actualizado"

# ------------------------------------------------------------
# 6. Redash schema (create_db es idempotente)
# ------------------------------------------------------------
step "Inicializando schema de Redash"
docker compose --profile tools run --rm redash-init 2>&1 | grep -v "^ \(Container\|Pulled\|Pull\|Status\)" || true
done_ "schema de Redash listo"

# ------------------------------------------------------------
# 6.5. Renderizar casdoor/init_data.json desde el template
# ------------------------------------------------------------
# Casdoor lee init_data.json al arrancar y crea cualquier objeto
# declarado que aún no exista en su DB (idempotente: ediciones
# manuales en la UI ganan hasta el próximo `down -v`).
# Renderizamos en el host (no en el contenedor) porque la imagen
# corre como UID 1000 sin write en `/`.
step "Renderizando casdoor/init_data.json"
TPL=casdoor/init_data.json.tpl
OUT=casdoor/init_data.json
[[ ! -f "$TPL" ]] && { echo "ERROR: falta $TPL" >&2; exit 1; }
cp "$TPL" "$OUT"
# Mantener sincronizado con los placeholders del template.
for var in APP_PUBLIC_URL CASDOOR_CLIENT_ID CASDOOR_CLIENT_SECRET \
           GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET; do
    val="${!var:-}"
    if [[ -z "$val" ]]; then
        echo "  ${WARN}⚠${RST} \$$var vacía; el placeholder quedará literal en $OUT"
        continue
    fi
    sedi "s|\${$var}|${val}|g" "$OUT"
done
if grep -qE '\$\{[A-Z_]+\}' "$OUT"; then
    echo "  ${WARN}⚠${RST} placeholders sin sustituir en $OUT:"
    grep -oE '\$\{[A-Z_]+\}' "$OUT" | sort -u | sed 's/^/     - /'
fi
done_ "$OUT renderizado"

# ------------------------------------------------------------
# 6.6. Build del SPA (web/) → internal/web/dist/
# ------------------------------------------------------------
# El binario Go embebe el bundle Vite vía go:embed (internal/web/embed.go).
# Sin este folder existente, `go build` falla. Lo generamos cada setup
# (idempotente: si no hay cambios en web/, vite reusará el cache).
step "Compilando SPA (web)"
if [[ ! -d web ]]; then
    skip "web/ no existe (saltando build SPA)"
else
    pushd web >/dev/null
    if [[ ! -d node_modules ]]; then
        npm install --no-audit --no-fund --silent
    fi
    npm run build --silent
    popd >/dev/null
    done_ "internal/web/dist/ generado"
fi

# ------------------------------------------------------------
# 7. Levantar el resto del stack
# ------------------------------------------------------------
step "Levantando el resto del stack"
docker compose up -d >/dev/null
done_ "stack arriba"

# ------------------------------------------------------------
# 8. Alinear admin de Casdoor con el .env
# ------------------------------------------------------------
# Casdoor seedea su admin la primera vez con `built-in/admin/123`.
# Sobreescribimos la password con la del .env para que el .env sea
# fuente de verdad. UPDATE es idempotente: si ya coincide, no-op.
if [[ -n "${CASDOOR_ADMIN_USERNAME:-}" && -n "${CASDOOR_ADMIN_PASSWORD:-}" ]]; then
    step "Alineando admin de Casdoor con .env"
    printf "  esperando casdoor healthy"
    casdoor_ready=false
    for _ in $(seq 1 60); do
        status=$(docker inspect -f '{{.State.Health.Status}}' einar-casdoor 2>/dev/null || echo "")
        if [[ "$status" == "healthy" ]]; then
            casdoor_ready=true; break
        fi
        printf "."; sleep 1
    done
    echo
    if $casdoor_ready; then
        # password_type=plain en la imagen por defecto, basta con UPDATE.
        docker compose exec -T -e PGPASSWORD="$CASDOOR_DB_PASSWORD" db \
            psql -U "$CASDOOR_DB_USER" -d "$CASDOOR_DB_NAME" -v ON_ERROR_STOP=1 -q <<EOF >/dev/null
UPDATE "user"
SET    password = '${CASDOOR_ADMIN_PASSWORD}',
       password_type = 'plain'
WHERE  owner = 'built-in' AND name = '${CASDOOR_ADMIN_USERNAME}';
EOF
        done_ "admin '${CASDOOR_ADMIN_USERNAME}' alineado con .env"
    else
        echo "  ${WARN}⚠${RST} casdoor no quedó healthy a tiempo; salté la rotación del admin."
        echo "     Rerun ./scripts/setup.sh más tarde para reintentar."
    fi
fi

echo
echo "${OK}${BOLD}✓ Setup completo${RST}"
echo "  App      → http://localhost:${APP_PORT:-8080}"
echo "  Casdoor  → http://localhost:${CASDOOR_PORT:-8000}"
echo "  Redash   → http://localhost:${REDASH_PORT:-5000}"
echo "  Postgres → localhost:5432  (user: ${EINAR_DB_USER})"
