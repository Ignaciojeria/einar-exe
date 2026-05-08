-- ============================================================
-- 0002_tenants — DOWN
-- ============================================================

SET ROLE einar;

DROP TABLE IF EXISTS embedded_apps;

ALTER TABLE users
    DROP COLUMN IF EXISTS role,
    DROP COLUMN IF EXISTS email,
    DROP COLUMN IF EXISTS tenant_id;

DROP TABLE IF EXISTS tenants;

RESET ROLE;
