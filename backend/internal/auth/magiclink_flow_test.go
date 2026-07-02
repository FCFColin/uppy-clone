package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func setupMagicLinkCrypto(t *testing.T) {
	t.Helper()
	if err := crypto.Init(testsecrets.TestEncryptionKeyHex); err != nil {
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

	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
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

	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)

	_, _, err = issueQuickPlayCredentials(context.Background(), jwtMgr, refreshMgr, "user-1", "Player", req)
	if err == nil {
		t.Fatal("expected refresh token generation error")
	}
}

func TestIssueQuickPlayCredentials_SignTokenError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)

	defer SetRandReadHook(func([]byte) (int, error) { return 0, errors.New("rand failed") })()

	_, _, err = issueQuickPlayCredentials(context.Background(), jwtMgr, refreshMgr, "user-1", "Player", req)
	if err == nil {
		t.Fatal("expected sign token error")
	}
}

func TestStoreMagicLinkToken_EncryptNotInitialized(t *testing.T) {
	crypto.ResetKeyForTest()
	t.Cleanup(func() { _ = crypto.Init(testsecrets.TestEncryptionKeyHex) })

	redisStore, _ := setupMagicLinkRedis(t)
	err := storeMagicLinkToken(context.Background(), redisStore, "hashed-token", "user@example.com")
	if err == nil {
		t.Fatal("expected encrypt error when crypto not initialized")
	}
}

func TestRequestMagicLink_GenerateTokenError(t *testing.T) {
	setupMagicLinkCrypto(t)
	defer SetRandReadHook(func([]byte) (int, error) { return 0, errors.New("rand failed") })()

	svc := NewMagicLinkService()
	redisStore, _ := setupMagicLinkRedis(t)
	err := svc.RequestMagicLink(redisStore, nil, "", "", "user@example.com", httptest.NewRequest(http.MethodPost, "/", nil), config.DefaultTimeoutConfig())
	if err == nil {
		t.Fatal("expected generate token error")
	}
}

func TestGenerateMagicLinkToken_RandFailure(t *testing.T) {
	defer SetRandReadHook(func([]byte) (int, error) { return 0, errors.New("rand failed") })()

	if _, _, err := generateMagicLinkToken(); err == nil {
		t.Fatal("expected generateMagicLinkToken error")
	}
}

func TestPrepareQuickPlayNickname_SanitizeToEmpty(t *testing.T) {
	got := prepareQuickPlayNickname("<script>")
	if got == "" {
		t.Fatal("expected generated nickname after sanitize-to-empty")
	}
}

func TestPrepareQuickPlayNickname_Truncate(t *testing.T) {
	long := strings.Repeat("A", config.MaxNicknameLen+5)
	got := prepareQuickPlayNickname(long)
	if len([]rune(got)) != config.MaxNicknameLen {
		t.Fatalf("nickname length = %d, want %d", len([]rune(got)), config.MaxNicknameLen)
	}
}

func TestStoreMagicLinkToken_MarshalError(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	prev := magiclinkJSONMarshal
	magiclinkJSONMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal failed") }
	t.Cleanup(func() { magiclinkJSONMarshal = prev })

	err := storeMagicLinkToken(context.Background(), redisStore, "hashed", "user@example.com")
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestEnqueueMagicLinkEmail_MarshalError(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	prev := magiclinkJSONMarshal
	magiclinkJSONMarshal = func(v any) ([]byte, error) {
		if _, ok := v.(map[string]interface{}); ok {
			return nil, errors.New("marshal failed")
		}
		return json.Marshal(v)
	}
	t.Cleanup(func() { magiclinkJSONMarshal = prev })

	err := enqueueMagicLinkEmail(context.Background(), redisStore, httptest.NewRequest(http.MethodPost, "https://example.com/", nil),
		"user@example.com", "raw", "hashed")
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

type failRedisCmdHook struct {
	failMagicSet  bool
	failEmailPush bool
}

func (h failRedisCmdHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h failRedisCmdHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.failMagicSet && cmd.Name() == "set" {
			key := fmt.Sprint(cmd.Args()[1])
			if strings.HasPrefix(key, "magic:") {
				return errors.New("store magic token failed")
			}
		}
		if h.failEmailPush && cmd.Name() == "xadd" {
			return errors.New("enqueue email failed")
		}
		return next(ctx, cmd)
	}
}

func (h failRedisCmdHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestRequestMagicLink_StoreTokenError(t *testing.T) {
	setupMagicLinkCrypto(t)
	svc := NewMagicLinkService()
	redisStore, _ := setupMagicLinkRedis(t)
	redisStore.Client().AddHook(failRedisCmdHook{failMagicSet: true})

	err := svc.RequestMagicLink(redisStore, nil, "", "", "store-fail@example.com", httptest.NewRequest(http.MethodPost, "/", nil), config.DefaultTimeoutConfig())
	if err == nil {
		t.Fatal("expected store token error")
	}
}

func TestRequestMagicLink_EnqueueEmailError(t *testing.T) {
	setupMagicLinkCrypto(t)
	svc := NewMagicLinkService()
	redisStore, _ := setupMagicLinkRedis(t)
	redisStore.Client().AddHook(failRedisCmdHook{failEmailPush: true})

	err := svc.RequestMagicLink(redisStore, nil, "", "", "enqueue-fail@example.com", httptest.NewRequest(http.MethodPost, "/", nil), config.DefaultTimeoutConfig())
	if err == nil {
		t.Fatal("expected enqueue email error")
	}
}

func TestValidateMagicToken_DecryptFallback(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	ctx := context.Background()

	token, hashed, err := generateMagicLinkToken()
	if err != nil {
		t.Fatalf("generateMagicLinkToken: %v", err)
	}
	data, _ := json.Marshal(magicTokenData{
		Email:     "plaintext@example.com",
		CreatedAt: time.Now().UnixMilli(),
	})
	if err := redisStore.StoreMagicToken(ctx, hashed, data, config.MagicLinkTTL); err != nil {
		t.Fatalf("StoreMagicToken: %v", err)
	}
	email, err := validateMagicToken(ctx, redisStore, token)
	if err != nil || email != "plaintext@example.com" {
		t.Fatalf("validateMagicToken = %q, %v", email, err)
	}
}
