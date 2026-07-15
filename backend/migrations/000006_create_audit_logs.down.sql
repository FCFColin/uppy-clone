-- store-028: Drop the trigger function explicitly. Dropping the table
-- automatically removes the triggers, but the function lingers in pg_proc.
DROP FUNCTION IF EXISTS prevent_audit_log_modification();
DROP TABLE IF EXISTS audit_logs;
