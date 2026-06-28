package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
)

func TestNewAuthHandler(t *testing.T) {
	t.Parallel()

	jwtMgr := auth.NewJWTManager("test-secret-key-0123456789abcdef0123456789")
	cfg := &Config{ResendAPIKey: "test", EmailFrom: "test@test.com"}
	h := NewAuthHandler(jwtMgr, nil, nil, nil, cfg, config.DefaultTimeoutConfig())
	if h == nil {
		t.Fatal("NewAuthHandler returned nil")
	}
	if h.jwtMgr != jwtMgr || h.config != cfg || h.magicLink == nil {
		t.Error("NewAuthHandler did not wire dependencies")
	}
}

func TestWriteAuthCookies(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	access := auth.BuildAuthCookie("quickplay", "access-token", config.CookieMaxAge, false)
	writeAuthCookies(w, r, access, "refresh-token")

	cookies := w.Result().Cookies()
	names := map[string]bool{}
	for _, c := range cookies {
		names[c.Name] = true
	}
	if !names["quickplay"] {
		t.Error("expected quickplay cookie")
	}
	if !names["refresh"] {
		t.Error("expected refresh cookie")
	}
}

func TestWriteAuthCookies_AccessOnly(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	access := auth.BuildAuthCookie("quickplay", "access-token", config.CookieMaxAge, false)
	writeAuthCookies(w, r, access, "")

	for _, c := range w.Result().Cookies() {
		if c.Name == "refresh" {
			t.Error("refresh cookie should be omitted when refresh token is empty")
		}
	}
}

func TestVerifyMagicLinkPost_InvalidBody(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", strings.NewReader("{bad"))
	r.Header.Set("Content-Type", "application/json")
	h.VerifyMagicLinkPost(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestVerifyMagicLinkPost_MissingToken(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", strings.NewReader(`{"token":""}`))
	r.Header.Set("Content-Type", "application/json")
	h.VerifyMagicLinkPost(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestVerifyMagicLinkPost_NilRedis_Degraded(t *testing.T) {
	h := newTestAuthHandler()
	token := strings.Repeat("a", config.MagicLinkTokenLen)
	w := httptest.NewRecorder()
	body := `{"token":"` + token + `"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.VerifyMagicLinkPost(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestVerifyMagicLink_InvalidTokenLength(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify?token=short", nil)
	h.VerifyMagicLink(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRefreshToken_NilRedis_Degraded(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"some-token"}`))
	r.Header.Set("Content-Type", "application/json")
	h.RefreshToken(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 degraded", w.Code)
	}
}

func TestExportUserData_Unauthorized(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	h.ExportUserData(w, httptest.NewRequest(http.MethodGet, "/api/v1/user/data", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestDeleteUserData_Unauthorized(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	h.DeleteUserData(w, httptest.NewRequest(http.MethodDelete, "/api/v1/user/data", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestLogout_InvalidBody(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader("{bad"))
	r.Header.Set("Content-Type", "application/json")
	h.Logout(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (invalid JSON body is ignored)", w.Code)
	}
}
