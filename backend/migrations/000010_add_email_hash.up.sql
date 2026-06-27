-- email_hash for HMAC-indexed lookup; email column stores AES-GCM ciphertext for new rows.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_hash VARCHAR(64) DEFAULT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_hash ON users(email_hash) WHERE email_hash IS NOT NULL;
