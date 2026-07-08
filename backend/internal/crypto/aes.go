// Package crypto provides AES encryption helpers for sensitive user data.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
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
	encKey []byte
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
	encKey = key
	initEmailHMACKey()
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

// MustInitFromEnv 从环境变量初始化密钥，失败时 panic。
// 供需要在初始化时 fail-fast 的场景使用。
func MustInitFromEnv() {
	if err := InitFromEnv(); err != nil {
		panic(err)
	}
}

// ResetKeyForTest clears the module encryption key. For tests only.
func ResetKeyForTest() {
	encKey = nil
}

// SetEncKeyForTest sets a raw encryption key. For tests only.
func SetEncKeyForTest(key []byte) {
	encKey = key
}

// aesRandRead is injectable for unit tests (e.g. simulate crypto/rand failures).
var aesRandRead = rand.Read

// aesNewGCM is injectable for unit tests (e.g. simulate cipher.NewGCM failures).
var aesNewGCM = cipher.NewGCM

func newGCM() (cipher.AEAD, error) {
	if encKey == nil {
		return nil, errors.New("encryption key not initialized: call Init() or InitFromEnv() first")
	}
	block, err := aes.NewCipher(encKey)
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

// ReEncryptWithKey decrypts ciphertext using oldKey and re-encrypts with newKey.
// This is the building block for batch key rotation: callers iterate over encrypted
// database fields, calling this function on each value, then call RotateKey to
// switch the active encryption key.
//
// 企业为何需要：密钥轮换要求将存量密文用新密钥重新加密。此函数提供逐字段轮换能力，
// 调用方（如 admin handler / migration script）遍历加密列，对每条记录调用此函数。
func ReEncryptWithKey(oldKey, newKey []byte, ciphertext string) (string, error) {
	if len(oldKey) != 32 {
		return "", fmt.Errorf("old key must be 32 bytes for AES-256, got %d", len(oldKey))
	}
	if len(newKey) != 32 {
		return "", fmt.Errorf("new key must be 32 bytes for AES-256, got %d", len(newKey))
	}

	// Strip version prefix if present.
	raw := strings.TrimPrefix(ciphertext, "v1:")
	cipherBytes, err := hex.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("decode hex: %w", err)
	}

	// Decrypt with old key.
	oldBlock, err := aes.NewCipher(oldKey)
	if err != nil {
		return "", fmt.Errorf("create old cipher: %w", err)
	}
	oldGCM, err := aesNewGCM(oldBlock)
	if err != nil {
		return "", fmt.Errorf("create old GCM: %w", err)
	}
	nonceSize := oldGCM.NonceSize()
	if len(cipherBytes) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := cipherBytes[:nonceSize], cipherBytes[nonceSize:]
	plaintext, err := oldGCM.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt with old key: %w", err)
	}

	// Re-encrypt with new key.
	newBlock, err := aes.NewCipher(newKey)
	if err != nil {
		return "", fmt.Errorf("create new cipher: %w", err)
	}
	newGCM, err := aesNewGCM(newBlock)
	if err != nil {
		return "", fmt.Errorf("create new GCM: %w", err)
	}
	newNonce := make([]byte, newGCM.NonceSize())
	if _, err := aesRandRead(newNonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	newCipher := newGCM.Seal(newNonce, newNonce, plaintext, nil)
	return "v1:" + hex.EncodeToString(newCipher), nil
}

// RotateKey activates a new encryption key for all subsequent Encrypt calls.
// The oldKey parameter is accepted for API symmetry but not stored — callers
// must use ReEncryptWithKey to migrate existing ciphertexts BEFORE calling
// this function, as prior ciphertexts will no longer be decryptable after
// the switch.
//
// 企业为何需要：密钥轮换流程为 (1) ReEncryptWithKey 遍历加密列，(2) RotateKey
// 切换活跃密钥。两步分离确保轮换过程中新写入的密文用新密钥，且可回滚。
func RotateKey(oldKey, newKey []byte) error {
	_ = oldKey // accepted for API symmetry; not stored
	if len(newKey) != 32 {
		return fmt.Errorf("new key must be 32 bytes for AES-256, got %d", len(newKey))
	}
	encKey = newKey
	initEmailHMACKey()
	return nil
}
