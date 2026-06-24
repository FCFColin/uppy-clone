package crypto //nolint:revive // intentional package name

import (
	"os"
	"testing"
)

// TestMain 初始化加密密钥供所有测试使用。
// 企业为何需要：全零密钥让 AES-256-GCM 加密形同虚设。
// 生产环境必须配置独立密钥，未配置时 fail-fast 防止静默降级为明文。
func TestMain(m *testing.M) {
	// 使用测试专用密钥初始化（不影响 ENCRYPTION_KEY 环境变量校验逻辑）
	if err := Init("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		panic("failed to initialize test encryption key: " + err.Error())
	}
	os.Exit(m.Run())
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		plaintext string
	}{
		{name: "empty string", plaintext: ""},
		{name: "simple ascii", plaintext: "hello world"},
		{name: "unicode", plaintext: "你好世界🎉"},
		{name: "long string", plaintext: "a quick brown fox jumps over the lazy dog " + "x"},
		{name: "special chars", plaintext: "<script>alert('xss')</script>"},
		{name: "api key format", plaintext: "re_sk_1234567890abcdef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encrypted, err := Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			if encrypted == tt.plaintext && tt.plaintext != "" {
				t.Error("Encrypt() returned plaintext unchanged")
			}

			decrypted, err := Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	t.Parallel()

	plaintext := "same input"
	enc1, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("first Encrypt() error = %v", err)
	}
	enc2, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("second Encrypt() error = %v", err)
	}

	if enc1 == enc2 {
		t.Error("two encryptions of the same plaintext should produce different ciphertexts (random nonce)")
	}
}

func TestDecryptInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "not hex", input: "zzzzzz"},
		{name: "too short", input: "ab"},
		{name: "empty string", input: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Decrypt(tt.input)
			if err == nil {
				t.Error("expected error for invalid input, got nil")
			}
		})
	}
}

// TestMustInitFromEnvPanicsOnEmptyKey 验证 ENCRYPTION_KEY 未设置时
// MustInitFromEnv 会 panic，防止静默降级到不安全状态。
// 企业为何需要：全零密钥让 AES-256-GCM 加密形同虚设。
// 生产环境必须配置独立密钥，未配置时 fail-fast 防止静默降级为明文。
func TestMustInitFromEnvPanicsOnEmptyKey(t *testing.T) {
	t.Parallel()

	// 确保 ENCRYPTION_KEY 未设置
	orig := os.Getenv("ENCRYPTION_KEY")
	os.Unsetenv("ENCRYPTION_KEY")
	defer func() {
		if orig != "" {
			os.Setenv("ENCRYPTION_KEY", orig)
		}
	}()

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected MustInitFromEnv to panic when ENCRYPTION_KEY is empty, but it did not")
		}
	}()

	MustInitFromEnv()
}

// TestInitFromEnvReturnsErrorOnEmptyKey 验证 InitFromEnv 在
// ENCRYPTION_KEY 未设置时返回错误（而非静默回退到全零密钥）。
func TestInitFromEnvReturnsErrorOnEmptyKey(t *testing.T) {
	t.Parallel()

	orig := os.Getenv("ENCRYPTION_KEY")
	os.Unsetenv("ENCRYPTION_KEY")
	defer func() {
		if orig != "" {
			os.Setenv("ENCRYPTION_KEY", orig)
		}
	}()

	err := InitFromEnv()
	if err == nil {
		t.Error("expected InitFromEnv to return error when ENCRYPTION_KEY is empty, got nil")
	}
}

// TestInitRejectsInvalidKeyLength 验证 Init 拒绝非 32 字节密钥。
func TestInitRejectsInvalidKeyLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{name: "too short", key: "0123456789abcdef"},
		{name: "too long", key: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00"},
		{name: "not hex", key: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Init(tt.key)
			if err == nil {
				t.Errorf("expected Init to reject key %q, got nil error", tt.name)
			}
		})
	}
}
