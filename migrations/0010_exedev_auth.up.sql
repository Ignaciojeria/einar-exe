-- ============================================================
-- 0010_exedev_auth — Migrate from Casdoor to exe.dev auth
-- ============================================================
-- The identity field `casdoor_sub` is renamed to `exedev_user_id`
-- because authentication now uses exe.dev proxy headers
-- (X-ExeDev-UserID, X-ExeDev-Email) instead of Casdoor OIDC.
--
-- The `casdoor_org` column on tenants becomes nullable legacy;
-- we keep it but it won't be populated for new tenants.
-- ============================================================

SET ROLE einar;

-- Rename identity column
ALTER TABLE users RENAME COLUMN casdoor_sub TO exedev_user_id;

COMMENT ON COLUMN users.exedev_user_id IS 'Stable user ID from exe.dev proxy (X-ExeDev-UserID header). Unique per user.';

RESET ROLE;
