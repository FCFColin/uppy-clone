package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
)

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
