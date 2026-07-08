package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"strings"
)

func newQuickPlayPostgresStore(t *testing.T) (*store.UserRepository, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return store.NewUserRepository(mock), mock
}

func TestQuickPlay_NewUser(t *testing.T) {
	db, mock := newQuickPlayPostgresStore(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs("user", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", nil)
	cookie, resp, err := QuickPlay(db, jwtMgr, refreshMgr, nil, "Alice", req)
	if err != nil {
		t.Fatalf("QuickPlay: %v", err)
	}
	if cookie == nil || resp == nil || resp.Nickname == "" {
		t.Fatalf("cookie=%+v resp=%+v", cookie, resp)
	}
}

func TestQuickPlay_ExistingCookie(t *testing.T) {
	db, mock := newQuickPlayPostgresStore(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	token, _ := jwtMgr.SignToken("existing-user", "Existing")

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs("existing-user").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
			AddRow("existing-user", "existing-user@quickplay", "Existing", 0, int64(1), nil))

	req := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	cookie, resp, err := QuickPlay(db, jwtMgr, refreshMgr, nil, "", req)
	if err != nil {
		t.Fatalf("QuickPlay existing user: %v", err)
	}
	if resp.UserID != "existing-user" {
		t.Fatalf("UserID = %q", resp.UserID)
	}
	if cookie == nil {
		t.Fatal("expected cookie")
	}
}

func TestQuickPlay_CreateUserDuplicateContinues(t *testing.T) {
	db, mock := newQuickPlayPostgresStore(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(&pgconn.PgError{Code: "23505"})
	mock.ExpectRollback()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", nil)
	_, _, err = QuickPlay(db, jwtMgr, refreshMgr, nil, "Bob", req)
	if err != nil {
		t.Fatalf("QuickPlay duplicate should continue: %v", err)
	}
}

func TestQuickPlay_ExistingCookieLookupError(t *testing.T) {
	db, mock := newQuickPlayPostgresStore(t)
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	token, _ := jwtMgr.SignToken("existing-user", "Existing")

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs("existing-user").
		WillReturnError(errors.New("db down"))

	req := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", nil)
	req.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	_, _, err := QuickPlay(db, jwtMgr, nil, nil, "", req)
	if err == nil {
		t.Fatal("expected lookup existing user error")
	}
}

func TestQuickPlay_CreateUserError(t *testing.T) {
	db, mock := newQuickPlayPostgresStore(t)
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	mock.ExpectBegin().WillReturnError(errors.New("db down"))

	req := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", nil)
	_, _, err := QuickPlay(db, jwtMgr, nil, nil, "Bob", req)
	if err == nil {
		t.Fatal("expected error when create user fails")
	}
}

func TestRefreshSession_Success(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()

	oldRefresh, err := refreshMgr.Generate(ctx, "user-refresh")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	dataStore := &mockUserDataStore{
		user: &domain.User{ID: "user-refresh", Nickname: "Refresher"},
	}

	result, err := RefreshSession(ctx, refreshMgr, jwtMgr, dataStore, oldRefresh)
	if err != nil {
		t.Fatalf("RefreshSession: %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRefreshSession_InvalidToken(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	_, err = RefreshSession(context.Background(), refreshMgr, jwtMgr, &mockUserDataStore{}, "bad-token")
	if err == nil {
		t.Fatal("expected error for invalid refresh token")
	}
}

func TestRefreshSession_GetUserError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()

	token, _ := refreshMgr.Generate(ctx, "ghost-user")
	_, err = RefreshSession(ctx, refreshMgr, jwtMgr, &mockUserDataStore{userErr: errors.New("db down")}, token)
	if err == nil {
		t.Fatal("expected error when user lookup fails")
	}
}

func TestRefreshSession_UserNotFound(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()

	token, _ := refreshMgr.Generate(ctx, "ghost-user")
	_, err = RefreshSession(ctx, refreshMgr, jwtMgr, &mockUserDataStore{user: nil}, token)
	if err == nil {
		t.Fatal("expected error when user not found")
	}
}

func TestRefreshSession_GenerateError(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()

	oldRefresh, err := refreshMgr.Generate(ctx, "user-refresh-gen-err")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	dataStore := &mockUserDataStore{
		user: &domain.User{ID: "user-refresh-gen-err", Nickname: "Refresher"},
	}

	orig := randRead
	n := 0
	defer SetRandReadHook(func(b []byte) (int, error) {
		n++
		if n == 1 {
			return orig(b)
		}
		return 0, errors.New("rand failed")
	})()

	_, err = RefreshSession(ctx, refreshMgr, jwtMgr, dataStore, oldRefresh)
	if err == nil || !strings.Contains(err.Error(), "generate refresh token") {
		t.Fatalf("expected generate refresh token error, got: %v", err)
	}
}

func TestRefreshSession_SignTokenError(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	refreshMgr := NewRefreshTokenManager(rdb)
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)

	ctx := context.Background()
	userID := "user-sign-err"
	oldRefresh, err := refreshMgr.Generate(ctx, userID)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	dataStore := &mockUserDataStore{
		user: &domain.User{ID: userID, Nickname: "Signer"},
	}

	defer SetRandReadHook(func([]byte) (int, error) { return 0, errors.New("rand failed") })()

	_, err = RefreshSession(ctx, refreshMgr, jwtMgr, dataStore, oldRefresh)
	if err == nil || !strings.Contains(err.Error(), "sign token") {
		t.Fatalf("expected sign token error, got: %v", err)
	}
}

func TestRefreshSession_RevokesOldToken(t *testing.T) {
	mr := miniredis.RunT(t)
	refreshMgr := NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	jwtMgr := NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	ctx := context.Background()

	oldRefresh, err := refreshMgr.Generate(ctx, "user-refresh")
	if err != nil {
		t.Fatal(err)
	}

	_, err = RefreshSession(ctx, refreshMgr, jwtMgr,
		&mockUserDataStore{user: &domain.User{ID: "user-refresh", Nickname: "R"}}, oldRefresh)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := refreshMgr.Validate(ctx, oldRefresh); err == nil {
		t.Fatal("old refresh token should be revoked")
	}
}
