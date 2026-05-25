#!/usr/bin/env bash
# ============================================================
# einar-exe — Reset completo del entorno
# ============================================================
# Destruye TODOS los contenedores, volúmenes, imágenes, .env y
# secrets, y luego reinicializa todo desde cero con setup.sh.
#
# ⚠️  ESTO BORRA TODA LA DATA (Postgres, OpenObserve, Metabase, etc.)
# ============================================================
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ -t 1 ]]; then
    BOLD=$'\e[1m'; WARN=$'\e[33m'; RST=$'\e[0m'
else
    BOLD=""; WARN=""; RST=""
fi

echo "${WARN}${BOLD}⚠️  Esto borrará TODA la data (volumes, .env, secrets, imágenes).${RST}"
read -rp "   ¿Continuar? [y/N] " confirm
if [[ ! "$confirm" =~ ^[yYsS]$ ]]; then
    echo "Cancelado."
    exit 0
fi

echo
echo "${BOLD}→ Deteniendo y eliminando contenedores + volúmenes + imágenes${RST}"
docker compose down -v --rmi all 2>&1 || true

echo "${BOLD}→ Limpiando recursos huérfanos de Docker${RST}"
docker system prune -af --volumes 2>&1 | tail -1

echo "${BOLD}→ Eliminando .env, secrets y archivos temporales${RST}"
rm -f .env
rm -f secrets/casdoor-jwt.pem
rm -rf logs/ tmp/

echo
echo "${BOLD}→ Reinicializando con setup.sh${RST}"
echo
bash scripts/setup.sh
