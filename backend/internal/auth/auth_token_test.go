package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestNewJWTManager_PanicsOnWeakSecret(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for JWT secret shorter than 32 bytes")
		}
	}()
	NewJWTManager("too-short")
}

// ─── SignToken + VerifyToken round-trip ──────────────────────────────

func TestSignVerifyToken_RoundTrip(t *testing.T) {
	mgr := setupJWTManager()

	token, err := mgr.SignToken("user-123", "快乐的气球")
	if err != nil {
		t.Fatalf("SignToken 失败: %v", err)
	}

	userId, nickname, jti, _, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken 失败: %v", err)
	}
	if userId != "user-123" {
		t.Fatalf("userId 不匹配: got=%s, want=user-123", userId)
	}
	if nickname != "快乐的气球" {
		t.Fatalf("nickname 不匹配: got=%s, want=快乐的气球", nickname)
	}
	if jti == "" {
		t.Fatal("jti 不应为空")
	}
	if len(jti) != 32 {
		t.Fatalf("jti 应为 32 字符 hex (16 bytes); got len=%d", len(jti))
	}
}

func TestVerifyToken_LegacyTokenDefaultsToUser(t *testing.T) {
	mgr := setupJWTManager()

	claims := customClaims{
		Nickname: "legacy",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-legacy",
			ID:        "legacy-jti",
			Issuer:    config.JWTIssuer,
			Audience:  []string{config.JWTAudience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tokenString, err := token.SignedString(mgr.privateKey)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	_, _, _, role, err := mgr.VerifyToken(tokenString)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if role != domain.RoleUser {
		t.Fatalf("legacy token role = %q, want %q (default)", role, domain.RoleUser)
	}
}

// ─── JTI uniqueness ──────────────────────────────────────────────────

func TestSignToken_JTIUnique(t *testing.T) {
	mgr := setupJWTManager()

	token1, _ := mgr.SignToken("user-1", "alice")
	token2, _ := mgr.SignToken("user-1", "alice")

	_, _, jti1, _, _ := mgr.VerifyToken(token1)
	_, _, jti2, _, _ := mgr.VerifyToken(token2)

	if jti1 == jti2 {
		t.Fatal("两个 token 的 jti 应不同")
	}
}

// ─── Expired token ───────────────────────────────────────────────────

func TestVerifyToken_Expired(t *testing.T) {
	mgr := setupJWTManager()

	claims := customClaims{
		Nickname: "test",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-exp",
			ID:        "test-jti-expired",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-8 * 24 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Second)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tokenString, err := token.SignedString(mgr.privateKey)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	_, _, _, _, err = mgr.VerifyToken(tokenString)
	if err == nil {
		t.Fatal("已过期的 token 应验证失败")
	}
}

// ─── Invalid token ───────────────────────────────────────────────────

func TestVerifyToken_Invalid(t *testing.T) {
	mgr := setupJWTManager()

	_, _, _, _, err := mgr.VerifyToken("this.is.not.a.valid.token")
	if err == nil {
		t.Fatal("无效 token 应验证失败")
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	key1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	key2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	mgr1 := NewJWTManagerWithKeys(key1, &key1.PublicKey)
	mgr2 := NewJWTManagerWithKeys(key2, &key2.PublicKey)

	token, err := mgr1.SignToken("user-1", "test")
	if err != nil {
		t.Fatalf("SignToken 失败: %v", err)
	}

	_, _, _, _, err = mgr2.VerifyToken(token)
	if err == nil {
		t.Fatal("使用错误密钥验证应失败")
	}
}

// ─── BuildAuthCookie ─────────────────────────────────────────────────

func TestBuildAuthCookie_HttpOnly(t *testing.T) {
	cookie := BuildAuthCookie("auth_token", "jwt-value", 3600, true)

	if !cookie.HttpOnly {
		t.Fatal("cookie 应设置 HttpOnly=true")
	}
	if cookie.Name != "auth_token" {
		t.Fatalf("cookie name 不匹配: got=%s, want=auth_token", cookie.Name)
	}
	if cookie.Value != "jwt-value" {
		t.Fatalf("cookie value 不匹配: got=%s, want=jwt-value", cookie.Value)
	}
	if cookie.Path != "/" {
		t.Fatalf("cookie path 不匹配: got=%s, want=/", cookie.Path)
	}
	if cookie.MaxAge != 3600 {
		t.Fatalf("cookie MaxAge 不匹配: got=%d, want=3600", cookie.MaxAge)
	}
	if !cookie.Secure {
		t.Fatal("cookie 应设置 Secure=true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite 应为 Lax，got=%v", cookie.SameSite)
	}
}

// ─── ParseAuthCookie ─────────────────────────────────────────────────

func TestParseAuthCookie_Valid(t *testing.T) {
	mgr := setupJWTManager()

	token, _ := mgr.SignToken("user-1", "nickname")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "auth",
		Value: token,
	})

	userId, nickname, jti, _, err := ParseAuthCookie(req, "auth", mgr)
	if err != nil {
		t.Fatalf("ParseAuthCookie 失败: %v", err)
	}
	if userId != "user-1" {
		t.Fatalf("userId 不匹配: got=%s, want=user-1", userId)
	}
	if nickname != "nickname" {
		t.Fatalf("nickname 不匹配: got=%s, want=nickname", nickname)
	}
	if jti == "" {
		t.Fatal("jti 不应为空")
	}
}

func TestParseAuthCookie_Missing(t *testing.T) {
	mgr := setupJWTManager()

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, _, _, _, err := ParseAuthCookie(req, "auth", mgr)
	if err == nil {
		t.Fatal("缺少 cookie 应返回错误")
	}
}

// ─── generateJTI ─────────────────────────────────────────────────────

func TestGenerateJTI(t *testing.T) {
	jti1, err := generateJTI()
	if err != nil {
		t.Fatalf("generateJTI 失败: %v", err)
	}
	if len(jti1) != 32 {
		t.Fatalf("jti 长度应为 32; got %d", len(jti1))
	}

	jti2, _ := generateJTI()
	if jti1 == jti2 {
		t.Fatal("两次生成的 jti 应不同")
	}
}

const testUserID = "user-123"

func TestRefreshTokenManager_GenerateSecureToken(t *testing.T) {
	t.Run("generateSecureToken produces hex string", func(t *testing.T) {
		token, err := generateSecureToken(32)
		if err != nil {
			t.Fatalf("generateSecureToken error: %v", err)
		}
		if len(token) != 64 {
			t.Errorf("token length = %d, want 64", len(token))
		}
	})

	t.Run("generateSecureToken produces unique tokens", func(t *testing.T) {
		token1, _ := generateSecureToken(32)
		token2, _ := generateSecureToken(32)
		if token1 == token2 {
			t.Error("two generated tokens should not be equal")
		}
	})
}

func TestRefreshTokenManager_Integration(t *testing.T) {
	_, rdb := testutil.NewTestMiniredis(t)
	mgr := NewRefreshTokenManager(rdb)
	ctx := context.Background()

	t.Run("Generate creates token in Redis", func(t *testing.T) {
		token, err := mgr.Generate(ctx, testUserID)
		if err != nil {
			t.Fatalf("Generate error: %v", err)
		}
		if token == "" {
			t.Error("Generate should return non-empty token")
		}

		key := refreshTokenPrefix + token
		val, err := rdb.Get(ctx, key).Result()
		if err != nil {
			t.Fatalf("token not found in Redis: %v", err)
		}
		if val != testUserID {
			t.Errorf("token value = %q, want %q", val, testUserID)
		}
	})
	t.Run("Validate accepts valid token", func(t *testing.T) {
		token, err := mgr.Generate(ctx, "user-validate")
		if err != nil {
			t.Fatalf("Generate error: %v", err)
		}

		userID, err := mgr.Validate(ctx, token)
		if err != nil {
			t.Fatalf("Validate error: %v", err)
		}
		if userID != "user-validate" {
			t.Errorf("userID = %q, want %q", userID, "user-validate")
		}
	})
	t.Run("Validate rejects invalid token", func(t *testing.T) {
		_, err := mgr.Validate(ctx, "nonexistent-token")
		if err == nil {
			t.Error("Validate should return error for invalid token")
		}
	})
	t.Run("Revoke removes token", func(t *testing.T) {
		token, err := mgr.Generate(ctx, "user-revoke")
		if err != nil {
			t.Fatalf("Generate error: %v", err)
		}

		err = mgr.Revoke(ctx, token)
		if err != nil {
			t.Fatalf("Revoke error: %v", err)
		}

		_, err = mgr.Validate(ctx, token)
		if err == nil {
			t.Error("Validate should fail after Revoke")
		}
	})
	t.Run("RevokeAllForUser removes all tokens for a user", func(t *testing.T) {
		token1, _ := mgr.Generate(ctx, "user-revokeall")
		token2, _ := mgr.Generate(ctx, "user-revokeall")

		err := mgr.RevokeAllForUser(ctx, "user-revokeall")
		if err != nil {
			t.Fatalf("RevokeAllForUser error: %v", err)
		}

		_, err1 := mgr.Validate(ctx, token1)
		_, err2 := mgr.Validate(ctx, token2)
		if err1 == nil || err2 == nil {
			t.Error("both tokens should be invalid after RevokeAllForUser")
		}
	})
}

func TestRevokeAllTokens_NilRequest(_ *testing.T) {
	mgr := setupJWTManager()
	_ = RevokeAllTokens(context.Background(), mgr, nil, nil, nil)
}

func TestRevokeAllTokens_NoCookie(_ *testing.T) {
	mgr := setupJWTManager()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	_ = RevokeAllTokens(context.Background(), mgr, nil, nil, req)
}

func TestRevokeAllTokens_InvalidToken(t *testing.T) {
	mgr := setupJWTManager()
	redisStore := testutil.SetupMiniredisStore(t)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid.token.here"})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: "also.invalid"})

	_ = RevokeAllTokens(context.Background(), mgr, nil, redisStore, req)
}

func TestRevokeAllTokens_NilRedis(t *testing.T) {
	mgr := setupJWTManager()

	token, err := mgr.SignToken("user-nil-redis", "TestPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	_ = RevokeAllTokens(context.Background(), mgr, nil, nil, req)
}

func TestRevokeAllTokens_Cookies_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		cookies []struct {
			name, user, player string
		}
	}{
		{
			name: "SessionCookie",
			cookies: []struct {
				name, user, player string
			}{{"session", "user-session-revoke", "SessionPlayer"}},
		},
		{
			name: "QuickPlayCookie",
			cookies: []struct {
				name, user, player string
			}{{"quickplay", "user-quickplay-revoke", "QuickPlayer"}},
		},
		{
			name: "BothCookies",
			cookies: []struct {
				name, user, player string
			}{
				{"session", "user-both-session", "SessionUser"},
				{"quickplay", "user-both-quickplay", "QuickUser"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := setupJWTManager()
			redisStore := testutil.SetupMiniredisStore(t)
			ctx := context.Background()

			req := httptest.NewRequest(http.MethodPost, "/logout", nil)
			jtis := make([]string, 0, len(tt.cookies))
			for _, c := range tt.cookies {
				token, err := mgr.SignToken(c.user, c.player)
				if err != nil {
					t.Fatalf("SignToken failed: %v", err)
				}
				_, _, jti, _, err := mgr.VerifyToken(token)
				if err != nil {
					t.Fatalf("VerifyToken failed: %v", err)
				}
				jtis = append(jtis, jti)
				req.AddCookie(&http.Cookie{Name: c.name, Value: token})
			}

			_ = RevokeAllTokens(ctx, mgr, nil, redisStore, req)

			for _, jti := range jtis {
				revoked, err := redisStore.IsJWTRevoked(ctx, jti)
				if err != nil {
					t.Fatalf("IsJWTRevoked failed: %v", err)
				}
				if !revoked {
					t.Fatal("jti should be revoked after RevokeAllTokens")
				}
			}
		})
	}
}

func TestRevokeAllTokens_Concurrent(t *testing.T) {
	mgr := setupJWTManager()
	redisStore := testutil.SetupMiniredisStore(t)

	done := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			token, _ := mgr.SignToken("user-concurrent", "Concurrent")
			req := httptest.NewRequest(http.MethodPost, "/logout", nil)
			req.AddCookie(&http.Cookie{Name: "session", Value: token})
			_ = RevokeAllTokens(context.Background(), mgr, nil, redisStore, req)
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestSignToken_RandFailure(t *testing.T) {
	defer SetRandReadHook(func([]byte) (int, error) { return 0, errRandFail })()

	mgr := setupJWTManager()
	if _, err := mgr.SignToken("user-1", "Nick"); err == nil {
		t.Fatal("expected SignToken error when rand fails")
	}
}

var errRandFail = &randFailError{}

type randFailError struct{}

func (e *randFailError) Error() string { return "rand failed" }
