-- Reverse: drop the least-privilege roles created in the up migration.
REASSIGN OWNED BY app_user TO migrator;
DROP OWNED BY app_user;
DROP ROLE IF EXISTS app_user;

REASSIGN OWNED BY migrator TO postgres;
DROP OWNED BY migrator;
DROP ROLE IF EXISTS migrator;
