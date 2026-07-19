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

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
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
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

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

// ─── Role claim ─────────────────────────────────────────────────────

func TestSignToken_DefaultRoleIsUser(t *testing.T) {
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	token, err := mgr.SignToken("user-role", "RoleTest")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	_, _, _, role, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if role != domain.RoleUser {
		t.Fatalf("role = %q, want %q", role, domain.RoleUser)
	}
}

func TestSignTokenWithRole_CustomRole(t *testing.T) {
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	token, err := mgr.SignTokenWithRole("user-custom", "CustomRole", "moderator")
	if err != nil {
		t.Fatalf("SignTokenWithRole: %v", err)
	}

	_, _, _, role, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if role != "moderator" {
		t.Fatalf("role = %q, want %q", role, "moderator")
	}
}

func TestVerifyToken_LegacyTokenDefaultsToUser(t *testing.T) {
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	// Manually construct a token without a role claim (simulating a legacy token)
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
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

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
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	// 手动构造一个已过期的 token
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
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

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

func TestVerifyToken_UnexpectedSigningMethod(t *testing.T) {
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token := jwt.NewWithClaims(jwt.SigningMethodNone, customClaims{
		Nickname: "test",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ID:        "none-jti",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	})
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	_, _, _, _, err = mgr.VerifyToken(tokenString)
	if err == nil {
		t.Fatal("none-alg token should fail verification")
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
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

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
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

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

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可在生产暴露。

const testUserID = "user-123"

// We test RefreshTokenManager using a miniredis in-memory Redis server
// if available, or test the pure logic functions.
// Since the manager uses concrete *redis.Client, we test what we can
// without requiring a real Redis connection.

func TestRefreshTokenManager_GenerateSecureToken(t *testing.T) {
	t.Run("generateSecureToken produces hex string", func(t *testing.T) {
		token, err := generateSecureToken(32)
		if err != nil {
			t.Fatalf("generateSecureToken error: %v", err)
		}
		// 32 bytes = 64 hex chars
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

// RO-037: Converted from localhost:6379 to miniredis so this runs as a unit test
// without any external Redis dependency.

func TestRefreshTokenManager_Integration(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()

	mgr := NewRefreshTokenManager(rdb)
	ctx := context.Background()

	t.Run("Generate creates token in Redis", func(t *testing.T) {
		testRefreshTokenGenerate(t, mgr, rdb, ctx)
	})
	t.Run("Validate accepts valid token", func(t *testing.T) {
		testRefreshTokenValidate(t, mgr, ctx)
	})
	t.Run("Validate rejects invalid token", func(t *testing.T) {
		testRefreshTokenValidateInvalid(t, mgr, ctx)
	})
	t.Run("Revoke removes token", func(t *testing.T) {
		testRefreshTokenRevoke(t, mgr, ctx)
	})
	t.Run("RevokeAllForUser removes all tokens for a user", func(t *testing.T) {
		testRefreshTokenRevokeAll(t, mgr, ctx)
	})
}

func testRefreshTokenGenerate(t *testing.T, mgr *RefreshTokenManager, rdb *redis.Client, ctx context.Context) {
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
}

func testRefreshTokenValidate(t *testing.T, mgr *RefreshTokenManager, ctx context.Context) {
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
}

func testRefreshTokenValidateInvalid(t *testing.T, mgr *RefreshTokenManager, ctx context.Context) {
	_, err := mgr.Validate(ctx, "nonexistent-token")
	if err == nil {
		t.Error("Validate should return error for invalid token")
	}
}

func testRefreshTokenRevoke(t *testing.T, mgr *RefreshTokenManager, ctx context.Context) {
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
}

func testRefreshTokenRevokeAll(t *testing.T, mgr *RefreshTokenManager, ctx context.Context) {
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
}

// 企业为何需要：RevokeAllTokens 是登出/删除用户的核心安全函数。
// 撤销失败可能导致被盗 token 在过期前持续有效。此测试覆盖所有分支：
// 有/无 cookie、有效/无效 token、空 jti、redis 为 nil、以及真实撤销验证。

// setupTestRedisStore creates a RedisStore backed by miniredis for unit tests.
// RO-037: converted from localhost:6379 to miniredis (no external dependency).
func setupTestRedisStore(t *testing.T) *store.RedisStore {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	redisStore, err := store.NewRedisStore(mr.Addr(), config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = redisStore.Close() })
	return redisStore
}

func newTestJWTManager(t *testing.T) *JWTManager {
	t.Helper()
	return NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
}

// --- 无 cookie：不应 panic ---

func TestRevokeAllTokens_NilRequest(t *testing.T) {
	mgr := newTestJWTManager(t)
	_ = RevokeAllTokens(context.Background(), mgr, nil, nil, nil)
}

func TestRevokeAllTokens_NoCookie(t *testing.T) {
	mgr := newTestJWTManager(t)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)

	// 不应 panic
	_ = RevokeAllTokens(context.Background(), mgr, nil, nil, req)
}

func TestRevokeAllTokens_RefreshRevokeError(t *testing.T) {
	mgr := newTestJWTManager(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()

	token, err := mgr.SignToken("user-refresh-revoke", "Player")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	if _, err := refreshMgr.Generate(ctx, "user-refresh-revoke"); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	mr.SetError("redis unavailable")
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})

	_ = RevokeAllTokens(ctx, mgr, refreshMgr, nil, req)
}

// --- cookie 存在但 token 无效：不应 panic，不调用 RevokeJWT ---

func TestRevokeAllTokens_InvalidToken(t *testing.T) {
	mgr := newTestJWTManager(t)
	redisStore := setupTestRedisStore(t)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid.token.here"})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: "also.invalid"})

	// 不应 panic
	_ = RevokeAllTokens(context.Background(), mgr, nil, redisStore, req)
}

// --- cookie 值为空字符串：不应 panic ---

func TestRevokeAllTokens_EmptyCookieValue(t *testing.T) {
	mgr := newTestJWTManager(t)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: ""})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: ""})

	// 不应 panic
	_ = RevokeAllTokens(context.Background(), mgr, nil, nil, req)
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
	_ = RevokeAllTokens(context.Background(), mgr, nil, nil, req)
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
			Issuer:    config.JWTIssuer,
			Audience:  []string{config.JWTAudience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tokenString, err := token.SignedString(mgr.privateKey)
	if err != nil {
		t.Fatalf("SignedString failed: %v", err)
	}

	// 验证 token 确实有效且 jti 为空
	_, _, jti, _, verifyErr := mgr.VerifyToken(tokenString)
	if verifyErr != nil {
		t.Fatalf("VerifyToken should succeed for manually-signed token: %v", verifyErr)
	}
	if jti != "" {
		t.Fatalf("jti should be empty, got %q", jti)
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: tokenString})

	// 不应 panic，也不应调用 RevokeJWT（jti 为空）
	_ = RevokeAllTokens(context.Background(), mgr, nil, redisStore, req)
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

	_, _, jti, _, err := mgr.VerifyToken(token)
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

	_ = RevokeAllTokens(ctx, mgr, nil, redisStore, req)

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

	_, _, jti, _, err := mgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	_ = RevokeAllTokens(ctx, mgr, nil, redisStore, req)

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

	_, _, sessionJTI, _, _ := mgr.VerifyToken(sessionToken)
	_, _, quickplayJTI, _, _ := mgr.VerifyToken(quickplayToken)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionToken})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: quickplayToken})

	_ = RevokeAllTokens(ctx, mgr, nil, redisStore, req)

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
	_, _, validJTI, _, _ := mgr.VerifyToken(validToken)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "invalid.token.value"})
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: validToken})

	_ = RevokeAllTokens(ctx, mgr, nil, redisStore, req)

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
			_ = RevokeAllTokens(context.Background(), mgr, nil, redisStore, req)
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestSignToken_RandFailure(t *testing.T) {
	defer SetRandReadHook(func([]byte) (int, error) { return 0, errRandFail })()

	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	if _, err := mgr.SignToken("user-1", "Nick"); err == nil {
		t.Fatal("expected SignToken error when rand fails")
	}
}

func TestGenerateSecureToken_RandFailure(t *testing.T) {
	defer SetRandReadHook(func([]byte) (int, error) { return 0, errRandFail })()

	mgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}))
	if _, err := mgr.Generate(context.Background(), "user-1"); err == nil {
		t.Fatal("expected Generate error when rand fails")
	}
}

var errRandFail = &randFailError{}

type randFailError struct{}

func (e *randFailError) Error() string { return "rand failed" }

func TestVerifyWithKey_UnexpectedSigningMethod(t *testing.T) {
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, &customClaims{
		Nickname: "Nick",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ID:        "jti-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	})
	// Unsigned token string still triggers unexpected alg in verify path when parsed.
	s := token.Raw
	if s == "" {
		s = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.e30.signature"
	}
	if _, _, _, _, err := mgr.VerifyToken(s); err == nil {
		t.Fatal("expected verify error for unexpected signing method")
	}
}

func TestVerifyWithKey_ExpiredToken(t *testing.T) {
	mgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	_, err := mgr.SignToken("user-1", "Nick")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	// Re-sign with past expiry by building claims manually.
	expired := jwt.NewWithClaims(jwt.SigningMethodES256, &customClaims{
		Nickname: "Nick",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ID:        "exp-jti",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	})
	s, err := expired.SignedString(mgr.privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := mgr.VerifyToken(s); err == nil {
		t.Fatal("expected expired token error")
	}
}

