package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
)

// newTestAuthHandler creates an AuthHandler with nil DB/Redis for testing
// only the HTTP-layer logic (request parsing, error responses).
func newTestAuthHandler() *AuthHandler {
	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	return &AuthHandler{
		jwtMgr:     jwtMgr,
		refreshMgr: nil,
		db:         nil,
		redis:      nil,
		config:     &Config{ResendAPIKey: "test", EmailFrom: "test@test.com"},
		magicLink:  nil,
		timeouts:   config.DefaultTimeoutConfig(),
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

func TestCheckAuth_Authenticated(t *testing.T) {
	t.Parallel()

	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	token, err := jwtMgr.SignToken("user-123", "TestPlayer")
	if err != nil {
		t.Fatalf("SignToken() error = %v", err)
	}

	h := &AuthHandler{
		jwtMgr:     jwtMgr,
		refreshMgr: nil,
		db:         nil,
		redis:      nil,
		config:     &Config{},
		magicLink:  nil,
		timeouts:   config.DefaultTimeoutConfig(),
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

func TestLogout_ClearsCookies(t *testing.T) {
	t.Parallel()

	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	h := &AuthHandler{
		jwtMgr:     jwtMgr,
		refreshMgr: nil,
		db:         nil,
		redis:      nil,
		config:     &Config{},
		magicLink:  nil,
		timeouts:   config.DefaultTimeoutConfig(),
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
