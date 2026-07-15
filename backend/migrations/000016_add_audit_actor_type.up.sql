-- project-08-003: Add actor_type column to disambiguate ActorID semantics.
-- Previously actor_id was overloaded: UUID for users, "admin" for admins,
-- "system" for automated actions, role strings for RBAC denials.
-- actor_type makes the semantic category explicit for SIEM/compliance queries.
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS actor_type VARCHAR(20) DEFAULT '';

-- Backfill existing rows based on known actor_id patterns:
--  - "system" → system
--  - "admin" → admin
--  - anything else (UUIDs, role strings) → user
UPDATE audit_logs SET actor_type = 'system' WHERE actor_id = 'system' AND actor_type = '';
UPDATE audit_logs SET actor_type = 'admin'  WHERE actor_id = 'admin'  AND actor_type = '';
UPDATE audit_logs SET actor_type = 'user'   WHERE actor_type = '' AND actor_id NOT IN ('system', 'admin');

-- Index for actor_type queries (e.g., "show all admin actions")
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_type ON audit_logs(actor_type);
