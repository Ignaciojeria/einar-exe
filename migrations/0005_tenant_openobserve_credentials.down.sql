SET ROLE einar;
ALTER TABLE tenants
    DROP COLUMN IF EXISTS openobserve_user_password,
    DROP COLUMN IF EXISTS openobserve_user_email;
RESET ROLE;
