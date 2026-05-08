-- ============================================================
-- 0005_tenant_openobserve_credentials
-- ============================================================
-- Cada tenant tiene su propio user en OpenObserve, scopeado a su
-- org. Esto evita que distintos tenants compartan el root user
-- (que puede ver datos de todos).
--
-- Trade-off: la password se guarda plaintext por ahora. Cuando
-- llegue el primer caso real con datos sensibles, encriptamos
-- at-rest usando una key del entorno (APP_SECRET_KEY).
-- ============================================================

SET ROLE einar;

ALTER TABLE tenants
    ADD COLUMN openobserve_user_email    TEXT,
    ADD COLUMN openobserve_user_password TEXT;

COMMENT ON COLUMN tenants.openobserve_user_email    IS
  'Email del user dedicado en OpenObserve para este tenant. Distinto del root user.';
COMMENT ON COLUMN tenants.openobserve_user_password IS
  'Password plaintext temporal. TODO: encriptar at-rest con APP_SECRET_KEY.';

RESET ROLE;
