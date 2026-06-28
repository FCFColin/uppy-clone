package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestCreateRoom(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil)

	h.CreateRoom(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] == "" {
		t.Error("expected non-empty room code")
	}
}

func TestCheckRoom_Exists(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	// Create a room first
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil)
	h.CreateRoom(w, r)

	var createResp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&createResp)
	code := createResp["code"]

	// Check the room
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/"+code, nil)

	// Use chi router to set URL params
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("code", code)
	r2 = r2.WithContext(context.WithValue(r2.Context(), chi.RouteCtxKey, rctx))

	h.CheckRoom(w2, r2)

	if w2.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body = %s", w2.Code, http.StatusOK, w2.Body.String())
	}

	var checkResp map[string]interface{}
	_ = json.NewDecoder(w2.Body).Decode(&checkResp)
	if checkResp["code"] != code {
		t.Errorf("code = %v, want %q", checkResp["code"], code)
	}
}

func TestCheckRoom_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ZZZZZ", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("code", "ZZZZZ")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	h.CheckRoom(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCheckRoom_MissingCode(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("code", "")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	h.CheckRoom(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestMatchRoom(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/registry/match", nil)

	h.MatchRoom(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["lobbyCode"] == "" {
		t.Error("expected non-empty lobbyCode")
	}
}

// ─── WebSocket integration test helpers ──────────────────────────────
//
// These tests verify the WebSocket handler (LobbyHandler.WebSocket) end-to-end
// using httptest.NewServer + gorilla/websocket.Dialer. The Hub is created with
// nil store/redis so tests run without external dependencies.
//
// Authentication is injected via auth.WithAuthenticatedUser (the inverse of
// auth.GetAuthenticatedUser) to bypass JWT cookie validation. Origin validation
// is exercised by setting the Origin header on the dialer.

// testAuthMiddleware injects an authenticated user into the request context,
// bypassing JWT cookie validation. This is the public inverse of AuthMiddleware.

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
