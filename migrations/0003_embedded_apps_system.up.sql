-- ============================================================
-- 0003_embedded_apps_system — distinguir apps "system" vs "custom"
-- ============================================================
-- Las apps system (OpenObserve, Redash, Casdoor admin, etc.) las
-- aprovisiona automáticamente la plataforma al crear el tenant.
-- Los users no pueden borrarlas, solo reordenarlas.
-- ============================================================

SET ROLE einar;

ALTER TABLE embedded_apps
    ADD COLUMN is_system BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN embedded_apps.is_system IS
  'TRUE = aprovisionada por la plataforma (OpenObserve, Redash, ...). No se puede borrar.';

-- Índice para que el sidenav pueda separar fácilmente las dos secciones.
CREATE INDEX embedded_apps_tenant_system_pos_idx
    ON embedded_apps(tenant_id, is_system, position);

RESET ROLE;
