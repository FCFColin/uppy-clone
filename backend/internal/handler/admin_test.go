package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
)

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

// --- AdminHandler.Login tests ---

func TestAdminHandler_Login_AdminNotConfigured(t *testing.T) {
	// AdminHandler.Login calls h.db.GetConfig which is nil — will panic.
	// Since we can't easily mock the concrete *store.PostgresStore,
	// we test the password comparison and JWT signing logic directly.
	// The full Login flow requires an integration test with a real DB.

	// Test the signAdminToken method directly
	h := newTestAdminHandler()
	token, _, err := h.signAdminToken()
	if err != nil {
		t.Fatalf("signAdminToken error: %v", err)
	}
	if token == "" {
		t.Error("signAdminToken should return non-empty token")
	}

	// Verify the token has admin claims
	parsed, err := jwt.ParseWithClaims(token, &adminClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return h.adminJwtMgr.PublicKey(), nil
	})
	if err != nil {
		t.Fatalf("parse admin token error: %v", err)
	}
	claims, ok := parsed.Claims.(*adminClaims)
	if !ok {
		t.Fatal("token claims should be adminClaims")
	}
	if claims.Role != "admin" {
		t.Errorf("role = %q, want %q", claims.Role, "admin")
	}
	if claims.Subject != "admin" {
		t.Errorf("subject = %q, want %q", claims.Subject, "admin")
	}
}

func TestAdminHandler_VerifyAdminToken_ValidToken(t *testing.T) {
	h := newTestAdminHandler()

	// Generate a valid admin token
	token, _, err := h.signAdminToken()
	if err != nil {
		t.Fatalf("signAdminToken error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{
		Name:  "admin_token",
		Value: token,
	})

	if !h.VerifyAdminToken(req) {
		t.Error("VerifyAdminToken should return true for valid admin token")
	}
}

func TestAdminHandler_VerifyAdminToken_RejectionCases(t *testing.T) {
	h := newTestAdminHandler()

	// Build tokens for various rejection cases
	now := time.Now()

	wrongMethodToken := jwt.NewWithClaims(jwt.SigningMethodNone, adminClaims{
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	})
	wrongMethodStr, _ := wrongMethodToken.SignedString(jwt.UnsafeAllowNoneSignatureType)

	nonAdminToken, _ := h.adminJwtMgr.SignWithClaims(map[string]any{
		"role": "user", "sub": "admin",
		"iat": now.Unix(), "exp": now.Add(24 * time.Hour).Unix(),
	})

	expiredToken, _ := h.adminJwtMgr.SignWithClaims(map[string]any{
		"role": "admin", "sub": "admin",
		"iat": now.Add(-48 * time.Hour).Unix(), "exp": now.Add(-24 * time.Hour).Unix(),
	})

	tests := []struct {
		name   string
		cookie string
	}{
		{"no cookie", ""},
		{"invalid token", "invalid-token"},
		{"wrong signing method", wrongMethodStr},
		{"non-admin claims", nonAdminToken},
		{"expired token", expiredToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "admin_token", Value: tt.cookie})
			}
			if h.VerifyAdminToken(req) {
				t.Error("VerifyAdminToken should return false")
			}
		})
	}
}

// --- AdminHandler.GetConfig tests (requires DB, test masking logic directly) ---

func TestAdminHandler_MaskedKey(t *testing.T) {
	if maskedKey != "••••••••" {
		t.Errorf("maskedKey = %q, want %q", maskedKey, "••••••••")
	}
}

// TestUpdateConfig_OldPasswordVerification verifies that the old password
// verification logic used by UpdateConfig correctly rejects wrong old passwords
// and accepts correct ones. The full UpdateConfig handler requires a DB, so we
// test the verification primitive (compareAdminPassword) here.
// 企业为何需要：密码修改必须验证旧密码，防止攻击者通过窃取的 JWT 修改密码长期接管账户。
func TestUpdateConfig_OldPasswordVerification(t *testing.T) {
	correctOldPassword := "correct-old-password"
	storedHash, err := hashAdminPassword(correctOldPassword)
	if err != nil {
		t.Fatalf("hashAdminPassword error: %v", err)
	}

	t.Run("correct old password verifies", func(t *testing.T) {
		if !compareAdminPassword(correctOldPassword, storedHash) {
			t.Error("correct old password should verify against stored hash")
		}
	})

	t.Run("wrong old password rejected", func(t *testing.T) {
		if compareAdminPassword("wrong-old-password", storedHash) {
			t.Error("wrong old password should be rejected")
		}
	})

	t.Run("plaintext stored password rejected (no fallback)", func(t *testing.T) {
		// Even if old password matches plaintext, it should be rejected
		// because plaintext storage is no longer supported.
		if compareAdminPassword("plaintext-pwd", "plaintext-pwd") {
			t.Error("plaintext stored password should be rejected — bcrypt only")
		}
	})
}

// Avoid unused imports
var _ = json.Marshal
var _ = domain.AppConfig{}
var _ = crypto.Encrypt
var _ = fmt.Sprintf

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

func TestHashAdminPassword(t *testing.T) {
	t.Run("produces bcrypt hash", func(t *testing.T) {
		hashed, err := hashAdminPassword("admin123")
		if err != nil {
			t.Fatalf("hashAdminPassword error: %v", err)
		}
		if !isBcryptHash(hashed) {
			t.Errorf("hashAdminPassword should produce bcrypt hash, got %q", hashed)
		}
	})

	t.Run("different passwords produce different hashes", func(t *testing.T) {
		hash1, _ := hashAdminPassword("password1")
		hash2, _ := hashAdminPassword("password2")
		if hash1 == hash2 {
			t.Error("different passwords should produce different hashes")
		}
	})

	t.Run("same password produces different hashes (salt)", func(t *testing.T) {
		hash1, _ := hashAdminPassword("same-password")
		hash2, _ := hashAdminPassword("same-password")
		if hash1 == hash2 {
			t.Error("same password should produce different hashes due to salt")
		}
	})

	t.Run("hashed password can be verified", func(t *testing.T) {
		password := "test-password-123"
		hashed, err := hashAdminPassword(password)
		if err != nil {
			t.Fatalf("hashAdminPassword error: %v", err)
		}
		if !compareAdminPassword(password, hashed) {
			t.Error("hashed password should be verifiable with compareAdminPassword")
		}
	})

	t.Run("hash error", func(t *testing.T) {
		orig := bcryptGenerate
		bcryptGenerate = func(_ []byte, _ int) ([]byte, error) {
			return nil, errors.New("bcrypt failed")
		}
		t.Cleanup(func() { bcryptGenerate = orig })

		_, err := hashAdminPassword("pw")
		if err == nil {
			t.Fatal("expected hash error")
		}
	})
}

func TestIsBcryptHash(t *testing.T) {
	// bcrypt hashes are always 60 characters: $2a$10$ + 53 chars
	validBcrypt := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"$2a$ hash", validBcrypt, true},
		{"$2b$ hash", "$2b$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		{"$2y$ hash", "$2y$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		{"too short", "$2a$10$abc", false},
		{"too long", validBcrypt + "extra", false},
		{"wrong prefix", "$1a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhW", false},
		{"empty string", "", false},
		{"plaintext", "admin123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBcryptHash(tt.input)
			if got != tt.want {
				t.Errorf("isBcryptHash(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
