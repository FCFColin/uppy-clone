package crypto

import (
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

func TestMustInitFromEnv_Panics(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "")
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	MustInitFromEnv()
}

func TestRotateKey_NotImplemented(t *testing.T) {
	if err := RotateKey(nil, []byte("x")); err == nil {
		t.Fatal("expected not implemented error")
	}
}
