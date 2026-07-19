package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestCheckRoom_InvalidCodes(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"missing code", ""},
		{"invalid charset (digits)", "ABCD0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestLobbyHandler()
			w := httptest.NewRecorder()
			r := withPathParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/"+tt.code, nil), "code", tt.code)
			h.CheckRoom(w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
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
