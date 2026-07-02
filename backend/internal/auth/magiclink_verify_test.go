package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func TestVerifyMagicLink_ExistingUser(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	ctx := context.Background()

	token, hashed, err := generateMagicLinkToken()
	if err != nil {
		t.Fatalf("generateMagicLinkToken: %v", err)
	}
	if err := storeMagicLinkToken(ctx, redisStore, hashed, "verify@example.com"); err != nil {
		t.Fatalf("storeMagicLinkToken: %v", err)
	}

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	db := store.NewPostgresStoreWithPool(mock)
	lastLogin := int64(1)
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "verify@example.com").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
			AddRow("user-verify", "verify@example.com", "verify", 0, int64(100), &lastLogin))
	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-verify").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
	req := httptest.NewRequest("GET", "https://example.com/", nil)

	cookie, resp, err := VerifyMagicLink(redisStore, db, jwtMgr, refreshMgr, token, req)
	if err != nil {
		t.Fatalf("VerifyMagicLink: %v", err)
	}
	if cookie == nil || resp == nil || resp.UserID != "user-verify" {
		t.Fatalf("cookie=%+v resp=%+v", cookie, resp)
	}
}

func TestFindOrCreateUserByEmail_CreatesUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	db := store.NewPostgresStoreWithPool(mock)
	ctx := context.Background()

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "new@example.com").
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "new", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs("user", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	user, err := findOrCreateUserByEmail(ctx, db, "new@example.com")
	if err != nil {
		t.Fatalf("findOrCreateUserByEmail: %v", err)
	}
	if user == nil || user.Email != "new@example.com" || user.Nickname != "new" {
		t.Fatalf("user = %+v", user)
	}
}

func TestValidateMagicToken_Expired(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	ctx := context.Background()

	token, hashed, _ := generateMagicLinkToken()
	encrypted, _ := crypto.Encrypt("old@example.com")
	data, _ := json.Marshal(magicTokenData{
		Email:     encrypted,
		CreatedAt: time.Now().Add(-2 * config.MagicLinkTTL).UnixMilli(),
	})
	_ = redisStore.StoreMagicToken(ctx, hashed, data, config.MagicLinkTTL)

	_, err := validateMagicToken(ctx, redisStore, token)
	if err == nil {
		t.Fatal("expected expired token error")
	}
}

func TestIssueMagicLinkSession(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	user := &domain.User{ID: "user-1", Nickname: "Magic", Email: "magic@example.com"}

	cookie, resp, err := issueMagicLinkSession(context.Background(), db, jwtMgr, refreshMgr, user, httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("issueMagicLinkSession: %v", err)
	}
	if cookie == nil || resp.RefreshToken == "" {
		t.Fatalf("cookie=%+v resp=%+v", cookie, resp)
	}
}

func TestIssueMagicLinkSession_LastLoginErrorIgnored(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-1").
		WillReturnError(errors.New("update failed"))

	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	user := &domain.User{ID: "user-1", Nickname: "Magic", Email: "magic@example.com"}

	_, resp, err := issueMagicLinkSession(context.Background(), db, jwtMgr, refreshMgr, user, httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("issueMagicLinkSession should continue when last login update fails: %v", err)
	}
	if resp == nil || resp.RefreshToken == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestIssueMagicLinkSession_RefreshError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	mr.SetError("redis unavailable")

	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	user := &domain.User{ID: "user-1", Nickname: "Magic", Email: "magic@example.com"}

	_, _, err = issueMagicLinkSession(context.Background(), db, jwtMgr, refreshMgr, user, httptest.NewRequest("GET", "/", nil))
	if err == nil {
		t.Fatal("expected refresh token generation error")
	}
}

func TestValidateMagicToken_InvalidJSON(t *testing.T) {
	redisStore, _ := setupMagicLinkRedis(t)
	ctx := context.Background()

	token, hashed, _ := generateMagicLinkToken()
	if err := redisStore.StoreMagicToken(ctx, hashed, []byte("not-json"), config.MagicLinkTTL); err != nil {
		t.Fatalf("StoreMagicToken: %v", err)
	}

	_, err := validateMagicToken(ctx, redisStore, token)
	if err == nil {
		t.Fatal("expected invalid token data error")
	}
}

func TestValidateMagicToken_LegacyPlaintextEmail(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	ctx := context.Background()

	token, hashed, _ := generateMagicLinkToken()
	data, _ := json.Marshal(magicTokenData{
		Email:     "legacy@example.com",
		CreatedAt: time.Now().UnixMilli(),
	})
	if err := redisStore.StoreMagicToken(ctx, hashed, data, config.MagicLinkTTL); err != nil {
		t.Fatalf("StoreMagicToken: %v", err)
	}

	email, err := validateMagicToken(ctx, redisStore, token)
	if err != nil {
		t.Fatalf("validateMagicToken: %v", err)
	}
	if email != "legacy@example.com" {
		t.Fatalf("email = %q, want legacy@example.com", email)
	}
}

func TestValidateMagicToken_LookupError(t *testing.T) {
	redisStore, mr := setupMagicLinkRedis(t)
	mr.SetError("redis unavailable")

	_, err := validateMagicToken(context.Background(), redisStore, "any-token")
	if err == nil {
		t.Fatal("expected lookup token error")
	}
}

func TestValidateMagicToken_DeleteError(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	redisStore.Client().AddHook(delFailHook{})
	ctx := context.Background()

	token, hashed, _ := generateMagicLinkToken()
	if err := storeMagicLinkToken(ctx, redisStore, hashed, "delete-err@example.com"); err != nil {
		t.Fatalf("storeMagicLinkToken: %v", err)
	}

	_, err := validateMagicToken(ctx, redisStore, token)
	if err == nil {
		t.Fatal("expected delete token error")
	}
}

type delFailHook struct{}

func (delFailHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (delFailHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "del" {
			return errors.New("del failed")
		}
		return next(ctx, cmd)
	}
}

func (delFailHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestValidateMagicToken_Success(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	ctx := context.Background()

	token, hashed, _ := generateMagicLinkToken()
	if err := storeMagicLinkToken(ctx, redisStore, hashed, "valid@example.com"); err != nil {
		t.Fatalf("storeMagicLinkToken: %v", err)
	}

	email, err := validateMagicToken(ctx, redisStore, token)
	if err != nil {
		t.Fatalf("validateMagicToken: %v", err)
	}
	if email != "valid@example.com" {
		t.Fatalf("email = %q", email)
	}
}

func TestFindOrCreateUserByEmail_LookupError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "fail@example.com").
		WillReturnError(errors.New("db down"))

	_, err = findOrCreateUserByEmail(context.Background(), db, "fail@example.com")
	if err == nil {
		t.Fatal("expected lookup user error")
	}
}

func TestFindOrCreateUserByEmail_CreateError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "create-fail@example.com").
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "create-fail", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	_, err = findOrCreateUserByEmail(context.Background(), db, "create-fail@example.com")
	if err == nil {
		t.Fatal("expected create user error")
	}
}

func TestVerifyMagicLink_InvalidToken(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
	req := httptest.NewRequest("GET", "/", nil)

	_, _, err := VerifyMagicLink(redisStore, nil, jwtMgr, nil, "bad-token", req)
	if err == nil {
		t.Fatal("expected verify magic link error")
	}
}

func TestVerifyMagicLink_UserLookupError(t *testing.T) {
	setupMagicLinkCrypto(t)
	redisStore, _ := setupMagicLinkRedis(t)
	ctx := context.Background()

	token, hashed, err := generateMagicLinkToken()
	if err != nil {
		t.Fatalf("generateMagicLinkToken: %v", err)
	}
	if err := storeMagicLinkToken(ctx, redisStore, hashed, "lookup-fail@example.com"); err != nil {
		t.Fatalf("storeMagicLinkToken: %v", err)
	}

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "lookup-fail@example.com").
		WillReturnError(errors.New("db down"))

	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
	req := httptest.NewRequest("GET", "/", nil)

	_, _, err = VerifyMagicLink(redisStore, db, jwtMgr, nil, token, req)
	if err == nil {
		t.Fatal("expected user lookup error")
	}
}

func TestIssueMagicLinkSession_SignTokenError(t *testing.T) {
	mr := miniredis.RunT(t)
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	db := store.NewPostgresStoreWithPool(mock)
	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-1").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	jwtMgr := NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	defer SetRandReadHook(func([]byte) (int, error) { return 0, errors.New("rand failed") })()

	user := &domain.User{ID: "user-1", Nickname: "Magic"}
	_, _, err = issueMagicLinkSession(context.Background(), db, jwtMgr, refreshMgr, user,
		httptest.NewRequest("GET", "/", nil))
	if err == nil {
		t.Fatal("expected sign token error")
	}
}
