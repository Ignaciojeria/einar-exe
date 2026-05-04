-- ============================================================
-- 0001_init — Bootstrap del schema de la aplicación einar-exe
-- ============================================================
-- Esta migración corre como `postgres` (superuser) porque:
--   1. Las extensiones requieren superuser (postgis no es "trusted").
--   2. Tras crear las extensiones, cambiamos al rol `einar` para que
--      las tablas queden con el owner correcto (privilegio mínimo).
-- ============================================================

-- ------------------------------------------------------------
-- Extensiones (idempotentes)
-- ------------------------------------------------------------
-- pgcrypto: gen_random_uuid() para PKs UUID v4.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- PostGIS: tipos y funciones geoespaciales (la imagen postgis/postgis ya
-- la trae instalada en el cluster, pero hay que habilitarla por database).
CREATE EXTENSION IF NOT EXISTS postgis;

-- ------------------------------------------------------------
-- Schema de aplicación creado como `einar` (owner correcto)
-- ------------------------------------------------------------
SET ROLE einar;

-- Tabla `users`: vínculo entre la identidad gestionada por Casdoor
-- (claim `sub` del JWT) y los datos de dominio de la app.
-- El perfil completo (email, name, avatar) vive en Casdoor; aquí solo
-- guardamos lo mínimo necesario para FK desde otras tablas.
CREATE TABLE users (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    casdoor_sub TEXT        NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE  users               IS 'Usuarios de la app. La identidad real vive en Casdoor.';
COMMENT ON COLUMN users.casdoor_sub   IS 'Claim "sub" del JWT emitido por Casdoor (identificador estable del usuario).';

RESET ROLE;
