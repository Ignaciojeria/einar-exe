SET ROLE einar;

DROP INDEX IF EXISTS projects_db_name_idx;

ALTER TABLE projects
    DROP COLUMN IF EXISTS db_name,
    DROP COLUMN IF EXISTS db_user,
    DROP COLUMN IF EXISTS db_password;

RESET ROLE;
