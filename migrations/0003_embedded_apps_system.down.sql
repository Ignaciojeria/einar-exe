SET ROLE einar;

DROP INDEX IF EXISTS embedded_apps_tenant_system_pos_idx;
ALTER TABLE embedded_apps DROP COLUMN IF EXISTS is_system;

RESET ROLE;
