package store

import (
	"fmt"

	"github.com/uppy-clone/backend/internal/crypto"
)

// prepareEmailForStorage returns HMAC hash and encrypted email for DB persistence.
func prepareEmailForStorage(email string) (hash, stored string, err error) {
	hash = crypto.EmailHMAC(email)
	stored, err = crypto.EncryptEmailForStorage(email)
	if err != nil {
		return "", "", fmt.Errorf("encrypt email: %w", err)
	}
	return hash, stored, nil
}

// emailFromStorage decrypts a stored email value (legacy plaintext passes through).
func emailFromStorage(stored string) (string, error) {
	plain, err := crypto.DecryptEmailFromStorage(stored)
	if err != nil {
		return "", fmt.Errorf("decrypt email: %w", err)
	}
	return plain, nil
}
