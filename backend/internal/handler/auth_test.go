package handler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	appMiddleware "github.com/uppy-clone/backend/internal/middleware"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testutil"
)

// newTestAuthHandler creates an AuthHandler with nil DB/Redis for testing
// only the HTTP-layer logic (request parsing, error responses).
func newTestAuthHandler() *AuthHandler {
	return &AuthHandler{
		db:     nil,
		redis:  nil,
		config: &Config{ResendAPIKey: "test", EmailFrom: "test@test.com"},
	}
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

func TestCheckAuth_NoMiddleware(t *testing.T) {
	t.Parallel()

	jwtMgr := newTestJWTManager()
	token := signTestToken(t, jwtMgr, "user-456", "CookiePlayer")
	h := NewAuthHandler(nil, nil, jwtMgr, nil, &Config{})

	tests := []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{"unauthenticated", nil, http.StatusUnauthorized},
		{"authenticated via cookie", &http.Cookie{Name: "quickplay", Value: token}, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
			if tt.cookie != nil {
				r.AddCookie(tt.cookie)
			}
			h.CheckAuth(w, r)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

func TestCheckAuth_RevokedSession(t *testing.T) {
	t.Parallel()

	h, redisStore, jwtMgr := newTestAuthHandlerWithRedis(t)
	token := signTestToken(t, jwtMgr, "user-revoked", "Revoked")
	_, _, jti, _, err := jwtMgr.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if err := redisStore.RevokeJWT(context.Background(), jti, time.Minute); err != nil {
		t.Fatalf("RevokeJWT: %v", err)
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

	jwtMgr := newTestJWTManager()
	token := signTestToken(t, jwtMgr, "user-123", "TestPlayer")
	h := NewAuthHandler(nil, nil, jwtMgr, nil, &Config{})

	// Use the actual auth middleware to set context
	handler := appMiddleware.AuthMiddleware(jwtMgr, h.CheckAuth)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
	r.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	handler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]interface{}
	testutil.DecodeJSONBody(t, w, &body)
	if body["authenticated"] != true {
		t.Errorf("authenticated = %v, want true", body["authenticated"])
	}
	if body["userId"] != "user-123" {
		t.Errorf("userId = %v, want %q", body["userId"], "user-123")
	}
}

func TestRefreshToken_BadRequest(t *testing.T) {
	t.Parallel()

	h := newTestAuthHandler()

	tests := []struct {
		name string
		body string
	}{
		{"missing body", ""},
		{"empty token", `{"refresh_token":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(tt.body))
			if tt.body != "" {
				r.Header.Set("Content-Type", "application/json")
			}

			h.RefreshToken(w, r)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
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

	w, r := newJSONRequest(http.MethodPost, "/api/v1/auth/quickplay", `{"nickname":"Test"}`)
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

	if w.Code != http.StatusBadRequest {
		t.Errorf("QuickPlay error: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLogout_ClearsCookies(t *testing.T) {
	t.Parallel()

	jwtMgr := newTestJWTManager()
	h := NewAuthHandler(nil, nil, jwtMgr, nil, &Config{})

	// Test logout without refresh_token (avoids nil refreshMgr)
	w, r := newJSONRequest(http.MethodPost, "/api/v1/auth/logout", `{}`)
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
	mock, db := newTestUserRepo(t)
	h, _, _, _ := newTestAuthHandlerWithRefreshMgr(t, db)

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
	h, mock, _ := newAuthHandlerWithDB(t)
	expectGetUserByID(mock, "user-1", "user-1@quickplay", "Nick")

	w := httptest.NewRecorder()
	r := withAuthUser(httptest.NewRequest(http.MethodGet, "/api/v1/user/data", nil), "user-1", "Nick")
	h.ExportUserData(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteUserData_Success(t *testing.T) {
	mock, db := newTestUserRepo(t)
	h, _, _, _ := newTestAuthHandlerWithRefreshMgr(t, db)

	// AnonymizeUser uses pool.Exec directly (non-transactional, via withRetry circuit breaker)
	mock.ExpectExec("UPDATE users SET email").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "user-del").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 1"))
	// AnonymizeUser also updates outbox_events
	mock.ExpectExec("UPDATE outbox_events SET payload").
		WithArgs("user-del").
		WillReturnResult(pgconn.NewCommandTag("UPDATE 0"))

	w := httptest.NewRecorder()
	r := withAuthUser(httptest.NewRequest(http.MethodDelete, "/api/v1/user/data", nil), "user-del", "Nick")
	h.DeleteUserData(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteUserData_DBError(t *testing.T) {
	mock, db := newTestUserRepo(t)
	h, _, _, _ := newTestAuthHandlerWithRefreshMgr(t, db)

	// AnonymizeUser uses pool.Exec directly (non-transactional, via withRetry circuit breaker)
	mock.ExpectExec("UPDATE users SET email").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "user-err").
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	r := withAuthUser(httptest.NewRequest(http.MethodDelete, "/api/v1/user/data", nil), "user-err", "Nick")
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
	jwtMgr := newTestJWTManager()
	h := NewAuthHandler(nil, redisStore, jwtMgr, nil, &Config{ResendAPIKey: "re_test", EmailFrom: "test@test.com"})

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
	t.Parallel()

	h, mock, jwtMgr := newAuthHandlerWithDB(t)
	token := signTestToken(t, jwtMgr, "user-db", "CookieNick")
	expectGetUserByID(mock, "user-db", "user@example.com", "DbNick")

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/check", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: token})
	h.CheckAuth(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	testutil.DecodeJSONBody(t, w, &body)
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
	_, rdb := testutil.NewTestMiniredis(t)
	redisStore := store.NewRedisStoreFromClient(rdb)
	jwtMgr := newTestJWTManager()
	h := NewAuthHandler(nil, redisStore, jwtMgr, nil, &Config{ResendAPIKey: "re_test", EmailFrom: "test@test.com"})

	ctx := context.Background()
	email := strings.Repeat("a", 20) + "@example.com"
	for i := 0; i < 6; i++ {
		_, _ = redisStore.CheckRateLimit(ctx, "ml:"+email, 5, config.MagicLinkTTL)
	}

	w, r := newJSONRequest(http.MethodPost, "/api/v1/auth/request", `{"email":"`+email+`"}`)
	h.RequestMagicLink(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
}

func TestRequestMagicLink_InvalidEmail(t *testing.T) {
	h, _, _ := newTestAuthHandlerWithRedis(t)

	w, r := newJSONRequest(http.MethodPost, "/api/v1/auth/request", `{"email":"bad-email"}`)
	h.RequestMagicLink(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func TestVerifyMagicLinkToken_Success(t *testing.T) {
	if err := crypto.Init("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}

	mock, db := newTestUserRepo(t)
	h, redisStore, _, _ := newTestAuthHandlerWithRefreshMgr(t, db)

	ctx := context.Background()
	token := strings.Repeat("c", config.MagicLinkTokenLen)
	hashed := auth.HashToken(token)
	encEmail, err := crypto.EncryptPIIForStorage("verify-handler@example.com")
	if err != nil {
		t.Fatalf("EncryptPIIForStorage: %v", err)
	}
	tokenData, _ := json.Marshal(map[string]interface{}{
		"email": encEmail, "createdAt": time.Now().UnixMilli(),
	})
	if err := redisStore.StoreMagicToken(ctx, hashed, tokenData, config.MagicLinkTTL); err != nil {
		t.Fatalf("StoreMagicToken: %v", err)
	}

	// VerifyMagicLink looks up the user by email (not by ID), so the SQL
	// pattern and arg count differ from expectGetUserByID.
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
	h, _, _ := newTestAuthHandlerWithRedis(t)

	token := strings.Repeat("b", config.MagicLinkTokenLen)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify?token="+token, nil)
	h.VerifyMagicLink(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestRefreshToken_Success(t *testing.T) {
	mock, db := newTestUserRepo(t)
	h, _, _, refreshMgr := newTestAuthHandlerWithRefreshMgr(t, db)

	ctx := context.Background()
	refreshToken, err := refreshMgr.Generate(ctx, "user-refresh")
	if err != nil {
		t.Fatalf("Generate refresh: %v", err)
	}

	expectGetUserByID(mock, "user-refresh", "user-refresh@quickplay", "Nick")

	w, r := newJSONRequest(http.MethodPost, "/api/v1/auth/refresh", `{"refresh_token":"`+refreshToken+`"}`)
	h.RefreshToken(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestQuickPlay_ExistingUserLookupError(t *testing.T) {
	mock, db := newTestUserRepo(t)
	refreshMgr := auth.NewRefreshTokenManager(testutil.SetupMiniredisStore(t).Client())
	jwtMgr := newTestJWTManager()
	h := NewAuthHandler(db, nil, jwtMgr, refreshMgr, &Config{})

	token := signTestToken(t, jwtMgr, "user-qp-err", "Nick")
	expectGetUserByIDError(mock, "user-qp-err", context.Canceled)

	w, r := newJSONRequest(http.MethodPost, "/api/v1/auth/quickplay", `{"nickname":"Test"}`)
	r.AddCookie(&http.Cookie{Name: "quickplay", Value: token})
	h.QuickPlay(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestExportUserData_NilDB(t *testing.T) {
	h := NewAuthHandler(nil, nil, newTestJWTManager(), nil, &Config{})

	w := httptest.NewRecorder()
	r := withAuthUser(httptest.NewRequest(http.MethodGet, "/api/v1/user/data", nil), "user-1", "Nick")
	h.ExportUserData(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestExportUserData_NotFound(t *testing.T) {
	h, mock, _ := newAuthHandlerWithDB(t)
	expectGetUserByIDError(mock, "missing-user", domain.ErrNotFound)

	w := httptest.NewRecorder()
	r := withAuthUser(httptest.NewRequest(http.MethodGet, "/api/v1/user/data", nil), "missing-user", "Nick")
	h.ExportUserData(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestCheckAuth_DBErrorDegraded(t *testing.T) {
	t.Parallel()

	h, mock, jwtMgr := newAuthHandlerWithDB(t)
	token := signTestToken(t, jwtMgr, "user-db-err", "Nick")
	expectGetUserByIDError(mock, "user-db-err", context.Canceled)

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
	_, db := newTestUserRepo(t)
	h, _, _, _ := newTestAuthHandlerWithRefreshMgr(t, db)

	w, r := newJSONRequest(http.MethodPost, "/api/v1/auth/refresh", `{"refresh_token":"invalid-token"}`)
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
	jwtMgr := newTestJWTManager()
	h := NewAuthHandler(nil, redisStore, jwtMgr, nil, &Config{ResendAPIKey: "re_test", EmailFrom: "test@test.com"})

	w, r := newJSONRequest(http.MethodPost, "/api/v1/auth/request", `{"email":"user@example.com"}`)
	h.RequestMagicLink(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestLogout_RevokesRefreshToken(t *testing.T) {
	_, rdb := testutil.NewTestMiniredis(t)
	redisStore := store.NewRedisStoreFromClient(rdb)
	jwtMgr := newTestJWTManager()
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())
	h := NewAuthHandler(nil, redisStore, jwtMgr, refreshMgr, &Config{})

	ctx := context.Background()
	refreshToken, err := refreshMgr.Generate(ctx, "user-logout")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	w, r := newJSONRequest(http.MethodPost, "/api/v1/auth/logout", `{"refresh_token":"`+refreshToken+`"}`)
	r.TLS = &tls.ConnectionState{}
	h.Logout(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if _, err := refreshMgr.ConsumeRefreshToken(ctx, refreshToken); err == nil {
		t.Fatal("refresh token should be revoked")
	}
}

// TestParseQuickPlayRequest_ChineseNickname verifies v2-R-83: Chinese nicknames
// are measured by rune count, not byte length. A 7-Chinese-char nickname is
// 21 UTF-8 bytes (would fail the old byte-length check > 20) but only 7 runes.
func TestParseQuickPlayRequest_ChineseNickname(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		nick    string
		wantErr bool
	}{
		{"7 chinese chars (21 bytes, 7 runes) — accepted", "快乐的气球玩家", false},
		{"1 rune — too short", "快", true},
		{"21 runes — too long", strings.Repeat("A", 21), true},
		{"ascii 2 chars — ok", "AB", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			body := `{"nickname":"` + tc.nick + `"}`
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/quickplay", strings.NewReader(body))
			_, apiErr := parseQuickPlayRequest(w, r)
			if tc.wantErr && apiErr == nil {
				t.Fatalf("expected error for %q, got nil", tc.nick)
			}
			if !tc.wantErr && apiErr != nil {
				t.Fatalf("unexpected error for %q: %v", tc.nick, apiErr)
			}
		})
	}
}
