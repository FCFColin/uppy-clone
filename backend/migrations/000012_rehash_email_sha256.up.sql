-- Re-hash email_hash with plain SHA-256 to decouple from encryption key.
-- Previously email_hash used HMAC-SHA256(derived_from_encKey, email).
-- After this migration, all email_hash values use SHA-256(email) so that
-- AES key rotation (RotateKey) does not invalidate existing hashes.
--
-- Requires pgcrypto extension for digest() function.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

UPDATE users
SET email_hash = encode(digest(lower(trim(email)), 'sha256'), 'hex')
WHERE email IS NOT NULL;
