package store

import (
	"fmt"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func TestPrepareEmailForStorage(t *testing.T) {
	t.Parallel()

	hash, stored, err := prepareEmailForStorage("user@example.com")
	if err != nil {
		t.Fatalf("prepareEmailForStorage: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty email hash")
	}
	if stored == "" {
		t.Fatal("expected non-empty stored email")
	}
	if hash != crypto.EmailHMAC("user@example.com") {
		t.Error("hash should match EmailHMAC of input")
	}
}

func TestPrepareEmailForStorage_WithEncryptionKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto.InitFromEnv: %v", err)
	}

	hash, stored, err := prepareEmailForStorage("secure@example.com")
	if err != nil {
		t.Fatalf("prepareEmailForStorage: %v", err)
	}
	if hash == "" || stored == "" {
		t.Fatal("expected hash and stored email")
	}
	if stored == "secure@example.com" {
		t.Error("expected encrypted stored email when key is set")
	}
	if !strings.HasPrefix(stored, "v1:") {
		t.Errorf("stored email = %q, want v1: prefix", stored)
	}
}

func TestEmailFromStorage_PlaintextLegacy(t *testing.T) {
	t.Parallel()

	got, err := emailFromStorage("legacy@example.com")
	if err != nil {
		t.Fatalf("emailFromStorage: %v", err)
	}
	if got != "legacy@example.com" {
		t.Errorf("got %q, want legacy plaintext", got)
	}
}

func TestEmailFromStorage_EncryptedRoundTrip(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto.InitFromEnv: %v", err)
	}

	_, stored, err := prepareEmailForStorage("roundtrip@example.com")
	if err != nil {
		t.Fatalf("prepareEmailForStorage: %v", err)
	}
	got, err := emailFromStorage(stored)
	if err != nil {
		t.Fatalf("emailFromStorage: %v", err)
	}
	if got != "roundtrip@example.com" {
		t.Errorf("got %q, want roundtrip@example.com", got)
	}
}

func TestPrepareEmailForStorage_EncryptError(t *testing.T) {
	orig := encryptEmailForStorageFn
	t.Cleanup(func() { encryptEmailForStorageFn = orig })
	encryptEmailForStorageFn = func(string) (string, error) {
		return "", fmt.Errorf("encrypt failed")
	}

	_, _, err := prepareEmailForStorage("fail@example.com")
	if err == nil {
		t.Fatal("expected encrypt error")
	}
	if !strings.Contains(err.Error(), "encrypt email") {
		t.Errorf("error = %v, want encrypt email wrapper", err)
	}
}

func TestEmailFromStorage_CorruptedCiphertext(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto.InitFromEnv: %v", err)
	}

	_, err := emailFromStorage("v1:00112233445566778899aabbccddeeff")
	if err == nil {
		t.Fatal("expected decrypt error for corrupted ciphertext")
	}
	if !strings.Contains(err.Error(), "decrypt email") {
		t.Errorf("error = %v, want decrypt email wrapper", err)
	}
}
