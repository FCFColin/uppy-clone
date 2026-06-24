-- Reverse: drop the least-privilege roles created in the up migration.
DROP ROLE IF EXISTS app_user;
DROP ROLE IF EXISTS migrator;
