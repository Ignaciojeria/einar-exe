SET ROLE einar;

ALTER TABLE projects
    ADD COLUMN db_name     TEXT,
    ADD COLUMN db_user     TEXT,
    ADD COLUMN db_password TEXT;

-- Ensure uniqueness (important: prevents database name conflicts)
CREATE UNIQUE INDEX projects_db_name_idx ON projects(db_name) WHERE db_name IS NOT NULL;

COMMENT ON COLUMN projects.db_name     IS 'Dedicated Postgres database for this project (format: {slug}_{shortid})';
COMMENT ON COLUMN projects.db_user     IS 'Postgres role that owns the project database';
COMMENT ON COLUMN projects.db_password IS 'Password for db_user (TODO: encrypt in production!)';

RESET ROLE;
