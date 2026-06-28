package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/store"
)

func setupMagicLinkCrypto(t *testing.T) {
	t.Helper()
	if err := crypto.Init("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}
}

func setupMagicLinkRedis(t *testing.T) (*store.RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	timeouts := config.DefaultTimeoutConfig()
	rdb, err := store.NewRedisStore(mr.Addr(), timeouts)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb, mr
}

func TestMagicLinkService_RequestMagicLink_InvalidEmail(t *testing.T) {
	svc := NewMagicLinkService()
	redisStore, _ := setupMagicLinkRedis(t)
	err := svc.RequestMagicLink(redisStore, nil, "", "", "not-an-email", httptest.NewRequest(http.MethodPost, "/", nil), config.DefaultTimeoutConfig())
	if err != ErrInvalidEmail {
		t.Fatalf("err = %v, want ErrInvalidEmail", err)
	}
}

func TestMagicLinkService_RequestMagicLink_Success(t *testing.T) {
	setupMagicLinkCrypto(t)
	svc := NewMagicLinkService()
	redisStore, mr := setupMagicLinkRedis(t)

	req := httptest.NewRequest(http.MethodPost, "https://example.com/auth/request", nil)
	req.Host = "example.com"
	err := svc.RequestMagicLink(redisStore, nil, "", "", "user@example.com", req, config.DefaultTimeoutConfig())
	if err != nil {
		t.Fatalf("RequestMagicLink: %v", err)
	}

	if len(mr.Keys()) == 0 {
		t.Fatal("expected redis keys after magic link request")
	}
}

func TestGenerateMagicLinkToken(t *testing.T) {
	token, hashed, err := generateMagicLinkToken()
	if err != nil || token == "" || hashed == "" {
		t.Fatalf("generateMagicLinkToken = (%q, %q, %v)", token, hashed, err)
	}
	if HashToken(token) != hashed {
		t.Fatal("hashed token mismatch")
	}
}

func TestCheckMagicLinkRateLimit_Denied(t *testing.T) {
	redisStore, _ := setupMagicLinkRedis(t)
	ctx := context.Background()
	email := "limited@example.com"
	for i := 0; i < 6; i++ {
		_, _ = redisStore.CheckRateLimit(ctx, "ml:"+email, 5, config.MagicLinkTTL)
	}
	err := checkMagicLinkRateLimit(ctx, redisStore, email)
	if err != ErrTooManyRequests {
		t.Fatalf("err = %v, want ErrTooManyRequests", err)
	}
}

func TestCheckMagicLinkRateLimit_RedisError(t *testing.T) {
	redisStore, mr := setupMagicLinkRedis(t)
	mr.SetError("redis unavailable")
	err := checkMagicLinkRateLimit(context.Background(), redisStore, "fail@example.com")
	if err == nil {
		t.Fatal("expected rate limit check error")
	}
}

func TestRequestMagicLink_RateLimitRedisError(t *testing.T) {
	setupMagicLinkCrypto(t)
	svc := NewMagicLinkService()
	redisStore, mr := setupMagicLinkRedis(t)
	mr.SetError("redis unavailable")

	err := svc.RequestMagicLink(redisStore, nil, "", "", "user@example.com", httptest.NewRequest(http.MethodPost, "/", nil), config.DefaultTimeoutConfig())
	if err == nil {
		t.Fatal("expected error when rate limit redis fails")
	}
}

func TestStoreMagicLinkToken_RedisError(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, mr := setupMagicLinkRedis(t)
	mr.SetError("redis unavailable")

	err := storeMagicLinkToken(context.Background(), redisStore, "hashed-token", "user@example.com")
	if err == nil {
		t.Fatal("expected store magic token error")
	}
}

func TestEnqueueMagicLinkEmail_RedisError(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, mr := setupMagicLinkRedis(t)
	mr.SetError("redis unavailable")

	req := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	req.Host = "example.com"
	err := enqueueMagicLinkEmail(context.Background(), redisStore, req, "user@example.com", "raw-token", "hashed-token")
	if err == nil {
		t.Fatal("expected enqueue email error")
	}
}

func TestRequestMagicLink_TooManyRequests(t *testing.T) {
	setupMagicLinkCrypto(t)
	svc := NewMagicLinkService()
	redisStore, _ := setupMagicLinkRedis(t)
	ctx := context.Background()
	email := "limited-req@example.com"
	for i := 0; i < 6; i++ {
		_, _ = redisStore.CheckRateLimit(ctx, "ml:"+email, 5, config.MagicLinkTTL)
	}

	err := svc.RequestMagicLink(redisStore, nil, "", "", email, httptest.NewRequest(http.MethodPost, "/", nil), config.DefaultTimeoutConfig())
	if err != ErrTooManyRequests {
		t.Fatalf("err = %v, want ErrTooManyRequests", err)
	}
}

func TestValidateMagicToken_NotFound(t *testing.T) {
	redisStore, _ := setupMagicLinkRedis(t)
	_, err := validateMagicToken(context.Background(), redisStore, "missing-token")
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestPrepareQuickPlayNickname(t *testing.T) {
	if got := prepareQuickPlayNickname("  Alice  "); got != "Alice" {
		t.Fatalf("got %q", got)
	}
	if got := prepareQuickPlayNickname(""); got == "" {
		t.Fatal("empty nickname should generate random name")
	}
	longName := "abcdefghijklmnopqrstuvwxyz"
	if got := prepareQuickPlayNickname(longName); len([]rune(got)) > config.MaxNicknameLen {
		t.Fatalf("nickname should truncate to %d runes, got %q", config.MaxNicknameLen, got)
	}
}

func TestIssueQuickPlayCredentials(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)

	cookie, resp, err := issueQuickPlayCredentials(context.Background(), jwtMgr, refreshMgr, "user-1", "Player", req)
	if err != nil {
		t.Fatalf("issueQuickPlayCredentials: %v", err)
	}
	if cookie == nil || resp == nil || resp.RefreshToken == "" {
		t.Fatalf("cookie=%+v resp=%+v", cookie, resp)
	}
}

func TestIssueQuickPlayCredentials_RefreshError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	mr.SetError("redis unavailable")

	jwtMgr := NewJWTManager("test-secret-key-padded-to-32-bytes!!")
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)

	_, _, err = issueQuickPlayCredentials(context.Background(), jwtMgr, refreshMgr, "user-1", "Player", req)
	if err == nil {
		t.Fatal("expected refresh token generation error")
	}
}
