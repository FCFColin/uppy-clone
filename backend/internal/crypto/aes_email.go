package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// emailHMACKey is derived from encKey with domain separation, so the same
// encryption key is never reused as an HMAC key (cryptographic key separation).
var emailHMACKey []byte

// initEmailHMACKey derives a separate HMAC key from the encryption key.
func initEmailHMACKey() {
	if encKey == nil {
		emailHMACKey = nil
		return
	}
	mac := hmac.New(sha256.New, encKey)
	mac.Write([]byte("uppy-email-hmac-v1"))
	emailHMACKey = mac.Sum(nil)
}

// EmailHMAC computes HMAC-SHA256 of a normalized email for indexed lookup.
func EmailHMAC(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if emailHMACKey == nil {
		sum := sha256.Sum256([]byte(normalized))
		return hex.EncodeToString(sum[:])
	}
	mac := hmac.New(sha256.New, emailHMACKey)
	_, _ = mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}

// EncryptEmailForStorage encrypts email for DB storage.
func EncryptEmailForStorage(email string) (string, error) {
	if encKey == nil {
		return email, nil
	}
	return Encrypt(email)
}

// DecryptEmailFromStorage decrypts stored email; passes through legacy plaintext.
func DecryptEmailFromStorage(stored string) (string, error) {
	if stored == "" || !strings.HasPrefix(stored, "v1:") {
		return stored, nil
	}
	return Decrypt(stored)
}
