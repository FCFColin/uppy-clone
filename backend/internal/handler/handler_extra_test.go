package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func withChiParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestCheckRoom_InvalidCharset(t *testing.T) {
	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ABCD0", nil), "code", "ABCD0")
	h.CheckRoom(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCheckRoom_InvalidLength(t *testing.T) {
	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ABC", nil), "code", "ABC")
	h.CheckRoom(w, r)
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
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCreateRoom_NilHub_Degraded(t *testing.T) {
	h := &LobbyHandler{hub: nil}
	w := httptest.NewRecorder()
	h.CreateRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestMatchRoom_NilHub_Degraded(t *testing.T) {
	h := &LobbyHandler{hub: nil}
	w := httptest.NewRecorder()
	h.MatchRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/match", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}
