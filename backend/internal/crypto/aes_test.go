package crypto

import (
	"strings"
	"testing"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestInitFromEnv_Success(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testKeyHex)
	if err := InitFromEnv(); err != nil {
		t.Fatalf("InitFromEnv: %v", err)
	}
	enc, err := Encrypt("via-env")
	if err != nil {
		t.Fatalf("Encrypt after InitFromEnv: %v", err)
	}
	got, err := Decrypt(enc)
	if err != nil || got != "via-env" {
		t.Fatalf("Decrypt = %q, %v", got, err)
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	plain := "secret-api-key-value"
	enc, err := Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, "v1:") {
		t.Errorf("expected v1 prefix, got %q", enc)
	}
	got, err := Decrypt(enc)
	if err != nil || got != plain {
		t.Fatalf("Decrypt = %q, %v", got, err)
	}
}

func TestDecrypt_LegacyRawHex(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	withPrefix, err := Encrypt("legacy-compat")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	rawHex := strings.TrimPrefix(withPrefix, "v1:")
	got, err := Decrypt(rawHex)
	if err != nil || got != "legacy-compat" {
		t.Fatalf("Decrypt legacy = %q, %v", got, err)
	}
}

func TestEncrypt_NotInitialized(t *testing.T) {
	encKey = nil
	if _, err := Encrypt("secret"); err == nil {
		t.Fatal("expected not initialized error")
	}
}

func TestEncrypt_InvalidKeyLength(t *testing.T) {
	SetEncKeyForTest(make([]byte, 15))
	t.Cleanup(ResetKeyForTest)

	if _, err := Encrypt("secret"); err == nil {
		t.Fatal("expected create cipher error for invalid key length")
	}
}

func TestDecrypt_NotInitialized(t *testing.T) {
	encKey = nil
	if _, err := Decrypt("v1:abcd"); err == nil {
		t.Fatal("expected not initialized error")
	}
}

func TestDecrypt_InvalidKeyLength(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt("plain")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	SetEncKeyForTest(make([]byte, 15))
	t.Cleanup(ResetKeyForTest)

	if _, err := Decrypt(enc); err == nil {
		t.Fatal("expected decrypt error for invalid key length")
	}
}

func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	// Adversarial: truncated ciphertext must fail authentication, not return partial plaintext.
	if _, err := Decrypt("v1:0102"); err == nil {
		t.Fatal("expected decrypt error for truncated input")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt("tamper-me")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	raw := strings.TrimPrefix(enc, "v1:")
	if len(raw) < 4 {
		t.Fatal("ciphertext too short")
	}
	tampered := "v1:" + raw[:len(raw)-2] + "ff"
	if _, err := Decrypt(tampered); err == nil {
		t.Fatal("expected decrypt error for tampered ciphertext")
	}
}

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

func TestDecryptEmailFromStorage_Corrupted(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	_, err := DecryptEmailFromStorage("v1:00112233445566778899aabbccddeeff")
	if err == nil {
		t.Error("DecryptEmailFromStorage should reject corrupted ciphertext")
	}
}
