-- ============================================================
-- 0006_tenant_metabase
-- ============================================================
-- Aprovisionamiento de Metabase: cada tenant tiene su propio
-- group + collection + user. Persistimos los IDs para poder
-- borrar/limpiar al de-aprovisionar.
-- ============================================================

SET ROLE einar;

ALTER TABLE tenants
    ADD COLUMN metabase_group_id      INTEGER UNIQUE,
    ADD COLUMN metabase_collection_id INTEGER UNIQUE,
    ADD COLUMN metabase_user_email    TEXT,
    ADD COLUMN metabase_user_password TEXT;

COMMENT ON COLUMN tenants.metabase_group_id      IS
  'ID del permission group en Metabase. NULL hasta que se aprovisiona.';
COMMENT ON COLUMN tenants.metabase_collection_id IS
  'ID de la collection (folder) del tenant en Metabase.';
COMMENT ON COLUMN tenants.metabase_user_password IS
  'Password plaintext temporal. TODO: encriptar at-rest con APP_SECRET_KEY.';

RESET ROLE;
