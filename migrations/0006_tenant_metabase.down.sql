SET ROLE einar;
ALTER TABLE tenants
    DROP COLUMN IF EXISTS metabase_user_password,
    DROP COLUMN IF EXISTS metabase_user_email,
    DROP COLUMN IF EXISTS metabase_collection_id,
    DROP COLUMN IF EXISTS metabase_group_id;
RESET ROLE;
