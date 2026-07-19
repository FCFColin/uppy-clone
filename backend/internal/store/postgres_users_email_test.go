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

func TestEmailFromStorage(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto.InitFromEnv: %v", err)
	}

	// Round-trip: prepare then read back.
	_, stored, err := prepareEmailForStorage("roundtrip@example.com")
	if err != nil {
		t.Fatalf("prepareEmailForStorage: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "plaintext legacy", input: "legacy@example.com", want: "legacy@example.com"},
		{name: "encrypted round trip", input: stored, want: "roundtrip@example.com"},
		{name: "corrupted ciphertext", input: "v1:00112233445566778899aabbccddeeff", wantErr: "decrypt email"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := emailFromStorage(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("emailFromStorage = %v, want %q error", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("emailFromStorage: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
