package crypto

import (
	"testing"
)

func TestEmailHMAC_WithoutKey(t *testing.T) {
	encKey = nil
	h := EmailHMAC("  Alice@Example.COM  ")
	if h == "" {
		t.Fatal("EmailHMAC should return non-empty hash")
	}
	// Must be lowercase hex (64 hex chars = 32 bytes = SHA-256)
	if len(h) != 64 {
		t.Errorf("EmailHMAC length = %d, want 64", len(h))
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("non-hex character %c in hash", c)
		}
	}
}

func TestEmailHMAC_WithoutKey_CaseInsensitive(t *testing.T) {
	encKey = nil
	h1 := EmailHMAC("Alice@Example.COM")
	h2 := EmailHMAC("alice@example.com")
	if h1 != h2 {
		t.Error("EmailHMAC should be case-insensitive when no key set")
	}
}

func TestEmailHMAC_WithKey(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	h := EmailHMAC("bob@test.com")
	if h == "" {
		t.Fatal("EmailHMAC should return non-empty hash")
	}
	if len(h) != 64 {
		t.Errorf("EmailHMAC length = %d, want 64", len(h))
	}
}

func TestEmailHMAC_WithKey_Deterministic(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	h1 := EmailHMAC("charlie@example.org")
	h2 := EmailHMAC("charlie@example.org")
	if h1 != h2 {
		t.Error("EmailHMAC should be deterministic with key set")
	}
}

func TestEmailHMAC_TrailingSpace(t *testing.T) {
	encKey = nil
	h1 := EmailHMAC("test@test.com")
	h2 := EmailHMAC("  test@test.com  ")
	if h1 != h2 {
		t.Error("EmailHMAC should trim spaces")
	}
}

func TestEncryptEmailForStorage_WithoutKey(t *testing.T) {
	encKey = nil
	result, err := EncryptEmailForStorage("user@example.com")
	if err != nil {
		t.Fatalf("EncryptEmailForStorage without key: %v", err)
	}
	if result != "user@example.com" {
		t.Errorf("EncryptEmailForStorage = %q, want original email", result)
	}
}

func TestEncryptEmailForStorage_WithKey(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	result, err := EncryptEmailForStorage("secure@example.com")
	if err != nil {
		t.Fatalf("EncryptEmailForStorage with key: %v", err)
	}
	if result == "secure@example.com" {
		t.Error("EncryptEmailForStorage should encrypt when key is set")
	}
	if len(result) < 10 {
		t.Errorf("EncryptEmailForStorage result too short: %q", result)
	}
}

func TestDecryptEmailFromStorage_Empty(t *testing.T) {
	result, err := DecryptEmailFromStorage("")
	if err != nil || result != "" {
		t.Errorf("DecryptEmailFromStorage('') = (%q, %v), want ('', nil)", result, err)
	}
}

func TestDecryptEmailFromStorage_LegacyPlaintext(t *testing.T) {
	result, err := DecryptEmailFromStorage("legacy@example.com")
	if err != nil || result != "legacy@example.com" {
		t.Errorf("DecryptEmailFromStorage(legacy) = (%q, %v), want ('legacy@example.com', nil)", result, err)
	}
}

func TestDecryptEmailFromStorage_Encrypted(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	enc, err := EncryptEmailForStorage("encrypted@example.com")
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptEmailFromStorage(enc)
	if err != nil {
		t.Fatalf("DecryptEmailFromStorage: %v", err)
	}
	if got != "encrypted@example.com" {
		t.Errorf("DecryptEmailFromStorage = %q, want original email", got)
	}
}

func TestDecryptEmailFromStorage_Corrupted(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	_, err := DecryptEmailFromStorage("v1:00112233445566778899aabbccddeeff")
	if err == nil {
		t.Error("DecryptEmailFromStorage should reject corrupted ciphertext")
	}
}
