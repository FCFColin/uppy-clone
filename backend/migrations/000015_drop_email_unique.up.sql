-- email column stores AES-GCM ciphertext (random nonce), so UNIQUE on ciphertext is ineffective.
-- Uniqueness is enforced by idx_users_email_hash (HMAC-SHA256, deterministic).
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
