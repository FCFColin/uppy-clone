-- Backfill email_hash for rows added before migration 000010.
-- Requires pgcrypto extension.
-- Note: plaintext email in the `email` column from before migration 000010
-- must be encrypted via an application-level script (requires ENCRYPTION_KEY).
CREATE EXTENSION IF NOT EXISTS pgcrypto;

UPDATE users
SET email_hash = encode(digest(lower(trim(email)), 'sha256'), 'hex')
WHERE email IS NOT NULL AND email_hash IS NULL;
