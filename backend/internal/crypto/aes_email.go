package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// EmailHMAC computes SHA-256 of a normalized email for indexed lookup.
// Uses unkeyed SHA-256 to ensure email_hash remains stable across key rotation.
// If keyed HMAC is needed for additional rainbow-table resistance, the HMAC key
// must be stored independently from the encryption key and rotated separately.
func EmailHMAC(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// EncryptEmailForStorage encrypts email for DB storage.
//
// Deprecated: Use EncryptPIIForStorage for new code — same logic, generic name.
func EncryptEmailForStorage(email string) (string, error) {
	return EncryptPIIForStorage(email)
}

// EncryptPIIForStorage encrypts any PII field (email, nickname, etc.) for
// DB or outbox storage. Returns plaintext if no encryption key is configured
// (dev/test environments).
func EncryptPIIForStorage(plaintext string) (string, error) {
	encKeyMu.RLock()
	key := encKey
	encKeyMu.RUnlock()
	if key == nil {
		return plaintext, nil
	}
	return Encrypt(plaintext)
}

// DecryptEmailFromStorage decrypts stored email; passes through legacy plaintext.
func DecryptEmailFromStorage(stored string) (string, error) {
	if stored == "" || !strings.HasPrefix(stored, "v1:") {
		return stored, nil
	}
	return Decrypt(stored)
}
