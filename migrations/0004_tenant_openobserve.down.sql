SET ROLE einar;
ALTER TABLE tenants DROP COLUMN IF EXISTS openobserve_org_id;
RESET ROLE;
