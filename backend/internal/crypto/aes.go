// Package crypto provides AES encryption helpers for sensitive user data.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Enterprise rationale: Database breaches expose all data at rest. Encrypting
// sensitive fields (API keys, secrets) at the application layer provides
// defense-in-depth — even if the DB is dumped, the attacker needs the
// encryption key (stored in environment variables, not the DB).
// AES-GCM provides both confidentiality and integrity (authenticated encryption).
// Trade-off: Extra CPU for encrypt/decrypt, and key rotation requires re-encryption.

// 企业为何需要：全零密钥是公开已知的，用它"加密"等于明文存储。
// 生产环境必须显式提供密钥，缺失时 fail-fast 而非静默降级到不安全状态。
var (
	// Encryption key must be 32 bytes (AES-256). Set via Init() or InitFromEnv().
	encKey   []byte
	encKeyMu sync.RWMutex
)

// Init 使用给定的 hex 编码密钥初始化 AES-256 加密器。
// 密钥必须是 64 个十六进制字符（32 字节，AES-256）。
func Init(encryptionKey string) error {
	key, err := hex.DecodeString(encryptionKey)
	if err != nil {
		return fmt.Errorf("ENCRYPTION_KEY is not valid hex: %w", err)
	}
	if len(key) != 32 {
		return fmt.Errorf("ENCRYPTION_KEY must be 32 bytes (64 hex chars) for AES-256, got %d bytes", len(key))
	}
	encKeyMu.Lock()
	encKey = key
	encKeyMu.Unlock()
	return nil
}

// InitFromEnv 从 ENCRYPTION_KEY 环境变量读取并初始化密钥。
// 如果环境变量未设置，返回错误（不回退到全零密钥）。
func InitFromEnv() error {
	keyHex := os.Getenv("ENCRYPTION_KEY")
	if keyHex == "" {
		return errors.New("ENCRYPTION_KEY environment variable is required (set a 32-byte hex key, 64 hex chars). Refusing to start with no encryption key")
	}
	return Init(keyHex)
}

// ResetKeyForTest clears the module encryption key. For tests only.
func ResetKeyForTest() {
	encKeyMu.Lock()
	encKey = nil
	encKeyMu.Unlock()
}

// SetEncKeyForTest sets a raw encryption key. For tests only.
func SetEncKeyForTest(key []byte) {
	encKeyMu.Lock()
	encKey = key
	encKeyMu.Unlock()
}

// aesRandRead is injectable for unit tests (e.g. simulate crypto/rand failures).
var aesRandRead = rand.Read

// aesNewGCM is injectable for unit tests (e.g. simulate cipher.NewGCM failures).
var aesNewGCM = cipher.NewGCM

func newGCM() (cipher.AEAD, error) {
	encKeyMu.RLock()
	key := encKey
	encKeyMu.RUnlock()
	if key == nil {
		return nil, errors.New("encryption key not initialized: call Init() or InitFromEnv() first")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := aesNewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return gcm, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns versioned hex-encoded ciphertext.
// Output format: "v1:hex_ciphertext" for versioned key rotation support.
func Encrypt(plaintext string) (string, error) {
	gcm, err := newGCM()
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := aesRandRead(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "v1:" + hex.EncodeToString(ciphertext), nil
}

// Decrypt decrypts hex-encoded AES-256-GCM ciphertext.
// Supports both "v1:hex" format (new) and raw hex (legacy, for backward compatibility).
func Decrypt(encoded string) (string, error) {
	encoded = strings.TrimPrefix(encoded, "v1:")

	ciphertext, err := hex.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode hex: %w", err)
	}

	gcm, err := newGCM()
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// EmailHMAC computes SHA-256 of a normalized email for indexed lookup.
// Uses unkeyed SHA-256 to ensure email_hash remains stable across key rotation.
// If keyed HMAC is needed for additional rainbow-table resistance, the HMAC key
// must be stored independently from the encryption key and rotated separately.
func EmailHMAC(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
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
