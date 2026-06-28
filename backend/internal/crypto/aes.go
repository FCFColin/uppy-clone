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

// Encrypt encrypts plaintext using AES-256-GCM and returns versioned hex-encoded ciphertext.
// Output format: "v1:hex_ciphertext" for versioned key rotation support.
// 企业为何需要：版本前缀允许未来密钥轮换时区分新旧密文，批量重新加密可按版本筛选。
func Encrypt(plaintext string) (string, error) {
	if encKey == nil {
		return "", errors.New("encryption key not initialized: call Init() or InitFromEnv() first")
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	// nonce is prepended to ciphertext for decryption
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "v1:" + hex.EncodeToString(ciphertext), nil
}

// Decrypt decrypts hex-encoded AES-256-GCM ciphertext.
// Supports both "v1:hex" format (new) and raw hex (legacy, for backward compatibility).
// 企业为何需要：向后兼容旧密文格式，避免密钥轮换时数据不可读。
func Decrypt(encoded string) (string, error) {
	if encKey == nil {
		return "", errors.New("encryption key not initialized: call Init() or InitFromEnv() first")
	}

	// Check for version prefix (new format); strip it for decryption.
	// Legacy raw-hex ciphertext (without prefix) is still accepted.
	encoded = strings.TrimPrefix(encoded, "v1:")

	ciphertext, err := hex.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode hex: %w", err)
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
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

// RotateKey re-encrypts all AES-encrypted fields with a new key.
// TODO: implement batch re-encryption of database fields when key rotation is needed.
// 企业为何需要：密钥轮换要求将存量密文用新密钥重新加密，此处为预留接口。
func RotateKey(_, _ []byte) error {
	return fmt.Errorf("RotateKey not yet implemented")
}
