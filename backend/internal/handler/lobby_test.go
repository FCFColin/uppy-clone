package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/testutil"
)

type stubLobbyRepo struct {
	result *domain.LobbyListResult
	err    error
}

func (s *stubLobbyRepo) SaveLobbyState(context.Context, *domain.LobbyState) error { return nil }
func (s *stubLobbyRepo) LoadLobbyState(context.Context, string) (*domain.LobbyState, error) {
	return nil, nil
}
func (s *stubLobbyRepo) DeleteLobbyState(context.Context, string) error { return nil }
func (s *stubLobbyRepo) LoadAllActiveLobbies(context.Context, int, string) (*domain.LobbyListResult, error) {
	return s.result, s.err
}
func (s *stubLobbyRepo) CreateGameSession(context.Context, *domain.GameSession) error { return nil }
func (s *stubLobbyRepo) RecordGameResult(context.Context, string, string, int64, int, []domain.GameResultPlayer) error {
	return nil
}

func TestListLobbies_DegradedWhenStoreUnavailable(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	h.ListLobbies(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 degraded", w.Code)
	}
	var body map[string]interface{}
	testutil.DecodeJSONBody(t, w, &body)
	if body["degraded"] != true {
		t.Errorf("degraded = %v, want true", body["degraded"])
	}
}

func TestListLobbies_SuccessWithETag(t *testing.T) {
	t.Parallel()

	repo := &stubLobbyRepo{
		result: &domain.LobbyListResult{
			Lobbies: []domain.LobbyState{{Code: "ABCDE", State: "waiting"}},
			Total:   1,
		},
	}
	hub := game.NewHub(repo, nil, config.DefaultTimeoutConfig(), 0, 0)
	h := NewLobbyHandler(hub, nil)

	w := httptest.NewRecorder()
	h.ListLobbies(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header")
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil)
	r2.Header.Set("If-None-Match", etag)
	h.ListLobbies(w2, r2)
	if w2.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", w2.Code)
	}
}

func TestCreateRoom_Success(t *testing.T) {
	t.Parallel()

	repo := &stubLobbyRepo{result: &domain.LobbyListResult{Total: 0}}
	hub := game.NewHub(repo, nil, config.DefaultTimeoutConfig(), 0, 0)
	h := NewLobbyHandler(hub, nil)

	w := httptest.NewRecorder()
	h.CreateRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]string
	testutil.DecodeJSONBody(t, w, &body)
	if body["code"] == "" {
		t.Fatalf("body = %+v", body)
	}
}

func TestCreateRoom_HubUnavailable(t *testing.T) {
	t.Parallel()

	h := &LobbyHandler{hub: nil}
	w := httptest.NewRecorder()
	h.CreateRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestCreateRoom_CodeConflict(t *testing.T) {
	hub := game.NewHub(nil, nil, config.DefaultTimeoutConfig(), 0, 0)
	h := NewLobbyHandler(hub, nil)

	orig := hub.SetGenerateRoomCodeHook(func() string { return "CONFL" })
	t.Cleanup(orig)
	if _, err := hub.CreateRoom(context.Background()); err != nil {
		t.Fatal(err)
	} // occupies CONFL

	w := httptest.NewRecorder()
	h.CreateRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
}

// TestCheckRoom_InvalidCodes consolidates invalid room-code rejection cases:
// missing code, invalid charset (digits), and an otherwise malformed code.
func TestCheckRoom_InvalidCodes(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"missing code", ""},
		{"invalid charset (digits)", "ABCD0"},
		{"invalid code", "BAD"},
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

func TestCheckRoom_Success(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	code, err := h.hub.CreateRoom(context.Background())
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	w := httptest.NewRecorder()
	r := withPathParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/"+code, nil), "code", code)
	h.CheckRoom(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestRegistryCheckRoom_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	r := withPathParam(httptest.NewRequest(http.MethodGet, "/api/v1/registry/check/ABCDE", nil), "code", "ABCDE")
	h.CheckRoom(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestMatchRoom_Success(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	h.MatchRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/match", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]string
	testutil.DecodeJSONBody(t, w, &body)
	if body["lobbyCode"] == "" {
		t.Fatalf("body = %+v", body)
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

func TestListLobbies_MarshalError(t *testing.T) {
	prev := jsonMarshalFn
	jsonMarshalFn = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	t.Cleanup(func() { jsonMarshalFn = prev })

	repo := &stubLobbyRepo{
		result: &domain.LobbyListResult{
			Lobbies: []domain.LobbyState{{Code: "ABCDE", State: "waiting"}},
			Total:   1,
		},
	}
	hub := game.NewHub(repo, nil, config.DefaultTimeoutConfig(), 0, 0)
	h := NewLobbyHandler(hub, nil)

	w := httptest.NewRecorder()
	h.ListLobbies(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/lobbies", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 degraded", w.Code)
	}
	var body map[string]interface{}
	testutil.DecodeJSONBody(t, w, &body)
	if body["degraded"] != true {
		t.Errorf("degraded = %v, want true", body["degraded"])
	}
}

func TestHandleRegistryRoom_OpError(t *testing.T) {
	t.Parallel()

	h := newTestLobbyHandler()
	w := httptest.NewRecorder()
	h.handleRegistryRoom(w, httptest.NewRequest(http.MethodPost, "/api/v1/registry/create", nil), registryRoomParams{
		emptyKey:      "code",
		emptyVal:      "",
		unavailMsg:    "unavailable",
		unavailLog:    "hub unavailable",
		failLog:       "op failed",
		degradedMsg:   "degraded",
		responseField: "code",
	}, func(context.Context) (string, error) {
		return "", context.Canceled
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
