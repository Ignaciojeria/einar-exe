SET ROLE einar;
ALTER TABLE users RENAME COLUMN exedev_user_id TO casdoor_sub;
COMMENT ON COLUMN users.casdoor_sub IS 'Claim "sub" del JWT emitido por Casdoor (identificador estable del usuario).';
RESET ROLE;
