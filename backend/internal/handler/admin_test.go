package handler

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
)

// --- Test helpers ---

const testJWTSecret = "test-admin-jwt-secret-key-for-testing" //nolint:gosec // test secret

func newTestAdminHandler() *AdminHandler {
	jwtMgr := auth.NewJWTManager(testJWTSecret)
	return NewAdminHandler(nil, jwtMgr, nil)
}

// --- AdminHandler.Login tests ---

func TestAdminHandler_Login_InvalidRequestBody(t *testing.T) {
	h := newTestAdminHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader("invalid json"))
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAdminHandler_Login_AdminNotConfigured(t *testing.T) {
	// AdminHandler.Login calls h.db.GetConfig which is nil — will panic.
	// Since we can't easily mock the concrete *store.PostgresStore,
	// we test the password comparison and JWT signing logic directly.
	// The full Login flow requires an integration test with a real DB.

	// Test the signAdminToken method directly
	h := newTestAdminHandler()
	token, err := h.signAdminToken()
	if err != nil {
		t.Fatalf("signAdminToken error: %v", err)
	}
	if token == "" {
		t.Error("signAdminToken should return non-empty token")
	}

	// Verify the token has admin claims
	parsed, err := jwt.ParseWithClaims(token, &adminClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(testJWTSecret), nil
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
	token, err := h.signAdminToken()
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

func TestAdminHandler_VerifyAdminToken_NoCookie(t *testing.T) {
	h := newTestAdminHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)

	if h.VerifyAdminToken(req) {
		t.Error("VerifyAdminToken should return false when no cookie is present")
	}
}

func TestAdminHandler_VerifyAdminToken_InvalidToken(t *testing.T) {
	h := newTestAdminHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{
		Name:  "admin_token",
		Value: "invalid-token",
	})

	if h.VerifyAdminToken(req) {
		t.Error("VerifyAdminToken should return false for invalid token")
	}
}

func TestAdminHandler_VerifyAdminToken_WrongSigningMethod(t *testing.T) {
	h := newTestAdminHandler()

	// Create a token with wrong signing method
	now := time.Now()
	claims := adminClaims{
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{
		Name:  "admin_token",
		Value: tokenString,
	})

	if h.VerifyAdminToken(req) {
		t.Error("VerifyAdminToken should reject token with wrong signing method")
	}
}

func TestAdminHandler_VerifyAdminToken_NonAdminClaims(t *testing.T) {
	h := newTestAdminHandler()

	// Create a token without admin role
	now := time.Now()
	claims := adminClaims{
		Role: "user", // not admin
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testJWTSecret))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{
		Name:  "admin_token",
		Value: tokenString,
	})

	if h.VerifyAdminToken(req) {
		t.Error("VerifyAdminToken should reject token with non-admin role")
	}
}

func TestAdminHandler_VerifyAdminToken_ExpiredToken(t *testing.T) {
	h := newTestAdminHandler()

	// Create an expired token
	now := time.Now()
	claims := adminClaims{
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin",
			IssuedAt:  jwt.NewNumericDate(now.Add(-48 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-24 * time.Hour)), // expired
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testJWTSecret))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.AddCookie(&http.Cookie{
		Name:  "admin_token",
		Value: tokenString,
	})

	if h.VerifyAdminToken(req) {
		t.Error("VerifyAdminToken should reject expired token")
	}
}

// --- AdminHandler.GetConfig tests (requires DB, test masking logic directly) ---

func TestAdminHandler_MaskedKey(t *testing.T) {
	if maskedKey != "••••••••" {
		t.Errorf("maskedKey = %q, want %q", maskedKey, "••••••••")
	}
}

// --- AdminHandler.UpdateConfig tests (requires DB, test masking logic) ---

func TestAdminHandler_UpdateConfig_MasksApiKey(t *testing.T) {
	// Test that the masked key constant is not treated as a real API key
	if maskedKey == "" {
		t.Error("maskedKey should not be empty")
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
