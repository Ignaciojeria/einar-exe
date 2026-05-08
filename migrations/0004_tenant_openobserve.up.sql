-- ============================================================
-- 0004_tenant_openobserve — Link tenant ↔ OpenObserve org
-- ============================================================
-- OpenObserve auto-genera un identifier random al crear org. Lo
-- persistimos para construir URLs del iframe (/o2/web/{id}/...).
-- ============================================================

SET ROLE einar;

ALTER TABLE tenants
    ADD COLUMN openobserve_org_id TEXT UNIQUE;

COMMENT ON COLUMN tenants.openobserve_org_id IS
  'Identifier random asignado por OpenObserve a la org del tenant. NULL hasta que se aprovisiona.';

RESET ROLE;
