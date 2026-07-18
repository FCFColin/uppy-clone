-- Reverse folded 000010/000012/000013/000015: restore email UNIQUE, drop email_hash column+index.
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
DROP INDEX IF EXISTS idx_users_email_hash;
ALTER TABLE users DROP COLUMN IF EXISTS email_hash;
