package crypto

import (
	"crypto/cipher"
	"errors"
	"strings"
	"testing"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestInit_ValidKey(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

func TestInit_InvalidHex(t *testing.T) {
	if err := Init("not-hex"); err == nil {
		t.Fatal("expected hex error")
	}
}

func TestInit_WrongLength(t *testing.T) {
	if err := Init(strings.Repeat("ab", 16)); err == nil {
		t.Fatal("expected length error")
	}
}

func TestDecrypt_InvalidKeyLength(t *testing.T) {
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

func TestEncrypt_InvalidKeyLength(t *testing.T) {
	SetEncKeyForTest(make([]byte, 15))
	t.Cleanup(ResetKeyForTest)

	if _, err := Encrypt("secret"); err == nil {
		t.Fatal("expected create cipher error for invalid key length")
	}
}

func TestEncrypt_RandReadFailure(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	prev := aesRandRead
	aesRandRead = func([]byte) (int, error) { return 0, errors.New("rand failed") }
	t.Cleanup(func() { aesRandRead = prev })

	if _, err := Encrypt("secret"); err == nil {
		t.Fatal("expected encrypt error when rand.Read fails")
	}
}

func TestResetKeyForTest(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	ResetKeyForTest()
	if _, err := Encrypt("x"); err == nil {
		t.Fatal("expected not initialized after ResetKeyForTest")
	}
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
}

func TestEncrypt_NotInitialized(t *testing.T) {
	encKey = nil
	if _, err := Encrypt("secret"); err == nil {
		t.Fatal("expected not initialized error")
	}
}

func TestDecrypt_InvalidHex(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt("v1:not-valid-hex"); err == nil {
		t.Fatal("expected hex decode error")
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

func TestDecrypt_NotInitialized(t *testing.T) {
	encKey = nil
	if _, err := Decrypt("v1:abcd"); err == nil {
		t.Fatal("expected not initialized error")
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

func TestInitFromEnv_Missing(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "")
	if err := InitFromEnv(); err == nil {
		t.Fatal("expected missing key error")
	}
}

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

func TestMustInitFromEnv_Panics(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "")
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	MustInitFromEnv()
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

func TestRotateKey_SwitchesActiveKey(t *testing.T) {
	oldKey := make([]byte, 32)
	for i := range oldKey {
		oldKey[i] = byte(i)
	}
	newKey := make([]byte, 32)
	for i := range newKey {
		newKey[i] = byte(i + 100)
	}

	SetEncKeyForTest(oldKey)
	t.Cleanup(ResetKeyForTest)

	// Encrypt with old key.
	enc, err := Encrypt("secret-data")
	if err != nil {
		t.Fatalf("Encrypt with old key: %v", err)
	}

	// Rotate to new key.
	if err := RotateKey(oldKey, newKey); err != nil {
		t.Fatalf("RotateKey: %v", err)
	}

	// New encryptions should use new key.
	enc2, err := Encrypt("new-data")
	if err != nil {
		t.Fatalf("Encrypt with new key: %v", err)
	}

	// New ciphertext should decrypt with new key (encKey is now newKey).
	got, err := Decrypt(enc2)
	if err != nil || got != "new-data" {
		t.Fatalf("Decrypt new = %q, %v", got, err)
	}

	// Old ciphertext cannot be decrypted with new key.
	if _, err := Decrypt(enc); err == nil {
		t.Fatal("expected decrypt error for old ciphertext with new key")
	}
}

func TestRotateKey_InvalidNewKey(t *testing.T) {
	if err := RotateKey(nil, []byte("too-short")); err == nil {
		t.Fatal("expected error for short new key")
	}
}

func TestReEncryptWithKey_RoundTrip(t *testing.T) {
	oldKey := make([]byte, 32)
	for i := range oldKey {
		oldKey[i] = byte(i)
	}
	newKey := make([]byte, 32)
	for i := range newKey {
		newKey[i] = byte(i + 50)
	}

	SetEncKeyForTest(oldKey)
	t.Cleanup(ResetKeyForTest)

	// Encrypt with old key.
	original := "my-secret-email@example.com"
	enc, err := Encrypt(original)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Re-encrypt with new key.
	reEncrypted, err := ReEncryptWithKey(oldKey, newKey, enc)
	if err != nil {
		t.Fatalf("ReEncryptWithKey: %v", err)
	}

	// Switch active key to new.
	if err := RotateKey(oldKey, newKey); err != nil {
		t.Fatalf("RotateKey: %v", err)
	}

	// Re-encrypted ciphertext should decrypt with new key.
	got, err := Decrypt(reEncrypted)
	if err != nil {
		t.Fatalf("Decrypt re-encrypted: %v", err)
	}
	if got != original {
		t.Fatalf("got %q, want %q", got, original)
	}
}

func TestReEncryptWithKey_InvalidKeys(t *testing.T) {
	validKey := make([]byte, 32)
	shortKey := []byte("short")

	if _, err := ReEncryptWithKey(shortKey, validKey, "v1:abcd"); err == nil {
		t.Fatal("expected error for short old key")
	}
	if _, err := ReEncryptWithKey(validKey, shortKey, "v1:abcd"); err == nil {
		t.Fatal("expected error for short new key")
	}
}

func TestEncrypt_GCMError(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	prev := aesNewGCM
	aesNewGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, errors.New("gcm failed") }
	t.Cleanup(func() { aesNewGCM = prev })

	if _, err := Encrypt("secret"); err == nil {
		t.Fatal("expected GCM creation error")
	}
}

func TestDecrypt_GCMError(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt("plain")
	if err != nil {
		t.Fatal(err)
	}
	prev := aesNewGCM
	aesNewGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, errors.New("gcm failed") }
	t.Cleanup(func() { aesNewGCM = prev })

	if _, err := Decrypt(enc); err == nil {
		t.Fatal("expected GCM creation error on decrypt")
	}
}

func TestEncrypt_NewGCMError(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	prev := aesNewGCM
	aesNewGCM = func(cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("gcm failed")
	}
	t.Cleanup(func() { aesNewGCM = prev })

	if _, err := Encrypt("secret"); err == nil {
		t.Fatal("expected NewGCM error")
	}
}

func TestDecrypt_NewGCMError(t *testing.T) {
	if err := Init(testKeyHex); err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt("plain")
	if err != nil {
		t.Fatal(err)
	}
	prev := aesNewGCM
	aesNewGCM = func(cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("gcm failed")
	}
	t.Cleanup(func() { aesNewGCM = prev })

	if _, err := Decrypt(enc); err == nil {
		t.Fatal("expected NewGCM error on decrypt")
	}
}
