package auth

// 企业为何需要：RevokeAllTokens 是登出/删除用户的核心安全函数。
// 撤销失败可能导致被盗 token 在过期前持续有效。此测试覆盖所有分支：
// 有/无 cookie、有效/无效 token、空 jti、redis 为 nil、以及真实撤销验证。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/store"
)

// setupTestRedisStore creates a RedisStore for integration tests.
// Skips the test if Redis is not available at localhost:6379.
func setupTestRedisStore(t *testing.T) *store.RedisStore {
	t.Helper()
	redisStore, err := store.NewRedisStore("localhost:6379", config.DefaultTimeoutConfig())
	if err != nil {
		t.Skipf("Redis not available, skipping integration test: %v", err)
	}
	t.Cleanup(func() { _ = redisStore.Close() })
	return redisStore
}

func newTestJWTManager(t *testing.T) *JWTManager {
	t.Helper()
	return NewJWTManager("test-secret-key-0123456789abcdef0123456789")
}

// --- 无 cookie：不应 panic ---

func TestRevokeAllTokens_NoCookie(t *testing.T) {
	mgr := newTestJWTManager(t)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)

	// 不应 panic
	RevokeAllTokens(context.Background(), mgr, nil, req)
}

// --- cookie 存在但 token 无效：不应 panic，不调用 RevokeJWT ---

func TestRevokeAllTokens_InvalidToken(t *testing.T) {
	mgr := newTestJWTManager(t)
	redisStore := setupTestRedisStore(t)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid.token.here"})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: "also.invalid"})

	// 不应 panic
	RevokeAllTokens(context.Background(), mgr, redisStore, req)
}

// --- cookie 值为空字符串：不应 panic ---

func TestRevokeAllTokens_EmptyCookieValue(t *testing.T) {
	mgr := newTestJWTManager(t)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: ""})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: ""})

	// 不应 panic
	RevokeAllTokens(context.Background(), mgr, nil, req)
}

// --- redis 为 nil：有效 token 也不应 panic ---

func TestRevokeAllTokens_NilRedis(t *testing.T) {
	mgr := newTestJWTManager(t)

	token, err := mgr.SignToken("user-nil-redis", "TestPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	// 不应 panic — redis 为 nil 时跳过 RevokeJWT
	RevokeAllTokens(context.Background(), mgr, nil, req)
}

// --- jti 为空：VerifyToken 成功但 jti==""，不调用 RevokeJWT ---

func TestRevokeAllTokens_EmptyJTI(t *testing.T) {
	mgr := newTestJWTManager(t)
	redisStore := setupTestRedisStore(t)

	// 手动构造 jti 为空的有效 token（同包可访问 customClaims）
	claims := customClaims{
		Nickname: "EmptyJTI",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-empty-jti",
			ID:        "", // 空 jti
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(mgr.primarySecret)
	if err != nil {
		t.Fatalf("SignedString failed: %v", err)
	}

	// 验证 token 确实有效且 jti 为空
	_, _, jti, verifyErr := mgr.VerifyToken(tokenString)
	if verifyErr != nil {
		t.Fatalf("VerifyToken should succeed for manually-signed token: %v", verifyErr)
	}
	if jti != "" {
		t.Fatalf("jti should be empty, got %q", jti)
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: tokenString})

	// 不应 panic，也不应调用 RevokeJWT（jti 为空）
	RevokeAllTokens(context.Background(), mgr, redisStore, req)
}

// --- session cookie：有效 token + 真实 Redis → jti 被撤销 ---

func TestRevokeAllTokens_SessionCookie(t *testing.T) {
	mgr := newTestJWTManager(t)
	redisStore := setupTestRedisStore(t)
	ctx := context.Background()

	token, err := mgr.SignToken("user-session-revoke", "SessionPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	_, _, jti, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	// 撤销前 jti 不应在撤销列表中
	revoked, err := redisStore.IsJWTRevoked(ctx, jti)
	if err != nil {
		t.Fatalf("IsJWTRevoked before revoke failed: %v", err)
	}
	if revoked {
		t.Fatal("jti should NOT be revoked before RevokeAllTokens")
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	RevokeAllTokens(ctx, mgr, redisStore, req)

	// 撤销后 jti 应在撤销列表中
	revoked, err = redisStore.IsJWTRevoked(ctx, jti)
	if err != nil {
		t.Fatalf("IsJWTRevoked after revoke failed: %v", err)
	}
	if !revoked {
		t.Fatal("jti should be revoked after RevokeAllTokens with session cookie")
	}
}

// --- quickplay cookie：有效 token + 真实 Redis → jti 被撤销 ---

func TestRevokeAllTokens_QuickPlayCookie(t *testing.T) {
	mgr := newTestJWTManager(t)
	redisStore := setupTestRedisStore(t)
	ctx := context.Background()

	token, err := mgr.SignToken("user-quickplay-revoke", "QuickPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	_, _, jti, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	RevokeAllTokens(ctx, mgr, redisStore, req)

	revoked, err := redisStore.IsJWTRevoked(ctx, jti)
	if err != nil {
		t.Fatalf("IsJWTRevoked failed: %v", err)
	}
	if !revoked {
		t.Fatal("jti should be revoked after RevokeAllTokens with quickplay cookie")
	}
}

// --- 两个 cookie 同时存在：两个 jti 都应被撤销 ---

func TestRevokeAllTokens_BothCookies(t *testing.T) {
	mgr := newTestJWTManager(t)
	redisStore := setupTestRedisStore(t)
	ctx := context.Background()

	sessionToken, err := mgr.SignToken("user-both-session", "SessionUser")
	if err != nil {
		t.Fatalf("SignToken session failed: %v", err)
	}
	quickplayToken, err := mgr.SignToken("user-both-quickplay", "QuickUser")
	if err != nil {
		t.Fatalf("SignToken quickplay failed: %v", err)
	}

	_, _, sessionJTI, _ := mgr.VerifyToken(sessionToken)
	_, _, quickplayJTI, _ := mgr.VerifyToken(quickplayToken)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionToken})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: quickplayToken})

	RevokeAllTokens(ctx, mgr, redisStore, req)

	// 两个 jti 都应被撤销
	sessionRevoked, err := redisStore.IsJWTRevoked(ctx, sessionJTI)
	if err != nil {
		t.Fatalf("IsJWTRevoked session failed: %v", err)
	}
	if !sessionRevoked {
		t.Fatal("session jti should be revoked")
	}

	quickplayRevoked, err := redisStore.IsJWTRevoked(ctx, quickplayJTI)
	if err != nil {
		t.Fatalf("IsJWTRevoked quickplay failed: %v", err)
	}
	if !quickplayRevoked {
		t.Fatal("quickplay jti should be revoked")
	}
}

// --- 一个有效一个无效：只有有效的被撤销 ---

func TestRevokeAllTokens_OneValidOneInvalid(t *testing.T) {
	mgr := newTestJWTManager(t)
	redisStore := setupTestRedisStore(t)
	ctx := context.Background()

	validToken, err := mgr.SignToken("user-mixed-valid", "ValidPlayer")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}
	_, _, validJTI, _ := mgr.VerifyToken(validToken)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid.token.value"})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: validToken})

	RevokeAllTokens(ctx, mgr, redisStore, req)

	// 有效的 quickplay token 应被撤销
	revoked, err := redisStore.IsJWTRevoked(ctx, validJTI)
	if err != nil {
		t.Fatalf("IsJWTRevoked failed: %v", err)
	}
	if !revoked {
		t.Fatal("valid quickplay jti should be revoked even when session token is invalid")
	}
}

// --- 并发安全：多个 goroutine 同时调用不应 panic ---

func TestRevokeAllTokens_Concurrent(t *testing.T) {
	mgr := newTestJWTManager(t)
	redisStore := setupTestRedisStore(t)

	done := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			token, _ := mgr.SignToken("user-concurrent", "Concurrent")
			req := httptest.NewRequest(http.MethodPost, "/logout", nil)
			req.AddCookie(&http.Cookie{Name: "session", Value: token})
			RevokeAllTokens(context.Background(), mgr, redisStore, req)
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
}
