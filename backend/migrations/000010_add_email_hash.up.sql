-- email_hash for SHA-256-indexed lookup; email column stores AES-GCM ciphertext for new rows.
-- Folded from 000010/000012/000013/000015 (spec deep-arch-slim-v2 Task 27):
--   - add email_hash column + unique index (originally 000010)
--   - rehash to SHA-256 / backfill (originally 000012/000013, no-op on fresh DB)
--   - drop users_email_key UNIQUE constraint (originally 000015)
-- Application code computes SHA-256(lower(trim(email))) and stores it in email_hash at insert time.
-- email column stores AES-GCM ciphertext (random nonce), so UNIQUE on ciphertext is ineffective;
-- uniqueness is enforced by idx_users_email_hash (SHA-256, deterministic).
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_hash VARCHAR(64) DEFAULT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_hash ON users(email_hash) WHERE email_hash IS NOT NULL;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
