package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/testsecrets"
)

func TestNewAuthHandler(t *testing.T) {
	t.Parallel()

	cfg := &Config{ResendAPIKey: "test", EmailFrom: "test@test.com"}
	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	h := NewAuthHandler(nil, nil, jwtMgr, nil, cfg)
	if h == nil {
		t.Fatal("NewAuthHandler returned nil")
	}
	if h.jwtMgr == nil || h.magicLink == nil || h.config != cfg {
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

func TestVerifyMagicLinkPost_BadRequest(t *testing.T) {
	h := newTestAuthHandler()
	tests := []struct {
		name string
		body string
	}{
		{"invalid body", "{bad"},
		{"missing token", `{"token":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")
			h.VerifyMagicLinkPost(w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
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

func TestUserData_Unauthorized(t *testing.T) {
	h := newTestAuthHandler()
	tests := []struct {
		name   string
		method string
	}{
		{"export unauthorized", http.MethodGet},
		{"delete unauthorized", http.MethodDelete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tt.method, "/api/v1/user/data", nil)
			switch tt.method {
			case http.MethodGet:
				h.ExportUserData(w, r)
			case http.MethodDelete:
				h.DeleteUserData(w, r)
			}
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", w.Code)
			}
		})
	}
}

func TestLogout_InvalidBody(t *testing.T) {
	h := newTestAuthHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader("{bad"))
	r.Header.Set("Content-Type", "application/json")
	h.Logout(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (invalid JSON body should be rejected)", w.Code)
	}
}
