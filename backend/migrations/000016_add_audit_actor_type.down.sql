DROP INDEX IF EXISTS idx_audit_logs_actor_type;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS actor_type;
