package handler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

// newTestAuthHandler creates an AuthHandler with nil DB/Redis for testing
// only the HTTP-layer logic (request parsing, error responses).
func newTestAuthHandler() *AuthHandler {
	return &AuthHandler{
		db:     nil,
		redis:  nil,
		config: &Config{ResendAPIKey: "test", EmailFrom: "test@test.com"},
		auth:   stubAuthService{},
	}
}

// stubAuthService implements AuthService for HTTP-layer tests.
type stubAuthService struct{}

func (stubAuthService) RequestMagicLink(_ context.Context, _ string, _ *http.Request) error {
	return errors.New("not implemented")
}
func (stubAuthService) RefreshSession(_ context.Context, _ string, _ *http.Request) (string, string, int, error) {
	return "", "", 0, errors.New("not implemented")
}
func (stubAuthService) VerifyMagicLink(_ context.Context, _ string, _ *http.Request) (string, string, string, error) {
	return "", "", "", errors.New("not implemented")
}
func (stubAuthService) QuickPlay(_ context.Context, _ string, _ *http.Request) (string, string, string, error) {
	return "", "", "", errors.New("not implemented")
}
func (stubAuthService) ExportUserData(_ context.Context, _ string) (*domain.User, []domain.GameResult, error) {
	return nil, nil, errors.New("not implemented")
}
func (stubAuthService) DeleteUserData(_ context.Context, _ string, _ *http.Request) error { return nil }
func (stubAuthService) RevokeRefreshToken(_ context.Context, _ string) error                { return nil }
func (stubAuthService) RevokeAllTokens(_ context.Context, _ *http.Request) error              { return nil }
func (stubAuthService) AuthenticatedUserFromRequest(r *http.Request) (string, string, bool) {
	return getAuthenticatedUser(r)
}
func (stubAuthService) GetJTI(r *http.Request) string           { return getJTI(r) }
func (stubAuthService) IsJWTRevoked(_ context.Context, _ string) (bool, error) { return false, nil }

// testUserStore combines auth.UserDB and auth.UserDataStore for the test adapter.
type testUserStore interface {
	auth.UserDB
	auth.UserDataStore
}

// testAuthService wraps real auth functions for integration tests.
type testAuthService struct {
	jwtMgr     *auth.JWTManager
	refreshMgr *auth.RefreshTokenManager
	tokens     auth.TokenStore
	users      testUserStore
	magicLink  *auth.MagicLinkService
	resendKey  string
	emailFrom  string
	timeouts   config.TimeoutConfig
}

func newMockAuthSvc(jwtMgr *auth.JWTManager, refreshMgr *auth.RefreshTokenManager, tokens auth.TokenStore, users testUserStore, resendKey, emailFrom string, timeouts config.TimeoutConfig) *testAuthService {
	svc := &testAuthService{
		jwtMgr:     jwtMgr,
		refreshMgr: refreshMgr,
		tokens:     tokens,
		users:      users,
		resendKey:  resendKey,
		emailFrom:  emailFrom,
		timeouts:   timeouts,
	}
	if tokens != nil {
		svc.magicLink = auth.NewMagicLinkService()
	}
	return svc
}

func (s *testAuthService) RequestMagicLink(ctx context.Context, email string, r *http.Request) error {
	err := s.magicLink.RequestMagicLink(s.tokens, s.users, s.resendKey, s.emailFrom, email, r, s.timeouts)
	if errors.Is(err, auth.ErrTooManyRequests) {
		return ErrTooManyRequests
	}
	if errors.Is(err, auth.ErrInvalidEmail) {
		return ErrInvalidEmail
	}
	return err
}
func (s *testAuthService) RefreshSession(ctx context.Context, refreshToken string, r *http.Request) (string, string, int, error) {
	result, err := auth.RefreshSession(ctx, s.refreshMgr, s.jwtMgr, s.users, refreshToken)
	if err != nil {
		return "", "", 0, err
	}
	return result.AccessToken, result.RefreshToken, config.CookieMaxAge, nil
}
func (s *testAuthService) VerifyMagicLink(ctx context.Context, token string, r *http.Request) (string, string, string, error) {
	cookie, resp, err := auth.VerifyMagicLink(s.tokens, s.users, s.jwtMgr, s.refreshMgr, token, r)
	if err != nil {
		return "", "", "", err
	}
	access := ""
	if cookie != nil {
		access = cookie.Value
	}
	return resp.UserID, access, resp.RefreshToken, nil
}
func (s *testAuthService) QuickPlay(ctx context.Context, nickname string, r *http.Request) (string, string, string, error) {
	cookie, resp, err := auth.QuickPlay(s.users, s.jwtMgr, s.refreshMgr, s.tokens, nickname, r)
	if err != nil {
		return "", "", "", err
	}
	access := ""
	if cookie != nil {
		access = cookie.Value
	}
	return resp.UserID, access, resp.RefreshToken, nil
}
func (s *testAuthService) ExportUserData(ctx context.Context, userID string) (*domain.User, []domain.GameResult, error) {
	if s.users == nil {
		return nil, nil, fmt.Errorf("store not available")
	}
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, nil
	}
	results, err := s.users.GetGameResultsByUserID(ctx, userID)
	if err != nil {
		results = nil
	}
	return user, results, nil
}
func (s *testAuthService) DeleteUserData(ctx context.Context, userID string, r *http.Request) error {
	return auth.DeleteUserData(ctx, s.jwtMgr, s.refreshMgr, s.tokens, s.users, userID, r)
}
func (s *testAuthService) RevokeRefreshToken(ctx context.Context, token string) error {
	return s.refreshMgr.Revoke(ctx, token)
}
func (s *testAuthService) RevokeAllTokens(ctx context.Context, r *http.Request) error {
	auth.RevokeAllTokens(ctx, s.jwtMgr, s.refreshMgr, s.tokens, r)
	return nil
}
func (s *testAuthService) AuthenticatedUserFromRequest(r *http.Request) (string, string, bool) {
	return auth.AuthenticatedUserFromRequestWithRevocation(r, s.jwtMgr, s.tokens)
}
func (s *testAuthService) GetJTI(r *http.Request) string                { return auth.GetJTI(r) }
func (s *testAuthService) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	return s.tokens.IsJWTRevoked(ctx, jti)
}

func TestRequestMagicLink_MissingEmail(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler()

	tests := []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "empty email", body: `{"email":""}`},
		{name: "invalid json", body: `{invalid}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/request", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")

			h.RequestMagicLink(w, r)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}

func TestVerifyMagicLink_MissingToken(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)

	h.VerifyMagicLink(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCheckAuth_Unauthenticated(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)

	h.CheckAuth(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestCheckAuth_AuthenticatedViaCookieWithoutMiddleware(t *testing.T) {
	t.Parallel()

	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	token, err := jwtMgr.SignToken("user-456", "CookiePlayer")
	if err != nil {
		t.Fatalf("SignToken() error = %v", err)
	}

	h := &AuthHandler{
		db:     nil,
		redis:  nil,
		config: &Config{},
		auth:   newMockAuthSvc(jwtMgr, nil, nil, nil, "", "", config.DefaultTimeoutConfig()),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
	r.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	h.CheckAuth(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestCheckAuth_RevokedSession(t *testing.T) {
	t.Parallel()

	redisStore := testutil.SetupMiniredisStore(t)
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	token, err := jwtMgr.SignToken("user-revoked", "Revoked")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	_, _, jti, err := jwtMgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if err := redisStore.RevokeJWT(context.Background(), jti, time.Minute); err != nil {
		t.Fatalf("RevokeJWT: %v", err)
	}

	h := &AuthHandler{
		redis:  redisStore,
		config: &Config{},
		auth:   newMockAuthSvc(jwtMgr, nil, redisStore, nil, "", "", config.DefaultTimeoutConfig()),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: token})
	h.CheckAuth(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestCheckAuth_Authenticated(t *testing.T) {
	t.Parallel()

	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	token, err := jwtMgr.SignToken("user-123", "TestPlayer")
	if err != nil {
		t.Fatalf("SignToken() error = %v", err)
	}

	h := &AuthHandler{
		db:     nil,
		redis:  nil,
		config: &Config{},
		auth:   newMockAuthSvc(jwtMgr, nil, nil, nil, "", "", config.DefaultTimeoutConfig()),
	}

	// Use the actual auth middleware to set context
	handler := auth.AuthMiddleware(jwtMgr, h.CheckAuth)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
	r.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	handler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["authenticated"] != true {
		t.Errorf("authenticated = %v, want true", body["authenticated"])
	}
	if body["userId"] != "user-123" {
		t.Errorf("userId = %v, want %q", body["userId"], "user-123")
	}
}

func TestRefreshToken_MissingBody(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)

	h.RefreshToken(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestRefreshToken_EmptyToken(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":""}`))
	r.Header.Set("Content-Type", "application/json")

	h.RefreshToken(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestRefreshToken_FromCookie(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	r.AddCookie(auth.BuildRefreshCookie("some-token", false))

	h.RefreshToken(w, r)

	// nil redis in test handler → service unavailable, but not bad request
	if w.Code == http.StatusBadRequest {
		t.Errorf("cookie refresh token should not require JSON body, got 400")
	}
}

// --- QuickPlay ---

func TestQuickPlay_ServiceError(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/quickplay", strings.NewReader(`{"nickname":"Test"}`))
	r.Header.Set("Content-Type", "application/json")

	h.QuickPlay(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("QuickPlay error: status = %d, want %d; body = %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}

func TestQuickPlay_MissingBody(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/quickplay", nil)

	h.QuickPlay(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("QuickPlay error: status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestLogout_ClearsCookies(t *testing.T) {
	t.Parallel()

	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	h := &AuthHandler{
		db:     nil,
		redis:  nil,
		config: &Config{},
		auth:   newMockAuthSvc(jwtMgr, nil, nil, nil, "", "", config.DefaultTimeoutConfig()),
	}

	// Test logout without refresh_token (avoids nil refreshMgr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")

	h.Logout(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	cookies := w.Result().Cookies()
	cookieNames := map[string]bool{}
	for _, c := range cookies {
		cookieNames[c.Name] = true
		if c.MaxAge >= 0 {
			t.Errorf("cookie %q should have MaxAge < 0, got %d", c.Name, c.MaxAge)
		}
	}
	if !cookieNames["quickplay"] {
		t.Error("expected quickplay cookie to be cleared")
	}
	if !cookieNames["session"] {
		t.Error("expected session cookie to be cleared")
	}
}

func TestQuickPlay_WithDB(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	redisStore := store.NewRedisStoreFromClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, redisStore, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, redisStore, authSvc, &Config{})

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO users").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs("user", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgconn.NewCommandTag("INSERT 1"))
	mock.ExpectCommit()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/quickplay", strings.NewReader(`{"nickname":"Alice"}`))
	h.QuickPlay(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestExportUserData_WithDB(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	authSvc := newMockAuthSvc(jwtMgr, nil, nil, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, nil, authSvc, &Config{})

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
			AddRow("user-1", "user-1@quickplay", "Nick", 0, int64(1), nil))
	mock.ExpectQuery("SELECT id, session_id, user_id, score_contribution, taps_count, created_at FROM game_results").
		WithArgs("user-1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "session_id", "user_id", "score_contribution", "taps_count", "created_at"}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/user/data", nil)
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-1", "Nick"))
	h.ExportUserData(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteUserData_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	redisStore := store.NewRedisStoreFromClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, redisStore, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, redisStore, authSvc, &Config{})

	mock.ExpectExec("UPDATE users SET email").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "user-del").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/user/data", nil)
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-del", "Nick"))
	h.DeleteUserData(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteUserData_DBError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	redisStore := testutil.SetupMiniredisStore(t)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, redisStore, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, redisStore, authSvc, &Config{})

	mock.ExpectExec("UPDATE users SET email").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "user-err").
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/user/data", nil)
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-err", "Nick"))
	h.DeleteUserData(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestRequestMagicLink_Success(t *testing.T) {
	if err := crypto.Init("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}
	redisStore := testutil.SetupMiniredisStore(t)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	authSvc := newMockAuthSvc(jwtMgr, nil, redisStore, nil, "re_test", "test@test.com", config.DefaultTimeoutConfig())
	h := NewAuthHandler(nil, redisStore, authSvc, &Config{ResendAPIKey: "re_test", EmailFrom: "test@test.com"})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "https://example.com/api/v1/auth/request", strings.NewReader(`{"email":"user@example.com"}`))
	r.Host = "example.com"
	r.Header.Set("Content-Type", "application/json")
	h.RequestMagicLink(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestCheckAuth_WithDB(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	token, err := jwtMgr.SignToken("user-db", "CookieNick")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	authSvc := newMockAuthSvc(jwtMgr, nil, nil, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, nil, authSvc, &Config{})

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs("user-db").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
			AddRow("user-db", "user@example.com", "DbNick", 0, int64(1), nil))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: token})
	h.CheckAuth(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["nickname"] != "DbNick" {
		t.Fatalf("nickname = %v, want DbNick", body["nickname"])
	}
	if body["email"] != "user@example.com" {
		t.Fatalf("email = %v", body["email"])
	}
	if degraded, _ := body["degraded"].(bool); degraded {
		t.Fatal("should not be degraded with successful DB lookup")
	}
}

func TestRequestMagicLink_TooManyRequests(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	redisStore := store.NewRedisStoreFromClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	authSvc := newMockAuthSvc(jwtMgr, nil, redisStore, nil, "re_test", "test@test.com", config.DefaultTimeoutConfig())
	h := NewAuthHandler(nil, redisStore, authSvc, &Config{ResendAPIKey: "re_test", EmailFrom: "test@test.com"})

	ctx := context.Background()
	email := strings.Repeat("a", 20) + "@example.com"
	for i := 0; i < 6; i++ {
		_, _ = redisStore.CheckRateLimit(ctx, "ml:"+email, 5, config.MagicLinkTTL)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/request", strings.NewReader(`{"email":"`+email+`"}`))
	r.Header.Set("Content-Type", "application/json")
	h.RequestMagicLink(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
}

func TestRequestMagicLink_InvalidEmail(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	authSvc := newMockAuthSvc(jwtMgr, nil, redisStore, nil, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(nil, redisStore, authSvc, &Config{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/request", strings.NewReader(`{"email":"bad-email"}`))
	r.Header.Set("Content-Type", "application/json")
	h.RequestMagicLink(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func TestVerifyMagicLinkToken_Success(t *testing.T) {
	if err := crypto.Init("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}
	redisStore := testutil.SetupMiniredisStore(t)
	ctx := context.Background()

	token := strings.Repeat("c", config.MagicLinkTokenLen)
	hashed := auth.HashToken(token)
	encEmail, err := crypto.EncryptEmailForStorage("verify-handler@example.com")
	if err != nil {
		t.Fatalf("EncryptEmailForStorage: %v", err)
	}
	tokenData, _ := json.Marshal(map[string]interface{}{
		"email": encEmail, "createdAt": time.Now().UnixMilli(),
	})
	if err := redisStore.StoreMagicToken(ctx, hashed, tokenData, config.MagicLinkTTL); err != nil {
		t.Fatalf("StoreMagicToken: %v", err)
	}

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	refreshMgr := auth.NewRefreshTokenManager(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, redisStore, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, redisStore, authSvc, &Config{})

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users").
		WithArgs(pgxmock.AnyArg(), "verify-handler@example.com").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
			AddRow("user-vh", "verify-handler@example.com", "Nick", 0, int64(1), nil))
	mock.ExpectExec("UPDATE users SET last_login").
		WithArgs("user-vh").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify?token="+token, nil)
	h.VerifyMagicLink(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestVerifyMagicLinkToken_InvalidToken(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	authSvc := newMockAuthSvc(jwtMgr, nil, redisStore, nil, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(nil, redisStore, authSvc, &Config{})

	token := strings.Repeat("b", config.MagicLinkTokenLen)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify?token="+token, nil)
	h.VerifyMagicLink(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestRefreshToken_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	redisStore := store.NewRedisStoreFromClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, redisStore, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, redisStore, authSvc, &Config{})

	ctx := context.Background()
	refreshToken, err := refreshMgr.Generate(ctx, "user-refresh")
	if err != nil {
		t.Fatalf("Generate refresh: %v", err)
	}

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs("user-refresh").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "nickname", "palette", "created_at", "last_login"}).
			AddRow("user-refresh", "user-refresh@quickplay", "Nick", 0, int64(1), nil))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":"`+refreshToken+`"}`))
	r.Header.Set("Content-Type", "application/json")
	h.RefreshToken(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestQuickPlay_ExistingUserLookupError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := auth.NewRefreshTokenManager(testutil.SetupMiniredisStore(t).Client())
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, nil, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, nil, authSvc, &Config{})

	token, err := jwtMgr.SignToken("user-qp-err", "Nick")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs("user-qp-err").
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/quickplay", nil)
	r.AddCookie(&http.Cookie{Name: "quickplay", Value: token})
	h.QuickPlay(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestExportUserData_NilDB(t *testing.T) {
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	authSvc := newMockAuthSvc(jwtMgr, nil, nil, nil, "", "", config.DefaultTimeoutConfig())
		h := NewAuthHandler(nil, nil, authSvc, &Config{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/user/data", nil)
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "user-1", "Nick"))
	h.ExportUserData(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestExportUserData_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	authSvc := newMockAuthSvc(jwtMgr, nil, nil, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, nil, authSvc, &Config{})

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs("missing-user").
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/user/data", nil)
	r = r.WithContext(auth.WithAuthenticatedUser(r.Context(), "missing-user", "Nick"))
	h.ExportUserData(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestCheckAuth_DBErrorDegraded(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	token, err := jwtMgr.SignToken("user-db-err", "Nick")
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	authSvc := newMockAuthSvc(jwtMgr, nil, nil, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, nil, authSvc, &Config{})

	mock.ExpectQuery("SELECT id, email, nickname, palette, created_at, last_login FROM users WHERE id").
		WithArgs("user-db-err").
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
	r.AddCookie(&http.Cookie{Name: "quickplay", Value: token})
	h.CheckAuth(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 degraded", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"degraded":true`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, redisStore, db, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(db, redisStore, authSvc, &Config{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":"invalid-token"}`))
	r.Header.Set("Content-Type", "application/json")
	h.RefreshToken(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestRequestMagicLink_InternalError(t *testing.T) {
	redisStore := testutil.SetupMiniredisStore(t)
	if err := redisStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	authSvc := newMockAuthSvc(jwtMgr, nil, redisStore, nil, "re_test", "test@test.com", config.DefaultTimeoutConfig())
	h := NewAuthHandler(nil, redisStore, authSvc, &Config{ResendAPIKey: "re_test", EmailFrom: "test@test.com"})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/request", strings.NewReader(`{"email":"user@example.com"}`))
	r.Header.Set("Content-Type", "application/json")
	h.RequestMagicLink(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestLogout_RevokesRefreshToken(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	redisStore := store.NewRedisStoreFromClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, redisStore, nil, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(nil, redisStore, authSvc, &Config{})

	ctx := context.Background()
	refreshToken, err := refreshMgr.Generate(ctx, "user-logout")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(`{"refresh_token":"`+refreshToken+`"}`))
	r.Header.Set("Content-Type", "application/json")
	r.TLS = &tls.ConnectionState{}
	h.Logout(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if _, err := refreshMgr.Validate(ctx, refreshToken); err == nil {
		t.Fatal("refresh token should be revoked")
	}
}

func TestRefreshToken_NilDB(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	redisStore := store.NewRedisStoreFromClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())
	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, redisStore, nil, "", "", config.DefaultTimeoutConfig())
	h := NewAuthHandler(nil, redisStore, authSvc, &Config{})

	ctx := context.Background()
	refreshToken, err := refreshMgr.Generate(ctx, "user-nodb")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	r.AddCookie(&http.Cookie{Name: auth.RefreshCookieName, Value: refreshToken})
	h.RefreshToken(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
